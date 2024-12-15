package model

import (
	"path"

	"github.com/helixml/helix/api/pkg/types"
)

// a generic lora dir downloader for a session
func downloadLoraDir(
	session *types.Session,
	fileManager ModelSessionFileManager,
) (*types.Session, error) {
	if session.LoraDir == "" {
		return session, nil
	}
	localFolderPath := path.Join(fileManager.GetFolder(), "lora_dir")
	err := fileManager.DownloadFolder(session.LoraDir, localFolderPath)
	if err != nil {
		return nil, err
	}
	session.LoraDir = localFolderPath
	return session, nil
}
