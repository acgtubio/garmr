package main

import (
	"bytes"
	"testing"
)

func TestRun(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "default", args: nil, want: "garmr: nothing to do yet\n"},
		{name: "version", args: []string{"-version"}, want: "garmr dev\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			if err := run(tt.args, &out); err != nil {
				t.Fatalf("run(%v) error: %v", tt.args, err)
			}
			if got := out.String(); got != tt.want {
				t.Errorf("run(%v) = %q, want %q", tt.args, got, tt.want)
			}
		})
	}
}
