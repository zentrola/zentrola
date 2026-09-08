package logging

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"sync"
)

// RotatingFile 按大小轮转 JSON Lines 日志。每次 Write 对应一整条日志，不会拆分记录。
type RotatingFile struct {
	path       string
	maxBytes   int64
	maxBackups int
	mu         sync.Mutex
	file       *os.File
	size       int64
}

func OpenRotatingFile(path string, maxSizeMB, maxBackups int) (*RotatingFile, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	writer := &RotatingFile{path: path, maxBytes: int64(maxSizeMB) << 20, maxBackups: maxBackups}
	if err := writer.open(); err != nil {
		return nil, err
	}
	return writer, nil
}

func (w *RotatingFile) Write(data []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file == nil {
		return 0, os.ErrClosed
	}
	if w.size > 0 && w.size+int64(len(data)) > w.maxBytes {
		if err := w.rotate(); err != nil {
			return 0, err
		}
	}
	n, err := w.file.Write(data)
	w.size += int64(n)
	return n, err
}

func (w *RotatingFile) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file == nil {
		return nil
	}
	err := w.file.Close()
	w.file = nil
	return err
}

func (w *RotatingFile) open() error {
	file, err := os.OpenFile(w.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return err
	}
	w.file = file
	w.size = info.Size()
	return nil
}

func (w *RotatingFile) rotate() error {
	if err := w.file.Close(); err != nil {
		return err
	}
	w.file = nil
	for index := w.maxBackups; index >= 1; index-- {
		destination := w.path + "." + strconv.Itoa(index)
		if err := os.Remove(destination); err != nil && !errors.Is(err, os.ErrNotExist) {
			return w.reopenAfterRotationError(err)
		}
		source := w.path
		if index > 1 {
			source += "." + strconv.Itoa(index-1)
		}
		if err := os.Rename(source, destination); err != nil && !errors.Is(err, os.ErrNotExist) {
			return w.reopenAfterRotationError(err)
		}
	}
	return w.open()
}

func (w *RotatingFile) reopenAfterRotationError(rotationErr error) error {
	if err := w.open(); err != nil {
		return errors.Join(rotationErr, err)
	}
	return rotationErr
}
