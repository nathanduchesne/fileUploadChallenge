package cryptography

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
)

// Cipher interface provides methods for stream encryption and decryption.
type Cipher interface {
	Init()
	EncryptStream(reader io.Reader, writer io.Writer) error
	DecryptStream(ciphertext []byte) []byte
}

type StreamCipher struct {
	block cipher.Block
}

// EncryptStream reads data from the provided io.Reader and encrypts it using a stream cipher which is written to the io.Writer.
func (c *StreamCipher) EncryptStream(reader io.Reader, writer io.Writer) error {

	iv := make([]byte, aes.BlockSize)
	if _, err := io.ReadFull(rand.Reader, iv); err != nil {
		return err
	}

	// StreamWriter will encrypt data and write it to the writer as it is read from the reader
	stream := cipher.NewCTR(c.block, iv)

	// Write nonce to the output (important for decryption)
	if _, err := writer.Write(iv); err != nil {
		return err
	}

	// Stream and encrypt the data
	sw := &cipher.StreamWriter{S: stream, W: writer}

	_, err := io.Copy(sw, reader)
	if err != nil {
		return err
	}
	return nil
}

// DecryptStream reads the stream of ciphertext from the io.Reader and decrypts it on the fly into the io.Writer.
func (c *StreamCipher) DecryptStream(reader io.Reader, writer io.Writer) error {
	// Read iv from the beginning of the stream
	iv := make([]byte, aes.BlockSize)
	if _, err := io.ReadFull(reader, iv); err != nil {
		return fmt.Errorf("unable to read iv: %v", err)
	}

	stream := cipher.NewCTR(c.block, iv)
	sr := &cipher.StreamReader{S: stream, R: reader}

	// Copy the decrypted stream to the writer
	if _, err := io.Copy(writer, sr); err != nil {
		return fmt.Errorf("error while decrypting stream: %v", err)
	}

	return nil
}

// Init initializes the stream cipher using a secret key. If this key is derived from a passcode, ensure it was passed through a KDF.
func (c *StreamCipher) Init(hexKey string) {
	key, _ := hex.DecodeString(hexKey)
	block, err := aes.NewCipher(key)
	if err != nil {
		panic(err.Error())
	}
	c.block = block
}
