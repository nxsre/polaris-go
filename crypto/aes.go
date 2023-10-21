package crypto

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"errors"
	"math/rand"
)

// AesCryptor AES cryptor
type AesCryptor struct {
}

// GenerateKey generate key
func (c *AesCryptor) GenerateKey() ([]byte, error) {
	key := make([]byte, 16)
	_, err := rand.Read(key)
	if err != nil {
		return nil, err
	}
	return key, nil
}

// Encrypt AES encrypt plaintext and base64 encode ciphertext
func (c *AesCryptor) Encrypt(plaintext string, key []byte) (string, error) {
	ciphertext, err := c.doEncrypt([]byte(plaintext), key)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// Decrypt base64 decode ciphertext and AES decrypt
func (c *AesCryptor) Decrypt(ciphertext string, key []byte) (string, error) {
	ciphertextBytes, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return "", err
	}
	plaintext, err := c.doDecrypt(ciphertextBytes, key)
	if err != nil {
		return "", err
	}
	return string(plaintext), nil
}

// encrypt AES encryption
func (c *AesCryptor) doEncrypt(plaintext []byte, key []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	blockSize := block.BlockSize()
	paddingData := pkcs7Padding(plaintext, blockSize)
	ciphertext := make([]byte, len(paddingData))
	blockMode := cipher.NewCBCEncrypter(block, key[:blockSize])
	blockMode.CryptBlocks(ciphertext, paddingData)
	return ciphertext, nil
}

// Decrypt AES decryption
func (c *AesCryptor) doDecrypt(ciphertext []byte, key []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	blockSize := block.BlockSize()
	blockMode := cipher.NewCBCDecrypter(block, key[:blockSize])
	paddingPlaintext := make([]byte, len(ciphertext))
	blockMode.CryptBlocks(paddingPlaintext, ciphertext)
	plaintext, err := pkcs7UnPadding(paddingPlaintext)
	if err != nil {
		return nil, err
	}
	return plaintext, nil
}

func pkcs7Padding(data []byte, blockSize int) []byte {
	padding := blockSize - len(data)%blockSize
	padText := bytes.Repeat([]byte{byte(padding)}, padding)
	return append(data, padText...)
}

func pkcs7UnPadding(data []byte) ([]byte, error) {
	length := len(data)
	if length == 0 {
		return nil, errors.New("invalid encryption data")
	}
	unPadding := int(data[length-1])
	return data[:(length - unPadding)], nil
}
