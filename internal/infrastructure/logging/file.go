package logging

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

var ErrFileInUse = errors.New("log directory already in use")

const (
	logDateLayout = "2006-01-02"
	logLockName   = ".zentrola-log.lock"
)

// RotatingFile 按本地日期和大小轮转日志文件。每次 Write 对应一整条日志，不会拆分记录。
type RotatingFile struct {
	directory  string
	maxBytes   int64
	maxBackups int
	mu         sync.Mutex
	file       *os.File
	lock       *os.File
	size       int64
	date       string
	index      int
	now        func() time.Time
}

func OpenRotatingFile(directory string, maxSizeMB, maxBackups int) (*RotatingFile, error) {
	if err := os.MkdirAll(directory, 0700); err != nil {
		return nil, err
	}
	lock, err := acquireFileLock(filepath.Join(directory, logLockName))
	if err != nil {
		return nil, err
	}
	writer := &RotatingFile{
		directory: directory, maxBytes: int64(maxSizeMB) << 20, maxBackups: maxBackups,
		lock: lock, now: time.Now,
	}
	if err := writer.open(); err != nil {
		_ = writer.Close()
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
	date := w.currentDate()
	if date != w.date {
		if err := w.openLatest(date); err != nil {
			return 0, err
		}
	} else if w.size > 0 && w.size+int64(len(data)) > w.maxBytes {
		if err := w.openFile(date, w.index+1); err != nil {
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
	var result error
	if w.file != nil {
		result = w.file.Close()
		w.file = nil
	}
	if w.lock != nil {
		result = errors.Join(result, w.lock.Close())
		w.lock = nil
	}
	return result
}

func (w *RotatingFile) open() error {
	if w.now == nil {
		w.now = time.Now
	}
	return w.openLatest(w.currentDate())
}

func (w *RotatingFile) currentDate() string {
	return w.now().Local().Format(logDateLayout)
}

func (w *RotatingFile) openLatest(date string) error {
	files, err := w.logFiles()
	if err != nil {
		return err
	}
	index := 1
	var latest *logFile
	for i := range files {
		candidate := &files[i]
		if candidate.date == date && (latest == nil || candidate.index > latest.index) {
			latest = candidate
		}
	}
	if latest != nil {
		index = latest.index
		info, err := os.Stat(latest.path)
		if err != nil {
			return err
		}
		if info.Size() >= w.maxBytes {
			index++
		}
	}
	return w.openFile(date, index)
}

func (w *RotatingFile) openFile(date string, index int) error {
	path := filepath.Join(w.directory, logFileName(date, index))
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return err
	}
	if w.file != nil {
		if err := w.file.Close(); err != nil {
			_ = file.Close()
			return err
		}
	}
	w.file = file
	w.size = info.Size()
	w.date = date
	w.index = index
	return w.removeExpiredFiles(path)
}

type logFile struct {
	path  string
	date  string
	index int
}

func (w *RotatingFile) logFiles() ([]logFile, error) {
	entries, err := os.ReadDir(w.directory)
	if err != nil {
		return nil, err
	}
	files := make([]logFile, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		date, index, ok := parseLogFileName(entry.Name())
		if ok {
			files = append(files, logFile{path: filepath.Join(w.directory, entry.Name()), date: date, index: index})
		}
	}
	sort.Slice(files, func(i, j int) bool {
		if files[i].date != files[j].date {
			return files[i].date < files[j].date
		}
		return files[i].index < files[j].index
	})
	return files, nil
}

func (w *RotatingFile) removeExpiredFiles(activePath string) error {
	files, err := w.logFiles()
	if err != nil {
		return err
	}
	history := make([]logFile, 0, len(files))
	for _, file := range files {
		if file.path != activePath {
			history = append(history, file)
		}
	}
	removeCount := len(history) - w.maxBackups
	for i := 0; i < removeCount; i++ {
		if err := os.Remove(history[i].path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}

func logFileName(date string, index int) string {
	return fmt.Sprintf("log-%s-%d.log", date, index)
}

func parseLogFileName(name string) (string, int, bool) {
	if !strings.HasPrefix(name, "log-") || !strings.HasSuffix(name, ".log") {
		return "", 0, false
	}
	base := strings.TrimSuffix(strings.TrimPrefix(name, "log-"), ".log")
	separator := strings.LastIndexByte(base, '-')
	if separator < 0 {
		return "", 0, false
	}
	date := base[:separator]
	if _, err := time.Parse(logDateLayout, date); err != nil {
		return "", 0, false
	}
	index, err := strconv.Atoi(base[separator+1:])
	if err != nil || index < 1 || strconv.Itoa(index) != base[separator+1:] {
		return "", 0, false
	}
	return date, index, true
}
