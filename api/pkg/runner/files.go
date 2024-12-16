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

	"github.com/dustin/go-humanize"
	"github.com/helixml/helix/api/pkg/filestore"
	"github.com/helixml/helix/api/pkg/model"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

type SessionFileHandler struct {
	folder         string
	sessionID      string
	downloadFile   func(sessionID string, remotePath string, localPath string) error
	downloadFolder func(sessionID string, remotePath string, localPath string) error
}

func (handler *SessionFileHandler) GetFolder() string {
	return handler.folder
}

func (handler *SessionFileHandler) DownloadFile(remotePath string, localPath string) error {
	return handler.downloadFile(handler.sessionID, remotePath, localPath)
}

func (handler *SessionFileHandler) DownloadFolder(remotePath string, localPath string) error {
	return handler.downloadFolder(handler.sessionID, remotePath, localPath)
}

// Compile-time interface check:
var _ model.ModelSessionFileManager = (*SessionFileHandler)(nil)

type FileHandler struct {
	runnerID          string
	httpClientOptions system.ClientOptions
	eventHandler      func(res *types.RunnerTaskResponse)
}

func NewFileHandler(
	runnerID string,
	clientOptions system.ClientOptions,
	eventHandler func(res *types.RunnerTaskResponse),
) *FileHandler {
	return &FileHandler{
		runnerID:          runnerID,
		httpClientOptions: clientOptions,
		eventHandler:      eventHandler,
	}
}

func (handler *FileHandler) uploadWorkerResponse(res *types.RunnerTaskResponse) (*types.RunnerTaskResponse, error) {
	log.Info().
		Msgf("🟢 upload worker response: %+v", res)

	if len(res.Files) > 0 {
		uploadedFiles, err := handler.uploadFiles(res.SessionID, res.Files, types.FILESTORE_RESULTS_DIR)
		if err != nil {
			return nil, err
		}
		res.Files = uploadedFiles
	}

	if res.LoraDir != "" {
		// we add the interaction ID into the Lora path so we can keep mutiple Loras for one session
		// this means that we can "re-train" (i.e. add more files and produce a new lora)
		// by keeping each actual lora dir at one level lower inside the interaction
		// we keep a history of re-trainings and can always go back to a previous step
		// (because the previous lora dir is still there)
		// the api server will "hoist" this folder to the session.LoraDir which is the "live" LoraDir
		uploadedLoraDir, err := handler.uploadFolder(res.SessionID, res.LoraDir, path.Join(types.FILESTORE_LORA_DIR, res.InteractionID))
		if err != nil {
			return nil, err
		}
		res.LoraDir = uploadedLoraDir
	}

	log.Info().
		Msgf("🟢 worker response uploaded: %+v", res)

	return res, nil
}

func (handler *FileHandler) downloadFile(sessionID string, remotePath string, localPath string) error {
	if err := os.MkdirAll(path.Dir(localPath), os.ModePerm); err != nil {
		return fmt.Errorf("failed to create folder: %w", err)
	}
	if _, err := os.Stat(localPath); err == nil {
		return nil
	}

	url := system.URL(handler.httpClientOptions, system.GetApiPath(fmt.Sprintf("/runner/%s/session/%s/download/file", handler.runnerID, sessionID)))
	urlValues := urllib.Values{}
	urlValues.Add("path", remotePath)

	fullURL := fmt.Sprintf("%s?%s", url, urlValues.Encode())

	log.Debug().
		Msgf("🔵 runner downloading interaction file: %s", fullURL)

	req, err := http.NewRequest("GET", fullURL, nil)
	if err != nil {
		return err
	}
	if err := system.AddAutheaders(req, handler.httpClientOptions.Token); err != nil {
		return err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code for file download: %d %s", resp.StatusCode, fullURL)
	}

	file, err := os.Create(localPath)
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = io.Copy(file, resp.Body)
	if err != nil {
		return err
	}

	log.Debug().
		Msgf("🔵 runner downloaded interaction file: %s -> %s", remotePath, localPath)

	return nil
}

func (handler *FileHandler) downloadFolder(sessionID string, remotePath string, localPath string) error {
	// if the folder already exists, then assume we have already downloaded everything
	if _, err := os.Stat(localPath); err == nil {
		log.Debug().Msgf("🟠 runner already downloaded folder: %s %s", sessionID, localPath)
		return nil
	}

	if err := os.MkdirAll(localPath, os.ModePerm); err != nil {
		return fmt.Errorf("failed to create folder: %w", err)
	}
	url := system.URL(handler.httpClientOptions, system.GetApiPath(fmt.Sprintf("/runner/%s/session/%s/download/folder", handler.runnerID, sessionID)))
	urlValues := urllib.Values{}
	urlValues.Add("path", remotePath)
	fullURL := fmt.Sprintf("%s?%s", url, urlValues.Encode())

	log.Debug().
		Msgf("🔵 runner downloading folder: %s %s", sessionID, remotePath)

	req, err := http.NewRequest("GET", fullURL, nil)
	if err != nil {
		return err
	}
	if err := system.AddAutheaders(req, handler.httpClientOptions.Token); err != nil {
		return err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code for file download: %d %s", resp.StatusCode, fullURL)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	var buffer bytes.Buffer
	_, err = buffer.Write(body)
	if err != nil {
		return err
	}

	log.Debug().Msgf("🟠 runner expanding tar buffer folder: %s %s", sessionID, localPath)

	err = system.ExpandTarBuffer(&buffer, localPath)
	if err != nil {
		return err
	}

	log.Debug().Msgf("🟠 runner downloaded folder: %s %s", sessionID, localPath)

	return nil
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

	url := system.URL(handler.httpClientOptions, system.GetApiPath(fmt.Sprintf("/runner/%s/session/%s/upload/files", handler.runnerID, sessionID)))
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
	if err := system.AddAutheaders(req, handler.httpClientOptions.Token); err != nil {
		return nil, err
	}

	// send the request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// handle the response
	if resp.StatusCode != http.StatusOK {
		bts, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
		}

		return nil, fmt.Errorf("unexpected status code: %d (%s)", resp.StatusCode, string(bts))
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

type ProgressReader struct {
	reader         io.Reader
	totalSize      uint64
	bytesRead      uint64
	percent        int
	emitPercentage func(int, uint64)
}

func (pr *ProgressReader) Read(p []byte) (int, error) {
	n, err := pr.reader.Read(p)
	pr.bytesRead += uint64(n)
	percent := int(float64(pr.bytesRead) / float64(pr.totalSize) * 100)
	if percent != pr.percent {
		pr.percent = percent
		pr.emitPercentage(percent, pr.bytesRead)
	}
	return n, err
}

func (handler *FileHandler) uploadFolder(sessionID string, localPath string, remoteFolder string) (string, error) {
	log.Debug().Msgf("🟠 Uploading task folder %s %+v", sessionID, localPath)

	dirInfo, err := os.Stat(localPath)
	if err != nil {
		return "", err
	}

	if !dirInfo.IsDir() {
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

	// Get the file size for progress calculation
	fileInfo, err := file.Stat()
	if err != nil {
		return "", err
	}

	handler.eventHandler(&types.RunnerTaskResponse{
		Type:      types.WorkerTaskResponseTypeProgress,
		SessionID: sessionID,
		Progress:  1,
		Status:    "uploading fine tuned files...",
	})

	totalSize := uint64(fileInfo.Size())

	progressReader := &ProgressReader{
		reader:    file,
		totalSize: totalSize,
		bytesRead: 0,
		percent:   0,
		emitPercentage: func(percent int, bytesRead uint64) {
			handler.eventHandler(&types.RunnerTaskResponse{
				Type:      types.WorkerTaskResponseTypeProgress,
				SessionID: sessionID,
				Progress:  percent,
				Status:    fmt.Sprintf("uploaded %s of %s", humanize.Bytes(bytesRead), humanize.Bytes(totalSize)),
			})
		},
	}

	url := system.URL(handler.httpClientOptions, system.GetApiPath(fmt.Sprintf("/runner/%s/session/%s/upload/folder", handler.runnerID, sessionID)))
	urlValues := urllib.Values{}
	urlValues.Add("path", remoteFolder)
	fullURL := fmt.Sprintf("%s?%s", url, urlValues.Encode())

	log.Debug().Msgf("🟠 upload task folder %s", fullURL)

	req, err := http.NewRequest("POST", fullURL, progressReader)
	if err != nil {
		return "", err
	}
	if err := system.AddAutheaders(req, handler.httpClientOptions.Token); err != nil {
		return "", err
	}

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
