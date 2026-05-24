package modal

import (
	"testing"

	"charm.land/huh/v2"
)

// newTestForm builds the smallest valid huh.Form we can hand to formContent.
// The content of the form does not matter for these tests — only its State
// field, which the adapter reads in Done().
func newTestForm() *huh.Form {
	return huh.NewForm(
		huh.NewGroup(
			huh.NewConfirm().Key("ok"),
		),
	)
}

func TestFormContent_DoneMapsHuhStateToResolution(t *testing.T) {
	cases := []struct {
		name      string
		state     huh.FormState
		wantDone  bool
		wantConf  bool
		wantValue any
	}{
		{
			name:      "completed → confirmed with form payload",
			state:     huh.StateCompleted,
			wantDone:  true,
			wantConf:  true,
			wantValue: "form", // sentinel: we assert identity below
		},
		{
			name:      "aborted → cancelled with nil payload",
			state:     huh.StateAborted,
			wantDone:  true,
			wantConf:  false,
			wantValue: nil,
		},
		{
			name:      "normal → not done",
			state:     huh.StateNormal,
			wantDone:  false,
			wantConf:  false,
			wantValue: nil,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			form := newTestForm()
			form.State = tc.state
			fc := &formContent{form: form}

			done, confirmed, value := fc.Done()

			if done != tc.wantDone {
				t.Errorf("done = %v, want %v", done, tc.wantDone)
			}
			if confirmed != tc.wantConf {
				t.Errorf("confirmed = %v, want %v", confirmed, tc.wantConf)
			}
			switch want := tc.wantValue.(type) {
			case nil:
				if value != nil {
					t.Errorf("value = %v, want nil", value)
				}
			case string:
				// Completed case: value must be the *huh.Form itself, not a copy.
				if want == "form" {
					if value != form {
						t.Errorf("value identity differs from input form")
					}
				}
			}
		})
	}
}

func TestNewForm_ResolvedMsgCarriesFormOnCompletion(t *testing.T) {
	form := newTestForm()
	form.State = huh.StateCompleted

	m := NewForm("wizard", form)
	_, cmd := m.Update(nil)

	// Pull the ResolvedMsg out via the shared helper used by modal_test.go.
	got := drainResolved(t, cmd)
	if got.ID != "wizard" {
		t.Errorf("ID = %q, want %q", got.ID, "wizard")
	}
	if !got.Confirmed {
		t.Errorf("Confirmed = false, want true on StateCompleted")
	}
	if got.Value != form {
		t.Errorf("Value is not the original *huh.Form")
	}
}
