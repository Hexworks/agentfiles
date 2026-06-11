package shell

import "testing"

// modalSize clamps to (modalMinWidth, modalMinHeight) before tea has
// delivered the first WindowSizeMsg. The floor is what keeps
// bubbles/table from receiving a non-positive dimension on the very
// first keystroke.
func TestModalSize_FloorsBelowMinimum(t *testing.T) {
	cases := []struct {
		w, h         int
		wantW, wantH int
		name         string
	}{
		{0, 0, modalMinWidth, modalMinHeight, "pre-window-size"},
		{30, 12, modalMinWidth, modalMinHeight, "below floor on both axes"},
		{120, 40, 120, 40 - chromeHeight, "above floor uses requested size minus chrome"},
		{50, 13, 50, modalMinHeight, "height clamps when chrome makes it tiny"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotW, gotH := modalSize(tc.w, tc.h)
			if gotW != tc.wantW || gotH != tc.wantH {
				t.Errorf("modalSize(%d, %d) = (%d, %d), want (%d, %d)",
					tc.w, tc.h, gotW, gotH, tc.wantW, tc.wantH)
			}
		})
	}
}
