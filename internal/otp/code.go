package otp

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math/big"
)

func GenerateCode(length int) (string, error) {
	max := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(length)), nil)
	n, err := rand.Int(rand.Reader, max)
	if err != nil {
		return "", fmt.Errorf("generate code: %w", err)
	}
	return fmt.Sprintf("%0*d", length, n.Int64()), nil
}

func NewSalt() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func HashCode(pepper, salt, code string) string {
	mac := hmac.New(sha256.New, []byte(pepper+salt))
	mac.Write([]byte(code))
	return hex.EncodeToString(mac.Sum(nil))
}

func CompareCodes(hashA, hashB string) bool {
	return hmac.Equal([]byte(hashA), []byte(hashB))
}

const BuiltInTemplate = "Your verification code is {{code}}. It expires in 5 minutes. Do not share it with anyone."
