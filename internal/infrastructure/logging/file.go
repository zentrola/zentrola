package logging

import (
	"errors"
	"fmt"
	"io"
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
	appLogName    = "app"
	errorLogName  = "error"
)

// RotatingFile 按 UTC 日期和大小轮转一种日志文件。每次 Write 对应一整条日志，不会拆分记录。
type RotatingFile struct {
	directory     string
	name          string
	maxBytes      int64
	maxBackups    int
	retentionDays int
	mu            sync.Mutex
	file          *os.File
	lock          *sharedFileLock
	rotation      *rotationGroup
	cleanup       *cleanupReporter
	remove        func(string) error
	size          int64
	date          string
	index         int
	now           func() time.Time
}

type sharedFileLock struct {
	mu        sync.Mutex
	file      *os.File
	remaining int
	result    error
}

type rotationGroup struct {
	mu      sync.Mutex
	writers []*RotatingFile
}

type cleanupReporter struct {
	once sync.Once
	out  io.Writer
}

// OpenRotatingFile 打开主日志文件。新代码需要同时输出错误日志时应使用 OpenRotatingFiles。
func OpenRotatingFile(directory string, maxSizeMB, maxBackups int) (*RotatingFile, error) {
	writers, err := openRotatingFiles(directory, maxSizeMB, maxBackups, 0, appLogName)
	if err != nil {
		return nil, err
	}
	return writers[0], nil
}

// OpenRotatingFiles 打开共享目录锁的主日志和错误日志文件。
func OpenRotatingFiles(directory string, maxSizeMB, maxBackups, retentionDays int) (*RotatingFile, *RotatingFile, error) {
	writers, err := openRotatingFiles(directory, maxSizeMB, maxBackups, retentionDays, appLogName, errorLogName)
	if err != nil {
		return nil, nil, err
	}
	return writers[0], writers[1], nil
}

func openRotatingFiles(directory string, maxSizeMB, maxBackups, retentionDays int, names ...string) ([]*RotatingFile, error) {
	if err := os.MkdirAll(directory, 0700); err != nil {
		return nil, err
	}
	lock, err := acquireFileLock(filepath.Join(directory, logLockName))
	if err != nil {
		return nil, err
	}
	shared := &sharedFileLock{file: lock, remaining: len(names)}
	reporter := &cleanupReporter{out: os.Stderr}
	writers := make([]*RotatingFile, len(names))
	for i, name := range names {
		writers[i] = &RotatingFile{
			directory: directory, name: name,
			maxBytes: int64(maxSizeMB) << 20, maxBackups: maxBackups, retentionDays: retentionDays,
			lock: shared, cleanup: reporter, remove: os.Remove, now: time.Now,
		}
	}
	if len(writers) > 1 {
		rotation := &rotationGroup{writers: writers}
		for _, writer := range writers {
			writer.rotation = rotation
		}
	}
	for _, writer := range writers {
		if err := writer.open(); err != nil {
			for _, opened := range writers {
				_ = opened.Close()
			}
			return nil, err
		}
	}
	return writers, nil
}

func (w *RotatingFile) Write(data []byte) (int, error) {
	date := w.currentDate()
	if w.rotation != nil {
		if err := w.rotation.rotate(date); err != nil {
			return 0, err
		}
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file == nil {
		return 0, os.ErrClosed
	}
	if w.rotation == nil && date != w.date {
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

func (g *rotationGroup) rotate(date string) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	for _, writer := range g.writers {
		writer.mu.Lock()
		if writer.file != nil && writer.date != date {
			if err := writer.openLatest(date); err != nil {
				writer.mu.Unlock()
				return err
			}
		}
		writer.mu.Unlock()
	}
	return nil
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
		result = errors.Join(result, w.lock.release())
		w.lock = nil
	}
	return result
}

func (l *sharedFileLock) release() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.remaining == 0 {
		return l.result
	}
	l.remaining--
	if l.remaining == 0 && l.file != nil {
		l.result = l.file.Close()
		l.file = nil
	}
	return l.result
}

func (w *RotatingFile) open() error {
	if w.name == "" {
		w.name = appLogName
	}
	if w.now == nil {
		w.now = time.Now
	}
	if w.remove == nil {
		w.remove = os.Remove
	}
	return w.openLatest(w.currentDate())
}

func (w *RotatingFile) currentDate() string {
	return w.now().UTC().Format(logDateLayout)
}

func (w *RotatingFile) openLatest(date string) error {
	index := 1
	dateDirectory := filepath.Join(w.directory, date)
	entries, err := os.ReadDir(dateDirectory)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		candidate, ok := parseLogFileName(entry.Name(), w.name)
		if ok && candidate > index {
			index = candidate
		}
	}
	path := filepath.Join(dateDirectory, logFileName(w.name, index))
	if info, err := os.Stat(path); err == nil {
		if info.Size() >= w.maxBytes {
			index++
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return w.openFile(date, index)
}

func (w *RotatingFile) openFile(date string, index int) error {
	dateDirectory := filepath.Join(w.directory, date)
	if err := os.MkdirAll(dateDirectory, 0700); err != nil {
		return err
	}
	path := filepath.Join(dateDirectory, logFileName(w.name, index))
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
	w.cleanupExpiredFiles(path)
	return nil
}

type logFile struct {
	path  string
	date  string
	index int
}

func (w *RotatingFile) logFiles() ([]logFile, error) {
	dateEntries, err := os.ReadDir(w.directory)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var files []logFile
	for _, dateEntry := range dateEntries {
		if !dateEntry.IsDir() || !validLogDate(dateEntry.Name()) {
			continue
		}
		dateDirectory := filepath.Join(w.directory, dateEntry.Name())
		entries, err := os.ReadDir(dateDirectory)
		if err != nil {
			return nil, err
		}
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			index, ok := parseLogFileName(entry.Name(), w.name)
			if ok {
				files = append(files, logFile{
					path: filepath.Join(dateDirectory, entry.Name()), date: dateEntry.Name(), index: index,
				})
			}
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

func (w *RotatingFile) cleanupExpiredFiles(activePath string) {
	if err := w.removeExpiredFiles(activePath); err != nil && w.cleanup != nil {
		w.cleanup.report(err)
	}
}

func (r *cleanupReporter) report(err error) {
	if r.out == nil {
		return
	}
	r.once.Do(func() {
		_, _ = fmt.Fprintf(r.out, "logging cleanup failed: %v\n", err)
	})
}

func (w *RotatingFile) removeExpiredFiles(activePath string) error {
	files, err := w.logFiles()
	if err != nil {
		return err
	}
	var result error
	retained := make([]logFile, 0, len(files))
	cutoff := ""
	if w.retentionDays > 0 {
		current, err := time.Parse(logDateLayout, w.date)
		if err != nil {
			return err
		}
		cutoff = current.AddDate(0, 0, -(w.retentionDays - 1)).Format(logDateLayout)
	}
	for _, file := range files {
		if cutoff != "" && file.path != activePath && file.date < cutoff {
			if err := w.removeLogFile(file.path); err != nil {
				result = errors.Join(result, err)
				retained = append(retained, file)
			}
			continue
		}
		retained = append(retained, file)
	}
	history := make([]logFile, 0, len(retained))
	for _, file := range retained {
		if file.path != activePath {
			history = append(history, file)
		}
	}
	removeCount := len(history) - w.maxBackups
	for i := 0; i < removeCount; i++ {
		if err := w.removeLogFile(history[i].path); err != nil {
			result = errors.Join(result, err)
		}
	}
	return result
}

func (w *RotatingFile) removeLogFile(path string) error {
	if err := w.remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return removeEmptyDirectory(filepath.Dir(path))
}

func removeEmptyDirectory(directory string) error {
	entries, err := os.ReadDir(directory)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil || len(entries) != 0 {
		return err
	}
	if err := os.Remove(directory); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func logFileName(name string, index int) string {
	return fmt.Sprintf("%s-%d.log", name, index)
}

func parseLogFileName(fileName, name string) (int, bool) {
	prefix := name + "-"
	if !strings.HasPrefix(fileName, prefix) || !strings.HasSuffix(fileName, ".log") {
		return 0, false
	}
	base := strings.TrimSuffix(strings.TrimPrefix(fileName, prefix), ".log")
	index, err := strconv.Atoi(base)
	if err != nil || index < 1 || strconv.Itoa(index) != base {
		return 0, false
	}
	return index, true
}

func validLogDate(value string) bool {
	parsed, err := time.Parse(logDateLayout, value)
	return err == nil && parsed.Format(logDateLayout) == value
}
