package applog

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}

// cleanup closes the global writer before t.TempDir's RemoveAll runs (LIFO), so
// Windows can delete the log file the writer would otherwise hold open.
func cleanup(t *testing.T) {
	t.Cleanup(func() {
		SetTap(nil)
		mu.Lock()
		w := writer
		writer, logger = nil, nil
		mu.Unlock()
		if w != nil {
			_ = w.Close()
		}
	})
}

// TestLevelSwitch verifies the baseline suppresses debug while always keeping
// errors, and that SetLevel("debug") takes effect at runtime.
func TestLevelSwitch(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agentsafe.log")
	if err := Init(path); err != nil {
		t.Fatal(err)
	}
	cleanup(t)

	Debug("d1") // info baseline -> suppressed
	Error("e1") // always recorded
	content := readFile(t, path)
	if strings.Contains(content, `"msg":"d1"`) {
		t.Errorf("debug record leaked at info level:\n%s", content)
	}
	if !strings.Contains(content, `"msg":"e1"`) {
		t.Errorf("error record missing at info level:\n%s", content)
	}

	if err := SetLevel("debug"); err != nil {
		t.Fatal(err)
	}
	Debug("d2")
	content = readFile(t, path)
	if !strings.Contains(content, `"msg":"d2"`) {
		t.Errorf("debug record missing after SetLevel(debug):\n%s", content)
	}
}

func TestSetLevelInvalid(t *testing.T) {
	if err := SetLevel("verbose"); err == nil {
		t.Error("expected error for invalid level, got nil")
	}
	if err := SetLevel("debug"); err != nil {
		t.Errorf("debug should be valid: %v", err)
	}
}

// TestRotation verifies the file rotates once past the size cap, preserving the
// prior contents in "<path>.1" and starting a fresh current file.
func TestRotation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agentsafe.log")
	if err := initWithMaxSize(path, 200); err != nil {
		t.Fatal(err)
	}
	cleanup(t)

	for i := 0; i < 50; i++ {
		Info("line", "i", i, "pad", "xxxxxxxxxxxxxxxxxxxx")
	}

	rotated := path + ".1"
	info, err := os.Stat(rotated)
	if err != nil {
		t.Fatalf("expected rotated file %s: %v", rotated, err)
	}
	if info.Size() == 0 {
		t.Errorf("rotated file is empty")
	}
	if got := readFile(t, path); !strings.Contains(got, `"i":49`) {
		t.Errorf("current log missing most recent line:\n%s", got)
	}
}

// TestTap verifies the tap receives each record with its safe attrs, and that
// an error value is normalized to its message rather than an empty object.
func TestTap(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agentsafe.log")
	if err := Init(path); err != nil {
		t.Fatal(err)
	}
	var (
		mu  sync.Mutex
		got []Entry
	)
	SetTap(func(e Entry) {
		mu.Lock()
		got = append(got, e)
		mu.Unlock()
	})
	cleanup(t)

	Info("hello", "repo", "demo", "err", errors.New("boom"))

	mu.Lock()
	defer mu.Unlock()
	if len(got) != 1 {
		t.Fatalf("tap received %d entries, want 1", len(got))
	}
	e := got[0]
	if e.Msg != "hello" {
		t.Errorf("msg=%q, want hello", e.Msg)
	}
	if e.Level != "info" {
		t.Errorf("level=%q, want info", e.Level)
	}
	if e.Attrs["repo"] != "demo" {
		t.Errorf("repo attr=%v, want demo", e.Attrs["repo"])
	}
	if e.Attrs["err"] != "boom" {
		t.Errorf("err attr=%v, want boom (error normalized to message)", e.Attrs["err"])
	}
}

func TestErrorAttrSerializedToFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agentsafe.log")
	if err := Init(path); err != nil {
		t.Fatal(err)
	}
	cleanup(t)

	Error("failed", "err", errors.New("disk gone"))
	if got := readFile(t, path); !strings.Contains(got, "disk gone") {
		t.Errorf("error message not serialized to file:\n%s", got)
	}
}

func TestLogFilePath(t *testing.T) {
	p, err := LogFilePath()
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join("agentsafe", "logs", "agentsafe.log")
	if !strings.HasSuffix(p, want) {
		t.Errorf("LogFilePath()=%q, want suffix %q", p, want)
	}
}
