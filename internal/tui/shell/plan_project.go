package shell

import (
	"fmt"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/hexworks/agentfiles/internal/actions"
	"github.com/hexworks/agentfiles/internal/app"
	"github.com/hexworks/agentfiles/internal/asset"
	"github.com/hexworks/agentfiles/internal/errs"
	"github.com/hexworks/agentfiles/internal/profile"
	"github.com/hexworks/agentfiles/internal/project"
	"github.com/hexworks/agentfiles/internal/tui/components/help"
	"github.com/hexworks/agentfiles/internal/tui/components/mnemonic"
	"github.com/hexworks/agentfiles/internal/tui/components/modal"
	"github.com/hexworks/agentfiles/internal/tui/components/treetable"
	"github.com/hexworks/agentfiles/internal/tui/editor"
	"github.com/hexworks/agentfiles/internal/tui/modals"
	"github.com/hexworks/agentfiles/internal/tui/styles"
	"github.com/hexworks/agentfiles/internal/utils"
)

// planProjectActions is the narrow slice of *actions.Actions the Plan
// Project screen invokes. Naming the interface here keeps the
// dependency direction tui→app explicit and lets tests substitute a
// fake.
type planProjectActions interface {
	LoadProfile(in actions.LoadProfileInput) (*profile.Profile, errs.DomainError)
	LoadProject(in actions.LoadProjectInput) (*project.Manifest, errs.DomainError)
	PlanProject(in actions.PlanProjectInput) (*app.Preview, errs.DomainError)
	SyncProject(in actions.SyncProjectInput) (*app.Preview, errs.DomainError)
	CreateAssetFromFolder(in actions.CreateAssetFromFolderInput) (string, errs.DomainError)
}

// planModalKind identifies which modal flow the Plan Project screen is
// hosting so the shared ResolvedMsg handler can route to the right
// post-action. none means no modal is open.
type planModalKind int

const (
	planModalNone planModalKind = iota
	planModalRegisterAsset
)

// planNodeKind classifies a treetable row payload. The root row is its
// own kind so callers do not have to special-case the empty path.
type planNodeKind int

const (
	planNodeRoot planNodeKind = iota
	planNodeDir
	planNodeFile
)

// planNode is the payload attached to every treetable node. path is the
// forward-slash relative key from app.FileChange.Path on file rows and
// the accumulated directory key on dir rows (preserved so a future
// per-directory bulk action can target the subtree without rebuilding
// the path from labels); change is the originating FileChange on file
// rows and the zero value on dir/root.
type planNode struct {
	kind   planNodeKind
	path   string
	change app.FileChange
}

// planFileNode unpacks n's payload and reports ok only for file rows so
// the three cell-rendering callsites share one boundary check.
func planFileNode(n *treetable.Node) (planNode, bool) {
	d, ok := n.Data.(planNode)
	if !ok || d.kind != planNodeFile {
		return planNode{}, false
	}
	return d, true
}

// planDirNode unpacks n's payload and reports ok only for directory rows,
// the rows that can host the [Register] action.
func planDirNode(n *treetable.Node) (planNode, bool) {
	d, ok := n.Data.(planNode)
	if !ok || d.kind != planNodeDir {
		return planNode{}, false
	}
	return d, true
}

// planProjectLoadedMsg is the envelope the Init command emits after
// resolving the project, profile, and preview triplet. Either every
// field is set or err carries the first failure.
type planProjectLoadedMsg struct {
	prof    *profile.Profile
	proj    *project.Manifest
	preview *app.Preview
	err     errs.DomainError
}

// planProjectScreen is the read-and-apply screen reached from the Edit
// Profile project row's [Plan] button and the Select Project Assets
// [Plan] button. It hosts a single treetable with two value columns
// (Status, Resolution), per-row [Open] on every file leaf plus a
// resolution toggle for drift / unknown rows, and screen-level [Apply]
// / [Back] buttons.
type planProjectScreen struct {
	actions   planProjectActions
	profileID string
	projectID string

	projectName string
	projectPath string
	profileName string
	preview     *app.Preview
	// driftResolutions stores only off-default drift selections
	// (app.DriftOverwrite). Default DriftKeep is encoded as map
	// absence so an empty map means the user wants Keep everywhere.
	driftResolutions map[string]app.DriftDecision
	// unknownResolutions stores only off-default unknown selections
	// (app.UnknownDelete). Same absence-as-default convention as
	// driftResolutions, and the two distinct maps mirror the domain's
	// two-enum decision space (see docs/architecture/12-glossary.md).
	unknownResolutions map[string]app.UnknownDecision

	tree     *treetable.Model
	applyBtn *mnemonic.Button
	backBtn  *mnemonic.Button
	set      *mnemonic.Set

	// registerableDirs is the set of directory keys whose subtree is all
	// unknown/unmanaged, derived from the loaded preview via
	// app.RegisterableDirs. The eligibility rule lives in app; the screen
	// renders the [Register] and [Ignore] buttons on rows the set contains.
	registerableDirs map[string]bool

	// ignoredDirs holds the directory keys the user toggled [Ignore] on this
	// session. It collapses those folders in the tree and, on Apply, is sent
	// to the service to persist into the project's ignored_paths. Reset on
	// every (re)load because a fresh preview re-derives the plan.
	ignoredDirs map[string]bool

	// modal hosts the Create Asset dialog opened by the row-level
	// [Register] action; nil when closed. registerDirKey is the
	// project-relative key of the folder being registered, passed to the
	// service which resolves and re-validates it. width/height track the
	// last WindowSizeMsg so the modal can be sized when opened.
	modal          *modal.Modal
	modalKind      planModalKind
	registerDirKey string
	width          int
	height         int
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
		actions:            a,
		profileID:          profileID,
		projectID:          projectID,
		driftResolutions:   map[string]app.DriftDecision{},
		unknownResolutions: map[string]app.UnknownDecision{},
		ignoredDirs:        map[string]bool{},
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
			treetable.ValueColumn{
				Title: "Status",
				Width: 10,
				Value: s.statusValue,
				Style: s.statusStyle,
			},
			treetable.ValueColumn{Title: "Resolution", Width: 14, Value: s.actionValue},
		),
		treetable.WithActions(treetable.Column{Title: "Actions", Width: 22}, s.treeActionsFn()),
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

func (s *planProjectScreen) ProfileID() string { return s.profileID }
func (s *planProjectScreen) ProjectID() string { return s.projectID }
func (s *planProjectScreen) Title() string     { return "Plan Project" }
func (s *planProjectScreen) InputFocused() bool {
	return s.modal != nil && s.modal.Active()
}

func (s *planProjectScreen) Description() string {
	if s.projectName == "" {
		return "Loading plan…"
	}
	return fmt.Sprintf("Planning project %q (%s)", s.projectName, s.profileName)
}

func (s *planProjectScreen) Topic() help.Topic {
	return help.Topic{Label: "Plan Project", File: "plan_project.md"}
}

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
	// While a modal is open it owns every message except the ones the
	// screen still needs to process itself (resize for sizing, the modal's
	// own resolution, and async results in flight).
	if s.modal != nil {
		switch msg.(type) {
		case modal.ResolvedMsg, tea.WindowSizeMsg, planProjectLoadedMsg, mutationDoneMsg, registerAssetDoneMsg:
			// fall through to type-specific handling
		default:
			return s.forwardToModal(msg)
		}
	}

	switch m := msg.(type) {
	case planProjectLoadedMsg:
		return s.handleLoaded(m)
	case mutationDoneMsg:
		return s.handleMutationDone(m)
	case registerAssetDoneMsg:
		return s.handleRegisterAssetDone(m)
	case modal.ResolvedMsg:
		return s, s.handleResolved(m)
	case tea.WindowSizeMsg:
		return s.handleResize(m)
	case editor.FinishedMsg:
		return s.handleEditorFinished(m)
	case tea.KeyPressMsg:
		return s.handleKey(m)
	}
	return s, nil
}

func (s *planProjectScreen) forwardToModal(msg tea.Msg) (Screen, tea.Cmd) {
	var cmd tea.Cmd
	s.modal, cmd = s.modal.Update(msg)
	return s, cmd
}

func (s *planProjectScreen) handleResize(m tea.WindowSizeMsg) (Screen, tea.Cmd) {
	s.width = m.Width
	s.height = m.Height
	if s.modal != nil {
		mw, mh := modalSize(m.Width, m.Height)
		s.modal.SetSize(mw, mh)
		var cmd tea.Cmd
		s.modal, cmd = s.modal.Update(m)
		return s, cmd
	}
	return s, nil
}

// handleEditorFinished re-runs the load command after the user exits the
// external editor so any on-disk change the edit produced is reflected
// in a freshly computed preview. A failed editor invocation surfaces a
// toast and leaves the existing preview intact.
func (s *planProjectScreen) handleEditorFinished(m editor.FinishedMsg) (Screen, tea.Cmd) {
	if m.Err != nil {
		return s, notificationCmd(errs.SeverityError, fmt.Sprintf("Editor failed: %v", m.Err))
	}
	return s, s.loadCmd()
}

func (s *planProjectScreen) handleLoaded(m planProjectLoadedMsg) (Screen, tea.Cmd) {
	if m.err != nil {
		return s, notificationCmd(m.err.Severity(), m.err.Error())
	}
	s.profileName = m.prof.Manifest.Name
	s.projectName = m.proj.Name
	s.projectPath = m.proj.Path
	s.preview = m.preview
	s.registerableDirs = app.RegisterableDirs(m.preview.Changes)
	s.ignoredDirs = map[string]bool{}
	s.tree.SetRoot(buildPlanTree(s.projectName, m.preview.Changes, s.ignoredDirs))
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
	if s.modal != nil {
		return s.forwardToModal(m)
	}
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
	if s.preview == nil {
		return styles.TextStyle.Render(" Loading…")
	}
	buttonRow := " " + s.applyBtn.View() + "  " + s.backBtn.View()
	background := lipgloss.JoinVertical(
		lipgloss.Left,
		s.tree.View(),
		"",
		buttonRow,
	)
	if s.modal == nil {
		return background
	}
	return s.modal.Render(background, width, lipgloss.Height(background))
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
	d, ok := planFileNode(n)
	if !ok {
		return ""
	}
	switch d.change.Kind {
	case app.ChangeCreate:
		return "+ add"
	case app.ChangeUpdate:
		return "~ update"
	case app.ChangeDelete:
		return "- delete"
	case app.ChangeDrift:
		return "* drift"
	case app.ChangeUnknown:
		return "? unknown"
	}
	return ""
}

func (s *planProjectScreen) statusStyle(n *treetable.Node) lipgloss.Style {
	d, ok := planFileNode(n)
	if !ok {
		return lipgloss.NewStyle()
	}
	switch d.change.Kind {
	case app.ChangeCreate:
		return styles.CreateStyle
	case app.ChangeUpdate:
		return styles.UpdateStyle
	case app.ChangeDelete:
		return styles.DeleteStyle
	case app.ChangeDrift:
		return styles.DriftStyle
	case app.ChangeUnknown:
		return styles.MutedStyle
	}
	return lipgloss.NewStyle()
}

func (s *planProjectScreen) actionValue(n *treetable.Node) string {
	d, ok := planFileNode(n)
	if !ok {
		return ""
	}
	switch d.change.Kind {
	case app.ChangeCreate, app.ChangeUpdate, app.ChangeDelete:
		return "-"
	case app.ChangeDrift:
		if s.driftResolutions[d.path] == app.DriftOverwrite {
			return "Overwrite"
		}
		return "Keep"
	case app.ChangeUnknown:
		if s.unknownResolutions[d.path] == app.UnknownDelete {
			return "Delete"
		}
		return "Keep"
	}
	return ""
}

// treeActionsFn returns the per-row action button factory the treetable
// invokes for the cursor row only. Every file leaf exposes [Open] so the
// user can inspect or edit the file before applying; drift / unknown
// rows additionally expose the resolution toggle which always shows the
// *other* option — pressing it swaps the resolution and re-renders.
func (s *planProjectScreen) treeActionsFn() treetable.ActionsFunc {
	return func(n *treetable.Node) []*mnemonic.Button {
		if d, ok := planDirNode(n); ok {
			if s.ignoredDirs[d.path] {
				return []*mnemonic.Button{s.showFolderBtn(d.path)}
			}
			if s.registerableDirs[d.path] {
				return []*mnemonic.Button{s.registerAssetBtn(d.path), s.ignoreFolderBtn(d.path)}
			}
			return nil
		}
		d, ok := planFileNode(n)
		if !ok {
			return nil
		}
		btns := []*mnemonic.Button{s.openFileBtn(d.path)}
		switch d.change.Kind {
		case app.ChangeDrift:
			btns = append(btns, s.driftToggleBtn(d.path))
		case app.ChangeUnknown:
			btns = append(btns, s.unknownToggleBtn(d.path))
		}
		return btns
	}
}

func (s *planProjectScreen) registerAssetBtn(dirPath string) *mnemonic.Button {
	return mnemonic.New("Register", 'r', func() tea.Cmd { return s.onRegisterAsset(dirPath) })
}

func (s *planProjectScreen) ignoreFolderBtn(dirPath string) *mnemonic.Button {
	return mnemonic.New("Ignore", 'i', func() tea.Cmd { return s.toggleIgnore(dirPath, true) })
}

func (s *planProjectScreen) showFolderBtn(dirPath string) *mnemonic.Button {
	return mnemonic.New("Show", 'w', func() tea.Cmd { return s.toggleIgnore(dirPath, false) })
}

func (s *planProjectScreen) openFileBtn(path string) *mnemonic.Button {
	return mnemonic.New("Open", 'o', func() tea.Cmd { return s.onOpen(path) })
}

func (s *planProjectScreen) driftToggleBtn(path string) *mnemonic.Button {
	if s.driftResolutions[path] == app.DriftOverwrite {
		return mnemonic.New("Keep", 'p', func() tea.Cmd { return s.toggleDrift(path, app.DriftKeep) })
	}
	return mnemonic.New("Overwrite", 'w', func() tea.Cmd { return s.toggleDrift(path, app.DriftOverwrite) })
}

func (s *planProjectScreen) unknownToggleBtn(path string) *mnemonic.Button {
	if s.unknownResolutions[path] == app.UnknownDelete {
		return mnemonic.New("Keep", 'p', func() tea.Cmd { return s.toggleUnknown(path, app.UnknownKeep) })
	}
	return mnemonic.New("Delete", 'd', func() tea.Cmd { return s.toggleUnknown(path, app.UnknownDelete) })
}

// onOpen suspends the program in the system editor pointed at the
// repo-relative path under the loaded project root. The path may not
// exist yet (ChangeCreate rows have no on-disk file); the editor opens
// an empty buffer in that case and the post-edit reload picks up any
// resulting change.
func (s *planProjectScreen) onOpen(path string) tea.Cmd {
	if s.projectPath == "" || path == "" {
		return nil
	}
	return editor.Open(filepath.Join(s.projectPath, path))
}

func (s *planProjectScreen) toggleDrift(path string, next app.DriftDecision) tea.Cmd {
	if next == app.DriftKeep {
		delete(s.driftResolutions, path)
	} else {
		s.driftResolutions[path] = next
	}
	s.tree.RefreshActions()
	s.rebuildSet()
	return nil
}

func (s *planProjectScreen) toggleUnknown(path string, next app.UnknownDecision) tea.Cmd {
	if next == app.UnknownKeep {
		delete(s.unknownResolutions, path)
	} else {
		s.unknownResolutions[path] = next
	}
	s.tree.RefreshActions()
	s.rebuildSet()
	return nil
}

// toggleIgnore flips a folder's ignored state. Unlike the drift/unknown
// toggles this changes the tree shape (the folder's children collapse or
// reappear), so it rebuilds the tree via SetRoot rather than RefreshActions.
// The cursor stays on the folder row, which keeps its position.
func (s *planProjectScreen) toggleIgnore(path string, ignore bool) tea.Cmd {
	if ignore {
		s.ignoredDirs[path] = true
	} else {
		delete(s.ignoredDirs, path)
	}
	s.tree.SetRoot(buildPlanTree(s.projectName, s.preview.Changes, s.ignoredDirs))
	s.rebuildSet()
	return nil
}

// registerAssetDoneMsg envelopes the result of the Register-as-Asset
// action. Unlike a generic mutationDoneMsg (which pops the screen on
// success), success here keeps the user on the plan and triggers a reload
// so the newly managed files are re-classified.
type registerAssetDoneMsg struct {
	name string
	err  errs.DomainError
}

// onRegisterAsset opens the Create Asset modal pre-scoped to the selected
// folder. The project-relative key is stored on the screen so the post-confirm
// handler can hand it to the service (which resolves and re-validates it); the
// modal only collects the manifest, with Name pre-filled from the folder's
// basename and a caption summarizing what the copy will move. The local join
// here is read-only — only for the file-count/size summary, not the trusted
// write path.
func (s *planProjectScreen) onRegisterAsset(dirKey string) tea.Cmd {
	if s.projectPath == "" || dirKey == "" {
		return nil
	}
	s.registerDirKey = dirKey
	abs := filepath.Join(s.projectPath, filepath.FromSlash(dirKey))
	caption := "Creating Asset"
	if count, size, statErr := utils.DirStats(abs); statErr == nil {
		caption = fmt.Sprintf("Register %s · %d files · %s", dirKey, count, formatBytes(size))
	}
	s.openModal(modals.NewCreateAssetFromFolder(asset.Manifest{Name: path.Base(dirKey)}, caption), planModalRegisterAsset)
	return s.modal.Init()
}

// formatBytes renders a byte count in the largest unit under which it stays
// below 1024, for the folder-register modal caption.
func formatBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for v := n / unit; v >= unit; v /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGTPE"[exp])
}

func (s *planProjectScreen) openModal(m *modal.Modal, kind planModalKind) {
	s.modal = m
	s.modalKind = kind
	if s.width > 0 && s.height > 0 {
		mw, mh := modalSize(s.width, s.height)
		s.modal.SetSize(mw, mh)
	}
}

// handleResolved clears the modal up-front then routes the resolution to
// the matching post-action, mirroring the Edit Profile screen's pattern.
func (s *planProjectScreen) handleResolved(msg modal.ResolvedMsg) tea.Cmd {
	s.modal = nil
	kind := s.modalKind
	s.modalKind = planModalNone
	dirKey := s.registerDirKey
	s.registerDirKey = ""
	if kind == planModalRegisterAsset {
		return s.afterRegisterAsset(msg, dirKey)
	}
	return nil
}

func (s *planProjectScreen) afterRegisterAsset(msg modal.ResolvedMsg, dirKey string) tea.Cmd {
	if !msg.Confirmed || dirKey == "" {
		return nil
	}
	manifest, ok := msg.Value.(asset.Manifest)
	if !ok {
		return nil
	}
	profileRef := s.profileID
	projectID := s.projectID
	return func() tea.Msg {
		_, err := s.actions.CreateAssetFromFolder(actions.CreateAssetFromFolderInput{
			ProfileRef: profileRef,
			ProjectID:  projectID,
			Manifest:   manifest,
			DirKey:     dirKey,
		})
		return registerAssetDoneMsg{name: manifest.Name, err: err}
	}
}

// handleRegisterAssetDone reloads the plan on success so the copied files
// show up as managed create/update rows, and surfaces the typed error
// otherwise. The screen is never popped — the user stays to inspect the
// re-classified plan.
func (s *planProjectScreen) handleRegisterAssetDone(m registerAssetDoneMsg) (Screen, tea.Cmd) {
	if m.err != nil {
		return s, notificationCmd(m.err.Severity(), m.err.Error())
	}
	return s, tea.Batch(
		notificationCmd(errs.SeverityInfo, fmt.Sprintf("Asset %q created from folder", m.name)),
		s.loadCmd(),
	)
}

// onApply iterates preview.Changes (not the resolution maps) so the
// output order is deterministic and emits an explicit decision for every
// drift/unknown row. The domain remains the single source of the default
// — DriftKeep / UnknownKeep — so a future change to that default needs
// no follow-up here.
func (s *planProjectScreen) onApply() tea.Cmd {
	if s.preview == nil || len(s.preview.Changes) == 0 {
		return nil
	}
	var drift []app.DriftResolution
	var unknown []app.UnknownResolution
	for _, ch := range s.preview.Changes {
		switch ch.Kind {
		case app.ChangeDrift:
			decision := app.DriftKeep
			if s.driftResolutions[ch.Path] == app.DriftOverwrite {
				decision = app.DriftOverwrite
			}
			drift = append(drift, app.DriftResolution{Path: ch.Path, Decision: decision})
		case app.ChangeUnknown:
			decision := app.UnknownKeep
			if s.unknownResolutions[ch.Path] == app.UnknownDelete {
				decision = app.UnknownDelete
			}
			unknown = append(unknown, app.UnknownResolution{Path: ch.Path, Decision: decision})
		}
	}
	ignored := make([]string, 0, len(s.ignoredDirs))
	for dir := range s.ignoredDirs {
		ignored = append(ignored, dir)
	}
	sort.Strings(ignored)
	profileRef := s.profileID
	projectID := s.projectID
	return mutationCmd(
		func() errs.DomainError {
			_, err := s.actions.SyncProject(actions.SyncProjectInput{
				ProfileRef: profileRef,
				ProjectID:  projectID,
				Drift:      drift,
				Unknown:    unknown,
				Ignored:    ignored,
			})
			return err
		},
		"Project synced",
	)
}

// buildPlanTree turns the preview's FileChange list into a directory
// tree rooted at the project name. Paths are split on "/" because
// app.FileChange.Path is forward-slash relative per the domain's
// validatePathKey rule.
//
// A directory key present in ignoredDirs renders as a collapsed leaf: its
// node is created without the trailing "/" and without any children, and the
// walk stops descending past it so its files (and their per-row buttons) never
// appear. Later changes under the same folder find the node already built and
// stop as well.
func buildPlanTree(projectName string, changes []app.FileChange, ignoredDirs map[string]bool) *treetable.Node {
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
			ignored := ignoredDirs[acc]
			node, exists := dirs[acc]
			if !exists {
				dirLabel := part + "/"
				if ignored {
					dirLabel = part
				}
				node = &treetable.Node{
					Label: dirLabel,
					Data:  planNode{kind: planNodeDir, path: acc},
				}
				dirs[acc] = node
				parent.Children = append(parent.Children, node)
			}
			if ignored {
				break
			}
			parent = node
		}
	}
	return root
}
