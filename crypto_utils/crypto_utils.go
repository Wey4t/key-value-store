package crypto_utils

import (
	"crypto"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"errors"
	"time"
)

func RandomBytes(size int) []byte {
	arr := make([]byte, size)

	if _, err := rand.Read(arr); err != nil {
		panic(err)
	}

	return arr
}

func NewPrivateKey() *rsa.PrivateKey  {
	reader := rand.Reader
	bitSize := 2048
	privateKey, err := rsa.GenerateKey(reader, bitSize)
	if err != nil {
		panic(err)
	}

	return privateKey
}

func PrivateKeyToBytes(privateKey *rsa.PrivateKey) []byte {
	return x509.MarshalPKCS1PrivateKey(privateKey)
}

func BytesToPrivateKey(bytes []byte) (*rsa.PrivateKey, error) {
	return x509.ParsePKCS1PrivateKey(bytes)
}

func PublicKeyToBytes(publicKey *rsa.PublicKey) []byte {
	return x509.MarshalPKCS1PublicKey(publicKey)
}

func BytesToPublicKey(bytes []byte) (*rsa.PublicKey, error) {
	return x509.ParsePKCS1PublicKey(bytes)
}

func EncryptPK(plaintext []byte, publicKey *rsa.PublicKey) []byte {
	ciphertext, err := rsa.EncryptOAEP(sha256.New(), rand.Reader, publicKey, plaintext, nil)
	if err != nil {
		panic(err)
	}
	return ciphertext
}

func DecryptPK(ciphertext []byte, privateKey *rsa.PrivateKey) ([]byte, error) {
	plaintext, err := privateKey.Decrypt(nil, ciphertext, &rsa.OAEPOptions{Hash: crypto.SHA256})
	if err != nil {
		return nil, errors.New("AuthenticationError")
	}
	return plaintext, nil
}

func NewSessionKey() []byte {
	sessionKey := make([]byte, 32)
	_, err := rand.Read(sessionKey)
    if err != nil {
        panic(err)
    }
    return sessionKey
}

func EncryptSK(plaintext []byte, sessionKey []byte) []byte {
	aes, err := aes.NewCipher(sessionKey)
	if err != nil {
		panic(err)
	}

	gcm, err := cipher.NewGCM(aes)
	if err != nil {
		panic(err)
	}

	// We need a 12-byte nonce for GCM (modifiable if you use cipher.NewGCMWithNonceSize())
	// A nonce should always be randomly generated for every encryption.
	nonce := make([]byte, gcm.NonceSize())
	_, err = rand.Read(nonce)
	if err != nil {
		panic(err)
	}

	// ciphertext here is actually nonce+ciphertext
	// So that when we decrypt, just knowing the nonce size
	// is enough to separate it from the ciphertext.
	ciphertext := gcm.Seal(nonce, nonce, plaintext, nil)
	return ciphertext
}

// Input: ciphertext, sessionKey. Returns (string, error).
// Decrypts ciphertext using sessionKey. If error != nil, then decryption failed.
func DecryptSK(ciphertext []byte, sessionKey []byte) ([]byte, error) {
	aes, err := aes.NewCipher(sessionKey)
	if err != nil {
		panic(err)
	}

	gcm, err := cipher.NewGCM(aes)
	if err != nil {
		panic(err)
	}

	// Since we know the ciphertext is actually nonce+ciphertext
	// And len(nonce) == NonceSize(). We can separate the two.
	nonceSize := gcm.NonceSize()
	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]

	plaintext, err := gcm.Open(nil, []byte(nonce), []byte(ciphertext), nil)
	if err != nil {
		return nil, errors.New("AuthenticationError")
	}

	return plaintext, nil
}

func Hash(msg []byte) []byte {
	hash := sha256.New()
	_, err := hash.Write(msg)
	if err != nil {
		panic(err)
	}
	return hash.Sum(nil)
}

func Sign(msg []byte, privateKey *rsa.PrivateKey) []byte {
	msgHash := Hash(msg)

	// In order to generate the signature, we provide a random number generator,
	// our private key, the hashing algorithm that we used, and the hash sum
	// of our message
	signature, err := rsa.SignPSS(rand.Reader, privateKey, crypto.SHA256, msgHash, nil)
	if err != nil {
		panic(err)
	}
	return signature
}

// Input: signature, msgHash, publicKey. Returns bool.
// Returns true if signature matches Hashed message. Otherwise, returns false.
func Verify(signature []byte, msgHash []byte, publicKey *rsa.PublicKey) bool {
	err := rsa.VerifyPSS(publicKey, crypto.SHA256, msgHash, signature, nil)
	return err == nil
}

func ReadClock() time.Time {
	return time.Now()
}

func TodToBytes(t time.Time) []byte {
	tod, err := t.MarshalBinary()
	if err != nil {
		panic(err)
	}
	return tod
}

func BytesToTod(t []byte) time.Time {
	var tod time.Time 
	err := tod.UnmarshalBinary(t)
	if err != nil {
		panic(err)
	}
	return tod
}
