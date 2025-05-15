package controller

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"path"
	"strings"

	"github.com/helixml/helix/api/pkg/filestore"
	"github.com/helixml/helix/api/pkg/types"
	openai "github.com/sashabaranov/go-openai"
)

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
	return append([]string{}, fileList...)
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

func getQAChunkErrors(
	interaction *types.Interaction,
) int {
	errorCount := 0
	for _, chunkArray := range interaction.DataPrepChunks {
		for _, chunk := range chunkArray {
			if chunk.Error != "" {
				errorCount++
			}
		}
	}
	return errorCount
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
			if existingChunk.Index == chunkIndex && existingChunk.PromptName == promptName {
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
	reader, err := fs.OpenFile(ctx, path)
	if err != nil {
		return "", err
	}
	defer reader.Close()

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
	_, err = fs.WriteFile(ctx, path, strings.NewReader(strings.Join(newParts, "\n")))
	if err != nil {
		return err
	}
	return nil
}

// for the moment, we append question pairs to the same file
// eventually we will append questions to a JSONL file per source file
func getQuestionsFilename(sourceFilename string) string {
	return path.Join(path.Dir(sourceFilename), types.TextDataPrepQuestionsFile)
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

func getLastMessage(req openai.ChatCompletionRequest) string {
	if len(req.Messages) > 0 {
		lastMessage := req.Messages[len(req.Messages)-1]
		// Prioritize multi-content messages
		if len(lastMessage.MultiContent) > 0 {
			// Find the first text message
			for _, content := range lastMessage.MultiContent {
				if content.Type == openai.ChatMessagePartTypeText {
					return content.Text
				}
			}
		}

		return req.Messages[len(req.Messages)-1].Content
	}

	return ""
}
