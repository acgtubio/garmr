package schedule

import (
	"encoding/json"
	"testing"
	"time"
)

func at(hhmm string) time.Time {
	c, err := ParseClock(hhmm)
	if err != nil {
		panic(err)
	}
	return time.Date(2026, 1, 1, int(c)/60, int(c)%60, 30, 0, time.Local)
}

func TestParseClock(t *testing.T) {
	for _, s := range []string{"00:00", "09:05", "23:59"} {
		c, err := ParseClock(s)
		if err != nil || c.String() != s {
			t.Errorf("ParseClock(%q) = %v, %v", s, c, err)
		}
	}
	for _, s := range []string{"", "9:00", "24:00", "12:60", "12-00", "ab:cd", "12:00x", "+9:00"} {
		if _, err := ParseClock(s); err == nil {
			t.Errorf("ParseClock(%q) succeeded, want error", s)
		}
	}
}

func TestWindowContains(t *testing.T) {
	tests := []struct {
		window string
		time   string
		want   bool
	}{
		{"09:00-17:00", "08:59", false},
		{"09:00-17:00", "09:00", true},
		{"09:00-17:00", "16:59", true},
		{"09:00-17:00", "17:00", false},
		{"22:00-06:00", "21:59", false},
		{"22:00-06:00", "22:00", true},
		{"22:00-06:00", "00:00", true},
		{"22:00-06:00", "05:59", true},
		{"22:00-06:00", "06:00", false},
		{"00:00-00:00", "00:00", true},
		{"08:00-08:00", "23:59", true},
	}
	for _, tt := range tests {
		w, err := ParseWindow(tt.window)
		if err != nil {
			t.Fatal(err)
		}
		if got := w.Contains(at(tt.time)); got != tt.want {
			t.Errorf("%s contains %s = %v, want %v", tt.window, tt.time, got, tt.want)
		}
	}
}

func TestParseWindowErrors(t *testing.T) {
	for _, s := range []string{"", "09:00", "09:00-", "09:00-25:00", "09:00 17:00"} {
		if _, err := ParseWindow(s); err == nil {
			t.Errorf("ParseWindow(%q) succeeded, want error", s)
		}
	}
}

func TestWindowJSON(t *testing.T) {
	w, _ := ParseWindow("09:00-17:30")
	data, err := json.Marshal(w)
	if err != nil {
		t.Fatal(err)
	}
	if want := `{"start":"09:00","end":"17:30"}`; string(data) != want {
		t.Fatalf("Marshal = %s, want %s", data, want)
	}
	var got Window
	if err := json.Unmarshal(data, &got); err != nil || got != w {
		t.Fatalf("Unmarshal = %v, %v; want %v", got, err, w)
	}
	if err := json.Unmarshal([]byte(`{"start":"9am","end":"17:00"}`), &got); err == nil {
		t.Error("expected error for invalid time")
	}
}
