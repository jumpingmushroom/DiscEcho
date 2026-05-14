package pipelines_test

import (
	"testing"

	"github.com/jumpingmushroom/DiscEcho/daemon/pipelines"
	"github.com/jumpingmushroom/DiscEcho/daemon/state"
)

func TestIntOption(t *testing.T) {
	cases := []struct {
		name string
		opts map[string]any
		want int
	}{
		{"missing key → default", map[string]any{}, 99},
		{"nil options → default", nil, 99},
		{"int value", map[string]any{"k": 18}, 18},
		{"float64 value (JSON-decoded)", map[string]any{"k": float64(20)}, 20},
		{"int64 value", map[string]any{"k": int64(21)}, 21},
		{"wrong type → default", map[string]any{"k": "nope"}, 99},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := pipelines.IntOption(&state.Profile{Options: c.opts}, "k", 99)
			if got != c.want {
				t.Errorf("IntOption = %d, want %d", got, c.want)
			}
		})
	}
	if got := pipelines.IntOption(nil, "k", 7); got != 7 {
		t.Errorf("nil profile: got %d, want 7", got)
	}
}

func TestStringOption(t *testing.T) {
	cases := []struct {
		name string
		opts map[string]any
		want string
	}{
		{"missing key → default", map[string]any{}, "def"},
		{"nil options → default", nil, "def"},
		{"string value", map[string]any{"k": "slow"}, "slow"},
		{"empty string → default", map[string]any{"k": ""}, "def"},
		{"wrong type → default", map[string]any{"k": 5}, "def"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := pipelines.StringOption(&state.Profile{Options: c.opts}, "k", "def")
			if got != c.want {
				t.Errorf("StringOption = %q, want %q", got, c.want)
			}
		})
	}
	if got := pipelines.StringOption(nil, "k", "def"); got != "def" {
		t.Errorf("nil profile: got %q, want def", got)
	}
}
