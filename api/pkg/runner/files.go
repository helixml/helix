package runner

import (
	"archive/tar"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	urllib "net/url"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/lukemarsden/helix/api/pkg/filestore"
	"github.com/lukemarsden/helix/api/pkg/model"
	"github.com/lukemarsden/helix/api/pkg/server"
	"github.com/lukemarsden/helix/api/pkg/system"
	"github.com/lukemarsden/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

type FileHandler struct {
	runnerID          string
	httpClientOptions server.ClientOptions
}

func NewFileHandler(
	runnerID string,
	clientOptions server.ClientOptions,
) *FileHandler {
	return &FileHandler{
		runnerID:          runnerID,
		httpClientOptions: clientOptions,
	}
}

func (handler *FileHandler) uploadWorkerResponse(res *types.RunnerTaskResponse) (*types.RunnerTaskResponse, error) {
	log.Info().
		Msgf("🟢 upload worker response: %+v", res)

	if len(res.Files) > 0 {
		uploadedFiles, err := handler.uploadFiles(res.SessionID, res.Files, "results")
		if err != nil {
			return nil, err
		}
		res.Files = uploadedFiles
	}

	if res.LoraDir != "" {
		uploadedLoraDir, err := handler.uploadFolder(res.SessionID, res.LoraDir, "lora")
		if err != nil {
			return nil, err
		}
		res.LoraDir = uploadedLoraDir
	}
	return res, nil
}

// download the lora dir and the most recent user interaction files for a session
func (handler *FileHandler) downloadSession(session *types.Session, isInitialSession bool) (*types.Session, error) {
	var err error
	if isInitialSession {
		session, err = handler.downloadLoraDir(session)
		if err != nil {
			return nil, err
		}
	}

	session, err = handler.downloadUserInteractionFiles(session)
	if err != nil {
		return nil, err
	}
	return session, nil
}

func (handler *FileHandler) downloadLoraDir(session *types.Session) (*types.Session, error) {
	if session.LoraDir == "" {
		return session, nil
	}
	downloadedPath, err := handler.downloadFolder(session.ID, "lora_dir", session.LoraDir)
	if err != nil {
		return nil, err
	}
	session.LoraDir = downloadedPath
	return session, nil
}

// get the most recent user interaction - download all the files
// and return the session with the filepaths remapped
func (handler *FileHandler) downloadUserInteractionFiles(session *types.Session) (*types.Session, error) {
	interaction, err := model.GetUserInteraction(session)
	if err != nil {
		return nil, err
	}

	if interaction == nil {
		return nil, fmt.Errorf("no model interaction")
	}

	remappedFilepaths := []string{}

	for _, filepath := range interaction.Files {
		downloadedPath, err := handler.downloadFile(session.ID, interaction.ID, filepath)
		if err != nil {
			return nil, err
		}

		remappedFilepaths = append(remappedFilepaths, downloadedPath)
	}

	interaction.Files = remappedFilepaths

	newInteractions := []types.Interaction{}

	for _, existingInteraction := range session.Interactions {
		if existingInteraction.ID == interaction.ID {
			newInteractions = append(newInteractions, *interaction)
		} else {
			newInteractions = append(newInteractions, existingInteraction)
		}
	}

	session.Interactions = newInteractions

	return session, nil
}

func (handler *FileHandler) downloadFile(sessionID string, localFolder string, filepath string) (string, error) {
	downloadFolder := path.Join(os.TempDir(), "helix", "downloads", sessionID, localFolder)
	if err := os.MkdirAll(downloadFolder, os.ModePerm); err != nil {
		return "", fmt.Errorf("failed to create folder: %w", err)
	}
	filename := path.Base(filepath)
	finalPath := path.Join(downloadFolder, filename)

	if _, err := os.Stat(finalPath); err == nil {
		return finalPath, nil
	}

	url := server.URL(handler.httpClientOptions, fmt.Sprintf("/runner/%s/session/%s/download/file", handler.runnerID, sessionID))
	urlValues := urllib.Values{}
	urlValues.Add("path", filepath)

	fullURL := fmt.Sprintf("%s?%s", url, urlValues.Encode())

	log.Debug().
		Msgf("🔵 runner downloading interaction file: %s", fullURL)

	req, err := http.NewRequest("GET", fullURL, nil)
	if err != nil {
		return "", err
	}
	server.AddHeadersVanilla(req, handler.httpClientOptions.Token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status code for file download: %d %s", resp.StatusCode, fullURL)
	}

	file, err := os.Create(finalPath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	_, err = io.Copy(file, resp.Body)
	if err != nil {
		return "", err
	}

	log.Debug().
		Msgf("🔵 runner downloaded interaction file: %s -> %s", fullURL, finalPath)

	return finalPath, nil
}

func (handler *FileHandler) downloadFolder(sessionID string, localFolder string, filepath string) (string, error) {
	downloadFolder := path.Join(os.TempDir(), "helix", "downloads", sessionID, localFolder)

	// if the folder already exists, then assume we have already downloaded everything
	if _, err := os.Stat(downloadFolder); err == nil {
		return downloadFolder, nil
	}

	if err := os.MkdirAll(downloadFolder, os.ModePerm); err != nil {
		return "", fmt.Errorf("failed to create folder: %w", err)
	}
	url := server.URL(handler.httpClientOptions, fmt.Sprintf("/runner/%s/session/%s/download/folder", handler.runnerID, sessionID))
	urlValues := urllib.Values{}
	urlValues.Add("path", filepath)
	fullURL := fmt.Sprintf("%s?%s", url, urlValues.Encode())

	log.Debug().
		Msgf("🔵 runner downloading folder: %s %s", sessionID, filepath)

	req, err := http.NewRequest("GET", fullURL, nil)
	if err != nil {
		return "", err
	}
	server.AddHeadersVanilla(req, handler.httpClientOptions.Token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status code for file download: %d %s", resp.StatusCode, fullURL)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var buffer bytes.Buffer
	_, err = buffer.Write(body)
	if err != nil {
		return "", err
	}

	log.Debug().Msgf("🟠 runner expanding tar buffer folder: %s %s", sessionID, downloadFolder)

	err = system.ExpandTarBuffer(&buffer, downloadFolder)
	if err != nil {
		return "", err
	}

	log.Debug().Msgf("🟠 runner downloaded folder: %s %s", sessionID, downloadFolder)

	return downloadFolder, nil
}

func (handler *FileHandler) uploadFiles(sessionID string, localFiles []string, remoteFolder string) ([]string, error) {
	// create a new multipart form
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	log.Debug().Msgf("🟠 Uploading task files %s %+v", sessionID, localFiles)

	// loop over each file and add it to the form
	for _, filepath := range localFiles {
		file, err := os.Open(filepath)
		if err != nil {
			return nil, err
		}
		defer file.Close()

		// create a new form field for the file
		part, err := writer.CreateFormFile("files", filepath)
		if err != nil {
			return nil, err
		}

		// copy the file contents into the form field
		_, err = io.Copy(part, file)
		if err != nil {
			return nil, err
		}
	}

	// close the multipart form
	err := writer.Close()
	if err != nil {
		return nil, err
	}

	url := server.URL(handler.httpClientOptions, fmt.Sprintf("/runner/%s/session/%s/upload/files", handler.runnerID, sessionID))
	urlValues := urllib.Values{}
	urlValues.Add("path", remoteFolder)
	fullURL := fmt.Sprintf("%s?%s", url, urlValues.Encode())

	log.Debug().Msgf("🟠 upload files %s", fullURL)

	// create a new POST request with the multipart form as the body
	req, err := http.NewRequest("POST", fullURL, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	server.AddHeadersVanilla(req, handler.httpClientOptions.Token)

	// send the request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// handle the response
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var data []filestore.FileStoreItem
	resultBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	// parse body as json into result
	err = json.Unmarshal(resultBody, &data)
	if err != nil {
		return nil, err
	}

	mappedFiles := []string{}

	for _, fileItem := range data {
		mappedFiles = append(mappedFiles, fileItem.Path)
	}

	return mappedFiles, nil
}

func (handler *FileHandler) uploadFolder(sessionID string, localPath string, remoteFolder string) (string, error) {
	// create a new multipart form
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	log.Debug().Msgf("🟠 Uploading task folder %s %+v", sessionID, localPath)

	fileInfo, err := os.Stat(localPath)
	if err != nil {
		return "", err
	}

	if !fileInfo.IsDir() {
		return "", fmt.Errorf("not a directory: %s", localPath)
	}

	// Create a .tar file from the directory
	tarFilePath, err := createTar(localPath)
	if err != nil {
		return "", err
	}

	file, err := os.Open(tarFilePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	// create a new form field for the file
	part, err := writer.CreateFormFile("files", tarFilePath)
	if err != nil {
		return "", err
	}

	// copy the file contents into the form field
	_, err = io.Copy(part, file)
	if err != nil {
		return "", err
	}

	// close the multipart form
	err = writer.Close()
	if err != nil {
		return "", err
	}

	url := server.URL(handler.httpClientOptions, fmt.Sprintf("/runner/%s/session/%s/upload/folder", handler.runnerID, sessionID))
	urlValues := urllib.Values{}
	urlValues.Add("path", remoteFolder)
	fullURL := fmt.Sprintf("%s?%s", url, urlValues.Encode())

	log.Debug().Msgf("🟠 upload files %s", fullURL)

	// create a new POST request with the multipart form as the body
	req, err := http.NewRequest("POST", fullURL, body)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	server.AddHeadersVanilla(req, handler.httpClientOptions.Token)

	// send the request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	// handle the response
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var data filestore.FileStoreItem
	resultBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	// parse body as json into result
	err = json.Unmarshal(resultBody, &data)
	if err != nil {
		return "", err
	}

	return data.Path, nil
}

// createTar takes a directory path and creates a .tar file from it.
// It returns the path of the created .tar file and any error encountered.
func createTar(dirPath string) (string, error) {
	// Define the .tar file name (it will be in the same directory as the input folder)
	tarFilePath := dirPath + ".tar"

	// Create the .tar file
	tarFile, err := os.Create(tarFilePath)
	if err != nil {
		return "", err
	}
	defer tarFile.Close()

	// Create a new tar writer
	tw := tar.NewWriter(tarFile)
	defer tw.Close()

	// Walk through every file in the folder
	err = filepath.Walk(dirPath, func(file string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Create a header for the current file
		header, err := tar.FileInfoHeader(fi, fi.Name())
		if err != nil {
			return err
		}

		// Ensure the header name is correct
		// This is to ensure that the path in the tar file is relative and not absolute.
		header.Name = strings.TrimPrefix(strings.Replace(file, dirPath, "", -1), string(filepath.Separator))

		// Write the header to the tarball archive
		if err := tw.WriteHeader(header); err != nil {
			return err
		}

		// If it's not a directory, write its content
		if !fi.IsDir() {
			data, err := os.Open(file)
			if err != nil {
				return err
			}
			defer data.Close()

			if _, err := io.Copy(tw, data); err != nil {
				return err
			}
		}
		return nil
	})

	if err != nil {
		return "", err
	}

	return tarFilePath, nil
}
