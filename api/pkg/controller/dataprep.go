package controller

import (
	"encoding/json"
	"fmt"
	"io"
	"path"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/lukemarsden/helix/api/pkg/data"
	"github.com/lukemarsden/helix/api/pkg/dataprep/text"
	"github.com/lukemarsden/helix/api/pkg/system"
	"github.com/lukemarsden/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// return the JSON of some fine tune conversation data
func (c *Controller) ReadTextFineTuneQuestions(filepath string) ([]types.DataPrepTextQuestion, error) {
	reader, err := c.Options.Filestore.DownloadFile(c.Ctx, filepath)
	if err != nil {
		return nil, err
	}

	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, err
	}

	var conversations []types.DataPrepTextQuestion
	lines := strings.Split(string(data), "\n")

	for _, line := range lines {
		if line == "" {
			continue
		}
		var conversation types.DataPrepTextQuestion
		err := json.Unmarshal([]byte(line), &conversation)
		if err != nil {
			return nil, err
		}
		conversations = append(conversations, conversation)
	}

	return conversations, nil
}

func (c *Controller) WriteTextFineTuneQuestions(filepath string, data []types.DataPrepTextQuestion) error {
	jsonLines := []string{}

	for _, conversationEntry := range data {
		jsonLine, err := json.Marshal(conversationEntry)
		if err != nil {
			return err
		}
		jsonLines = append(jsonLines, string(jsonLine))
	}

	_, err := c.Options.Filestore.UploadFile(c.Ctx, filepath, strings.NewReader(strings.Join(jsonLines, "\n")))
	if err != nil {
		return err
	}

	return nil
}

func (c *Controller) BeginTextFineTune(session *types.Session) error {
	session, err := data.UpdateSystemInteraction(session, func(systemInteraction *types.Interaction) (*types.Interaction, error) {
		systemInteraction.Finished = false
		systemInteraction.Progress = 1
		systemInteraction.Message = "fine tuning on data..."
		systemInteraction.Status = "fine tuning on data..."
		systemInteraction.State = types.InteractionStateWaiting
		systemInteraction.DataPrepStage = types.TextDataPrepStageFineTune
		systemInteraction.Files = []string{}
		return systemInteraction, nil
	})

	if err != nil {
		return err
	}

	c.WriteSession(session)
	c.AddSessionToQueue(session)
	c.BroadcastProgress(session, 1, "fine tuning on data...")

	return nil
}

type convertDocumentsToChunksRequest struct {
	URL string `json:"url"`
}

type convertDocumentsToChunksResponse struct {
	Text string `json:"text"`
}

type convertTextItem struct {
	Name    string `json:"name"`
	Content string `json:"content"`
}

// in the case of a text fine tune - we need to convert all the documents first
// TODO: there is no rate limiting on this path
func (c *Controller) convertDocumentsToText(session *types.Session) (*types.Session, error) {
	userInteraction, err := data.GetUserInteraction(session)
	if err != nil {
		return nil, err
	}

	systemInteraction, err := data.GetSystemInteraction(session)
	if err != nil {
		return nil, err
	}

	filesToConvert := []string{}
	existingFileNames := map[string]bool{}

	for _, file := range userInteraction.Files {
		existingFileNames[file] = true
	}

	shouldConvertFile := func(filename string) bool {
		if strings.HasSuffix(filename, ".txt") {
			// it is already converted - nothing to do
			return false
		}

		filename = strings.TrimSuffix(filename, ".url")

		if strings.HasSuffix(filename, ".jsonl") {
			return false
		}

		// if strings.HasSuffix(filename, types.TEXT_DATA_PREP_QUESTIONS_FILE_SUFFIX) {
		// 	// we've already converted this file into q&a pairs
		// 	return false
		// }

		// check if we have already got the chunks for this file
		_, ok := existingFileNames[fmt.Sprintf("%s.txt", filename)]
		if ok {
			// we've already chunked this file into chunks
			return false
		}

		// check if we have already got the chunks for this file
		// _, ok = existingFileNames[fmt.Sprintf("%s%s", filename, types.TEXT_DATA_PREP_QUESTIONS_FILE_SUFFIX)]
		// if ok {
		// 	// we've already chunked this file into chunks
		// 	return false
		// }

		return true
	}

	for _, file := range userInteraction.Files {
		if shouldConvertFile(file) {
			filesToConvert = append(filesToConvert, file)
		}
	}

	runningFileList := copyFileList(userInteraction.Files)

	// get the progress bar to display
	initialMessage := fmt.Sprintf("downloading and extracting text from %d files", len(filesToConvert))
	systemInteraction.Status = initialMessage
	systemInteraction.Progress = 1
	systemInteraction.DataPrepStage = types.TextDataPrepStageExtractText
	systemInteraction.State = types.InteractionStateWaiting
	session = c.WriteInteraction(session, systemInteraction)

	c.BroadcastProgress(session, 1, initialMessage)

	var completedCounter int64

	err = system.ForEachConcurrently[string](
		filesToConvert,
		c.Options.DataPrepConcurrency,
		func(file string, i int) error {
			fileURL := ""
			filenameParts := strings.Split(file, ".")
			originalFile := file

			if filenameParts[len(filenameParts)-1] == "url" {
				// if the file itself ends with .url then it's a textfile
				// that has a URL we should download as the actual file
				fileURL, err = getFileContent(c.Ctx, c.Options.Filestore, file)
				if err != nil {
					return err
				}
				file = strings.TrimSuffix(file, ".url")
			} else {
				// otherwise it's a file already living in the file store
				// so
				fileObject, err := c.Options.Filestore.Get(c.Ctx, file)
				if err != nil {
					return err
				}
				fileURL = fileObject.URL
			}

			// for local development - the file server hostname will not resolve
			// from inside the unstructured container
			fileURL = strings.Replace(fileURL, "http://localhost", "http://api", 1)

			res, err := system.PostRequest[convertDocumentsToChunksRequest, convertDocumentsToChunksResponse](
				system.ClientOptions{},
				c.Options.TextExtractionURL,
				convertDocumentsToChunksRequest{
					URL: fileURL,
				},
			)
			if err != nil {
				return err
			}

			atomic.AddInt64(&completedCounter, 1)
			newFilepath := strings.TrimSuffix(file, path.Ext(file)) + ".txt"

			_, err = c.Options.Filestore.UploadFile(c.Ctx, newFilepath, strings.NewReader(res.Text))
			if err != nil {
				return err
			}

			percentConverted := int(float64(completedCounter) / float64(len(filesToConvert)) * 100)
			message := fmt.Sprintf("extracted text from %s - %d of %d files extracted", path.Base(file), completedCounter, len(filesToConvert))
			c.BroadcastProgress(session, percentConverted, message)
			systemInteraction.Status = message
			systemInteraction.Progress = percentConverted
			session = c.WriteInteraction(session, systemInteraction)

			runningFileList = injectFileToList(runningFileList, originalFile, newFilepath)
			userInteraction.Files = runningFileList
			session = c.WriteInteraction(session, userInteraction)

			return nil
		},
	)

	if err != nil {
		return nil, err
	}

	finishedMessage := fmt.Sprintf("extracted %d files", len(filesToConvert))

	c.BroadcastProgress(session, 100, finishedMessage)

	// now we have added some text files let's update the user interaction
	userInteraction.Files = runningFileList
	// userInteraction.State = types.InteractionStateComplete
	session = c.WriteInteraction(session, userInteraction)

	systemInteraction.Status = finishedMessage
	session = c.WriteInteraction(session, systemInteraction)

	// for cases where the text conversion is very fast, give the UI a chance to display the text stage
	time.Sleep(1 * time.Second)

	return session, nil
}

func (c *Controller) convertChunksToQuestions(session *types.Session) (*types.Session, error) {
	userInteraction, err := data.GetUserInteraction(session)
	if err != nil {
		return nil, err
	}

	systemInteraction, err := data.GetSystemInteraction(session)
	if err != nil {
		return nil, err
	}

	filesToConvert := []string{}

	shouldConvertFile := func(filename string) bool {
		return strings.HasSuffix(filename, ".txt")
	}

	for _, file := range userInteraction.Files {
		if shouldConvertFile(file) {
			filesToConvert = append(filesToConvert, file)
		}
	}

	dataprep, splitter, err := c.Options.DataPrepTextFactory(session)
	if err != nil {
		return nil, err
	}

	// add all the files to the splitter so we know what chunks we have
	for _, file := range filesToConvert {
		fileContent, err := getFileContent(c.Ctx, c.Options.Filestore, file)
		if err != nil {
			return nil, err
		}
		err = splitter.AddDocument(file, fileContent)
		if err != nil {
			return nil, err
		}
	}

	chunksToProcess := []*text.DataPrepTextSplitterChunk{}
	for _, chunk := range splitter.Chunks {
		if !hasProcessedQAChunk(systemInteraction, chunk.Filename, chunk.Index) {
			chunksToProcess = append(chunksToProcess, chunk)
		}
	}

	// get the progress bar to display
	initialMessage := fmt.Sprintf("converting %d text chunks to question answer pairs", len(chunksToProcess))
	systemInteraction.Status = initialMessage
	systemInteraction.Progress = 1
	systemInteraction.DataPrepStage = types.TextDataPrepStageGenerateQuestions
	session = c.WriteInteraction(session, systemInteraction)
	c.BroadcastProgress(session, 1, initialMessage)

	var completedCounter int64
	var errorCounter int64
	var outerError error

	// we use this to only append questions to one file at a time
	var writeUpdatesMutex sync.Mutex

	runningFileList := copyFileList(userInteraction.Files)

	outerError = system.ForEachConcurrently[*text.DataPrepTextSplitterChunk](
		chunksToProcess,
		dataprep.GetConcurrency(),
		func(chunk *text.DataPrepTextSplitterChunk, i int) error {
			log.Info().Msgf("🔵 question conversion start %d of %d", i, len(chunksToProcess))
			questions, convertError := dataprep.ConvertChunk(chunk.Text, chunk.Index)

			// if this is set then we have a non GPT error and should just stop what we are doing
			if outerError != nil {
				return nil
			}

			// write the updates inside a mutex so we don't get a race
			err = func() error {
				writeUpdatesMutex.Lock()
				defer writeUpdatesMutex.Unlock()

				if convertError == nil {
					// if there is no JSONL file - make it appear
					if !hasQuestionsFile(userInteraction, chunk.Filename) {
						runningFileList = injectFileToList(runningFileList, chunk.Filename, getQuestionsFilename(chunk.Filename))
						userInteraction.Files = runningFileList

						// we want to write an empty file to the filestore here
						// because then appendQuestionsToFile doesn't need to deal with making it
						_, err = c.Options.Filestore.UploadFile(c.Ctx, getQuestionsFilename(chunk.Filename), strings.NewReader(""))
						if err != nil {
							log.Error().Msgf("error uploading file: %s", err.Error())
							return err
						}
					}
					innerErr := appendQuestionsToFile(c.Ctx, c.Options.Filestore, getQuestionsFilename(chunk.Filename), questions)
					if innerErr != nil {
						log.Error().Msgf("error adding questions to file: %s", innerErr.Error())
						return innerErr
					}
					atomic.AddInt64(&completedCounter, 1)
				} else {
					atomic.AddInt64(&errorCounter, 1)
				}

				// this marks the QA chunk as "done" - even with an error
				// we then give the user the choice to try again, abort or ignore the errors
				systemInteraction = updateProcessedQAChunk(systemInteraction, chunk.Filename, chunk.Index, len(questions), convertError)

				return nil
			}()

			if err != nil {
				return err
			}

			session = c.WriteInteraction(session, userInteraction)

			percentConverted := int((float64(completedCounter) + float64(errorCounter)) / float64(len(chunksToProcess)) * 100)
			message := fmt.Sprintf("%d total, %d converted and %d errors", len(chunksToProcess), completedCounter, errorCounter)
			c.BroadcastProgress(session, percentConverted, message)
			systemInteraction.Status = message
			systemInteraction.Progress = percentConverted
			session = c.WriteInteraction(session, systemInteraction)

			if convertError != nil {
				log.Error().Msgf("🔴 question conversion error %s", convertError.Error())
			} else {
				log.Info().Msgf("🟢 question conversion complete %d of %d", i, len(chunksToProcess))
			}

			return nil
		},
	)

	// if this error is hit - it means something has actually gone wrong rather than a data prep error
	// we catch the data prep errors and present them to the user once all processing is done
	if outerError != nil {
		return nil, outerError
	}

	finishedMessage := fmt.Sprintf("converted %d text chunks", len(chunksToProcess))

	c.BroadcastProgress(session, 100, finishedMessage)

	systemInteraction.Status = finishedMessage
	systemInteraction.DataPrepStage = types.TextDataPrepStageEditQuestions
	systemInteraction.Progress = 0
	systemInteraction.State = types.InteractionStateEditing
	session = c.WriteInteraction(session, systemInteraction)

	return session, nil
}
