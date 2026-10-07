// Package diagnostics provides a small persistent, privacy-aware event log.
// It is intentionally independent from the VPN runtime so events remain
// available after a tunnel is stopped or the Flutter window is closed.
package diagnostics

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	DefaultMaxBytes = int64(5 * 1024 * 1024)
	DefaultBackups  = 3
	logFileName     = "events.jsonl"
	maxMessageBytes = 4096
)

var (
	ansiPattern       = regexp.MustCompile(`\x1b\[[0-9;]*[[:alpha:]]`)
	urlPattern        = regexp.MustCompile(`(?i)https?://[^\s"'<>]+`)
	bearerPattern     = regexp.MustCompile(`(?i)bearer\s+[a-z0-9._~+/=-]+`)
	credentialPattern = regexp.MustCompile(`(?i)(password|token|secret|uuid)\s*[:=]\s*[^\s,;]+`)
)

// Entry is the safe subset exposed to the local diagnostics UI.
type Entry struct {
	Timestamp time.Time      `json:"timestamp"`
	Level     string         `json:"level"`
	Component string         `json:"component"`
	Event     string         `json:"event"`
	Message   string         `json:"message,omitempty"`
	Fields    map[string]any `json:"fields,omitempty"`
}

// Logger writes newline-delimited JSON with bounded local rotation.
type Logger struct {
	mu       sync.Mutex
	path     string
	maxBytes int64
	backups  int
	file     *os.File
	now      func() time.Time
}

func New(directory string) (*Logger, error) {
	return NewWithOptions(directory, DefaultMaxBytes, DefaultBackups)
}

func NewWithOptions(directory string, maxBytes int64, backups int) (*Logger, error) {
	directory = strings.TrimSpace(directory)
	if directory == "" {
		return nil, errors.New("diagnostics directory is empty")
	}
	if maxBytes <= 0 {
		return nil, errors.New("diagnostics max size must be positive")
	}
	if backups < 0 {
		return nil, errors.New("diagnostics backup count must not be negative")
	}
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return nil, fmt.Errorf("create diagnostics directory: %w", err)
	}
	if err := os.Chmod(directory, 0o700); err != nil {
		return nil, fmt.Errorf("secure diagnostics directory: %w", err)
	}
	logger := &Logger{
		path:     filepath.Join(directory, logFileName),
		maxBytes: maxBytes,
		backups:  backups,
		now:      time.Now,
	}
	if err := logger.open(); err != nil {
		return nil, err
	}
	return logger, nil
}

func (l *Logger) Info(component, event, message string, fields map[string]any) {
	l.write("info", component, event, message, fields)
}

func (l *Logger) Warn(component, event, message string, fields map[string]any) {
	l.write("warn", component, event, message, fields)
}

func (l *Logger) Error(component, event, message string, fields map[string]any) {
	l.write("error", component, event, message, fields)
}

func (l *Logger) Path() string {
	if l == nil {
		return ""
	}
	return l.path
}

func (l *Logger) Close() error {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file == nil {
		return nil
	}
	err := l.file.Close()
	l.file = nil
	return err
}

// Recent returns entries in chronological order, including rotated files.
func (l *Logger) Recent(limit int) ([]Entry, error) {
	if l == nil {
		return []Entry{}, nil
	}
	if limit <= 0 {
		limit = 200
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file != nil {
		_ = l.file.Sync()
	}
	entries := make([]Entry, 0, limit)
	for index := l.backups; index >= 1; index-- {
		var err error
		entries, err = readEntries(l.path+"."+strconv.Itoa(index), entries, limit)
		if err != nil {
			return nil, err
		}
	}
	var err error
	entries, err = readEntries(l.path, entries, limit)
	if err != nil {
		return nil, err
	}
	return entries, nil
}

func (l *Logger) write(level, component, event, message string, fields map[string]any) {
	if l == nil {
		return
	}
	entry := Entry{
		Timestamp: l.now().UTC(),
		Level:     cleanLabel(level, "info"),
		Component: cleanLabel(component, "app"),
		Event:     cleanLabel(event, "event"),
		Message:   redactText(message),
		Fields:    sanitizeFields(fields),
	}
	encoded, err := json.Marshal(entry)
	if err != nil {
		return
	}
	encoded = append(encoded, '\n')

	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file == nil {
		if err := l.open(); err != nil {
			return
		}
	}
	if err := l.rotateIfNeeded(int64(len(encoded))); err != nil {
		return
	}
	_, _ = l.file.Write(encoded)
}

func (l *Logger) open() error {
	file, err := os.OpenFile(l.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("open diagnostics log: %w", err)
	}
	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		return fmt.Errorf("secure diagnostics log: %w", err)
	}
	l.file = file
	return nil
}

func (l *Logger) rotateIfNeeded(incoming int64) error {
	info, err := l.file.Stat()
	if err != nil {
		return fmt.Errorf("stat diagnostics log: %w", err)
	}
	if info.Size() == 0 || info.Size()+incoming <= l.maxBytes {
		return nil
	}
	if err := l.file.Close(); err != nil {
		return fmt.Errorf("close diagnostics log for rotation: %w", err)
	}
	l.file = nil
	if l.backups == 0 {
		if err := os.Remove(l.path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("truncate diagnostics log: %w", err)
		}
		return l.open()
	}
	oldest := l.path + "." + strconv.Itoa(l.backups)
	if err := os.Remove(oldest); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove oldest diagnostics log: %w", err)
	}
	for index := l.backups - 1; index >= 1; index-- {
		from := l.path + "." + strconv.Itoa(index)
		to := l.path + "." + strconv.Itoa(index+1)
		if err := os.Rename(from, to); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("rotate diagnostics log: %w", err)
		}
	}
	if err := os.Rename(l.path, l.path+".1"); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("rotate current diagnostics log: %w", err)
	}
	return l.open()
}

func readEntries(path string, entries []Entry, limit int) ([]Entry, error) {
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return entries, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read diagnostics log: %w", err)
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 4096), 64*1024)
	for scanner.Scan() {
		var entry Entry
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			continue
		}
		entries = append(entries, entry)
		if len(entries) > limit {
			copy(entries, entries[len(entries)-limit:])
			entries = entries[:limit]
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan diagnostics log: %w", err)
	}
	return entries, nil
}

func cleanLabel(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	if len(value) > 64 {
		value = value[:64]
	}
	return value
}

func sanitizeFields(fields map[string]any) map[string]any {
	if len(fields) == 0 {
		return nil
	}
	result := make(map[string]any, len(fields))
	for key, value := range fields {
		key = cleanLabel(key, "field")
		if sensitiveKey(key) {
			result[key] = "<redacted>"
			continue
		}
		switch current := value.(type) {
		case nil, bool, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64:
			result[key] = current
		case string:
			result[key] = redactText(current)
		case fmt.Stringer:
			result[key] = redactText(current.String())
		case error:
			result[key] = redactText(current.Error())
		default:
			result[key] = redactText(fmt.Sprint(current))
		}
	}
	return result
}

func sensitiveKey(key string) bool {
	normalized := strings.ToLower(key)
	for _, fragment := range []string{"password", "token", "secret", "credential", "config", "source_url", "uuid"} {
		if strings.Contains(normalized, fragment) {
			return true
		}
	}
	return false
}

func redactText(value string) string {
	value = ansiPattern.ReplaceAllString(strings.TrimSpace(value), "")
	value = bearerPattern.ReplaceAllString(value, "Bearer <redacted>")
	value = credentialPattern.ReplaceAllString(value, "$1=<redacted>")
	value = urlPattern.ReplaceAllString(value, "<redacted-url>")
	if len(value) > maxMessageBytes {
		value = value[:maxMessageBytes] + "…"
	}
	return value
}
