package utils

import (
	"log"
	"os"
)

func NewLogger(filePath string) (*log.Logger, *os.File, error) {
	f, err := os.OpenFile(filePath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, nil, err
	}
	return log.New(f, "appcenter-agent ", log.LstdFlags|log.LUTC), f, nil
}
