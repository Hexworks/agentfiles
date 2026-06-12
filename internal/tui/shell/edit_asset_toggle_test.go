package shell

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/hexworks/agentfiles/internal/asset"
)

// TestEditAssetScreen_CompatibleToggleViaScreenUpdate guards the routing
// chain that makes the Compatible Agents MultiSelect editable: pressing
// space while the field holds focus must reach huh.MultiSelect.Update so
// the option toggles. The earlier bug was a zero-value keymap because the
// field was constructed outside huh.Form — without an explicit WithKeyMap
// the Toggle binding silently failed.
func TestEditAssetScreen_CompatibleToggleViaScreenUpdate(t *testing.T) {
	f := newEditAssetFixture(t, "skill", asset.TypeSkill)
	s := newEditAssetScreen(f.Actions, f.Profile.ID, f.AssetID)
	f.loadInto(t, s)

	if cmd := s.handler.FocusIndex(s.compatibleIdx); cmd != nil {
		_ = drainCmd(t, cmd)
	}

	if got := s.handler.Focused(); got != s.compatibleIdx {
		t.Fatalf("focused index = %d, want %d", got, s.compatibleIdx)
	}

	// Drive space through editAssetScreen.Update.
	_, _ = s.Update(tea.KeyPressMsg{Code: ' ', Text: " "})

	t.Logf("compatible after space = %v", s.form.compatibleAgents)
	if len(s.form.compatibleAgents) == 0 {
		t.Fatalf("space did not toggle any option")
	}
}
