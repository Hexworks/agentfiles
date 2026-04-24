// Package doctor produces a read-only health summary of a profile by planning
// each of its projects and reporting pending changes or drift. It never writes
// to disk; all mutation happens through internal/sync.
package doctor

import (
	"fmt"
	"strings"

	"github.com/addamsson/agentfiles/internal/profile"
	llmsync "github.com/addamsson/agentfiles/internal/sync"
)

// CheckProfile runs a sync plan for every project owned by the profile and
// returns a human-readable summary. Projects with no pending changes are
// reported as "clean".
// FIX: return error object instaed of string @see task#0005
func CheckProfile(p *profile.Profile) (string, error) {
	var out strings.Builder
	fmt.Fprintf(&out, "Profile: %s\n", p.Manifest.Name)
	for _, proj := range p.ProjectList() {
		preview, err := llmsync.Plan(p, proj)
		if err != nil {
			return "", err
		}
		fmt.Fprintf(&out, "\n[%s]\n", proj.Name)
		if len(preview.Changes) == 0 {
			out.WriteString("clean\n")
			continue
		}
		for _, change := range preview.Changes {
			fmt.Fprintf(&out, "%s %s\n", change.Kind, change.Path)
		}
	}
	return out.String(), nil
}
