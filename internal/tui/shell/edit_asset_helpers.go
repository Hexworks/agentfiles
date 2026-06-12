package shell

import (
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/hexworks/agentfiles/internal/asset"
	"github.com/hexworks/agentfiles/internal/tui/components/treetable"
	"github.com/hexworks/agentfiles/internal/tui/styles"
)

// assetHeader renders the bold section header used by the right column.
func assetHeader(title string) string {
	return styles.HeaderStyle.Render(title)
}

// focusAwareTreetableStyles returns the default treetable styles with
// cyan focused border + muted blurred border. Treetable picks between
// them on its own based on its focus state.
func focusAwareTreetableStyles() treetable.Styles {
	st := treetable.DefaultStyles()
	st.Border = lipgloss.NewStyle().Foreground(styles.ColorMuted)
	st.BorderFocused = lipgloss.NewStyle().Foreground(styles.ColorCyan)
	return st
}

// emptyTreeRoot is the placeholder used before the load command
// completes — a non-nil node satisfies treetable's `Label must not be
// empty` invariant.
func emptyTreeRoot() *treetable.Node {
	return &treetable.Node{
		Label: "(no asset loaded)",
		Data:  assetNode{kind: nodeRoot},
	}
}

// splitWidth divides the body width into a 50/50 left/right split, with
// the right column absorbing the rounding bit so totals add back to
// width when width is odd.
func splitWidth(width int) (int, int) {
	if width <= 0 {
		return 40, 40
	}
	left := width / 2
	right := width - left
	return left, right
}

// assetSummaryValue returns the styled summary value for one cell, or
// the empty string when no asset is loaded.
func assetSummaryValue(a *asset.Asset, pick func(*asset.Asset) string) string {
	if a == nil {
		return ""
	}
	return styles.Safe(pick(a))
}

// sortedRelativeFiles wraps asset.RelativeFiles and returns a stable
// sort order so the treetable rebuild is deterministic. A missing
// directory yields a nil slice.
func sortedRelativeFiles(dir string) []string {
	if dir == "" {
		return nil
	}
	files, err := asset.RelativeFiles(dir)
	if err != nil {
		return nil
	}
	sort.Strings(files)
	return files
}

// buildAssetTree turns the relative-file slice into a directory tree
// rooted at the asset's display name.
func buildAssetTree(a *asset.Asset, files []string) *treetable.Node {
	rootLabel := "(asset)"
	if a != nil {
		rootLabel = a.Name + "/"
	}
	root := &treetable.Node{
		Label: rootLabel,
		Data:  assetNode{kind: nodeRoot, rel: ""},
	}
	dirs := map[string]*treetable.Node{"": root}
	for _, rel := range files {
		parts := strings.Split(rel, string(filepath.Separator))
		parent := root
		acc := ""
		for i, part := range parts {
			if i == len(parts)-1 {
				parent.Children = append(parent.Children, &treetable.Node{
					Label: part,
					Data:  assetNode{kind: nodeFile, rel: rel},
				})
				continue
			}
			if acc == "" {
				acc = part
			} else {
				acc = acc + string(filepath.Separator) + part
			}
			node, ok := dirs[acc]
			if !ok {
				node = &treetable.Node{
					Label: part + "/",
					Data:  assetNode{kind: nodeDir, rel: acc},
				}
				dirs[acc] = node
				parent.Children = append(parent.Children, node)
			}
			parent = node
		}
	}
	return root
}

// formsEqual is the field-by-field equality the screen's dirty() relies
// on. slices.Equal treats nil and an empty slice as equal so a freshly
// loaded form is never reported as dirty.
func formsEqual(a, b editAssetForm) bool {
	return a.descriptionText == b.descriptionText &&
		a.tagsCSV == b.tagsCSV &&
		a.exclusiveGroup == b.exclusiveGroup &&
		slices.Equal(a.compatibleAgents, b.compatibleAgents)
}
