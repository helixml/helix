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
	"github.com/helixml/helix/api/pkg/filestore"
	"github.com/helixml/helix/api/pkg/types"
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

func getQAChunk(
	interaction *types.Interaction,
	filename string,
	chunkIndex int,
	promptName string,
) *types.DataPrepChunk {
	chunks, ok := interaction.DataPrepChunks[path.Base(filename)]
	if !ok {
		return nil
	}
	for _, chunk := range chunks {
		if chunk.Index == chunkIndex && chunk.PromptName == promptName {
			return &chunk
		}
	}
	return nil
}

func hasProcessedQAChunk(
	interaction *types.Interaction,
	filename string,
	chunkIndex int,
	promptName string,
) bool {
	chunk := getQAChunk(interaction, path.Base(filename), chunkIndex, promptName)
	if chunk == nil {
		return false
	}
	return chunk.Error == ""
}

func updateProcessedQAChunk(
	interaction *types.Interaction,
	filename string,
	chunkIndex int,
	promptName string,
	questionCount int,
	err error,
) *types.Interaction {
	useFilename := path.Base(filename)
	if hasProcessedQAChunk(interaction, useFilename, chunkIndex, promptName) {
		return interaction
	}
	allChunks := interaction.DataPrepChunks
	chunks, ok := allChunks[useFilename]
	if !ok {
		chunks = []types.DataPrepChunk{}
	}

	chunkExists := false
	var chunk *types.DataPrepChunk

	for _, existingChunk := range chunks {
		if existingChunk.Index == chunkIndex && existingChunk.PromptName == promptName {
			chunkExists = true
			chunk = &existingChunk
		}
	}

	if chunk == nil {
		chunk = &types.DataPrepChunk{
			Index:         chunkIndex,
			QuestionCount: questionCount,
			PromptName:    promptName,
		}
	}

	if err != nil {
		chunk.Error = err.Error()
	} else {
		chunk.Error = ""
	}

	if !chunkExists {
		chunks = append(chunks, *chunk)
	} else {
		newChunks := []types.DataPrepChunk{}
		for _, existingChunk := range chunks {
			if existingChunk.Index == chunkIndex {
				newChunks = append(newChunks, *chunk)
			} else {
				newChunks = append(newChunks, existingChunk)
			}
		}
		chunks = newChunks
	}

	allChunks[useFilename] = chunks
	interaction.DataPrepChunks = allChunks
	return interaction
}

func getFileContent(
	ctx context.Context,
	fs filestore.FileStore,
	path string,
) (string, error) {
	// load the actual file contents
	reader, err := fs.DownloadFile(ctx, path)
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
	existingParts := strings.Split(existingContent, "\n")
	newParts := append(existingParts, jsonLines...)
	_, err = fs.UploadFile(ctx, path, strings.NewReader(strings.Join(newParts, "\n")))
	if err != nil {
		return err
	}
	return nil
}

// for the moment, we append question pairs to the same file
// eventually we will append questions to a JSONL file per source file
func getQuestionsFilename(sourceFilename string) string {
	return path.Join(path.Dir(sourceFilename), types.TEXT_DATA_PREP_QUESTIONS_FILE)
	// return fmt.Sprintf("%s%s", sourceFilename, types.TEXT_DATA_PREP_QUESTIONS_FILE_SUFFIX)
}

// do we have a JSONL file already or do we need to create it?
func hasQuestionsFile(interaction *types.Interaction, sourceFilename string) bool {
	for _, file := range interaction.Files {
		if file == getQuestionsFilename(sourceFilename) {
			return true
		}
	}
	return false
}
