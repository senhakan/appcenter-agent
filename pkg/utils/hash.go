package utils

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"strings"
)

func VerifyFileHash(filePath, expectedHash string) (bool, error) {
	expectedHash = strings.TrimPrefix(strings.ToLower(expectedHash), "sha256:")

	f, err := os.Open(filePath)
	if err != nil {
		return false, err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return false, err
	}

	got := fmt.Sprintf("%x", h.Sum(nil))
	return got == expectedHash, nil
}
