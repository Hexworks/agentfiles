package shell

import (
	"fmt"
	"path/filepath"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
	"charm.land/lipgloss/v2"

	"github.com/hexworks/agentfiles/internal/actions"
	"github.com/hexworks/agentfiles/internal/asset"
	"github.com/hexworks/agentfiles/internal/errs"
	"github.com/hexworks/agentfiles/internal/tui/components/focus"
	"github.com/hexworks/agentfiles/internal/tui/components/help"
	"github.com/hexworks/agentfiles/internal/tui/components/mnemonic"
	"github.com/hexworks/agentfiles/internal/tui/components/modal"
	"github.com/hexworks/agentfiles/internal/tui/components/panel"
	"github.com/hexworks/agentfiles/internal/tui/components/treetable"
	"github.com/hexworks/agentfiles/internal/tui/editor"
	"github.com/hexworks/agentfiles/internal/tui/modals"
)

// editAssetActions is the narrow slice of *actions.Actions the Edit
// Asset screen actually uses. Naming the interface here keeps the
// dependency direction tui→app explicit and lets tests substitute
// without depending on the full Actions surface.
type editAssetActions interface {
	LoadAsset(in actions.LoadAssetInput) (*asset.Asset, errs.DomainError)
	UpdateAsset(in actions.UpdateAssetInput) (struct{}, errs.DomainError)
	AddAssetFile(in actions.AddAssetFileInput) (struct{}, errs.DomainError)
	RemoveAssetFile(in actions.RemoveAssetFileInput) (struct{}, errs.DomainError)
}

// modalKindAsset identifies which modal flow the Edit Asset screen
// currently hosts. Tests assert intent through this enum rather than the
// raw modal-id string the dialog carries.
type modalKindAsset int

const (
	modalKindAssetNone modalKindAsset = iota
	modalKindAssetDeleteFile
	modalKindAssetCreateFile
	modalKindAssetBackUnsaved
)

// nodeKind classifies a treetable row payload. The root row is its own
// kind so callers don't have to special-case `rel == ""`.
type nodeKind int

const (
	nodeRoot nodeKind = iota
	nodeDir
	nodeFile
)

// assetNode is the payload attached to every treetable node. The path is
// relative to asset.Dir; the root row has kind nodeRoot and an empty rel.
type assetNode struct {
	kind nodeKind
	rel  string
}

// editAssetForm is the form-state mirror the embedded huh fields bind to.
// It is the single source of truth for user-typed values; the in-memory
// *asset.Asset stays immutable between load and save. dirty() compares
// this struct against snapshot.
type editAssetForm struct {
	descriptionText  string
	tagsCSV          string
	compatibleAgents []string
	exclusiveGroup   string
}

// treetableHeight is the row count handed to treetable.WithHeight. The
// constant lives next to the tree builder so a future tweak does not
// have to scroll the file.
const treetableHeight = 8

// editAssetScreen is the management screen reached from the Edit Profile
// row-level `[Edit]` action on an asset row. It hosts a left-pane files
// treetable + a right-pane Summary (read-only) and Customize (editable
// huh fields) layout, mnemonic-driven row actions, modal-composited
// confirmation dialogs, and typed-message mutations.
type editAssetScreen struct {
	actions   editAssetActions
	profileID string
	assetID   string

	// asset is the loaded aggregate. It stays immutable between
	// editAssetLoadedMsg and the next loadCmd — every user edit lives in
	// form. The pointer doubles as a "loaded" sentinel.
	asset    *asset.Asset
	files    []string
	form     editAssetForm
	snapshot editAssetForm

	handler     *focus.Handler
	tree        *treetable.Model
	description *huh.Text
	tags        *huh.Input
	compatible  *huh.MultiSelect[string]
	exclusive   *huh.Input

	// per-field focus indices captured at registration time so
	// renderCustomize and the tests stay aligned with the actual order
	// the focus.Handler hands out.
	treeIdx       int
	descIdx       int
	tagsIdx       int
	compatibleIdx int
	exclusiveIdx  int

	openBtn   *mnemonic.Button // o (treetable, leaf rows only)
	deleteBtn *mnemonic.Button // d (treetable, file or dir rows)
	addBtn    *mnemonic.Button // a (treetable focus)
	saveBtn   *mnemonic.Button // e
	backBtn   *mnemonic.Button // b + esc

	set *mnemonic.Set

	modal         *modal.Modal
	modalKind     modalKindAsset
	modalHandlers map[modalKindAsset]func(modal.ResolvedMsg) tea.Cmd
	deletingFile  string // pending delete relative path
	editingFile   string // pending editor relative path (survives external editor suspend/resume)

	width, height int
}

// editAssetLoadedMsg carries the loaded asset (or load error) that Init's
// command produces.
type editAssetLoadedMsg struct {
	a   *asset.Asset
	err errs.DomainError
}

// filesChangedMsg is dispatched by a successful Add / Delete mutation so
// the Update goroutine — not the Cmd goroutine — refreshes the file list
// and rebuilds the treetable. info is the success text the screen
// surfaces as a notification.
type filesChangedMsg struct {
	files []string
	info  string
}

// saveSucceededMsg is dispatched by a successful Save so Update refreshes
// the dirty-tracking snapshot atomically with the toast emission.
type saveSucceededMsg struct {
	snapshot editAssetForm
	info     string
}

func newEditAssetScreen(a editAssetActions, profileID, assetID string) *editAssetScreen {
	if a == nil {
		panic("shell.newEditAssetScreen: nil actions")
	}
	if profileID == "" {
		panic("shell.newEditAssetScreen: empty profileID")
	}
	if assetID == "" {
		panic("shell.newEditAssetScreen: empty assetID")
	}
	s := &editAssetScreen{
		actions:   a,
		profileID: profileID,
		assetID:   assetID,
	}
	s.buildFields()
	s.buildButtons()
	s.buildTree()
	s.handler = focus.New()
	// Capture each focus index as the registration runs so renderer and
	// tests stay aligned with the actual order the handler hands out.
	s.treeIdx = 0
	s.handler.Add(s.tree)
	s.descIdx = 1
	s.handler.Add(s.description)
	s.tagsIdx = 2
	s.handler.Add(s.tags)
	s.compatibleIdx = 3
	s.handler.Add(s.compatible)
	s.exclusiveIdx = 4
	s.handler.Add(s.exclusive)
	s.registerModalHandlers()
	s.rebuildSet()
	return s
}

// ProfileID + AssetID expose the captured ids so tests don't reach into
// unexported fields.
func (s *editAssetScreen) ProfileID() string { return s.profileID }
func (s *editAssetScreen) AssetID() string   { return s.assetID }

// InputFocused reports whether a right-column huh.Field currently holds
// focus. The shell consults this to skip its single-rune global key
// intercept so `s`, `n`, `?`, `q` flow as text into the focused input
// instead of pushing a global screen.
func (s *editAssetScreen) InputFocused() bool {
	if s.modal.Active() {
		return true
	}
	return s.handler != nil && s.handler.Focused() > s.treeIdx
}

func (s *editAssetScreen) buildFields() {
	// huh fields default to a zero-value KeyMap + nil theme when
	// constructed outside a huh.Form. That silently breaks Toggle / Up /
	// Down shortcuts on MultiSelect and leaves selectors blank. Apply
	// the default keymap and Charm theme explicitly so every field
	// honors upstream defaults (space/x toggle, j/k navigation, visible
	// `[x]` / `[ ]` selectors).
	keymap := huh.NewDefaultKeyMap()
	theme := huh.ThemeFunc(huh.ThemeCharm)
	s.description = huh.NewText().
		Key("description").
		Title("Description").
		Description("Free-form description shown to selectors").
		Value(&s.form.descriptionText)
	s.description.WithKeyMap(keymap)
	s.description.WithTheme(theme)
	s.tags = huh.NewInput().
		Key("tags").
		Title("Tags").
		Description(`Comma-separated, eg "git, build"`).
		Value(&s.form.tagsCSV)
	s.tags.WithKeyMap(keymap)
	s.tags.WithTheme(theme)
	s.compatible = huh.NewMultiSelect[string]().
		Key("compatible_agents").
		Title("Compatible Agents").
		Description(`Empty means "all enabled agents"`).
		Value(&s.form.compatibleAgents).
		Options(modals.AgentOptions()...)
	s.compatible.WithKeyMap(keymap)
	s.compatible.WithTheme(theme)
	s.exclusive = huh.NewInput().
		Key("exclusive_group").
		Title("Exclusive Group").
		Description("Optional mutual-exclusion key, eg `main-agents-doc`").
		Value(&s.form.exclusiveGroup)
	s.exclusive.WithKeyMap(keymap)
	s.exclusive.WithTheme(theme)
}

func (s *editAssetScreen) buildButtons() {
	s.openBtn = mnemonic.New("Open", 'o', func() tea.Cmd { return s.onOpen() })
	s.deleteBtn = mnemonic.New("Delete", 'd', func() tea.Cmd { return s.onDeleteFile() })
	s.addBtn = mnemonic.New("Add", 'a', func() tea.Cmd { return s.onAdd() })
	s.saveBtn = mnemonic.New("Save", 'e', func() tea.Cmd { return s.onSave() })
	s.backBtn = mnemonic.New(
		"Back",
		'b',
		func() tea.Cmd { return s.onBack() },
		mnemonic.WithExtraBindingKeys("esc"),
	)
}

func (s *editAssetScreen) buildTree() {
	s.tree = treetable.New(
		treetable.WithRoot(emptyTreeRoot()),
		treetable.WithNameColumn(treetable.Column{Title: "Name", Width: 36}),
		treetable.WithActions(
			treetable.Column{Title: "Actions", Width: 18},
			s.treeActionsFn(),
		),
		treetable.WithTitle("Files"),
		treetable.WithHeight(treetableHeight),
		treetable.WithStyles(focusAwareTreetableStyles()),
	)
}

// registerModalHandlers wires the (kind, handler) registry consulted by
// handleResolved. Adding a new modal kind = add the constant + a method +
// one line here, instead of editing a switch.
func (s *editAssetScreen) registerModalHandlers() {
	s.modalHandlers = map[modalKindAsset]func(modal.ResolvedMsg) tea.Cmd{
		modalKindAssetDeleteFile:  s.afterDeleteFile,
		modalKindAssetCreateFile:  s.afterCreateFile,
		modalKindAssetBackUnsaved: s.afterBackUnsaved,
	}
}

// treeActionsFn returns the per-row action button list. The treetable
// invokes it only for the cursor row, so always-fresh button instances
// are cheap. File rows expose [Open] [Delete]; directory rows expose
// [Delete] only; the root row exposes nothing.
func (s *editAssetScreen) treeActionsFn() treetable.ActionsFunc {
	return func(n *treetable.Node) []*mnemonic.Button {
		d, _ := n.Data.(assetNode)
		switch d.kind {
		case nodeFile:
			return []*mnemonic.Button{s.openBtn, s.deleteBtn}
		case nodeDir:
			return []*mnemonic.Button{s.deleteBtn}
		}
		return nil
	}
}

// rebuildSet refreshes the mnemonic set so its registration-time
// uniqueness check covers the current focus + cursor state. Treetable
// focus exposes the row buttons + screen buttons; any right-column focus
// drops every screen mnemonic so the printable letters flow as text into
// the focused huh field.
func (s *editAssetScreen) rebuildSet() {
	set := mnemonic.NewSet()
	if s.handler.Focused() == s.treeIdx {
		switch s.selectedKind() {
		case nodeFile:
			set.Add(s.openBtn)
			set.Add(s.deleteBtn)
		case nodeDir:
			set.Add(s.deleteBtn)
		}
		set.Add(s.addBtn)
		set.Add(s.saveBtn)
		set.Add(s.backBtn)
	}
	s.set = set
}

// selectedKind reports the kind of the treetable's selected row. A nil
// selection returns nodeRoot so callers fall through to the empty-folder
// branch.
func (s *editAssetScreen) selectedKind() nodeKind {
	n := s.tree.SelectedNode()
	if n == nil {
		return nodeRoot
	}
	d, _ := n.Data.(assetNode)
	return d.kind
}

// selectedFileRel returns the cursor row's relative path when that row is
// a file. The second value is false for directory / root / no-selection
// rows so callers don't need a separate nil + kind + empty-string check.
func (s *editAssetScreen) selectedFileRel() (string, bool) {
	n := s.tree.SelectedNode()
	if n == nil {
		return "", false
	}
	d, _ := n.Data.(assetNode)
	if d.kind != nodeFile {
		return "", false
	}
	return d.rel, true
}

// selectedRel returns the relative path of the selected node (any kind),
// or empty for the root / no selection.
func (s *editAssetScreen) selectedRel() string {
	n := s.tree.SelectedNode()
	if n == nil {
		return ""
	}
	d, _ := n.Data.(assetNode)
	return d.rel
}

func (s *editAssetScreen) Init() tea.Cmd { return s.loadCmd() }

func (s *editAssetScreen) loadCmd() tea.Cmd {
	return func() tea.Msg {
		a, err := s.actions.LoadAsset(actions.LoadAssetInput{
			ProfileRef: s.profileID,
			AssetID:    s.assetID,
		})
		return editAssetLoadedMsg{a: a, err: err}
	}
}

func (s *editAssetScreen) Update(msg tea.Msg) (Screen, tea.Cmd) {
	// Single modal-guard for messages the modal owns. The typed state
	// messages below still need to update screen state before being
	// forwarded, so they get explicit branches.
	if s.modal != nil {
		switch msg.(type) {
		case modal.ResolvedMsg,
			tea.WindowSizeMsg,
			editAssetLoadedMsg,
			mutationDoneMsg,
			filesChangedMsg,
			saveSucceededMsg,
			editor.FinishedMsg:
			// fall through to type-specific handling
		default:
			return s.forwardToModal(msg)
		}
	}

	switch m := msg.(type) {
	case tea.WindowSizeMsg:
		return s.handleResize(m)
	case editAssetLoadedMsg:
		return s.handleLoaded(m)
	case mutationDoneMsg:
		return s, notificationCmd(m.severity, m.text)
	case filesChangedMsg:
		return s.handleFilesChanged(m)
	case saveSucceededMsg:
		return s.handleSaveSucceeded(m)
	case editor.FinishedMsg:
		return s.handleEditorFinished(m)
	case modal.ResolvedMsg:
		return s, s.handleResolved(m)
	case tea.KeyPressMsg:
		return s.handleKey(m)
	}
	return s, nil
}

func (s *editAssetScreen) handleResize(m tea.WindowSizeMsg) (Screen, tea.Cmd) {
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

func (s *editAssetScreen) handleLoaded(m editAssetLoadedMsg) (Screen, tea.Cmd) {
	if m.err != nil {
		return s, notificationCmd(m.err.Severity(), m.err.Error())
	}
	if m.a == nil {
		return s, nil
	}
	s.asset = m.a
	s.files = sortedRelativeFiles(m.a.Dir)
	s.hydrateForm(m.a)
	s.snapshot = s.form
	s.tree.SetRoot(buildAssetTree(m.a, s.files))
	focusCmd := s.handler.FocusIndex(s.treeIdx)
	s.rebuildSet()
	return s, focusCmd
}

// hydrateForm copies manifest fields into the form on load.
// syncForm (further down) is the inverse: it copies form values into a
// fresh manifest for save. Keeping the pair adjacent in this file makes
// "a new Manifest field needs both sides" obvious.
func (s *editAssetScreen) hydrateForm(a *asset.Asset) {
	s.form = editAssetForm{
		descriptionText:  a.Description,
		tagsCSV:          modals.JoinTags(a.Tags),
		compatibleAgents: append([]string(nil), a.CompatibleAgents...),
		exclusiveGroup:   a.ExclusiveGroup,
	}
}

// composeManifest builds the asset.Manifest the Save flow persists by
// overlaying the form on top of the immutable s.asset's identity fields.
// The pair (hydrateForm, composeManifest) is the only place manifest
// fields appear — adding a field touches both, and only both.
func (s *editAssetScreen) composeManifest() asset.Manifest {
	return asset.Manifest{
		ID:               s.asset.ID,
		Name:             s.asset.Name,
		Type:             s.asset.Type,
		Description:      s.form.descriptionText,
		Tags:             modals.ParseTags(s.form.tagsCSV),
		CompatibleAgents: append([]string(nil), s.form.compatibleAgents...),
		ExclusiveGroup:   s.form.exclusiveGroup,
		Projections:      append([]asset.Projection(nil), s.asset.Projections...),
	}
}

func (s *editAssetScreen) handleFilesChanged(m filesChangedMsg) (Screen, tea.Cmd) {
	s.files = m.files
	s.tree.SetRoot(buildAssetTree(s.asset, s.files))
	s.rebuildSet()
	if m.info == "" {
		return s, nil
	}
	return s, notificationCmd(errs.SeverityInfo, m.info)
}

func (s *editAssetScreen) handleSaveSucceeded(m saveSucceededMsg) (Screen, tea.Cmd) {
	s.snapshot = m.snapshot
	if m.info == "" {
		return s, nil
	}
	return s, notificationCmd(errs.SeverityInfo, m.info)
}

func (s *editAssetScreen) handleEditorFinished(m editor.FinishedMsg) (Screen, tea.Cmd) {
	rel := s.editingFile
	s.editingFile = ""
	if m.Err != nil {
		return s, notificationCmd(errs.SeverityError, fmt.Sprintf("Editor failed: %v", m.Err))
	}
	// The editor wrote to disk; refresh the file list (a brand-new
	// neighbour file would otherwise be invisible) and persist the
	// manifest so a future per-file hash store (see app/service.go's
	// UpdateAsset comment) refreshes too.
	dir := s.asset.Dir
	files := sortedRelativeFiles(dir)
	s.files = files
	s.tree.SetRoot(buildAssetTree(s.asset, s.files))
	return s, s.saveManifestCmd(fmt.Sprintf("Edited %q", rel))
}

func (s *editAssetScreen) handleKey(m tea.KeyPressMsg) (Screen, tea.Cmd) {
	if s.modal != nil {
		return s.forwardToModal(m)
	}
	// Focus handler consumes tab / shift+tab.
	if handled, cmd := s.handler.Update(m); handled {
		s.rebuildSet()
		return s, cmd
	}
	if btn := s.set.Match(m); btn != nil {
		return s, btn.Trigger()
	}
	return s, s.routeToFocusedComponent(m)
}

func (s *editAssetScreen) forwardToModal(msg tea.Msg) (Screen, tea.Cmd) {
	var cmd tea.Cmd
	s.modal, cmd = s.modal.Update(msg)
	return s, cmd
}

// routeToFocusedComponent forwards keypresses the handler and mnemonic
// set did not consume to whichever component the handler currently
// considers focused. For the treetable we additionally rebuild the
// mnemonic set when the cursor moves so the row-action buttons reflect
// the newly-selected row. An unrecognised focusable kind panics — silent
// no-op would swallow keys when a new focusable type is added.
func (s *editAssetScreen) routeToFocusedComponent(m tea.KeyPressMsg) tea.Cmd {
	c := s.handler.FocusedComponent()
	switch v := c.(type) {
	case *treetable.Model:
		before := s.tree.Cursor()
		var cmd tea.Cmd
		s.tree, cmd = s.tree.Update(m)
		if s.tree.Cursor() != before {
			s.rebuildSet()
		}
		return cmd
	case huh.Field:
		_, cmd := v.Update(m)
		return cmd
	case nil:
		// Pre-load: handler has no focused component yet.
		return nil
	default:
		panic(fmt.Sprintf("editAssetScreen: unsupported focusable %T", c))
	}
}

func (s *editAssetScreen) Title() string { return "Edit Asset" }

func (s *editAssetScreen) Topic() help.Topic {
	return help.Topic{Label: "Edit Asset", File: "edit_asset.md"}
}

// StatusKeys returns the focus-state mnemonics for the status bar. Per
// the parent task example (`e save b back tied to focus state`), Save
// and Back appear regardless of focus; row mnemonics only when the
// treetable holds focus; the screen-level `[Add]` is excluded because
// it is already visible on the body button row.
func (s *editAssetScreen) StatusKeys() []key.Binding {
	out := make([]key.Binding, 0, 4)
	if s.handler.Focused() == s.treeIdx {
		switch s.selectedKind() {
		case nodeFile:
			out = append(out, s.openBtn.Binding(), s.deleteBtn.Binding())
		case nodeDir:
			out = append(out, s.deleteBtn.Binding())
		}
	}
	out = append(out, s.saveBtn.Binding(), s.backBtn.Binding())
	return out
}

func (s *editAssetScreen) Body(width int) string {
	background := s.renderBody(width)
	if s.modal == nil {
		return background
	}
	return s.modal.Render(background, width, lipgloss.Height(background))
}

func (s *editAssetScreen) renderBody(width int) string {
	leftWidth, rightWidth := splitWidth(width)
	left := s.renderLeft(leftWidth)
	right := s.renderRight(rightWidth)
	row := lipgloss.JoinHorizontal(lipgloss.Top, left, right)
	buttons := " " + s.addBtn.View() + "  " + s.saveBtn.View() + "  " + s.backBtn.View()
	return lipgloss.JoinVertical(lipgloss.Left, row, "", buttons)
}

func (s *editAssetScreen) renderLeft(_ int) string {
	return s.tree.View()
}

func (s *editAssetScreen) renderRight(width int) string {
	summary := s.renderSummary(width)
	customize := s.renderCustomize(width)
	return lipgloss.JoinVertical(lipgloss.Left, summary, "", customize)
}

func (s *editAssetScreen) renderSummary(_ int) string {
	header := assetHeader("Summary")
	name := "Name: " + assetSummaryValue(s.asset, func(a *asset.Asset) string { return a.Name })
	typ := "Type: " + assetSummaryValue(s.asset, func(a *asset.Asset) string { return string(a.Type) })
	return lipgloss.JoinVertical(lipgloss.Left, header, name, typ)
}

func (s *editAssetScreen) renderCustomize(width int) string {
	header := assetHeader("Customize")
	clamp := width
	if clamp <= 0 {
		clamp = 40
	}
	// Borders consume two columns; clamp the inner field width so the
	// outer panel respects the right-column allotment.
	innerWidth := clamp - 2
	if innerWidth < 1 {
		innerWidth = 1
	}
	s.description.WithWidth(innerWidth)
	s.tags.WithWidth(innerWidth)
	s.compatible.WithWidth(innerWidth)
	s.exclusive.WithWidth(innerWidth)
	focus := s.handler.Focused()
	st := focusAwarePanelStyles()
	return lipgloss.JoinVertical(
		lipgloss.Left,
		header,
		panel.Render(focus == s.descIdx, "", s.description.View(), st),
		panel.Render(focus == s.tagsIdx, "", s.tags.View(), st),
		panel.Render(focus == s.compatibleIdx, "", s.compatible.View(), st),
		panel.Render(focus == s.exclusiveIdx, "", s.exclusive.View(), st),
	)
}

func (s *editAssetScreen) openModal(m *modal.Modal, kind modalKindAsset) {
	s.modal = m
	s.modalKind = kind
	if s.width > 0 && s.height > 0 {
		mw, mh := modalSize(s.width, s.height)
		s.modal.SetSize(mw, mh)
	}
}

// handleResolved dispatches a modal ResolvedMsg via the (kind, handler)
// registry. Clearing the modal + pending state up-front means an unknown
// kind cannot leave a stale id behind.
func (s *editAssetScreen) handleResolved(msg modal.ResolvedMsg) tea.Cmd {
	s.modal = nil
	kind := s.modalKind
	s.modalKind = modalKindAssetNone
	if handler, ok := s.modalHandlers[kind]; ok {
		return handler(msg)
	}
	return nil
}

func (s *editAssetScreen) onOpen() tea.Cmd {
	rel, ok := s.selectedFileRel()
	if !ok {
		return nil
	}
	s.editingFile = rel
	return editor.Open(filepath.Join(s.asset.Dir, rel))
}

func (s *editAssetScreen) onDeleteFile() tea.Cmd {
	rel, ok := s.selectedFileRel()
	if !ok {
		return nil
	}
	s.deletingFile = rel
	prompt := fmt.Sprintf("Delete file %q from the asset folder?", rel)
	s.openModal(modal.NewConfirm("delete-file", prompt, nil), modalKindAssetDeleteFile)
	return s.modal.Init()
}

func (s *editAssetScreen) onAdd() tea.Cmd {
	if s.asset == nil {
		return nil
	}
	s.openModal(modals.NewCreateFile(modals.CreateFileInput{}), modalKindAssetCreateFile)
	return s.modal.Init()
}

func (s *editAssetScreen) onSave() tea.Cmd {
	if s.asset == nil {
		return nil
	}
	return s.saveManifestCmd(fmt.Sprintf("Asset %q saved", s.assetID))
}

// saveManifestCmd is the single chokepoint for "persist current form as
// a manifest" — called from Save and from any post-mutation path that
// also wants pending edits committed (file create / delete, editor
// finished). On success it emits saveSucceededMsg so Update refreshes
// snapshot atomically; on failure it emits a mutationDoneMsg so the
// caller still sees the toast but the snapshot stays stale (which leaves
// dirty() returning true).
func (s *editAssetScreen) saveManifestCmd(info string) tea.Cmd {
	manifest := s.composeManifest()
	snapshot := s.form
	profileRef := s.profileID
	return func() tea.Msg {
		if _, err := s.actions.UpdateAsset(actions.UpdateAssetInput{
			ProfileRef: profileRef,
			Manifest:   &manifest,
		}); err != nil {
			return mutationDoneMsg{severity: err.Severity(), text: err.Error()}
		}
		return saveSucceededMsg{snapshot: snapshot, info: info}
	}
}

func (s *editAssetScreen) onBack() tea.Cmd {
	if !s.dirty() {
		return popCmd()
	}
	s.openModal(modal.NewConfirm("back-unsaved", "Discard unsaved changes?", nil), modalKindAssetBackUnsaved)
	return s.modal.Init()
}

func (s *editAssetScreen) afterDeleteFile(msg modal.ResolvedMsg) tea.Cmd {
	rel := s.deletingFile
	s.deletingFile = ""
	if !msg.Confirmed || rel == "" || s.asset == nil {
		return nil
	}
	dir := s.asset.Dir
	profileRef := s.profileID
	assetID := s.assetID
	info := fmt.Sprintf("Deleted %q", rel)
	return func() tea.Msg {
		if _, err := s.actions.RemoveAssetFile(actions.RemoveAssetFileInput{
			ProfileRef: profileRef,
			AssetID:    assetID,
			Rel:        rel,
		}); err != nil {
			return mutationDoneMsg{severity: err.Severity(), text: err.Error()}
		}
		return filesChangedMsg{files: sortedRelativeFiles(dir), info: info}
	}
}

func (s *editAssetScreen) afterCreateFile(msg modal.ResolvedMsg) tea.Cmd {
	if !msg.Confirmed || s.asset == nil {
		return nil
	}
	in, ok := msg.Value.(modals.CreateFileInput)
	if !ok {
		return nil
	}
	rel := strings.TrimSpace(in.Path)
	if rel == "" {
		return nil
	}
	dir := s.asset.Dir
	profileRef := s.profileID
	assetID := s.assetID
	info := fmt.Sprintf("Created %q", rel)
	return func() tea.Msg {
		if _, err := s.actions.AddAssetFile(actions.AddAssetFileInput{
			ProfileRef: profileRef,
			AssetID:    assetID,
			Rel:        rel,
		}); err != nil {
			return mutationDoneMsg{severity: err.Severity(), text: err.Error()}
		}
		return filesChangedMsg{files: sortedRelativeFiles(dir), info: info}
	}
}

func (s *editAssetScreen) afterBackUnsaved(msg modal.ResolvedMsg) tea.Cmd {
	if !msg.Confirmed {
		return nil
	}
	return popCmd()
}

// dirty reports whether the user has typed changes that have not been
// saved. The comparison is field-by-field against the snapshot captured
// on load and refreshed on save success — no reflect.DeepEqual
// nil-vs-empty subtleties.
func (s *editAssetScreen) dirty() bool {
	if s.asset == nil {
		return false
	}
	return !formsEqual(s.form, s.snapshot)
}
