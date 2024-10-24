package cryptography

import (
	"bytes"
	"log"
	"testing"
)

// Check the usual encryption function property that Dec(Enc(pt, k), k) == pt
func TestFileEncryption(t *testing.T) {
	plaintexts := []string{
		"test", "", "I never wanted it to end. I spent eight days in Paris, France. My best friends, Henry and Steve, went with me. We had a beautiful hotel room in the Latin Quarter, and it wasn’t even expensive. We had a balcony with a wonderful view.\n\nWe visited many famous tourist places. My favorite was the Louvre, a well-known museum. I was always interested in art, so that was a special treat for me. The museum is so huge, you could spend weeks there. Henry got tired walking around the museum and said “Enough! I need to take a break and rest.”\n\nWe took lots of breaks and sat in cafes along the river Seine. The French food we ate was delicious. The wines were tasty, too. Steve’s favorite part of the vacation was the hotel breakfast. He said he would be happy if he could eat croissants like those forever. We had so much fun that we’re already talking about our next vacation!\n",
	}

	c := StreamCipher{}
	c.Init("6368616e676520746869732070617373776f726420746f206120736563726574")
	for _, p := range plaintexts {
		plaintext := []byte(p)

		// Buffers to hold the encrypted and decrypted data
		var encryptedBuffer bytes.Buffer
		var decryptedBuffer bytes.Buffer

		err := c.EncryptStream(bytes.NewReader(plaintext), &encryptedBuffer)

		// Decrypt the data
		err = c.DecryptStream(&encryptedBuffer, &decryptedBuffer)
		if err != nil {
			log.Fatalf("Decryption failed for %q: %v", p, err)
		}

		// Compare the decrypted result with the original plaintext
		if !bytes.Equal(decryptedBuffer.Bytes(), plaintext) {
			t.Errorf("Decrypt(Encrypt(%s)) = %s, want %s", p, decryptedBuffer.Bytes(), p)
		}
	}

}

// Also verify that the encryption stream doesn't just return the plaintext stream, i.e that confidentiality is guaranteed using the secret key
func TestFileEncryptionSanity(t *testing.T) {
	plaintexts := []string{
		"test",
		"",
		"I never wanted it to end. I spent eight days in Paris, France. My best friends, Henry and Steve, went with me. We had a beautiful hotel room in the Latin Quarter, and it wasn’t even expensive. We had a balcony with a wonderful view.\n\nWe visited many famous tourist places. My favorite was the Louvre, a well-known museum. I was always interested in art, so that was a special treat for me. The museum is so huge, you could spend weeks there. Henry got tired walking around the museum and said “Enough! I need to take a break and rest.”\n\nWe took lots of breaks and sat in cafes along the river Seine. The French food we ate was delicious. The wines were tasty, too. Steve’s favorite part of the vacation was the hotel breakfast. He said he would be happy if he could eat croissants like those forever. We had so much fun that we’re already talking about our next vacation!\n",
	}

	c := StreamCipher{}
	c.Init("6368616e676520746869732070617373776f726420746f206120736563726574")
	for _, p := range plaintexts {
		plaintext := []byte(p)

		var encryptedBuffer bytes.Buffer

		_ = c.EncryptStream(bytes.NewReader(plaintext), &encryptedBuffer)

		if bytes.Equal(plaintext, encryptedBuffer.Bytes()) {
			t.Errorf("Confidentiality breach: Encrypt(%s) should not be equal to %s", p, p)
		}

	}
}
