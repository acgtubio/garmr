// Package runner ties config, schedule and hosts together: it decides which
// sites are blocked right now and applies that to the hosts file, either once
// (Tick) or continuously (Run).
package runner

import (
	"context"
	"fmt"
	"io"
	"slices"
	"strings"
	"time"

	"github.com/acgtubio/garmr/internal/config"
	"github.com/acgtubio/garmr/internal/hosts"
)

// Tick loads the config, computes the sites active at now, and writes them to
// the hosts file. It returns the active sites.
func Tick(cfgPath, hostsPath string, now time.Time) ([]string, error) {
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}
	active := cfg.ActiveSites(now)
	if err := hosts.Apply(hostsPath, active); err != nil {
		return nil, fmt.Errorf("update hosts: %w", err)
	}
	return active, nil
}

// Run ticks immediately and then at the start of every minute until ctx is
// cancelled. The config is re-read on every tick. Errors are logged and the
// hosts file is left as it was; blocks are never lifted because of a failure
// or on exit.
func Run(ctx context.Context, cfgPath, hostsPath string, log io.Writer) error {
	var last []string
	first := true
	for {
		now := time.Now()
		active, err := Tick(cfgPath, hostsPath, now)
		switch {
		case err != nil:
			fmt.Fprintf(log, "%s error: %v\n", now.Format(time.DateTime), err)
		case first || !slices.Equal(active, last):
			fmt.Fprintf(log, "%s blocking %d site(s): %s\n", now.Format(time.DateTime), len(active), strings.Join(active, " "))
			last, first = active, false
		}

		next := now.Truncate(time.Minute).Add(time.Minute)
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(time.Until(next)):
		}
	}
}
