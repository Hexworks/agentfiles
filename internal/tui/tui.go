// Package tui implements the interactive terminal interface that drives every
// agentfiles command. The CLI entry points only route to a flow in this package.
package tui

import (
	"errors"
	"fmt"

	"github.com/addamsson/agentfiles/internal/app"
	"github.com/charmbracelet/huh"
)

// errBack is used to unwind one menu level. Huh returns ErrUserAborted when the
// user hits ctrl+c or esc; we keep that as "go back" semantics because it is
// already the natural mental model in forms.
var errBack = errors.New("back")

// Run opens the top-level menu. It is the default af entrypoint and loops until
// the user picks Quit or aborts from the top level.
func Run(service *app.Service) error {
	for {
		if err := mainMenu(service); err != nil {
			if errors.Is(err, errBack) || errors.Is(err, huh.ErrUserAborted) {
				return nil
			}
			return err
		}
	}
}

// mainMenu shows the top-level category selector and dispatches to one of the
// category submenus.
func mainMenu(service *app.Service) error {
	var choice string
	err := huh.NewSelect[string]().
		Title("agentfiles").
		Description("Choose a category").
		Options(
			huh.NewOption("Profile  — manage profiles", "profile"),
			huh.NewOption("Asset    — scaffold reusable content", "asset"),
			huh.NewOption("Project  — plan and apply into a repo", "project"),
			huh.NewOption("Doctor   — health-check a profile", "doctor"),
			huh.NewOption("Quit", "quit"),
		).
		Value(&choice).
		Run()
	if err != nil {
		return err
	}
	switch choice {
	case "profile":
		return enterSubmenu(profileMenu(service))
	case "asset":
		return enterSubmenu(assetMenu(service))
	case "project":
		return enterSubmenu(projectMenu(service))
	case "doctor":
		return reportAction(RunDoctor(service))
	case "quit":
		return errBack
	}
	return nil
}

// enterSubmenu swallows errBack at submenu boundaries so the user returns to the
// main menu instead of exiting the program.
func enterSubmenu(err error) error {
	if errors.Is(err, errBack) || errors.Is(err, huh.ErrUserAborted) {
		return nil
	}
	return err
}

// reportAction renders the result of a command flow. Successful flows pause so
// their stdout output stays on screen until the user dismisses it; errors are
// printed and paused the same way; aborts simply return so the menu redraws
// without an extra prompt.
func reportAction(err error) error {
	if errors.Is(err, errBack) || errors.Is(err, huh.ErrUserAborted) {
		return nil
	}
	if err != nil {
		fmt.Printf("\nerror: %v\n", err)
	}
	pause()
	return nil
}

// profileMenu lists the profile-level actions. A Back option is the explicit
// way to return to the main menu; ctrl+c also works.
func profileMenu(service *app.Service) error {
	for {
		var action string
		err := huh.NewSelect[string]().
			Title("Profile").
			Options(
				huh.NewOption("Create   — scaffold and register a new profile", "create"),
				huh.NewOption("Register — adopt an existing profile folder", "register"),
				huh.NewOption("List     — show every registered profile", "list"),
				huh.NewOption("Back", "back"),
			).
			Value(&action).
			Run()
		if err != nil {
			return err
		}
		switch action {
		case "create":
			reportAction(RunProfileCreate(service))
		case "register":
			reportAction(RunProfileRegister(service))
		case "list":
			reportAction(RunProfileList(service))
		case "back":
			return errBack
		}
	}
}

// assetMenu currently only exposes Init, but the submenu shape is kept so
// future asset commands slot in without a UX change.
func assetMenu(service *app.Service) error {
	for {
		var action string
		err := huh.NewSelect[string]().
			Title("Asset").
			Options(
				huh.NewOption("Init — scaffold a new asset in a profile", "init"),
				huh.NewOption("Back", "back"),
			).
			Value(&action).
			Run()
		if err != nil {
			return err
		}
		switch action {
		case "init":
			reportAction(RunAssetInit(service))
		case "back":
			return errBack
		}
	}
}

// projectMenu groups the render/apply workflow behind one selector.
func projectMenu(service *app.Service) error {
	for {
		var action string
		err := huh.NewSelect[string]().
			Title("Project").
			Options(
				huh.NewOption("Add   — register a target repository", "add"),
				huh.NewOption("Plan  — preview pending changes", "plan"),
				huh.NewOption("Apply — write changes into the repository", "apply"),
				huh.NewOption("Back", "back"),
			).
			Value(&action).
			Run()
		if err != nil {
			return err
		}
		switch action {
		case "add":
			reportAction(RunProjectAdd(service))
		case "plan":
			reportAction(RunProjectPlan(service))
		case "apply":
			reportAction(RunProjectApply(service))
		case "back":
			return errBack
		}
	}
}

// pause blocks until the user acknowledges the output. It is used after
// command completion so results are not wiped by the next form render.
func pause() {
	var ack bool
	_ = huh.NewConfirm().
		Title("Continue").
		Affirmative("Ok").
		Negative("").
		Value(&ack).
		Run()
}
