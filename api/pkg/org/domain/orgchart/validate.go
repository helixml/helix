package orgchart

import (
	"fmt"
	"strings"
)

// ValidID rejects externally-supplied entity ids that are unsafe to use
// as a filesystem path segment. Worker ids land in
// os.MkdirAll(<envsDir>/<id>) at hire time (and the matching teardown
// removes that directory), so an id like "../../etc" would otherwise
// escape the envs directory — a path-traversal. Ids are short readable
// handles (`w-mark`, `r-engineer`, `w-<uuid>`); none legitimately
// contain a path separator, a NUL, or a ".." segment, so rejecting
// those is safe for every real id while closing the traversal.
func ValidID(id string) error {
	if id == "" {
		return fmt.Errorf("id is empty")
	}
	if strings.ContainsAny(id, "/\\\x00") {
		return fmt.Errorf("id %q must not contain a path separator or NUL byte", id)
	}
	if strings.Contains(id, "..") {
		return fmt.Errorf("id %q must not contain %q", id, "..")
	}
	return nil
}
