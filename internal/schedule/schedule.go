// Package schedule decides when blocking is active: daily time windows in
// local time.
package schedule

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// Clock is a time of day, in minutes since midnight.
type Clock int

// ParseClock parses "HH:MM" (24-hour).
func ParseClock(s string) (Clock, error) {
	if len(s) != 5 || s[2] != ':' || !isDigits(s[:2]) || !isDigits(s[3:]) {
		return 0, fmt.Errorf("invalid time %q (want HH:MM)", s)
	}
	h := int(s[0]-'0')*10 + int(s[1]-'0')
	m := int(s[3]-'0')*10 + int(s[4]-'0')
	if h > 23 || m > 59 {
		return 0, fmt.Errorf("invalid time %q (want 00:00-23:59)", s)
	}
	return Clock(h*60 + m), nil
}

func isDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// ClockOf returns the time of day of t, in t's location.
func ClockOf(t time.Time) Clock {
	return Clock(t.Hour()*60 + t.Minute())
}

func (c Clock) String() string {
	return fmt.Sprintf("%02d:%02d", int(c)/60, int(c)%60)
}

func (c Clock) MarshalJSON() ([]byte, error) {
	return json.Marshal(c.String())
}

func (c *Clock) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	v, err := ParseClock(s)
	if err != nil {
		return err
	}
	*c = v
	return nil
}

// Window is a daily time range. Start is inclusive, End exclusive. If End is
// before Start the window wraps past midnight; if they are equal it covers the
// whole day.
type Window struct {
	Start Clock `json:"start"`
	End   Clock `json:"end"`
}

// ParseWindow parses "HH:MM-HH:MM".
func ParseWindow(s string) (Window, error) {
	start, end, ok := strings.Cut(strings.TrimSpace(s), "-")
	if !ok {
		return Window{}, fmt.Errorf("invalid window %q (want HH:MM-HH:MM)", s)
	}
	var w Window
	var err error
	if w.Start, err = ParseClock(start); err != nil {
		return Window{}, err
	}
	if w.End, err = ParseClock(end); err != nil {
		return Window{}, err
	}
	return w, nil
}

// Contains reports whether t's local time of day falls inside the window.
func (w Window) Contains(t time.Time) bool {
	c := ClockOf(t)
	switch {
	case w.Start == w.End:
		return true
	case w.Start < w.End:
		return c >= w.Start && c < w.End
	default: // wraps midnight
		return c >= w.Start || c < w.End
	}
}

func (w Window) String() string {
	return w.Start.String() + "-" + w.End.String()
}

// AnyContains reports whether any window contains t.
func AnyContains(ws []Window, t time.Time) bool {
	for _, w := range ws {
		if w.Contains(t) {
			return true
		}
	}
	return false
}

// Format renders windows as "09:00-12:00, 13:00-17:00".
func Format(ws []Window) string {
	parts := make([]string, len(ws))
	for i, w := range ws {
		parts[i] = w.String()
	}
	return strings.Join(parts, ", ")
}
