package tui

import (
	"strings"
	"testing"
)

func TestRenderDiff(t *testing.T) {
	diff := `--- a/src/auth.go
+++ b/src/auth.go
@@ -10,4 +10,6 @@
 func validateEmail(email string) bool {
-    return regexp.MustCompile("^[a-z]+@").MatchString(email)
+    if len(email) > 254 {
+        return false
+    }
+    return strings.Contains(email, "@")
 }
`

	output := RenderDiff("src/auth.go", diff, 60)

	if !strings.Contains(output, "src/auth.go") {
		t.Error("expected filename in output")
	}
	if !strings.Contains(output, "┌") {
		t.Error("expected top border")
	}
	if !strings.Contains(output, "┘") {
		t.Error("expected bottom border")
	}
}

func TestRenderInlineDiff(t *testing.T) {
	output := RenderInlineDiff("test.go", "old line", "new line", 60)

	if !strings.Contains(output, "test.go") {
		t.Error("expected filename")
	}
	if !strings.Contains(output, "- old line") {
		t.Error("expected old line with - prefix")
	}
	if !strings.Contains(output, "+ new line") {
		t.Error("expected new line with + prefix")
	}
}

func TestParseToolCallDiff(t *testing.T) {
	args := map[string]interface{}{
		"file_path":  "src/main.go",
		"old_string": "foo",
		"new_string": "bar",
	}

	filename, old, new := ParseToolCallDiff("str_replace_editor", args)
	if filename != "src/main.go" {
		t.Errorf("expected src/main.go, got %s", filename)
	}
	if old != "foo" {
		t.Errorf("expected foo, got %s", old)
	}
	if new != "bar" {
		t.Errorf("expected bar, got %s", new)
	}
}
