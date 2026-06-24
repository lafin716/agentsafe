// Package applog is agentsafe's own program logger. It writes baseline
// (info+error) structured logs as JSON lines to an app-level file and lets a
// consumer install a tap to mirror each record live (the desktop app emits
// those records to its Log Console). The log level is held in a slog.LevelVar
// so it can be switched (info <-> debug) at runtime without a restart.
//
// The package is frontend-agnostic on purpose: it never imports Wails. The
// desktop binding connects event emission through SetTap, mirroring the
// internal/output.SetSink pattern, so internal/ stays decoupled from any UI.
//
// Redaction by construction: callers pass only safe fields (operation, repo,
// feature, path, duration, error message). There is no API that accepts secret
// material, file contents, tokens, or credentials.
package applog

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// defaultMaxSize is the size threshold at which the log file rotates once
// (agentsafe.log -> agentsafe.log.1).
const defaultMaxSize int64 = 5 * 1024 * 1024

// Entry is a single log record in the shape mirrored to the tap and written to
// the file. It carries only safe fields.
type Entry struct {
	Time  string         `json:"time"`
	Level string         `json:"level"`
	Msg   string         `json:"msg"`
	Attrs map[string]any `json:"attrs,omitempty"`
}

var (
	mu       sync.Mutex
	levelVar = new(slog.LevelVar) // zero value == slog.LevelInfo
	logger   *slog.Logger
	writer   *rotatingWriter

	tapMu sync.RWMutex
	tap   func(Entry)
)

// Init opens (or creates) the log file and installs a JSON-lines logger at the
// baseline info level. An empty path resolves to the app-level default
// (LogFilePath). Safe to call again; it replaces the previous writer.
func Init(path string) error { return initWithMaxSize(path, defaultMaxSize) }

func initWithMaxSize(path string, maxSize int64) error {
	if path == "" {
		p, err := LogFilePath()
		if err != nil {
			return err
		}
		path = p
	}
	w, err := newRotatingWriter(path, maxSize)
	if err != nil {
		return err
	}
	mu.Lock()
	defer mu.Unlock()
	if writer != nil {
		_ = writer.Close()
	}
	writer = w
	levelVar.Set(slog.LevelInfo)
	logger = slog.New(&handler{w: w, level: levelVar})
	return nil
}

// SetTap installs (or clears, with nil) a callback that receives every emitted
// record. The desktop app uses it to stream records to the Log Console.
func SetTap(t func(Entry)) {
	tapMu.Lock()
	tap = t
	tapMu.Unlock()
}

func getTap() func(Entry) {
	tapMu.RLock()
	defer tapMu.RUnlock()
	return tap
}

// SetLevel switches the runtime log level. Accepted: debug, info, warn, error
// (case-insensitive). Developer mode toggles between debug and info.
func SetLevel(name string) error {
	lvl, err := parseLevel(name)
	if err != nil {
		return err
	}
	levelVar.Set(lvl)
	return nil
}

// Level reports the current level name.
func Level() string { return strings.ToLower(levelVar.Level().String()) }

func parseLevel(name string) (slog.Level, error) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "debug":
		return slog.LevelDebug, nil
	case "info", "":
		return slog.LevelInfo, nil
	case "warn", "warning":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	}
	return 0, fmt.Errorf("invalid log level %q: must be debug, info, warn, or error", name)
}

// Debug/Info/Warn/Error log a message with optional key/value safe fields
// (slog-style alternating args). They no-op until Init has been called.
func Debug(msg string, args ...any) { emit(slog.LevelDebug, msg, args...) }
func Info(msg string, args ...any)  { emit(slog.LevelInfo, msg, args...) }
func Warn(msg string, args ...any)  { emit(slog.LevelWarn, msg, args...) }
func Error(msg string, args ...any) { emit(slog.LevelError, msg, args...) }

func emit(level slog.Level, msg string, args ...any) {
	mu.Lock()
	l := logger
	mu.Unlock()
	if l == nil {
		return
	}
	l.Log(context.Background(), level, msg, args...)
}

// LogDir is the app-level directory holding agentsafe logs
// (os.UserConfigDir()/agentsafe/logs). On Windows this is under %AppData%.
func LogDir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "agentsafe", "logs"), nil
}

// LogFilePath is the active log file path.
func LogFilePath() (string, error) {
	dir, err := LogDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "agentsafe.log"), nil
}

// handler is a slog.Handler that writes each record as one JSON line to the
// rotating writer and mirrors it to the tap. Groups are unused by agentsafe and
// are intentionally not supported.
type handler struct {
	w     *rotatingWriter
	level *slog.LevelVar
	attrs []slog.Attr
}

func (h *handler) Enabled(_ context.Context, l slog.Level) bool {
	return l >= h.level.Level()
}

func (h *handler) Handle(_ context.Context, r slog.Record) error {
	attrs := map[string]any{}
	for _, a := range h.attrs {
		attrs[a.Key] = attrValue(a)
	}
	r.Attrs(func(a slog.Attr) bool {
		attrs[a.Key] = attrValue(a)
		return true
	})
	if len(attrs) == 0 {
		attrs = nil
	}
	e := Entry{
		Time:  r.Time.Format("2006-01-02T15:04:05.000Z07:00"),
		Level: strings.ToLower(r.Level.String()),
		Msg:   r.Message,
		Attrs: attrs,
	}
	if t := getTap(); t != nil {
		t(e)
	}
	b, err := json.Marshal(e)
	if err != nil {
		return err
	}
	b = append(b, '\n')
	_, err = h.w.Write(b)
	return err
}

func (h *handler) WithAttrs(as []slog.Attr) slog.Handler {
	nh := *h
	nh.attrs = append(append([]slog.Attr{}, h.attrs...), as...)
	return &nh
}

func (h *handler) WithGroup(string) slog.Handler { return h }

// attrValue normalizes a slog attr value for JSON. Errors are rendered as their
// message (the default error type marshals to {}), so error fields stay useful.
func attrValue(a slog.Attr) any {
	v := a.Value.Any()
	if err, ok := v.(error); ok {
		if err == nil {
			return nil
		}
		return err.Error()
	}
	return v
}
