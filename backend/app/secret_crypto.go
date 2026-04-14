package app

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"io"
	"os"
	"strings"
)

// encryptSecret 将明文凭据加密为带前缀的 AES-GCM 密文串。
func encryptSecret(plain string) (string, error) {
	key := deriveMasterKey()
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	ciphertext := gcm.Seal(nil, nonce, []byte(plain), nil)
	blob := append(nonce, ciphertext...)
	return aesCredentialPrefix + base64.StdEncoding.EncodeToString(blob), nil
}

// decryptSecret 解密 AES-GCM 密文串并返回原始明文凭据。
func decryptSecret(enc string) (string, error) {
	if !strings.HasPrefix(enc, aesCredentialPrefix) {
		return "", errors.New("invalid encrypted payload")
	}
	raw := strings.TrimPrefix(enc, aesCredentialPrefix)
	blob, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return "", err
	}

	key := deriveMasterKey()
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(blob) < gcm.NonceSize() {
		return "", errors.New("ciphertext too short")
	}
	nonce := blob[:gcm.NonceSize()]
	ciphertext := blob[gcm.NonceSize():]
	plain, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}

// deriveMasterKey 基于环境变量派生 32 字节主密钥。
func deriveMasterKey() [32]byte {
	seed := strings.TrimSpace(os.Getenv(aesMasterKeyEnv))
	if seed == "" {
		seed = "loomidbx-dev-master-key"
	}
	return sha256.Sum256([]byte(seed))
}
