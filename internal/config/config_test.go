package config

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/acgtubio/garmr/internal/schedule"
)

func TestNormalizeDomain(t *testing.T) {
	tests := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{in: "example.com", want: "example.com"},
		{in: "  Example.COM.  ", want: "example.com"},
		{in: "https://www.YouTube.com/watch?v=1", want: "youtube.com"},
		{in: "www.co", want: "www.co"},
		{in: "m.youtube.com", want: "m.youtube.com"},
		{in: "reddit.com:443/r/golang", want: "reddit.com"},
		{in: "", wantErr: true},
		{in: "localhost", wantErr: true},
		{in: "foo.localhost", wantErr: true},
		{in: "127.0.0.1", wantErr: true},
		{in: "nodot", wantErr: true},
		{in: "-bad.com", wantErr: true},
		{in: "bad_char.com", wantErr: true},
	}
	for _, tt := range tests {
		got, err := NormalizeDomain(tt.in)
		if tt.wantErr {
			if err == nil {
				t.Errorf("NormalizeDomain(%q) = %q, want error", tt.in, got)
			}
			continue
		}
		if err != nil || got != tt.want {
			t.Errorf("NormalizeDomain(%q) = %q, %v; want %q", tt.in, got, err, tt.want)
		}
	}
}

func domains(c *Config) []string {
	var out []string
	for _, s := range c.Sites {
		out = append(out, s.Domain)
	}
	return out
}

func win(s string) schedule.Window {
	w, err := schedule.ParseWindow(s)
	if err != nil {
		panic(err)
	}
	return w
}

func TestAddRemove(t *testing.T) {
	var c Config
	for _, s := range []string{"b.com", "https://A.com/x", "b.com"} {
		if _, _, err := c.Add(s, nil); err != nil {
			t.Fatal(err)
		}
	}
	if want := []string{"a.com", "b.com"}; !slices.Equal(domains(&c), want) {
		t.Fatalf("Sites = %v, want %v", domains(&c), want)
	}
	if _, changed, _ := c.Add("a.com", nil); changed {
		t.Error("adding duplicate reported change")
	}
	ws := []schedule.Window{win("09:00-12:00")}
	if _, changed, _ := c.Add("a.com", ws); !changed {
		t.Error("setting windows on existing site reported no change")
	}
	if _, changed, _ := c.Add("a.com", ws); changed {
		t.Error("setting identical windows reported change")
	}
	if _, changed, _ := c.Add("a.com", nil); changed || len(c.Sites[0].Windows) != 1 {
		t.Error("re-adding without windows should keep existing windows")
	}
	if _, changed, _ := c.Remove("A.COM"); !changed {
		t.Error("remove a.com reported no change")
	}
	if _, changed, _ := c.Remove("missing.com"); changed {
		t.Error("removing missing site reported change")
	}
	if want := []string{"b.com"}; !slices.Equal(domains(&c), want) {
		t.Fatalf("Sites = %v, want %v", domains(&c), want)
	}
}

func TestActiveSites(t *testing.T) {
	at := func(h, m int) time.Time { return time.Date(2026, 1, 1, h, m, 0, 0, time.Local) }
	c := Config{Sites: []Site{
		{Domain: "default.com"},
		{Domain: "own.com", Windows: []schedule.Window{win("09:00-10:00"), win("20:00-21:00")}},
	}}

	// No default schedule: sites without windows are always blocked.
	if got, want := c.ActiveSites(at(15, 0)), []string{"default.com"}; !slices.Equal(got, want) {
		t.Errorf("no schedule, 15:00: %v, want %v", got, want)
	}

	sched := win("12:00-18:00")
	c.Schedule = &sched
	tests := []struct {
		h, m int
		want []string
	}{
		{8, 0, nil},
		{9, 30, []string{"own.com"}},
		{12, 0, []string{"default.com"}},
		{20, 59, []string{"own.com"}},
		{21, 0, nil},
	}
	for _, tt := range tests {
		if got := c.ActiveSites(at(tt.h, tt.m)); !slices.Equal(got, tt.want) {
			t.Errorf("%02d:%02d: %v, want %v", tt.h, tt.m, got, tt.want)
		}
	}
}

func TestSiteJSON(t *testing.T) {
	sched := win("09:00-17:00")
	c := Config{Schedule: &sched, Sites: []Site{
		{Domain: "a.com"},
		{Domain: "b.com", Windows: []schedule.Window{win("22:00-06:00")}},
	}}
	path := filepath.Join(t.TempDir(), "config.json")
	if err := c.Save(path); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), `"a.com"`) || strings.Contains(string(data), `"domain": "a.com"`) {
		t.Errorf("site without windows should be a bare string:\n%s", data)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if *got.Schedule != sched || !slices.EqualFunc(got.Sites, c.Sites, func(a, b Site) bool {
		return a.Domain == b.Domain && slices.Equal(a.Windows, b.Windows)
	}) {
		t.Errorf("round trip = %+v, want %+v", got, c)
	}
}

func TestLoadLegacyAndInvalid(t *testing.T) {
	dir := t.TempDir()
	legacy := filepath.Join(dir, "legacy.json")
	os.WriteFile(legacy, []byte(`{"sites": ["Example.com", "www.reddit.com"]}`), 0o644)
	c, err := Load(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"example.com", "reddit.com"}; !slices.Equal(domains(c), want) {
		t.Errorf("legacy sites = %v, want %v", domains(c), want)
	}

	for name, body := range map[string]string{
		"bad domain": `{"sites": ["evil.com\n1.2.3.4 bank.com"]}`,
		"bad window": `{"sites": [{"domain": "a.com", "windows": [{"start": "25:00", "end": "01:00"}]}]}`,
	} {
		p := filepath.Join(dir, "bad.json")
		os.WriteFile(p, []byte(body), 0o644)
		if _, err := Load(p); err == nil {
			t.Errorf("%s: Load succeeded, want error", name)
		}
	}
}

func TestLoadSave(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "config.json")

	c, err := Load(path)
	if err != nil {
		t.Fatalf("Load missing file: %v", err)
	}
	if len(c.Sites) != 0 {
		t.Fatalf("Sites = %v, want empty", c.Sites)
	}

	c.Sites = []Site{{Domain: "example.com"}}
	if err := c.Save(path); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(domains(got), domains(c)) {
		t.Fatalf("round trip = %v, want %v", domains(got), domains(c))
	}
}
