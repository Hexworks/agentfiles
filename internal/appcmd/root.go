package appcmd

import (
	"fmt"
	"os"

	"github.com/addamsson/agentfiles/internal/app"
	"github.com/addamsson/agentfiles/internal/registry"
	"github.com/addamsson/agentfiles/internal/tui"
	"github.com/spf13/cobra"
)

var registryPath string

// Execute builds the Cobra command tree and runs it. Every command is a thin
// wrapper that drops the user into the matching TUI flow; the TUI itself is
// the source of all interactive prompts.
func Execute() {
	root := &cobra.Command{
		Use:           "af",
		Short:         "Profile-based LLM workspace manager",
		Long:          "agentfiles uses a TUI for every interaction. Run af with no arguments to open the main menu, or pass a subcommand path to jump straight to that flow.",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(_ *cobra.Command, _ []string) error {
			return tui.Run(service())
		},
	}
	root.PersistentFlags().StringVar(&registryPath, "registry", registry.DefaultPath(), "path to profile registry")
	root.AddCommand(profileCmd(), assetCmd(), projectCmd(), doctorCmd())
	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// service constructs an application service for one command invocation. The
// service is cheap to build because heavy work happens lazily.
func service() *app.Service {
	return app.New(registryPath)
}

// profileCmd routes profile subcommands into the matching TUI form.
func profileCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "profile",
		Short: "Profile lifecycle and discovery",
	}
	cmd.AddCommand(
		&cobra.Command{
			Use:   "create",
			Short: "Create and register a profile",
			RunE:  func(_ *cobra.Command, _ []string) error { return tui.RunProfileCreate(service()) },
		},
		&cobra.Command{
			Use:   "register",
			Short: "Register an existing profile folder",
			RunE:  func(_ *cobra.Command, _ []string) error { return tui.RunProfileRegister(service()) },
		},
		&cobra.Command{
			Use:   "list",
			Short: "List profiles",
			RunE:  func(_ *cobra.Command, _ []string) error { return tui.RunProfileList(service()) },
		},
	)
	return cmd
}

// assetCmd routes asset subcommands into the matching TUI form.
func assetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "asset",
		Short: "Scaffold reusable profile assets",
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "init",
		Short: "Scaffold an asset in a profile",
		RunE:  func(_ *cobra.Command, _ []string) error { return tui.RunAssetInit(service()) },
	})
	return cmd
}

// projectCmd routes the render/apply workflow into the matching TUI forms.
func projectCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "project",
		Short: "Project manifest and sync workflow",
	}
	cmd.AddCommand(
		&cobra.Command{
			Use:   "add",
			Short: "Add a project manifest to a profile",
			RunE:  func(_ *cobra.Command, _ []string) error { return tui.RunProjectAdd(service()) },
		},
		&cobra.Command{
			Use:   "plan",
			Short: "Preview generated changes for a project",
			RunE:  func(_ *cobra.Command, _ []string) error { return tui.RunProjectPlan(service()) },
		},
		&cobra.Command{
			Use:   "apply",
			Short: "Render and write agent files into the target project",
			RunE:  func(_ *cobra.Command, _ []string) error { return tui.RunProjectApply(service()) },
		},
	)
	return cmd
}

// doctorCmd routes the read-only health check into its TUI form.
func doctorCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Check managed projects for drift and unmanaged files",
		RunE:  func(_ *cobra.Command, _ []string) error { return tui.RunDoctor(service()) },
	}
}
