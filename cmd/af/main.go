// Command af is the agentfiles CLI. It parses the --registry flag and drops
// the user into the TUI defined in internal/tui, which is the only interface
// agentfiles exposes.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/hexworks/agentfiles/internal/app"
	"github.com/hexworks/agentfiles/internal/registry"
	"github.com/hexworks/agentfiles/internal/tui"
)

func main() {
	registryPath := flag.String("registry", registry.DefaultPath(), "path to profile registry")
	flag.Parse()
	if err := tui.Run(app.New(*registryPath)); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
