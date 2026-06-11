// Command af is the agentfiles CLI. It parses the --registry flag and drops
// the user into the alt-screen Bubble Tea shell defined in
// internal/tui/shell, which is the only interface agentfiles exposes.
package main

import (
	"flag"
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"

	"github.com/hexworks/agentfiles/internal/actions"
	"github.com/hexworks/agentfiles/internal/app"
	"github.com/hexworks/agentfiles/internal/registry"
	"github.com/hexworks/agentfiles/internal/tui/notifications"
	"github.com/hexworks/agentfiles/internal/tui/shell"
)

func main() {
	registryPath := flag.String("registry", registry.DefaultPath(), "path to profile registry")
	flag.Parse()

	svc := app.New(*registryPath)
	a := actions.New(svc)
	log := notifications.NewLog()

	if _, err := tea.NewProgram(shell.New(a, log)).Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
