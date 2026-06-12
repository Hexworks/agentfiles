package shell

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
	"charm.land/lipgloss/v2"

	"github.com/hexworks/agentfiles/internal/actions"
	"github.com/hexworks/agentfiles/internal/asset"
	"github.com/hexworks/agentfiles/internal/errs"
	"github.com/hexworks/agentfiles/internal/tui/components/focus"
	"github.com/hexworks/agentfiles/internal/tui/components/mnemonic"
	"github.com/hexworks/agentfiles/internal/tui/components/modal"
	"github.com/hexworks/agentfiles/internal/tui/components/treetable"
	"github.com/hexworks/agentfiles/internal/tui/editor"
	"github.com/hexworks/agentfiles/internal/tui/modals"
	"github.com/hexworks/agentfiles/internal/tui/styles"
)

// editAssetActions is the narrow slice of *actions.Actions the Edit
// Asset screen actually uses. Naming the interface here keeps the
// dependency direction tui→app explicit and lets tests substitute
// without depending on the full Actions surface.
type editAssetActions interface {
	LoadAsset(in actions.LoadAssetInput) (*asset.Asset, errs.DomainError)
	UpdateAsset(in actions.UpdateAssetInput) (struct{}, errs.DomainError)
}

// ModalKindAsset identifies which modal flow the Edit Asset screen
// currently hosts. Exposed so tests can assert intent
// (`ModalKindAssetDeleteFile`) instead of the raw modal-id string the
// dialog happens to carry.
type ModalKindAsset int

const (
	ModalKindAssetNone ModalKindAsset = iota
	ModalKindAssetDeleteFile
	ModalKindAssetCreateFile
	ModalKindAssetBackUnsaved
)

// fileKind discriminates treetable row payloads so the action func
// knows which buttons to render. Directories expose [Delete] only; files
// expose [Open] [Delete].
type fileKind int

const (
	kindAssetDir fileKind = iota
	kindAssetFile
)

// fileData is the opaque payload attached to every treetable node. The
// path is relative to asset.Dir (empty for the root).
type fileData struct {
	kind fileKind
	rel  string
}

// editAssetState is the comma-string + slice mirror the embedded huh
// fields bind to. The screen reads from this struct in
// syncFieldsToAsset; huh writes to it on every Update.
type editAssetState struct {
	description string
	tagsCSV     string
	compatible  []string
	exclusive   string
}

// editAssetScreen is the management screen reached from the Edit Profile
// row-level `[Edit]` action on an asset row. It hosts a left-pane files
// treetable + a right-pane Summary (read-only) and Customize (editable
// huh fields) layout, mnemonic-driven row actions, modal-composited
// confirmation dialogs, and per-action mutationCmd dispatch.
type editAssetScreen struct {
	actions   editAssetActions
	profileID string
	assetID   string

	asset            *asset.Asset
	originalManifest asset.Manifest
	files            []string

	state editAssetState

	handler     *focus.Handler
	tree        *treetable.Model
	description *huh.Text
	tags        *huh.Input
	compatible  *huh.MultiSelect[string]
	exclusive   *huh.Input

	openBtn   *mnemonic.Button // o (treetable, leaf rows only)
	deleteBtn *mnemonic.Button // d (treetable, any row)
	addBtn    *mnemonic.Button // a (treetable focus)
	saveBtn   *mnemonic.Button // e
	backBtn   *mnemonic.Button // b + esc

	treeMnemonic *mnemonic.Button // [1]
	descMnemonic *mnemonic.Button // [2]

	set *mnemonic.Set

	modal             *modal.Modal
	modalKind         ModalKindAsset
	pendingDeleteFile string
	pendingOpenFile   string

	width, height int
}

// editAssetLoadedMsg carries the loaded asset (or load error) that
// Init's command produces.
type editAssetLoadedMsg struct {
	a   *asset.Asset
	err errs.DomainError
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
	s.handler = focus.New(focus.WithModifier(focus.ModCtrl))
	s.treeMnemonic = s.handler.AddMnemonic(s.tree, '1')
	s.tree.SetMnemonicButton(s.treeMnemonic)
	s.descMnemonic = s.handler.AddMnemonic(s.description, '2')
	s.handler.Add(s.tags)
	s.handler.Add(s.compatible)
	s.handler.Add(s.exclusive)
	s.rebuildSet()
	return s
}

// ProfileID + AssetID expose the captured ids so tests don't reach
// into unexported fields.
func (s *editAssetScreen) ProfileID() string { return s.profileID }
func (s *editAssetScreen) AssetID() string   { return s.assetID }

// InputFocused reports whether a right-column huh.Field currently holds
// focus. The shell consults this to skip its single-rune global key
// intercept so `s`, `n`, `?`, `q` flow as text into the focused input
// instead of pushing a global screen.
func (s *editAssetScreen) InputFocused() bool {
	return s.handler != nil && s.handler.Focused() > 0
}

func (s *editAssetScreen) buildFields() {
	s.description = huh.NewText().
		Key("description").
		Title("Description").
		Description("Free-form description shown to selectors").
		Value(&s.state.description)
	s.tags = huh.NewInput().
		Key("tags").
		Title("Tags").
		Description(`Comma-separated, eg "git, build"`).
		Value(&s.state.tagsCSV)
	s.compatible = huh.NewMultiSelect[string]().
		Key("compatible_agents").
		Title("Compatible Agents").
		Description(`Empty means "all enabled agents"`).
		Value(&s.state.compatible).
		Options(modals.AgentOptions()...)
	s.exclusive = huh.NewInput().
		Key("exclusive_group").
		Title("Exclusive Group").
		Description("Optional mutual-exclusion key, eg `main-agents-doc`").
		Value(&s.state.exclusive)
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
		treetable.WithHeight(treetableMinHeight),
	)
}

// emptyTreeRoot is the placeholder used before the load command
// completes — a non-nil node satisfies treetable's `Label must not be
// empty` invariant.
func emptyTreeRoot() *treetable.Node {
	return &treetable.Node{
		Label: "(no asset loaded)",
		Data:  fileData{kind: kindAssetDir},
	}
}

const treetableMinHeight = 8

// treeActionsFn returns the per-row action button list. The treetable
// invokes it only for the cursor row, so always-fresh button instances
// are cheap. Leaf rows expose [Open] [Delete]; the root and directory
// rows expose [Delete] only (the root delete is a no-op handled by the
// confirm modal's path-empty guard, kept symmetric with the rest of the
// treetable contract).
func (s *editAssetScreen) treeActionsFn() treetable.ActionsFunc {
	return func(n *treetable.Node) []*mnemonic.Button {
		d, _ := n.Data.(fileData)
		if d.rel == "" {
			// Root row: no row-level actions. Returning nil keeps the
			// actions cell blank.
			return nil
		}
		if d.kind == kindAssetFile {
			return []*mnemonic.Button{s.openBtn, s.deleteBtn}
		}
		return []*mnemonic.Button{s.deleteBtn}
	}
}

// rebuildSet refreshes the mnemonic set so its registration-time
// uniqueness check covers the current focus + cursor state. Treetable
// focus exposes [1] [2] + the row buttons + screen buttons; any right-
// column focus drops every screen mnemonic so the printable letters flow
// as text into the focused huh field.
func (s *editAssetScreen) rebuildSet() {
	set := mnemonic.NewSet()
	if s.handler.Focused() == 0 {
		if s.treeMnemonic != nil {
			set.Add(s.treeMnemonic)
		}
		if s.descMnemonic != nil {
			set.Add(s.descMnemonic)
		}
		switch s.selectedKind() {
		case kindAssetFile:
			set.Add(s.openBtn)
			set.Add(s.deleteBtn)
		case kindAssetDir:
			if s.selectedRel() != "" {
				set.Add(s.deleteBtn)
			}
		}
		set.Add(s.addBtn)
		set.Add(s.saveBtn)
		set.Add(s.backBtn)
	}
	s.set = set
}

// selectedKind reports the kind of the treetable's selected row. A nil
// selection returns kindAssetDir (root-like) so callers fall through to
// the empty-folder branch.
func (s *editAssetScreen) selectedKind() fileKind {
	n := s.tree.SelectedNode()
	if n == nil {
		return kindAssetDir
	}
	d, _ := n.Data.(fileData)
	return d.kind
}

// selectedRel returns the relative path of the selected node, or empty
// for the root / no selection.
func (s *editAssetScreen) selectedRel() string {
	n := s.tree.SelectedNode()
	if n == nil {
		return ""
	}
	d, _ := n.Data.(fileData)
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
	// Single modal-guard for messages the modal owns. WindowSizeMsg + the
	// load/mutation/editor messages still need to update screen state
	// before being forwarded, so they have explicit branches below.
	if s.modal != nil {
		switch msg.(type) {
		case modal.ResolvedMsg,
			tea.WindowSizeMsg,
			editAssetLoadedMsg,
			mutationDoneMsg,
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
		return s, tea.Batch(notificationCmd(m.severity, m.text), s.loadCmd())
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
	s.originalManifest = cloneManifest(m.a.Manifest)
	s.files = sortedRelativeFiles(m.a.Dir)
	s.state.description = m.a.Description
	s.state.tagsCSV = modals.JoinTags(m.a.Tags)
	s.state.compatible = append([]string(nil), m.a.CompatibleAgents...)
	s.state.exclusive = m.a.ExclusiveGroup
	s.tree.SetRoot(buildAssetTree(m.a, s.files))
	focusCmd := s.handler.FocusIndex(0)
	s.rebuildSet()
	return s, focusCmd
}

func (s *editAssetScreen) handleEditorFinished(m editor.FinishedMsg) (Screen, tea.Cmd) {
	rel := s.pendingOpenFile
	s.pendingOpenFile = ""
	if m.Err != nil {
		return s, notificationCmd(errs.SeverityError, fmt.Sprintf("Editor failed: %v", m.Err))
	}
	// The editor wrote to disk; refresh the file list so a newly-created
	// neighbour does not get lost, and re-save the manifest so any future
	// per-file hash store (see app/service.go:432-437) refreshes.
	s.files = sortedRelativeFiles(s.asset.Dir)
	s.tree.SetRoot(buildAssetTree(s.asset, s.files))
	return s, mutationCmd(
		func() errs.DomainError {
			_, err := s.actions.UpdateAsset(actions.UpdateAssetInput{
				ProfileRef: s.profileID,
				Manifest:   &s.asset.Manifest,
			})
			return err
		},
		fmt.Sprintf("Edited %q", rel),
	)
}

func (s *editAssetScreen) handleKey(m tea.KeyPressMsg) (Screen, tea.Cmd) {
	if s.modal != nil {
		return s.forwardToModal(m)
	}
	// Focus handler consumes tab / shift+tab / ctrl+1 / ctrl+2.
	if handled, cmd := s.handler.Update(m); handled {
		s.syncFieldsToAsset()
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
// the newly-selected row.
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
	}
	return nil
}

func (s *editAssetScreen) Title() string { return "Edit Asset" }

// StatusKeys returns the focus-state mnemonics for the status bar. Per
// the parent task example (`e save b back tied to focus state`), Save
// and Back appear regardless of focus; row mnemonics only when the
// treetable holds focus; the screen-level `[Add]` is excluded because
// it is already visible on the body button row.
func (s *editAssetScreen) StatusKeys() []key.Binding {
	out := make([]key.Binding, 0, 4)
	if s.handler.Focused() == 0 {
		switch s.selectedKind() {
		case kindAssetFile:
			out = append(out, s.openBtn.Binding(), s.deleteBtn.Binding())
		case kindAssetDir:
			if s.selectedRel() != "" {
				out = append(out, s.deleteBtn.Binding())
			}
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

// renderBody composes the two-column layout. Width is split 50/50; the
// shell still controls outer width and height (we render at natural
// height).
func (s *editAssetScreen) renderBody(width int) string {
	leftWidth, rightWidth := splitWidth(width)
	left := s.renderLeft(leftWidth)
	right := s.renderRight(rightWidth)
	row := lipgloss.JoinHorizontal(lipgloss.Top, left, right)
	buttons := " " + s.addBtn.View() + "  " + s.saveBtn.View() + "  " + s.backBtn.View()
	return lipgloss.JoinVertical(lipgloss.Left, row, "", buttons)
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

func (s *editAssetScreen) renderLeft(_ int) string {
	return s.tree.View()
}

func (s *editAssetScreen) renderRight(width int) string {
	summary := s.renderSummary(width)
	customize := s.renderCustomize(width)
	return lipgloss.JoinVertical(lipgloss.Left, summary, "", customize)
}

func (s *editAssetScreen) renderSummary(_ int) string {
	header := styles.HeaderStyle.Render("Summary")
	name := "Name: " + assetSummaryValue(s.asset, func(a *asset.Asset) string { return a.Name })
	typ := "Type: " + assetSummaryValue(s.asset, func(a *asset.Asset) string { return string(a.Type) })
	return lipgloss.JoinVertical(lipgloss.Left, header, name, typ)
}

func assetSummaryValue(a *asset.Asset, pick func(*asset.Asset) string) string {
	if a == nil {
		return ""
	}
	return styles.Safe(pick(a))
}

func (s *editAssetScreen) renderCustomize(width int) string {
	header := styles.HeaderStyle.Render("Customize")
	clamp := width
	if clamp <= 0 {
		clamp = 40
	}
	s.description.WithWidth(clamp)
	s.tags.WithWidth(clamp)
	s.compatible.WithWidth(clamp)
	s.exclusive.WithWidth(clamp)
	return lipgloss.JoinVertical(
		lipgloss.Left,
		header,
		s.description.View(),
		s.tags.View(),
		s.compatible.View(),
		s.exclusive.View(),
	)
}

func (s *editAssetScreen) openModal(m *modal.Modal, kind ModalKindAsset) {
	s.modal = m
	s.modalKind = kind
	if s.width > 0 && s.height > 0 {
		mw, mh := modalSize(s.width, s.height)
		s.modal.SetSize(mw, mh)
	}
}

// handleResolved dispatches a modal ResolvedMsg to the right post-action
// handler. The modal field + pending paths are cleared up-front so
// unrelated modal lifecycles cannot leave a stale id behind regardless
// of which arm fires.
func (s *editAssetScreen) handleResolved(msg modal.ResolvedMsg) tea.Cmd {
	s.modal = nil
	kind := s.modalKind
	s.modalKind = ModalKindAssetNone
	pendingDelete := s.pendingDeleteFile
	s.pendingDeleteFile = ""
	switch kind {
	case ModalKindAssetDeleteFile:
		return s.afterDeleteFile(msg, pendingDelete)
	case ModalKindAssetCreateFile:
		return s.afterCreateFile(msg)
	case ModalKindAssetBackUnsaved:
		return s.afterBackUnsaved(msg)
	}
	return nil
}

func (s *editAssetScreen) onOpen() tea.Cmd {
	s.syncFieldsToAsset()
	if s.asset == nil {
		return nil
	}
	if s.selectedKind() != kindAssetFile {
		return nil
	}
	rel := s.selectedRel()
	if rel == "" {
		return nil
	}
	s.pendingOpenFile = rel
	return editor.Open(filepath.Join(s.asset.Dir, rel))
}

func (s *editAssetScreen) onDeleteFile() tea.Cmd {
	s.syncFieldsToAsset()
	if s.asset == nil {
		return nil
	}
	rel := s.selectedRel()
	if rel == "" {
		return nil
	}
	s.pendingDeleteFile = rel
	prompt := fmt.Sprintf("Delete file %q from the asset folder?", rel)
	s.openModal(modal.NewConfirm("delete-file", prompt, nil), ModalKindAssetDeleteFile)
	return s.modal.Init()
}

func (s *editAssetScreen) onAdd() tea.Cmd {
	s.syncFieldsToAsset()
	if s.asset == nil {
		return nil
	}
	s.openModal(modals.NewCreateFile(modals.CreateFileInput{}), ModalKindAssetCreateFile)
	return s.modal.Init()
}

func (s *editAssetScreen) onSave() tea.Cmd {
	s.syncFieldsToAsset()
	if s.asset == nil {
		return nil
	}
	return mutationCmd(
		func() errs.DomainError {
			_, err := s.actions.UpdateAsset(actions.UpdateAssetInput{
				ProfileRef: s.profileID,
				Manifest:   &s.asset.Manifest,
			})
			if err == nil {
				s.originalManifest = cloneManifest(s.asset.Manifest)
			}
			return err
		},
		fmt.Sprintf("Asset %q saved", s.assetID),
	)
}

func (s *editAssetScreen) onBack() tea.Cmd {
	s.syncFieldsToAsset()
	if !s.dirty() {
		return popCmd()
	}
	s.openModal(modal.NewConfirm("back-unsaved", "Discard unsaved changes?", nil), ModalKindAssetBackUnsaved)
	return s.modal.Init()
}

func (s *editAssetScreen) afterDeleteFile(msg modal.ResolvedMsg, rel string) tea.Cmd {
	if !msg.Confirmed || rel == "" || s.asset == nil {
		return nil
	}
	abs := filepath.Join(s.asset.Dir, rel)
	return mutationCmd(
		func() errs.DomainError {
			if err := os.Remove(abs); err != nil && !os.IsNotExist(err) {
				return assetFileRemoveError{Path: rel, Err: err}
			}
			s.files = sortedRelativeFiles(s.asset.Dir)
			s.tree.SetRoot(buildAssetTree(s.asset, s.files))
			_, err := s.actions.UpdateAsset(actions.UpdateAssetInput{
				ProfileRef: s.profileID,
				Manifest:   &s.asset.Manifest,
			})
			return err
		},
		fmt.Sprintf("Deleted %q", rel),
	)
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
	abs := filepath.Join(s.asset.Dir, rel)
	return mutationCmd(
		func() errs.DomainError {
			if err := assertInsideAssetDir(s.asset.Dir, abs); err != nil {
				return err
			}
			if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
				return assetFileCreateError{Path: rel, Err: err}
			}
			if err := os.WriteFile(abs, nil, 0o644); err != nil {
				return assetFileCreateError{Path: rel, Err: err}
			}
			s.files = sortedRelativeFiles(s.asset.Dir)
			s.tree.SetRoot(buildAssetTree(s.asset, s.files))
			_, err := s.actions.UpdateAsset(actions.UpdateAssetInput{
				ProfileRef: s.profileID,
				Manifest:   &s.asset.Manifest,
			})
			return err
		},
		fmt.Sprintf("Created %q", rel),
	)
}

func (s *editAssetScreen) afterBackUnsaved(msg modal.ResolvedMsg) tea.Cmd {
	if !msg.Confirmed {
		return nil
	}
	return popCmd()
}

// syncFieldsToAsset copies the comma-string + slice mirrors back into
// the in-memory asset manifest. Safe to call with a nil asset (no-op).
func (s *editAssetScreen) syncFieldsToAsset() {
	if s.asset == nil {
		return
	}
	s.asset.Description = s.state.description
	s.asset.Tags = modals.ParseTags(s.state.tagsCSV)
	s.asset.CompatibleAgents = append([]string(nil), s.state.compatible...)
	s.asset.ExclusiveGroup = s.state.exclusive
}

// dirty reports whether the in-memory asset manifest differs from the
// snapshot taken on load.
func (s *editAssetScreen) dirty() bool {
	if s.asset == nil {
		return false
	}
	return !reflect.DeepEqual(s.asset.Manifest, s.originalManifest)
}

// cloneManifest deep-copies the slice fields of an asset.Manifest so the
// snapshot does not alias the live manifest's slices.
func cloneManifest(m asset.Manifest) asset.Manifest {
	out := m
	out.Tags = append([]string(nil), m.Tags...)
	out.CompatibleAgents = append([]string(nil), m.CompatibleAgents...)
	out.Projections = append([]asset.Projection(nil), m.Projections...)
	return out
}

// sortedRelativeFiles wraps asset.RelativeFiles and returns a stable
// sort order so the treetable rebuild is deterministic. A missing
// directory yields an empty slice.
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
		Data:  fileData{kind: kindAssetDir, rel: ""},
	}
	// Track directory nodes by relative path so siblings nest under the
	// same parent regardless of file insertion order.
	dirs := map[string]*treetable.Node{"": root}
	for _, rel := range files {
		parts := strings.Split(rel, string(filepath.Separator))
		parent := root
		acc := ""
		for i, part := range parts {
			if i == len(parts)-1 {
				parent.Children = append(parent.Children, &treetable.Node{
					Label: part,
					Data:  fileData{kind: kindAssetFile, rel: rel},
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
					Data:  fileData{kind: kindAssetDir, rel: acc},
				}
				dirs[acc] = node
				parent.Children = append(parent.Children, node)
			}
			parent = node
		}
	}
	return root
}

// assertInsideAssetDir refuses writes whose absolute path escapes
// asset.Dir (e.g. a `../sibling/file.txt` Path). The check is cheap and
// is the only safety rail we ship for the in-screen file mutations.
func assertInsideAssetDir(assetDir, abs string) errs.DomainError {
	cleanRoot, err := filepath.Abs(assetDir)
	if err != nil {
		return assetFileCreateError{Path: abs, Err: err}
	}
	cleanTarget, err := filepath.Abs(abs)
	if err != nil {
		return assetFileCreateError{Path: abs, Err: err}
	}
	rel, err := filepath.Rel(cleanRoot, cleanTarget)
	if err != nil || strings.HasPrefix(rel, "..") || rel == ".." {
		return assetFilePathError{Path: abs}
	}
	return nil
}
