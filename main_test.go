package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/acgtubio/garmr/internal/runner"
)

func TestVersion(t *testing.T) {
	var out bytes.Buffer
	if err := run([]string{"-version"}, &out); err != nil {
		t.Fatal(err)
	}
	if got, want := out.String(), "garmr dev\n"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

type env struct {
	t                *testing.T
	cfg, hosts, lock string
}

const baseHosts = "127.0.0.1 localhost\n"

func newEnv(t *testing.T) *env {
	dir := t.TempDir()
	e := &env{t: t, cfg: filepath.Join(dir, "config.json"), hosts: filepath.Join(dir, "hosts"), lock: filepath.Join(dir, "garmr.lock")}
	if err := os.WriteFile(e.hosts, []byte(baseHosts), 0o644); err != nil {
		t.Fatal(err)
	}
	return e
}

func (e *env) exec(args ...string) (string, error) {
	var out bytes.Buffer
	err := run(append([]string{"-config", e.cfg, "-hosts", e.hosts, "-lock", e.lock}, args...), &out)
	return out.String(), err
}

func (e *env) ok(args ...string) string {
	e.t.Helper()
	out, err := e.exec(args...)
	if err != nil {
		e.t.Fatalf("run(%v): %v", args, err)
	}
	return out
}

func (e *env) hostsFile() string {
	data, err := os.ReadFile(e.hosts)
	if err != nil {
		e.t.Fatal(err)
	}
	return string(data)
}

func setNow(t *testing.T, h, m int) {
	old := now
	now = func() time.Time { return time.Date(2026, 1, 1, h, m, 0, 0, time.Local) }
	t.Cleanup(func() { now = old })
}

func TestBlockApplyClear(t *testing.T) {
	e := newEnv(t)

	if got := e.ok("block", "add", "https://Example.com/x", "reddit.com"); !strings.Contains(got, "blocked: example.com") {
		t.Errorf("add output = %q", got)
	}
	if got := e.ok("block", "add", "example.com"); !strings.Contains(got, "already blocked: example.com") {
		t.Errorf("duplicate add output = %q", got)
	}
	if got := e.ok("block", "list"); !strings.Contains(got, "example.com") || !strings.Contains(got, "(default)") {
		t.Errorf("list = %q", got)
	}

	e.ok("apply") // no schedule: always blocked
	for _, want := range []string{"0.0.0.0 example.com", "0.0.0.0 www.reddit.com"} {
		if !strings.Contains(e.hostsFile(), want) {
			t.Errorf("hosts missing %q:\n%s", want, e.hostsFile())
		}
	}

	e.ok("block", "remove", "reddit.com")
	if got := e.ok("block", "list"); strings.Contains(got, "reddit.com") {
		t.Errorf("list after remove = %q", got)
	}

	e.ok("clear")
	if got := e.hostsFile(); got != baseHosts {
		t.Errorf("hosts after clear = %q, want %q", got, baseHosts)
	}
}

func TestScheduledApply(t *testing.T) {
	e := newEnv(t)
	e.ok("schedule", "set", "09:00-17:00")
	e.ok("block", "add", "work.com")
	if got := e.ok("block", "add", "-window", "20:00-22:00", "-window", "06:00-07:00", "night.com"); !strings.Contains(got, "06:00-07:00") {
		t.Errorf("add with windows output = %q", got)
	}

	tests := []struct {
		h, m        int
		work, night bool
	}{
		{10, 0, true, false},
		{18, 0, false, false},
		{21, 0, false, true},
		{6, 30, false, true},
	}
	for _, tt := range tests {
		setNow(t, tt.h, tt.m)
		e.ok("apply")
		hostsData := e.hostsFile()
		if got := strings.Contains(hostsData, "0.0.0.0 work.com\n"); got != tt.work {
			t.Errorf("%02d:%02d work.com blocked = %v, want %v", tt.h, tt.m, got, tt.work)
		}
		if got := strings.Contains(hostsData, "0.0.0.0 night.com\n"); got != tt.night {
			t.Errorf("%02d:%02d night.com blocked = %v, want %v", tt.h, tt.m, got, tt.night)
		}
	}

	setNow(t, 10, 0)
	status := e.ok("status")
	for _, want := range []string{"default:  09:00-17:00", "BLOCKED  work.com", "allowed  night.com", "runner:   not running"} {
		if !strings.Contains(status, want) {
			t.Errorf("status missing %q:\n%s", want, status)
		}
	}

	e.ok("schedule", "clear")
	if status := e.ok("status"); !strings.Contains(status, "default:  all day") {
		t.Errorf("status after schedule clear:\n%s", status)
	}
}

func TestLockedByRunner(t *testing.T) {
	e := newEnv(t)
	e.ok("block", "add", "a.com")

	l, err := runner.Acquire(e.lock)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Release()

	for _, cmd := range []string{"apply", "clear", "run"} {
		if _, err := e.exec(cmd); err == nil {
			t.Errorf("%s succeeded while runner holds the lock", cmd)
		}
	}
	if status := e.ok("status"); !strings.Contains(status, "runner:   active") {
		t.Errorf("status = %q", status)
	}
	if got := e.hostsFile(); got != baseHosts {
		t.Errorf("hosts modified while locked: %q", got)
	}
}

func TestErrors(t *testing.T) {
	e := newEnv(t)
	for _, args := range [][]string{
		{"bogus"},
		{"block"},
		{"block", "add"},
		{"block", "frob"},
		{"block", "add", "localhost"},
		{"block", "add", "-window", "9-5", "a.com"},
		{"schedule"},
		{"schedule", "set", "25:00-01:00"},
	} {
		if _, err := e.exec(args...); err == nil {
			t.Errorf("run(%v) succeeded, want error", args)
		}
	}
}
