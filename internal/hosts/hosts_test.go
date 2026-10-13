package hosts

import (
	"os"
	"path/filepath"
	"testing"
)

const base = "127.0.0.1 localhost\n::1 localhost\n"

const section = beginMarker + "\n" +
	"0.0.0.0 example.com\n:: example.com\n" +
	"0.0.0.0 www.example.com\n:: www.example.com\n" +
	endMarker + "\n"

func TestRender(t *testing.T) {
	tests := []struct {
		name    string
		content string
		domains []string
		want    string
	}{
		{name: "append", content: base, domains: []string{"example.com"}, want: base + section},
		{name: "no trailing newline", content: "127.0.0.1 localhost", domains: []string{"example.com"}, want: "127.0.0.1 localhost\n" + section},
		{name: "replace", content: base + beginMarker + "\n0.0.0.0 old.com\n" + endMarker + "\n", domains: []string{"example.com"}, want: base + section},
		{name: "empty list removes", content: base + section, domains: nil, want: base},
		{name: "www not duplicated", content: "", domains: []string{"www.example.com", "example.com"},
			want: beginMarker + "\n0.0.0.0 www.example.com\n:: www.example.com\n0.0.0.0 example.com\n:: example.com\n" + endMarker + "\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Render(tt.content, tt.domains)
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Errorf("Render() =\n%s\nwant\n%s", got, tt.want)
			}
		})
	}
}

func TestStripPreservesSurroundingContent(t *testing.T) {
	content := "# top\n" + section + "10.0.0.1 nas.lan\n"
	got, err := Strip(content)
	if err != nil {
		t.Fatal(err)
	}
	if want := "# top\n10.0.0.1 nas.lan\n"; got != want {
		t.Errorf("Strip() = %q, want %q", got, want)
	}
}

func TestStripUnterminated(t *testing.T) {
	if _, err := Strip(base + beginMarker + "\n0.0.0.0 x.com\n"); err == nil {
		t.Error("expected error for unterminated section")
	}
}

func TestApplyClear(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hosts")
	if err := os.WriteFile(path, []byte(base), 0o644); err != nil {
		t.Fatal(err)
	}

	for range 2 { // idempotent
		if err := Apply(path, []string{"example.com"}); err != nil {
			t.Fatal(err)
		}
		if got := read(t, path); got != base+section {
			t.Fatalf("after Apply:\n%s", got)
		}
	}

	for range 2 {
		if err := Clear(path); err != nil {
			t.Fatal(err)
		}
		if got := read(t, path); got != base {
			t.Fatalf("after Clear:\n%s", got)
		}
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o644 {
		t.Errorf("mode = %v, want 0644", info.Mode().Perm())
	}
}

func read(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
