package knowledge

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestScopedKnowledgeFilestorePath(t *testing.T) {
	tests := []struct {
		name       string
		sourcePath string
		want       string
		wantError  bool
	}{
		{name: "simple path", sourcePath: "documents/manuals", want: "dev/apps/app_1/documents/manuals"},
		{name: "logical root path", sourcePath: "/documents/manuals", want: "dev/apps/app_1/documents/manuals"},
		{name: "app-prefixed path", sourcePath: "apps/app_1/documents/manuals", want: "dev/apps/app_1/documents/manuals"},
		{name: "app root", sourcePath: "apps/app_1", want: "dev/apps/app_1"},
		{name: "simple traversal", sourcePath: "../../apps/app_2/secret", wantError: true},
		{name: "app-prefixed traversal", sourcePath: "apps/app_1/../../app_2/secret", wantError: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := scopedKnowledgeFilestorePath("dev", "app_1", test.sourcePath)
			if test.wantError {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, test.want, filepath.ToSlash(got))
		})
	}
}
