package knowledge

import (
	pathpkg "path"
	"strings"

	"github.com/helixml/helix/api/pkg/filestore"
)

func scopedKnowledgeFilestorePath(globalPrefix, appID, sourcePath string) (string, error) {
	appPrefix := filestore.GetAppPrefix(globalPrefix, appID)
	normalizedSourcePath := strings.ReplaceAll(sourcePath, `\`, "/")
	logicalAppPrefix := pathpkg.Join("apps", appID)

	relativePath := normalizedSourcePath
	if normalizedSourcePath == logicalAppPrefix {
		relativePath = ""
	} else if strings.HasPrefix(normalizedSourcePath, logicalAppPrefix+"/") {
		relativePath = strings.TrimPrefix(normalizedSourcePath, logicalAppPrefix+"/")
	}

	return filestore.JoinScopedPath(appPrefix, relativePath)
}
