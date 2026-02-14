package utils

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sync"
)

type rotatingWriter struct {
	mu         sync.Mutex
	path       string
	maxBytes   int64
	maxBackups int
	file       *os.File
}

func NewLogger(filePath string, maxSizeMB, maxBackups int) (*log.Logger, io.Closer, error) {
	if maxSizeMB <= 0 {
		maxSizeMB = 10
	}
	if maxBackups <= 0 {
		maxBackups = 5
	}

	w := &rotatingWriter{
		path:       filePath,
		maxBytes:   int64(maxSizeMB) * 1024 * 1024,
		maxBackups: maxBackups,
	}
	if err := w.open(); err != nil {
		return nil, nil, err
	}

	logger := log.New(w, "appcenter-agent ", log.LstdFlags|log.LUTC)
	return logger, w, nil
}

func (w *rotatingWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.file == nil {
		if err := w.open(); err != nil {
			return 0, err
		}
	}

	if err := w.rotateIfNeeded(int64(len(p))); err != nil {
		return 0, err
	}
	return w.file.Write(p)
}

func (w *rotatingWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file != nil {
		err := w.file.Close()
		w.file = nil
		return err
	}
	return nil
}

func (w *rotatingWriter) rotateIfNeeded(incoming int64) error {
	fi, err := w.file.Stat()
	if err != nil {
		return err
	}
	if fi.Size()+incoming <= w.maxBytes {
		return nil
	}
	return w.rotate()
}

func (w *rotatingWriter) rotate() error {
	if err := w.file.Close(); err != nil {
		return err
	}

	for i := w.maxBackups; i >= 1; i-- {
		src := fmt.Sprintf("%s.%d", w.path, i)
		dst := fmt.Sprintf("%s.%d", w.path, i+1)
		if _, err := os.Stat(src); err == nil {
			_ = os.Remove(dst)
			if err := os.Rename(src, dst); err != nil {
				return err
			}
		}
	}

	firstBackup := fmt.Sprintf("%s.1", w.path)
	_ = os.Remove(firstBackup)
	if _, err := os.Stat(w.path); err == nil {
		if err := os.Rename(w.path, firstBackup); err != nil {
			return err
		}
	}

	return w.open()
}

func (w *rotatingWriter) open() error {
	if err := os.MkdirAll(filepath.Dir(w.path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(w.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	w.file = f
	return nil
}
