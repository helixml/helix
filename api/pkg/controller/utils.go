package controller

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	stdlog "log"
	"mime/multipart"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/hashicorp/go-retryablehttp"
	"github.com/lukemarsden/helix/api/pkg/filestore"
	"github.com/lukemarsden/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

func containsString(slice []string, target string) bool {
	for _, value := range slice {
		if value == target {
			return true
		}
	}
	return false
}

func isOlderThan24Hours(t time.Time) bool {
	compareTime := time.Now().Add(-24 * time.Hour)
	return t.Before(compareTime)
}

func dumpObject(data interface{}) {
	bytes, _ := json.MarshalIndent(data, "", "    ")
	fmt.Printf("%s\n", string(bytes))
}

func createMultipartRequest(uri string, fieldName string, fileName string, fileReader io.Reader) (*retryablehttp.Request, error) {
	// Create a buffer to write our multipart form to
	var requestBody bytes.Buffer

	// Create a multipart writer
	multipartWriter := multipart.NewWriter(&requestBody)

	// Add the file part to the multipart writer
	fileWriter, err := multipartWriter.CreateFormFile(fieldName, fileName)
	if err != nil {
		return nil, err
	}

	// Copy the file data to the multipart writer
	_, err = io.Copy(fileWriter, fileReader)
	if err != nil {
		return nil, err
	}

	// Close the multipart writer to set the terminating boundary
	err = multipartWriter.Close()
	if err != nil {
		return nil, err
	}

	// Create the request
	request, err := retryablehttp.NewRequest("POST", uri, &requestBody)
	if err != nil {
		return nil, err
	}

	// Set the content type, this must be done after closing the writer
	request.Header.Set("Content-Type", multipartWriter.FormDataContentType())

	return request, nil
}

func newRetryClient() *retryablehttp.Client {
	retryClient := retryablehttp.NewClient()
	retryClient.RetryMax = 10
	retryClient.Logger = stdlog.New(io.Discard, "", stdlog.LstdFlags)
	retryClient.RequestLogHook = func(_ retryablehttp.Logger, req *http.Request, attempt int) {
		switch {
		case req.Method == "POST":
			log.Debug().
				Str(req.Method, req.URL.String()).
				Int("attempt", attempt).
				Msgf("")
		default:
			// GET, PUT, DELETE, etc.
			log.Trace().
				Str(req.Method, req.URL.String()).
				Int("attempt", attempt).
				Msgf("")
		}
	}
	return retryClient
}

func injectFileToList(fileList []string, existingFile string, addFile string) []string {
	ret := []string{}
	for _, file := range fileList {
		ret = append(ret, file)
		if file == existingFile {
			ret = append(ret, addFile)
		}
	}
	return ret
}

func copyFileList(fileList []string) []string {
	ret := []string{}
	for _, file := range fileList {
		ret = append(ret, file)
	}
	return ret
}

func getProcessedQAChunkKey(filename string) string {
	return fmt.Sprintf("%s%s", types.TEXT_DATA_PREP_FILES_CONVERTED_PREFIX, path.Base(filename))
}

func hasProcessedQAChunk(
	interaction *types.Interaction,
	filename string,
	chunkIndex int,
) bool {
	indexes, ok := interaction.Metadata[getProcessedQAChunkKey(filename)]
	if !ok {
		return false
	}
	parts := strings.Split(indexes, ",")
	for _, part := range parts {
		if part == fmt.Sprintf("%d", chunkIndex) {
			return true
		}
	}
	return false
}

func updateProcessedQAChunk(
	interaction *types.Interaction,
	filename string,
	chunkIndex int,
) {
	if hasProcessedQAChunk(interaction, filename, chunkIndex) {
		return
	}
	indexes, ok := interaction.Metadata[getProcessedQAChunkKey(filename)]
	if !ok {
		indexes = ""
	}
	parts := strings.Split(indexes, ",")
	parts = append(parts, fmt.Sprintf("%d", chunkIndex))
	interaction.Metadata[getProcessedQAChunkKey(filename)] = strings.Join(parts, ",")
}

func getFileContent(
	ctx context.Context,
	fs filestore.FileStore,
	path string,
) (string, error) {
	// load the actual file contents
	reader, err := fs.Download(ctx, path)
	if err != nil {
		return "", err
	}
	buf := new(bytes.Buffer)
	_, err = io.Copy(buf, reader)
	if err != nil {
		return "", err
	}
	return buf.String(), nil
}

// for text based fine tuning - once we've converted text into questions
// we need to append to the jsonl file with the new questions
// this is NOT atomic and should be run in some kind of mutex
// to prevent a race between writers loosing data
func appendQuestionsToFile(
	ctx context.Context,
	fs filestore.FileStore,
	path string,
	questions []types.DataPrepTextQuestion,
) error {
	jsonLines := []string{}
	for _, question := range questions {
		jsonLine, err := json.Marshal(question)
		if err != nil {
			return err
		}
		jsonLines = append(jsonLines, string(jsonLine))
	}
	existingContent, err := getFileContent(ctx, fs, path)
	if err != nil {
		return err
	}
	newContent := fmt.Sprintf("%s\n%s", existingContent, strings.Join(jsonLines, "\n"))
	_, err = fs.Upload(ctx, path, strings.NewReader(newContent))
	if err != nil {
		return err
	}
	return nil
}
