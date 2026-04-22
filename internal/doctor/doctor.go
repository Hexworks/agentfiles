package doctor

import (
	"fmt"
	"strings"

	"github.com/addamsson/agentfiles/internal/profile"
	llmsync "github.com/addamsson/agentfiles/internal/sync"
)

func CheckProfile(p *profile.Loaded) (string, error) {
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
