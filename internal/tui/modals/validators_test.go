package modals

import (
	"reflect"
	"testing"
)

func TestRequiredString(t *testing.T) {
	if err := requiredString("hello"); err != nil {
		t.Errorf("requiredString(non-empty) = %v, want nil", err)
	}
	if err := requiredString("   "); err == nil {
		t.Errorf("requiredString(whitespace-only) = nil, want error")
	}
	if err := requiredString(""); err == nil {
		t.Errorf("requiredString(empty) = nil, want error")
	}
}

func TestRequiredAgents(t *testing.T) {
	if err := requiredAgents([]string{AgentCodex}); err != nil {
		t.Errorf("requiredAgents(non-empty) = %v, want nil", err)
	}
	if err := requiredAgents(nil); err == nil {
		t.Errorf("requiredAgents(nil) = nil, want error")
	}
	if err := requiredAgents([]string{}); err == nil {
		t.Errorf("requiredAgents(empty) = nil, want error")
	}
}

func TestParseTags(t *testing.T) {
	cases := []struct {
		name, in string
		want     []string
	}{
		{"single", "git", []string{"git"}},
		{"multi", "git, build, ci", []string{"git", "build", "ci"}},
		{"trims spaces", "  a  ,b ,  c  ", []string{"a", "b", "c"}},
		{"drops empties", "a,,b,   ,c", []string{"a", "b", "c"}},
		{"empty input", "", nil},
		{"whitespace only", "   ", nil},
		{"commas only", ",,,", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := parseTags(tc.in); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("parseTags(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

func TestJoinTags(t *testing.T) {
	cases := []struct {
		name string
		in   []string
		want string
	}{
		{"empty", nil, ""},
		{"single", []string{"git"}, "git"},
		{"multi", []string{"a", "b", "c"}, "a, b, c"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := joinTags(tc.in); got != tc.want {
				t.Errorf("joinTags(%v) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
