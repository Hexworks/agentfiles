package tui

import (
	"errors"
	"fmt"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/hexworks/agentfiles/internal/app"
	"github.com/hexworks/agentfiles/internal/asset"
	"github.com/hexworks/agentfiles/internal/doctor"
	"github.com/hexworks/agentfiles/internal/registry"
)

// supportedAssetTypes mirrors the asset.Type constants so the select list is
// domain-driven rather than a free-form string input.
var supportedAssetTypes = []huh.Option[string]{
	huh.NewOption("skill       — reusable SKILL.md plus supporting files", string(asset.TypeSkill)),
	huh.NewOption("agents_doc  — project-level AGENTS.md", string(asset.TypeAgentsDoc)),
	huh.NewOption("settings    — per-agent configuration files", string(asset.TypeSettings)),
	huh.NewOption("mcp         — MCP server configuration", string(asset.TypeMCP)),
	huh.NewOption("rule        — agent rule files", string(asset.TypeRule)),
	huh.NewOption("hook        — shell hooks the agent harness runs", string(asset.TypeHook)),
}

// supportedAgents is the closed set of render targets. Keep in sync with the
// agents recognized by internal/render.
var supportedAgents = []huh.Option[string]{
	huh.NewOption("claude-code", "claude-code"),
	huh.NewOption("codex", "codex"),
	huh.NewOption("cursor", "cursor"),
	huh.NewOption("opencode", "opencode"),
}

// nonEmpty rejects whitespace-only inputs so users cannot accidentally submit
// blanks through a required field.
func nonEmpty(field string) func(string) error {
	return func(v string) error {
		if strings.TrimSpace(v) == "" {
			return fmt.Errorf("%s is required", field)
		}
		return nil
	}
}

// RunProfileCreate drives the "profile create" flow.
func RunProfileCreate(service *app.Service) error {
	var name, path string
	err := runForm(huh.NewGroup(
		huh.NewNote().Title("Create profile").Description("Scaffolds a new profile folder and registers it."),
		huh.NewInput().Title("Name").Description("Display name for the profile").Value(&name).Validate(nonEmpty("name")),
		huh.NewInput().Title("Path").Description("Directory to create. ~ is expanded.").Value(&path).Validate(nonEmpty("path")),
	))
	if err != nil {
		return err
	}
	ref, err := service.CreateProfile(name, path)
	if err != nil {
		return err
	}
	fmt.Printf("created profile %s at %s\n", ref.Name, ref.Path)
	return nil
}

// RunProfileRegister drives the "profile register" flow.
func RunProfileRegister(service *app.Service) error {
	var path string
	err := runForm(huh.NewGroup(
		huh.NewNote().Title("Register profile").Description("Adds an existing profile folder to the registry."),
		huh.NewInput().Title("Path").Description("Existing profile directory").Value(&path).Validate(nonEmpty("path")),
	))
	if err != nil {
		return err
	}
	ref, err := service.RegisterProfile(path)
	if err != nil {
		return err
	}
	fmt.Printf("registered profile %s\n", ref.Name)
	return nil
}

// RunProfileList prints the registry contents. It is a read-only flow with no
// input prompts.
func RunProfileList(service *app.Service) error {
	reg, err := service.Registry.Load()
	if err != nil {
		return err
	}
	if len(reg.Profiles) == 0 {
		fmt.Println("no profiles registered")
	} else {
		for _, p := range reg.Profiles {
			fmt.Printf("%s\t%s\t%s\n", p.ID, p.Name, p.Path)
		}
	}
	return nil
}

// RunAssetInit drives the "asset init" flow, starting with a profile selector.
func RunAssetInit(service *app.Service) error {
	profileID, err := selectProfile(service, "Profile", "Profile that owns the new asset")
	if err != nil {
		return err
	}
	var typ, id, name, description string
	err = runForm(huh.NewGroup(
		huh.NewNote().Title("Init asset").Description("Scaffolds a new asset under the selected profile."),
		huh.NewSelect[string]().Title("Type").Description("Asset type determines the starter files").Options(supportedAssetTypes...).Value(&typ),
		huh.NewInput().Title("ID").Description("Stable identifier used in filenames and references").Value(&id).Validate(nonEmpty("id")),
		huh.NewInput().Title("Name").Description("Human-readable name").Value(&name).Validate(nonEmpty("name")),
		huh.NewInput().Title("Description").Description("Optional short description").Value(&description),
	))
	if err != nil {
		return err
	}
	dir, err := service.InitAsset(profileID, asset.Manifest{
		ID:          id,
		Name:        name,
		Type:        asset.Type(typ),
		Description: description,
	})
	if err != nil {
		return err
	}
	fmt.Println(dir)
	return nil
}

// RunProjectAdd drives the "project add" flow.
//
// Profile selection and asset enumeration happen in two steps because asset
// options depend on the chosen profile; splitting keeps the form model simple.
func RunProjectAdd(service *app.Service) error {
	profileID, err := selectProfile(service, "Profile", "Profile that owns the new project")
	if err != nil {
		return err
	}
	loaded, err := service.LoadProfile(profileID)
	if err != nil {
		return err
	}
	var assetOpts []huh.Option[string]
	for _, a := range loaded.Assets {
		assetOpts = append(assetOpts, huh.NewOption(fmt.Sprintf("%s [%s]", a.Name, a.Type), a.ID))
	}
	var name, path string
	var agents []string
	var assetIDs []string
	fields := []huh.Field{
		huh.NewNote().Title("Add project").Description("Registers a target repository for the selected profile."),
		huh.NewInput().Title("Name").Description("Human-readable name for the project").Value(&name).Validate(nonEmpty("name")),
		huh.NewInput().Title("Path").Description("Absolute path to the target repository").Value(&path).Validate(nonEmpty("path")),
		huh.NewMultiSelect[string]().Title("Agents").Description("Agents to render for").Options(supportedAgents...).Value(&agents).Validate(func(v []string) error {
			if len(v) == 0 {
				return errors.New("select at least one agent")
			}
			return nil
		}),
	}
	if len(assetOpts) > 0 {
		fields = append(fields, huh.NewMultiSelect[string]().Title("Assets").Description("Assets to include in the render plan").Options(assetOpts...).Value(&assetIDs))
	} else {
		fields = append(fields, huh.NewNote().Title("Assets").Description("No assets in this profile yet. You can add some later."))
	}
	if err := runForm(huh.NewGroup(fields...)); err != nil {
		return err
	}
	manifest, addErrs := service.AddProject(profileID, name, path, agents, assetIDs)
	if len(addErrs) > 0 {
		fmt.Print("\n")
		fmt.Print(RenderErrors(addErrs))
		return errAlreadyReported
	}
	fmt.Printf("added project %s at %s\n", manifest.Name, manifest.Path)
	return nil
}

// RunProjectPlan drives the read-only "project plan" flow.
func RunProjectPlan(service *app.Service) error {
	profileID, projectID, err := selectProfileAndProject(service)
	if err != nil {
		return err
	}
	preview, err := service.Plan(profileID, projectID)
	if err != nil {
		return err
	}
	fmt.Print(RenderPreview(preview))
	return nil
}

// RunProjectApply drives the "project apply" flow. The preview is shown inline
// before any destructive step so the user always sees what will be written.
func RunProjectApply(service *app.Service) error {
	profileID, projectID, err := selectProfileAndProject(service)
	if err != nil {
		return err
	}
	preview, err := service.Plan(profileID, projectID)
	if err != nil {
		return err
	}
	fmt.Print(RenderPreview(preview))
	var deleteCandidates bool
	if len(preview.DeleteCandidates) > 0 {
		if err := runForm(huh.NewGroup(
			huh.NewConfirm().
				Title("Delete recognized unmanaged files?").
				Description(fmt.Sprintf("%d delete candidate(s) detected", len(preview.DeleteCandidates))).
				Value(&deleteCandidates),
		)); err != nil {
			return err
		}
	}
	var confirmed bool
	if err := runForm(huh.NewGroup(
		huh.NewConfirm().
			Title("Apply these changes?").
			Value(&confirmed),
	)); err != nil {
		return err
	}
	if !confirmed {
		fmt.Println("aborted")
		return nil
	}
	if _, err := service.Apply(profileID, projectID, deleteCandidates); err != nil {
		return err
	}
	fmt.Println("applied")
	return nil
}

// RunDoctor drives the read-only health check for one profile.
func RunDoctor(service *app.Service) error {
	profileID, err := selectProfile(service, "Profile", "Profile to inspect")
	if err != nil {
		return err
	}
	loaded, err := service.LoadProfile(profileID)
	if err != nil {
		return err
	}
	report, checkErrs := doctor.CheckProfile(loaded)
	fmt.Print(RenderReport(report))
	if len(checkErrs) > 0 {
		fmt.Print("\n")
		fmt.Print(RenderErrors(checkErrs))
		return errAlreadyReported
	}
	return nil
}

// selectProfile shows a Select populated from the global registry. Returning
// the id keeps the caller working with the stable identifier rather than a
// display string.
func selectProfile(service *app.Service, title, description string) (string, error) {
	reg, loadErr := service.Registry.Load()
	if loadErr != nil {
		return "", loadErr
	}
	if len(reg.Profiles) == 0 {
		return "", errors.New("no profiles registered; create one first")
	}
	opts := profileOptions(reg.Profiles)
	var id string
	if err := runForm(huh.NewGroup(
		huh.NewSelect[string]().Title(title).Description(description).Options(opts...).Value(&id),
	)); err != nil {
		return "", err
	}
	return id, nil
}

// selectProfileAndProject is the two-step picker used by plan/apply flows.
func selectProfileAndProject(service *app.Service) (string, string, error) {
	profileID, err := selectProfile(service, "Profile", "Profile that owns the project")
	if err != nil {
		return "", "", err
	}
	loaded, loadErr := service.LoadProfile(profileID)
	if loadErr != nil {
		return "", "", loadErr
	}
	projects := loaded.ProjectList()
	if len(projects) == 0 {
		return "", "", errors.New("profile has no projects")
	}
	var opts []huh.Option[string]
	for _, p := range projects {
		opts = append(opts, huh.NewOption(fmt.Sprintf("%s (%s)", p.Name, p.Path), p.ID))
	}
	var projectID string
	if err := runForm(huh.NewGroup(
		huh.NewSelect[string]().Title("Project").Description("Project to plan/apply").Options(opts...).Value(&projectID),
	)); err != nil {
		return "", "", err
	}
	return profileID, projectID, nil
}

// profileOptions renders the registry into Select options. The label combines
// name and path so users can still differentiate same-named profiles at a
// glance.
func profileOptions(profiles []registry.ProfileRef) []huh.Option[string] {
	opts := make([]huh.Option[string], 0, len(profiles))
	for _, p := range profiles {
		opts = append(opts, huh.NewOption(fmt.Sprintf("%s (%s)", p.Name, p.Path), p.ID))
	}
	return opts
}
