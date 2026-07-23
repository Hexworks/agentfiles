package git

import "strings"

// Covers reports whether pathspec contains an entry that matches path.
// Paths are compared as forward-slash strings; a literal entry must
// match exactly, while a `foo/bar/**` wildcard matches any descendant of
// `foo/bar`. Used by Repo.Commit to reject unrelated staged changes.
func Covers(pathspec []string, path string) bool {
	path = strings.TrimPrefix(path, "./")
	for _, spec := range pathspec {
		spec = strings.TrimPrefix(spec, "./")
		if strings.HasSuffix(spec, "/**") {
			prefix := strings.TrimSuffix(spec, "/**")
			if path == prefix || strings.HasPrefix(path, prefix+"/") {
				return true
			}
			continue
		}
		if spec == path {
			return true
		}
	}
	return false
}
