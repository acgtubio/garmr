// Package hosts enforces a blocklist by maintaining a marked section in a
// hosts file. Apply and Clear are idempotent, so a scheduler can call them
// repeatedly without tracking state.
package hosts

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// DefaultPath is the system hosts file.
const DefaultPath = "/etc/hosts"

const (
	beginMarker = "# BEGIN garmr (managed; do not edit)"
	beginPrefix = "# BEGIN garmr"
	endMarker   = "# END garmr"
)

// Render returns content with the garmr section replaced by entries for
// domains. Content outside the section is preserved. An empty domain list
// removes the section.
func Render(content string, domains []string) (string, error) {
	out, err := Strip(content)
	if err != nil {
		return "", err
	}
	if len(domains) == 0 {
		return out, nil
	}
	var b strings.Builder
	b.WriteString(out)
	if out != "" && !strings.HasSuffix(out, "\n") {
		b.WriteByte('\n')
	}
	b.WriteString(beginMarker + "\n")
	for _, d := range expand(domains) {
		fmt.Fprintf(&b, "0.0.0.0 %s\n:: %s\n", d, d)
	}
	b.WriteString(endMarker + "\n")
	return b.String(), nil
}

// Strip returns content with the garmr section removed.
func Strip(content string) (string, error) {
	lines := strings.SplitAfter(content, "\n")
	out := make([]string, 0, len(lines))
	inSection := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		switch {
		case !inSection && strings.HasPrefix(trimmed, beginPrefix):
			inSection = true
		case inSection && trimmed == endMarker:
			inSection = false
		case !inSection:
			out = append(out, line)
		}
	}
	if inSection {
		return "", errors.New("hosts file has a garmr BEGIN marker without a matching END marker; fix it manually")
	}
	return strings.Join(out, ""), nil
}

// expand adds a www. variant for each bare domain and removes duplicates,
// preserving order.
func expand(domains []string) []string {
	seen := make(map[string]bool)
	var out []string
	add := func(d string) {
		if !seen[d] {
			seen[d] = true
			out = append(out, d)
		}
	}
	for _, d := range domains {
		add(d)
		if !strings.HasPrefix(d, "www.") {
			add("www." + d)
		}
	}
	return out
}

// Apply writes the garmr section for domains into the hosts file at path.
func Apply(path string, domains []string) error {
	return update(path, func(s string) (string, error) { return Render(s, domains) })
}

// Clear removes the garmr section from the hosts file at path.
func Clear(path string) error {
	return update(path, Strip)
}

func update(path string, fn func(string) (string, error)) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	out, err := fn(string(data))
	if err != nil {
		return err
	}
	if out == string(data) {
		return nil
	}

	// Write a temp file next to the target and rename it into place so the
	// hosts file is never observed half-written.
	tmp, err := os.CreateTemp(filepath.Dir(path), ".hosts-garmr-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.WriteString(out); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(info.Mode().Perm()); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
