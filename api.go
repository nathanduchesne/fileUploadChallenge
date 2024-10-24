package main

import (
	"api/cryptography"
	"api/uid"
	"context"
	"crypto/aes"
	"fmt"
	_ "github.com/joho/godotenv/autoload"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"io"
	"log"
	"math"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"
)

// DONE: test encrypt/decrypt
// DONE: store key in other file and fetch it
// DONE: change package names
// DONE: create endpoint which is fed file
// DONE: deal with file not found
// TODO: add functionality of 1MB part at a time for file upload
// DONE: add user-side uid choice to avoid any problems
// DONE: after being fed a file, encrypt it and store in bucket
// DONE: setup minio client for uploading and serving
// DONE: check for adding timeout in context when calling minio
// DONE: return uid to user
// DONE: either use users provided file size, or have limitations of 5tb
// DONE: test uid with timeout

func uploadHandler(minioClient *minio.Client, cipher *cryptography.StreamCipher) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		// Get the file size provided by the user, necessary to be able to provide this length to the MinIO uploader.
		// If we were to remove this element in the header, we would need to call PutObject with the -1 size, which allocates
		// 700MB for this purpose. Since we aren't aware of daemon memory, we make this design choice.
		fileSize, err := strconv.ParseInt(r.Header["File-Size"][0], 10, 64)
		if err != nil {
			http.Error(w, "File-Size in header should be the file size in bytes", http.StatusPreconditionFailed)
			return
		}
		// The uploaded length corresponds to the number of bytes in the uploaded file and the IV used in the stream cipher.
		minioDataSize := fileSize + int64(aes.BlockSize)

		// Get the object name to be uniquely identified on MinIO. This value is returned to users upon upload completion
		// to tell them what UID to use to fetch this file.
		objectName, errOccurred := getUniqueObjectName(w, r)
		if errOccurred {
			return
		}

		// Create a pipe that connects the user uploaded data to the encryption stream
		uploadedDataReader, uploadedDataWriter := io.Pipe()
		// Create a pipe that connects the encryption stream to the MinIO upload stream
		ciphertextReader, ciphertextWriter := io.Pipe()

		// 3 goroutines are used:
		// 1) Streams the user's uploaded data by chunk
		// 2) Encrypts the data stream on-the-fly
		// 3) Uploads the encrypted data stream to MinIO
		var wg sync.WaitGroup
		wg.Add(3)

		// Define a blocking channel used for the MinIO uploading to wait until the uploaded file name has been read in the user data stream.
		// This allows us to store it in the metadata and to return the named file when a user fetches it later on.
		filenameChannel := make(chan string)

		// 1) Streams the user's uploaded data by chunk
		go func() {
			defer wg.Done()
			defer uploadedDataWriter.Close()
			// Process the user's uploaded file body as a stream
			fileStream, err := r.MultipartReader()
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			// Define a buffer to read chunks from this stream to upload to our encryption stream
			fileChunk := make([]byte, CHUNK_SIZE)
			var firstPart = true
			for {
				// Read parts of the multi-part upload.
				nextPart, err := fileStream.NextPart()
				if err == io.EOF {
					return
				} else if err != nil {
					// If any other error occurs, we return it as an unprocessable stream.
					http.Error(w, err.Error(), http.StatusUnprocessableEntity)
					return
				} else {
					for {
						nbrReadBytes, errEOF := nextPart.Read(fileChunk)
						// When we process the first part (the user uploaded file), we parse the header to get the filename.
						if firstPart {
							contentDetails := nextPart.Header.Get("Content-Disposition")
							_, params, err := mime.ParseMediaType(contentDetails)
							// If we fail to parse the file name, it should not be a problem, we simply cannot store the name in the metadata
							if err != nil {
								filenameChannel <- ""
							} else {
								filenameChannel <- params["filename"]
							}
							firstPart = false
						}
						// We then copy the byte chunk to send it to our encryption stream
						err = sendToEncryption(fileChunk[:nbrReadBytes], uploadedDataWriter)
						if err != nil {
							http.Error(w, err.Error(), http.StatusInternalServerError)
							return
						}
						// If these bytes were the last ones in this request multi-part, we move on to the next one.
						if errEOF == io.EOF {
							break
						}
					}
				}
			}
		}()

		// 2) Encrypts the data stream on-the-fly
		go func() {
			defer wg.Done()
			defer ciphertextWriter.Close()
			defer fmt.Println("Finished encrypting")

			// Encrypt the incoming file stream
			if err := cipher.EncryptStream(uploadedDataReader, ciphertextWriter); err != nil {
				ciphertextWriter.CloseWithError(err)
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
		}()

		// 3) Uploads the encrypted data stream to MinIO
		go func() {
			defer wg.Done()
			defer fmt.Println("Finished uploading")
			// Wait until a filename is provided before starting the upload, since metadata must be known at the function call time.
			filename := <-filenameChannel
			metadata := make(map[string]string)
			// If the user's request contained a filename, we add it to the metadata, otherwise we don't provide this service.
			if filename != "" {
				metadata["Filename"] = filepath.Base(filename)
			}
			// Set a timeout for uploads taking too long
			maxNbrRunNanoseconds := getMaxNbrRunSeconds(minioDataSize)
			timeoutCtx, timeoutCancel := context.WithTimeout(context.Background(), maxNbrRunNanoseconds)
			defer timeoutCancel()

			_, err := minioClient.PutObject(timeoutCtx, BUCKET_NAME, objectName, ciphertextReader, minioDataSize, minio.PutObjectOptions{
				ContentType:  "application/octet-stream",
				UserMetadata: metadata,
			})

			if err != nil {
				http.Error(w, "Upload to MinIO failed", http.StatusInternalServerError)
				return
			}
		}()

		wg.Wait()
		// If everything went well, send a success response
		fmt.Fprintf(w, "File successfully uploaded and encrypted with UID %s \n", objectName)
	}
}

func fetchAndDecryptHandler(minioClient *minio.Client, cipher *cryptography.StreamCipher) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		uidStr := r.URL.Query().Get("uid")
		if uidStr == "" {
			http.Error(w, "Missing UID", http.StatusBadRequest)
			return
		}
		uid, err := strconv.ParseUint(uidStr, 10, 64)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if !uidTracker.Contains(uid) {
			http.Error(w, "The MinIO bucket does not contain any object with the provided UID", http.StatusNotFound)
			return
		}

		// Prepare to fetch the encrypted object from MinIO
		objectName := uidStr
		ctx := context.Background()

		// Get the object from MinIO as a stream
		object, err := minioClient.GetObject(ctx, BUCKET_NAME, objectName, minio.GetObjectOptions{})
		if err != nil {
			http.Error(w, "Unable to fetch file from MinIO", http.StatusInternalServerError)
			return
		}
		defer object.Close()

		objectInfo, err := object.Stat()
		if err != nil {
			http.Error(w, "Failed to get object metadata", 408)
			return
		}
		filename, ok := objectInfo.UserMetadata["Filename"]
		if !ok {
			http.Error(w, "Filename not found in metadata", 408)
			return
		}

		// Decrypt the stream and send it to the response
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filename))

		// Decrypt the stream and write directly to the response writer
		err = cipher.DecryptStream(object, w)
		if err != nil {
			http.Error(w, "Error during decryption", http.StatusInternalServerError)
			return
		}

		// Success
		fmt.Fprintf(w, "File %s streamed and decrypted successfully.\n", objectName)
	}
}

var uidTracker = uid.UidTracker{}

// The chunk size was chosen for extreme cases where the daemon has very little RAM. For faster uploads, chunks of 16-64MB can easily be used.
const CHUNK_SIZE = 1024 * 1024 * 8
const BUCKET_NAME = "challenge-taurus"

func main() {
	c := cryptography.StreamCipher{}
	c.Init(os.Getenv("SYM_KEY"))

	endpoint := "minio:9000"
	accessKeyID := os.Getenv("MINIO_USER")
	secretAccessKey := os.Getenv("MINIO_PWD")

	// Initialize minio client object, with disabled SSL due to the toy example setting.
	minioClient, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKeyID, secretAccessKey, ""),
		Secure: false,
	})
	if err != nil {
		log.Fatalln(err)
	}

	// Fetch all current used object names at runtime to store this in RAM and avoid frequent calls to MinIO for unique ID generation.
	err = fetchUidsFromMinio(&uidTracker, minioClient)
	if err != nil {
		log.Fatalln(err)
	}

	// Set up the HTTP handler
	http.HandleFunc("/upload", uploadHandler(minioClient, &c))
	http.HandleFunc("/fetch", fetchAndDecryptHandler(minioClient, &c))

	// Start the server
	log.Println("Server started at :8080")
	log.Println(http.ListenAndServe(":8080", nil))
}

// fetchUidsFromMinio fetches the list of objects in the bucket to extract their uids and store them into the UID tracker in RAM.
func fetchUidsFromMinio(tracker *uid.UidTracker, client *minio.Client) error {
	currentObjectIds := make([]uint64, 0, 100)
	for obj := range client.ListObjects(context.Background(), BUCKET_NAME, minio.ListObjectsOptions{}) {
		newUid, err := strconv.ParseUint(obj.Key, 10, 64)
		if err == nil {
			currentObjectIds = append(currentObjectIds, newUid)
		}
	}
	tracker.Init(currentObjectIds)
	return nil
}

// getMaxNbrRunSeconds returns the maximal expected time it should take for the system to upload to MinIO.
// This time is determined in a very conservative manner, and should therefore be a reasonable upper-bound for a timeout.
func getMaxNbrRunSeconds(nbrUploadedBytes int64) time.Duration {
	// We assume that on such a system, the slowest rate we should be observing is 1MB/s.
	const minRateBytes float64 = 1024 * 1024 * 1
	// Also account for the fact starting the upload may have a little overhead, so add 10s for safety.
	safetySeconds := int64(10)
	// Calculate how many seconds it should take using the slowest assumed byte rate upload
	// Convert these seconds to nanoseconds for successful type change to time.Duration
	return time.Duration((safetySeconds + int64(math.Ceil(float64(nbrUploadedBytes)/minRateBytes))) * int64(math.Pow10(9)))
}

// getUniqueObjectName returns true if an error occurred, meaning the program should return.
// On the other hand, if it returns false, the returned string contains a unique identifier for the uploaded file.
// The appropriate error and error code will be sent to the user in the function directly.
func getUniqueObjectName(w http.ResponseWriter, r *http.Request) (string, bool) {
	var objectName string
	// If the request header contains a UID field, try using it
	if uidStr, ok := r.Header["Uid"]; ok {
		suggestedUid, err := strconv.ParseUint(uidStr[0], 10, 64)
		if err != nil {
			http.Error(w, "The UID provided in the header cannot be parsed as a uint64.", http.StatusPreconditionFailed)
			return "", true
		}
		added, err := uidTracker.AddUid(suggestedUid)
		if err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return "", true
		}
		objectName = strconv.FormatUint(added, 10)

	} else {
		// If it does not contain a UID field, generate one for them
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Millisecond)
		defer cancel()
		added, err := uidTracker.GenerateAndAdd(ctx)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return "", true
		}
		objectName = strconv.FormatUint(added, 10)
	}
	return objectName, false
}

// sendToEncryption reads the data in the buffer and copies it to a stream.
func sendToEncryption(data []byte, writer io.Writer) error {
	// Write the plaintext data to the writer
	_, err := writer.Write(data)
	return err
}
