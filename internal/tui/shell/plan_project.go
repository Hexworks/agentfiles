package shell

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/hexworks/agentfiles/internal/actions"
	"github.com/hexworks/agentfiles/internal/app"
	"github.com/hexworks/agentfiles/internal/errs"
	"github.com/hexworks/agentfiles/internal/profile"
	"github.com/hexworks/agentfiles/internal/project"
	llmsync "github.com/hexworks/agentfiles/internal/sync"
	"github.com/hexworks/agentfiles/internal/tui/components/mnemonic"
	"github.com/hexworks/agentfiles/internal/tui/components/treetable"
)

// planProjectActions is the narrow slice of *actions.Actions the Plan
// Project screen invokes. Naming the interface here keeps the
// dependency direction tui→app explicit and lets tests substitute a
// fake.
type planProjectActions interface {
	LoadProfile(in actions.LoadProfileInput) (*profile.Profile, errs.DomainError)
	LoadProject(in actions.LoadProjectInput) (*project.Manifest, errs.DomainError)
	PlanProject(in actions.PlanProjectInput) (*llmsync.Preview, errs.DomainError)
	SyncProject(in actions.SyncProjectInput) (*llmsync.Preview, errs.DomainError)
}

// planNodeKind classifies a treetable row payload. The root row is its
// own kind so callers do not have to special-case the empty path.
type planNodeKind int

const (
	planNodeRoot planNodeKind = iota
	planNodeDir
	planNodeFile
)

// planNode is the payload attached to every treetable node. path is the
// forward-slash relative key from llmsync.FileChange.Path; change is the
// originating FileChange on file rows and the zero value on dir/root.
type planNode struct {
	kind   planNodeKind
	path   string
	change llmsync.FileChange
}

// planActionState is the user's chosen per-row resolution for a drift
// or unknown row. planKeep is the default for both — drift+Keep adopts
// the on-disk hash; unknown+Keep leaves the stray file alone. The
// non-default states (planOverwrite, planDelete) trigger an actual write
// or delete on Apply.
type planActionState int

const (
	planKeep planActionState = iota
	planOverwrite
	planDelete
)

// planProjectLoadedMsg is the envelope the Init command emits after
// resolving the project, profile, and preview triplet. Either every
// field is set or err carries the first failure.
type planProjectLoadedMsg struct {
	prof    *profile.Profile
	proj    *project.Manifest
	preview *llmsync.Preview
	err     errs.DomainError
}

// planProjectScreen is the read-and-apply screen reached from the Edit
// Profile project row's [Plan] button and the Select Project Assets
// [Plan] button. It hosts a single treetable with two value columns
// (Status, Current Action), per-row mnemonic toggle buttons for drift
// and unknown rows, and screen-level [Apply] / [Back] buttons.
type planProjectScreen struct {
	actions   planProjectActions
	profileID string
	projectID string

	prof        *profile.Profile
	projectName string
	profileName string
	preview     *llmsync.Preview
	// resolutions stores only off-default selections. Default (planKeep)
	// is encoded as map absence so an empty map produces empty resolution
	// slices in onApply.
	resolutions map[string]planActionState

	tree     *treetable.Model
	applyBtn *mnemonic.Button
	backBtn  *mnemonic.Button
	set      *mnemonic.Set

	loaded bool
}

func newPlanProjectScreen(a planProjectActions, profileID, projectID string) *planProjectScreen {
	if a == nil {
		panic("shell.newPlanProjectScreen: nil actions")
	}
	if profileID == "" {
		panic("shell.newPlanProjectScreen: empty profileID")
	}
	if projectID == "" {
		panic("shell.newPlanProjectScreen: empty projectID")
	}
	s := &planProjectScreen{
		actions:     a,
		profileID:   profileID,
		projectID:   projectID,
		resolutions: map[string]planActionState{},
	}
	s.buildButtons()
	s.buildTree()
	s.rebuildSet()
	return s
}

func (s *planProjectScreen) buildButtons() {
	s.applyBtn = mnemonic.New("Apply", 'a', func() tea.Cmd { return s.onApply() })
	s.backBtn = mnemonic.New(
		"Back",
		'b',
		func() tea.Cmd { return popCmd() },
		mnemonic.WithExtraBindingKeys("esc"),
	)
}

func (s *planProjectScreen) buildTree() {
	s.tree = treetable.New(
		treetable.WithRoot(emptyPlanRoot()),
		treetable.WithNameColumn(treetable.Column{Title: "Name", Width: 40}),
		treetable.WithValueColumns(
			treetable.ValueColumn{Title: "Status", Width: 10, Value: s.statusValue},
			treetable.ValueColumn{Title: "Current Action", Width: 14, Value: s.actionValue},
		),
		treetable.WithActions(treetable.Column{Title: "Actions", Width: 14}, s.treeActionsFn()),
		treetable.WithHeight(treetableHeight),
		treetable.WithStyles(focusAwareTreetableStyles()),
		treetable.WithTitle("Changes"),
	)
	s.tree.Focus()
}

// emptyPlanRoot is the placeholder root used before the load command
// completes. A non-nil node satisfies treetable's "Label must not be
// empty" invariant.
func emptyPlanRoot() *treetable.Node {
	return &treetable.Node{
		Label: "(no plan loaded)",
		Data:  planNode{kind: planNodeRoot},
	}
}

func (s *planProjectScreen) ProfileID() string  { return s.profileID }
func (s *planProjectScreen) ProjectID() string  { return s.projectID }
func (s *planProjectScreen) Title() string      { return "Plan Project" }
func (s *planProjectScreen) InputFocused() bool { return false }

func (s *planProjectScreen) Init() tea.Cmd { return s.loadCmd() }

// loadCmd resolves project, profile, and plan preview in sequence so
// handleLoaded never has to guard against a partial result. Any failure
// short-circuits with the typed domain error; success produces the
// triplet in one envelope.
func (s *planProjectScreen) loadCmd() tea.Cmd {
	return func() tea.Msg {
		proj, projErr := s.actions.LoadProject(actions.LoadProjectInput{
			ProfileRef: s.profileID,
			ProjectID:  s.projectID,
		})
		if projErr != nil {
			return planProjectLoadedMsg{err: projErr}
		}
		prof, profErr := s.actions.LoadProfile(actions.LoadProfileInput{ProfileRef: s.profileID})
		if profErr != nil {
			return planProjectLoadedMsg{err: profErr}
		}
		preview, planErr := s.actions.PlanProject(actions.PlanProjectInput{
			ProfileRef: s.profileID,
			ProjectID:  s.projectID,
		})
		if planErr != nil {
			return planProjectLoadedMsg{err: planErr}
		}
		return planProjectLoadedMsg{prof: prof, proj: proj, preview: preview}
	}
}

func (s *planProjectScreen) Update(msg tea.Msg) (Screen, tea.Cmd) {
	switch m := msg.(type) {
	case planProjectLoadedMsg:
		return s.handleLoaded(m)
	case mutationDoneMsg:
		return s.handleMutationDone(m)
	case tea.KeyPressMsg:
		return s.handleKey(m)
	}
	return s, nil
}

func (s *planProjectScreen) handleLoaded(m planProjectLoadedMsg) (Screen, tea.Cmd) {
	if m.err != nil {
		return s, notificationCmd(m.err.Severity(), m.err.Error())
	}
	s.loaded = true
	s.prof = m.prof
	s.profileName = m.prof.Manifest.Name
	s.projectName = m.proj.Name
	s.preview = m.preview
	s.tree.SetRoot(buildPlanTree(s.projectName, m.preview.Changes))
	s.rebuildSet()
	return s, nil
}

// handleMutationDone pops the screen on success so the user lands back
// on the previous screen (Select Project Assets or Edit Profile) with
// the success toast still visible. Failure leaves the user on the plan
// so they can adjust resolutions and retry.
func (s *planProjectScreen) handleMutationDone(m mutationDoneMsg) (Screen, tea.Cmd) {
	note := notificationCmd(m.severity, m.text)
	if m.severity == errs.SeverityInfo {
		return s, tea.Batch(note, popCmd())
	}
	return s, note
}

func (s *planProjectScreen) handleKey(m tea.KeyPressMsg) (Screen, tea.Cmd) {
	if btn := s.set.Match(m); btn != nil {
		return s, btn.Trigger()
	}
	before := s.tree.Cursor()
	var cmd tea.Cmd
	s.tree, cmd = s.tree.Update(m)
	if s.tree.Cursor() != before {
		s.rebuildSet()
	}
	return s, cmd
}

// StatusKeys exposes the cursor row's toggle mnemonic (o, k, or d) plus
// the global [Back] mnemonic. [Apply] stays off the bar — the body's
// labelled button already shows it, and duplicating screen-level
// buttons in the bar would violate the shell contract documented on
// Screen.StatusKeys. [Back] is the universal escape hint kept per the
// Select Project Assets / Edit Profile convention.
func (s *planProjectScreen) StatusKeys() []key.Binding {
	out := make([]key.Binding, 0, 2)
	for _, b := range s.tree.Buttons() {
		out = append(out, b.Binding())
	}
	out = append(out, s.backBtn.Binding())
	return out
}

func (s *planProjectScreen) Body(width int) string {
	if !s.loaded {
		return " Loading…"
	}
	header := fmt.Sprintf(" Planning project %q (%s)", s.projectName, s.profileName)
	buttonRow := lipgloss.PlaceHorizontal(
		width, lipgloss.Right,
		s.applyBtn.View()+"  "+s.backBtn.View(),
	)
	return lipgloss.JoinVertical(
		lipgloss.Left,
		header,
		"",
		s.tree.View(),
		"",
		buttonRow,
	)
}

// rebuildSet refreshes the mnemonic set so its registration-time
// uniqueness check covers the current cursor + state. The cursor row's
// toggle button (if any) is always present; screen-level [Apply] and
// [Back] are always present.
func (s *planProjectScreen) rebuildSet() {
	set := mnemonic.NewSet()
	for _, b := range s.tree.Buttons() {
		set.Add(b)
	}
	set.Add(s.applyBtn)
	set.Add(s.backBtn)
	s.set = set
}

func (s *planProjectScreen) statusValue(n *treetable.Node) string {
	d, ok := n.Data.(planNode)
	if !ok || d.kind != planNodeFile {
		return ""
	}
	switch d.change.Kind {
	case llmsync.ChangeCreate:
		return "+ add"
	case llmsync.ChangeUpdate:
		return "~ update"
	case llmsync.ChangeDelete:
		return "- delete"
	case llmsync.ChangeDrift:
		return "* drift"
	case llmsync.ChangeUnknown:
		return "? unknown"
	}
	return ""
}

func (s *planProjectScreen) actionValue(n *treetable.Node) string {
	d, ok := n.Data.(planNode)
	if !ok || d.kind != planNodeFile {
		return ""
	}
	switch d.change.Kind {
	case llmsync.ChangeCreate, llmsync.ChangeUpdate, llmsync.ChangeDelete:
		return "-"
	}
	switch s.actionStateOf(d.path) {
	case planOverwrite:
		return "Overwrite"
	case planDelete:
		return "Delete"
	default:
		return "Keep"
	}
}

// actionStateOf returns the user's chosen resolution for path. Absence
// from the map means default — planKeep for both drift and unknown.
func (s *planProjectScreen) actionStateOf(path string) planActionState {
	if v, ok := s.resolutions[path]; ok {
		return v
	}
	return planKeep
}

// treeActionsFn returns the per-row toggle button factory the treetable
// invokes for the cursor row only. The button always shows the *other*
// option — pressing it swaps the resolution and re-renders.
func (s *planProjectScreen) treeActionsFn() treetable.ActionsFunc {
	return func(n *treetable.Node) []*mnemonic.Button {
		d, ok := n.Data.(planNode)
		if !ok || d.kind != planNodeFile {
			return nil
		}
		switch d.change.Kind {
		case llmsync.ChangeDrift:
			return []*mnemonic.Button{s.driftToggleBtn(d.path)}
		case llmsync.ChangeUnknown:
			return []*mnemonic.Button{s.unknownToggleBtn(d.path)}
		}
		return nil
	}
}

func (s *planProjectScreen) driftToggleBtn(path string) *mnemonic.Button {
	if s.actionStateOf(path) == planOverwrite {
		return mnemonic.New("Keep", 'k', func() tea.Cmd { return s.toggle(path, planKeep) })
	}
	return mnemonic.New("Overwrite", 'o', func() tea.Cmd { return s.toggle(path, planOverwrite) })
}

func (s *planProjectScreen) unknownToggleBtn(path string) *mnemonic.Button {
	if s.actionStateOf(path) == planDelete {
		return mnemonic.New("Keep", 'k', func() tea.Cmd { return s.toggle(path, planKeep) })
	}
	return mnemonic.New("Delete", 'd', func() tea.Cmd { return s.toggle(path, planDelete) })
}

// toggle stores the new state (or clears it on a return to default),
// rebuilds the tree so the row's Current Action and Actions cells
// re-render with the new state, and refreshes the mnemonic set so the
// status bar key matches the newly-shown toggle button.
//
// treetable.SetRoot preserves the underlying table cursor, so toggling
// does not jump the user off the active row.
func (s *planProjectScreen) toggle(path string, next planActionState) tea.Cmd {
	if next == planKeep {
		delete(s.resolutions, path)
	} else {
		s.resolutions[path] = next
	}
	if s.preview != nil {
		s.tree.SetRoot(buildPlanTree(s.projectName, s.preview.Changes))
	}
	s.rebuildSet()
	return nil
}

// onApply iterates preview.Changes (not the map) so the output order is
// deterministic and stale resolutions for paths no longer present in
// the plan are silently ignored.
func (s *planProjectScreen) onApply() tea.Cmd {
	if s.preview == nil {
		return nil
	}
	var drift []app.DriftResolution
	var unknown []app.UnknownResolution
	for _, ch := range s.preview.Changes {
		st, has := s.resolutions[ch.Path]
		if !has {
			continue
		}
		switch ch.Kind {
		case llmsync.ChangeDrift:
			if st == planOverwrite {
				drift = append(drift, app.DriftResolution{
					Path:     ch.Path,
					Decision: app.DriftOverwrite,
				})
			}
		case llmsync.ChangeUnknown:
			if st == planDelete {
				unknown = append(unknown, app.UnknownResolution{
					Path:     ch.Path,
					Decision: app.UnknownDelete,
				})
			}
		}
	}
	profileRef := s.profileID
	projectID := s.projectID
	return mutationCmd(
		func() errs.DomainError {
			_, err := s.actions.SyncProject(actions.SyncProjectInput{
				ProfileRef: profileRef,
				ProjectID:  projectID,
				Drift:      drift,
				Unknown:    unknown,
			})
			return err
		},
		"Project synced",
	)
}

// buildPlanTree turns the preview's FileChange list into a directory
// tree rooted at the project name. Paths are split on "/" because
// llmsync.FileChange.Path is forward-slash relative per
// llmsync.validatePathKey.
func buildPlanTree(projectName string, changes []llmsync.FileChange) *treetable.Node {
	label := "(plan)"
	if projectName != "" {
		label = projectName + "/"
	}
	root := &treetable.Node{
		Label: label,
		Data:  planNode{kind: planNodeRoot},
	}
	dirs := map[string]*treetable.Node{"": root}
	for _, ch := range changes {
		parts := strings.Split(ch.Path, "/")
		parent := root
		acc := ""
		for i, part := range parts {
			if i == len(parts)-1 {
				parent.Children = append(parent.Children, &treetable.Node{
					Label: part,
					Data:  planNode{kind: planNodeFile, path: ch.Path, change: ch},
				})
				continue
			}
			if acc == "" {
				acc = part
			} else {
				acc = acc + "/" + part
			}
			node, exists := dirs[acc]
			if !exists {
				node = &treetable.Node{
					Label: part + "/",
					Data:  planNode{kind: planNodeDir, path: acc},
				}
				dirs[acc] = node
				parent.Children = append(parent.Children, node)
			}
			parent = node
		}
	}
	return root
}
