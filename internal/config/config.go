// Package config manages garmr's blocklist configuration: which sites to block.
// It says nothing about how or when they are blocked.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/url"
	"os"
	"os/user"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/acgtubio/garmr/internal/schedule"
)

// Config is the on-disk configuration.
type Config struct {
	// Schedule is the default blocking window for sites without their own
	// windows. Nil means those sites are always blocked.
	Schedule *schedule.Window `json:"schedule,omitempty"`
	Sites    []Site           `json:"sites"`
}

// Site is a blocked domain. Windows, if set, override Config.Schedule.
// In JSON a site without windows is written as a bare domain string.
type Site struct {
	Domain  string            `json:"domain"`
	Windows []schedule.Window `json:"windows,omitempty"`
}

func (s Site) MarshalJSON() ([]byte, error) {
	if len(s.Windows) == 0 {
		return json.Marshal(s.Domain)
	}
	type plain Site
	return json.Marshal(plain(s))
}

func (s *Site) UnmarshalJSON(data []byte) error {
	var domain string
	if err := json.Unmarshal(data, &domain); err == nil {
		*s = Site{Domain: domain}
		return nil
	}
	type plain Site
	return json.Unmarshal(data, (*plain)(s))
}

// ActiveSites returns the domains that should be blocked at time now.
func (c *Config) ActiveSites(now time.Time) []string {
	var out []string
	for _, s := range c.Sites {
		windows := s.Windows
		if len(windows) == 0 && c.Schedule != nil {
			windows = []schedule.Window{*c.Schedule}
		}
		if len(windows) == 0 || schedule.AnyContains(windows, now) {
			out = append(out, s.Domain)
		}
	}
	return out
}

// DefaultPath returns the config file location: $XDG_CONFIG_HOME/garmr/config.json
// (usually ~/.config/garmr/config.json). When running as root via sudo, it
// resolves to the invoking user's config so that `sudo garmr apply` uses the
// same file as `garmr block add`.
func DefaultPath() (string, error) {
	if sudoUser := os.Getenv("SUDO_USER"); os.Geteuid() == 0 && sudoUser != "" && sudoUser != "root" {
		u, err := user.Lookup(sudoUser)
		if err != nil {
			return "", fmt.Errorf("look up sudo user %q: %w", sudoUser, err)
		}
		return filepath.Join(u.HomeDir, ".config", "garmr", "config.json"), nil
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "garmr", "config.json"), nil
}

// Load reads the config at path. A missing file yields an empty config.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return &Config{}, nil
	}
	if err != nil {
		return nil, err
	}
	var c Config
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	// The file may be hand-edited, and domains end up verbatim in the hosts
	// file, so validate them here.
	for i, s := range c.Sites {
		d, err := NormalizeDomain(s.Domain)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		c.Sites[i].Domain = d
	}
	return &c, nil
}

// Save atomically writes the config to path, creating parent directories.
func (c *Config) Save(path string) error {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".config-*.json")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(0o644); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// Add normalizes site and adds it to the blocklist. If the site already exists
// and windows is non-empty, its windows are replaced. It reports whether the
// config changed.
func (c *Config) Add(site string, windows []schedule.Window) (string, bool, error) {
	d, err := NormalizeDomain(site)
	if err != nil {
		return "", false, err
	}
	if i := c.index(d); i >= 0 {
		if len(windows) == 0 || slices.Equal(c.Sites[i].Windows, windows) {
			return d, false, nil
		}
		c.Sites[i].Windows = slices.Clone(windows)
		return d, true, nil
	}
	c.Sites = append(c.Sites, Site{Domain: d, Windows: slices.Clone(windows)})
	slices.SortFunc(c.Sites, func(a, b Site) int { return strings.Compare(a.Domain, b.Domain) })
	return d, true, nil
}

// Remove normalizes site and removes it from the blocklist. It reports whether
// the config changed.
func (c *Config) Remove(site string) (string, bool, error) {
	d, err := NormalizeDomain(site)
	if err != nil {
		return "", false, err
	}
	i := c.index(d)
	if i < 0 {
		return d, false, nil
	}
	c.Sites = slices.Delete(c.Sites, i, i+1)
	return d, true, nil
}

func (c *Config) index(domain string) int {
	return slices.IndexFunc(c.Sites, func(s Site) bool { return s.Domain == domain })
}

// NormalizeDomain turns user input such as "https://www.Example.com/path"
// into a bare lowercase hostname ("example.com") and validates it. A leading
// "www." is dropped; the hosts layer blocks both forms.
func NormalizeDomain(s string) (string, error) {
	raw := strings.TrimSpace(s)
	if !strings.Contains(raw, "://") {
		raw = "http://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("invalid site %q", s)
	}
	host := strings.TrimSuffix(strings.ToLower(u.Hostname()), ".")
	if err := validateHost(host); err != nil {
		return "", fmt.Errorf("invalid site %q: %w", s, err)
	}
	if rest, ok := strings.CutPrefix(host, "www."); ok && strings.Contains(rest, ".") {
		host = rest
	}
	return host, nil
}

func validateHost(host string) error {
	switch {
	case host == "":
		return errors.New("empty hostname")
	case net.ParseIP(host) != nil:
		return errors.New("IP addresses cannot be blocked via hosts file")
	case host == "localhost" || strings.HasSuffix(host, ".localhost"):
		return errors.New("refusing to block localhost")
	case len(host) > 253:
		return errors.New("hostname too long")
	case !strings.Contains(host, "."):
		return errors.New("hostname must contain a dot")
	}
	for label := range strings.SplitSeq(host, ".") {
		if len(label) == 0 || len(label) > 63 {
			return errors.New("invalid label length")
		}
		if label[0] == '-' || label[len(label)-1] == '-' {
			return errors.New("label cannot start or end with '-'")
		}
		for _, r := range label {
			if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-') {
				return fmt.Errorf("invalid character %q", r)
			}
		}
	}
	return nil
}
