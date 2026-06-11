package modal

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
)

// newTestForm builds the smallest valid huh.Form we can hand to formContent.
// The content of the form does not matter for these tests — only its State
// field, which the adapter reads in Lifecycle().
func newTestForm() *huh.Form {
	return huh.NewForm(
		huh.NewGroup(
			huh.NewConfirm().Key("ok"),
		),
	)
}

// identityExtract is the trivial extractor used by tests that want the
// adapter to surface the original *huh.Form pointer.
func identityExtract(f *huh.Form) any { return f }

func TestFormContent_LifecycleMapsHuhState(t *testing.T) {
	cases := []struct {
		name             string
		state            huh.FormState
		wantState        LifecycleState
		wantFormIdentity bool
	}{
		{
			name:             "completed → confirmed with form payload",
			state:            huh.StateCompleted,
			wantState:        Confirmed,
			wantFormIdentity: true,
		},
		{
			name:      "aborted → cancelled with nil payload",
			state:     huh.StateAborted,
			wantState: Cancelled,
		},
		{
			name:      "normal → active",
			state:     huh.StateNormal,
			wantState: Active,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			form := newTestForm()
			form.State = tc.state
			fc := &formContent{form: form, extract: identityExtract}

			state, value := fc.Lifecycle()

			if state != tc.wantState {
				t.Errorf("state = %v, want %v", state, tc.wantState)
			}
			switch {
			case tc.wantFormIdentity:
				if value != form {
					t.Errorf("value identity differs from input form")
				}
			default:
				if value != nil {
					t.Errorf("value = %v, want nil", value)
				}
			}
		})
	}
}

func TestNewForm_ResolvedMsgCarriesFormOnCompletion(t *testing.T) {
	form := newTestForm()
	form.State = huh.StateCompleted

	m := NewForm("wizard", form, identityExtract)
	_, cmd := m.Update(nil)

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

func TestNewForm_PanicsOnNilExtract(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatalf("expected panic on nil extract, got none")
		}
	}()
	NewForm("id", newTestForm(), nil)
}

func TestFormContent_EscAbortsForm(t *testing.T) {
	form := newTestForm()
	fc := &formContent{form: form, extract: identityExtract}

	_, _ = fc.Update(tea.KeyPressMsg{Code: tea.KeyEsc})

	if form.State != huh.StateAborted {
		t.Errorf("form.State = %v, want StateAborted after esc", form.State)
	}
}

func TestNewForm_ExtractShapesResolvedPayload(t *testing.T) {
	type result struct{ tag string }
	form := newTestForm()
	form.State = huh.StateCompleted

	m := NewForm("id", form, func(*huh.Form) any { return result{tag: "shaped"} })
	_, cmd := m.Update(nil)

	got := drainResolved(t, cmd)
	r, ok := got.Value.(result)
	if !ok {
		t.Fatalf("Value type = %T, want result", got.Value)
	}
	if r.tag != "shaped" {
		t.Errorf("result.tag = %q, want %q", r.tag, "shaped")
	}
}
