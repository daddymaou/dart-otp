package apikeys

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
)

const alphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

func Generate(isTest bool) (fullKey, prefix, keyHash string, err error) {
	var sb []byte
	if isTest {
		sb = append(sb, "dart_test_"...)
	} else {
		sb = append(sb, "dart_live_"...)
	}
	b := make([]byte, 32)
	if _, err = rand.Read(b); err != nil {
		return "", "", "", err
	}
	for _, n := range b {
		sb = append(sb, alphabet[int(n)%len(alphabet)])
	}
	fullKey = string(sb)
	prefix = fullKey[:12]
	sum := sha256.Sum256([]byte(fullKey))
	keyHash = hex.EncodeToString(sum[:])
	return fullKey, prefix, keyHash, nil
}

func HashKey(fullKey string) string {
	sum := sha256.Sum256([]byte(fullKey))
	return hex.EncodeToString(sum[:])
}
