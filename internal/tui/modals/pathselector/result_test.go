package pathselector

import (
	"testing"

	"github.com/hexworks/agentfiles/internal/tui/components/modal"
)

func TestResultFromMsg(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		msg  modal.ResolvedMsg
		want Result
		ok   bool
	}{
		{
			name: "confirmed with Result",
			msg:  modal.ResolvedMsg{Confirmed: true, Value: Result{Path: "/tmp/x", IsDir: true}},
			want: Result{Path: "/tmp/x", IsDir: true},
			ok:   true,
		},
		{
			name: "cancelled",
			msg:  modal.ResolvedMsg{Confirmed: false, Value: nil},
			want: Result{},
			ok:   false,
		},
		{
			name: "confirmed but wrong value type",
			msg:  modal.ResolvedMsg{Confirmed: true, Value: "not-a-result"},
			want: Result{},
			ok:   false,
		},
		{
			name: "confirmed with nil value",
			msg:  modal.ResolvedMsg{Confirmed: true, Value: nil},
			want: Result{},
			ok:   false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, ok := ResultFromMsg(tc.msg)
			if ok != tc.ok {
				t.Fatalf("ok = %v, want %v", ok, tc.ok)
			}
			if got != tc.want {
				t.Fatalf("Result = %#v, want %#v", got, tc.want)
			}
		})
	}
}
