package model

import (
	"fmt"
	"path"

	"github.com/helixml/helix/api/pkg/data"
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

// download all files for the latest user interaction
func downloadUserInteractionFiles(
	session *types.Session,
	fileManager ModelSessionFileManager,
) (*types.Session, error) {
	interaction, err := data.GetUserInteraction(session.Interactions)
	if err != nil {
		return nil, err
	}
	if interaction == nil {
		return nil, fmt.Errorf("no model interaction")
	}

	remappedFilepaths := []string{}

	for _, filepath := range interaction.Files {
		localFilePath := path.Join(fileManager.GetFolder(), path.Base(filepath))
		err = fileManager.DownloadFile(filepath, localFilePath)
		if err != nil {
			return nil, fmt.Errorf("error downloading file '%s' to '%s': %s", filepath, localFilePath, err.Error())
		}
		remappedFilepaths = append(remappedFilepaths, localFilePath)
	}

	interaction.Files = remappedFilepaths
	newInteractions := []*types.Interaction{}
	for _, existingInteraction := range session.Interactions {
		if existingInteraction.ID == interaction.ID {
			newInteractions = append(newInteractions, interaction)
		} else {
			newInteractions = append(newInteractions, existingInteraction)
		}
	}
	session.Interactions = newInteractions
	return session, nil
}
