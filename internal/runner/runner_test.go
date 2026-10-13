package runner

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

const base = "127.0.0.1 localhost\n"

func setup(t *testing.T, cfg string) (cfgPath, hostsPath string) {
	t.Helper()
	dir := t.TempDir()
	cfgPath = filepath.Join(dir, "config.json")
	hostsPath = filepath.Join(dir, "hosts")
	if err := os.WriteFile(cfgPath, []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(hostsPath, []byte(base), 0o644); err != nil {
		t.Fatal(err)
	}
	return cfgPath, hostsPath
}

func at(h, m int) time.Time { return time.Date(2026, 1, 1, h, m, 0, 0, time.Local) }

func TestTick(t *testing.T) {
	cfgPath, hostsPath := setup(t, `{
		"schedule": {"start": "09:00", "end": "17:00"},
		"sites": ["a.com", {"domain": "b.com", "windows": [{"start": "20:00", "end": "22:00"}]}]
	}`)

	tests := []struct {
		now  time.Time
		want []string
	}{
		{at(10, 0), []string{"a.com"}},
		{at(18, 0), nil},
		{at(21, 0), []string{"b.com"}},
	}
	for _, tt := range tests {
		got, err := Tick(cfgPath, hostsPath, tt.now)
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(got, tt.want) {
			t.Errorf("%s: active = %v, want %v", tt.now.Format("15:04"), got, tt.want)
		}
		data, _ := os.ReadFile(hostsPath)
		for _, d := range []string{"a.com", "b.com"} {
			if blocked := strings.Contains(string(data), "0.0.0.0 "+d+"\n"); blocked != slices.Contains(tt.want, d) {
				t.Errorf("%s: %s blocked in hosts = %v", tt.now.Format("15:04"), d, blocked)
			}
		}
		if len(tt.want) == 0 && string(data) != base {
			t.Errorf("%s: hosts = %q, want untouched base", tt.now.Format("15:04"), data)
		}
	}
}

func TestTickBadConfigLeavesHosts(t *testing.T) {
	cfgPath, hostsPath := setup(t, `{"sites": ["a.com"]}`)
	if _, err := Tick(cfgPath, hostsPath, at(10, 0)); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(hostsPath)

	os.WriteFile(cfgPath, []byte(`{not json`), 0o644)
	if _, err := Tick(cfgPath, hostsPath, at(10, 0)); err == nil {
		t.Fatal("expected error for bad config")
	}
	if after, _ := os.ReadFile(hostsPath); !bytes.Equal(before, after) {
		t.Errorf("hosts changed after bad config:\n%s", after)
	}
}

func TestRunStopsAndKeepsBlocks(t *testing.T) {
	cfgPath, hostsPath := setup(t, `{"sites": ["a.com"]}`)
	ctx, cancel := context.WithCancel(context.Background())
	var log bytes.Buffer
	done := make(chan error)
	go func() { done <- Run(ctx, cfgPath, hostsPath, &log) }()

	deadline := time.Now().Add(2 * time.Second)
	for {
		data, _ := os.ReadFile(hostsPath)
		if strings.Contains(string(data), "0.0.0.0 a.com") {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("Run did not apply blocks")
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(hostsPath); !strings.Contains(string(data), "0.0.0.0 a.com") {
		t.Error("blocks were removed on exit")
	}
	if !strings.Contains(log.String(), "blocking 1 site(s): a.com") {
		t.Errorf("log = %q", log.String())
	}
}

func TestLock(t *testing.T) {
	path := filepath.Join(t.TempDir(), "garmr.lock")

	if held, err := Held(path); err != nil || held {
		t.Fatalf("Held before create = %v, %v", held, err)
	}
	l, err := Acquire(path)
	if err != nil {
		t.Fatal(err)
	}
	if held, err := Held(path); err != nil || !held {
		t.Errorf("Held while locked = %v, %v", held, err)
	}
	if _, err := Acquire(path); !errors.Is(err, ErrLocked) {
		t.Errorf("second Acquire err = %v, want ErrLocked", err)
	}
	if err := l.Release(); err != nil {
		t.Fatal(err)
	}
	if held, err := Held(path); err != nil || held {
		t.Errorf("Held after release = %v, %v", held, err)
	}
}
