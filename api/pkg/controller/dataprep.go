package controller

import (
	"context"
	"fmt"
	"path"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/helixml/helix/api/pkg/data"
	"github.com/helixml/helix/api/pkg/dataprep/text"
	"github.com/helixml/helix/api/pkg/prompts"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

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

func (c *Controller) getDocumentsToConvertToText(session *types.Session) ([]string, error) {
	userInteraction, err := data.GetUserInteraction(session)
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

		if strings.HasSuffix(filename, ".md") {
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

	return filesToConvert, nil
}

// in the case of a text fine tune - we need to convert all the documents first
// TODO: there is no rate limiting on this path
func (c *Controller) convertDocumentsToText(session *types.Session) (*types.Session, int, error) {
	userInteraction, err := data.GetUserInteraction(session)
	if err != nil {
		return nil, 0, err
	}

	systemInteraction, err := data.GetSystemInteraction(session)
	if err != nil {
		return nil, 0, err
	}

	filesToConvert, err := c.getDocumentsToConvertToText(session)
	if err != nil {
		return nil, 0, err
	}

	if len(filesToConvert) == 0 {
		return session, 0, nil
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

	// converting to text is quite fast but we don't have a scaling strategy for llamaindex right now
	// so let's just have some control over large numbers of files in one session
	err = system.ForEachConcurrently[string](
		filesToConvert,
		5,
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
				fileURL, err = c.Options.Filestore.SignedURL(c.Ctx, file)
				if err != nil {
					return err
				}
			}

			// for local development - the file server hostname will not resolve
			// from inside the llamaindex container
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
		return nil, 0, err
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

	return session, len(filesToConvert), nil
}

func (c *Controller) getTextFilesToConvert(session *types.Session) ([]string, error) {
	userInteraction, err := data.GetUserInteraction(session)
	if err != nil {
		return nil, err
	}

	filesToConvert := []string{}

	shouldConvertFile := func(filename string) bool {
		return strings.HasSuffix(filename, ".txt") || strings.HasSuffix(filename, ".md")
	}

	for _, file := range userInteraction.Files {
		if shouldConvertFile(file) {
			filesToConvert = append(filesToConvert, file)
		}
	}

	return filesToConvert, nil
}

func (c *Controller) getQAChunksToProcess(session *types.Session, dataprep text.DataPrepTextQuestionGenerator) ([]*text.DataPrepTextSplitterChunk, error) {
	filesToConvert, err := c.getTextFilesToConvert(session)
	if err != nil {
		return nil, err
	}

	systemInteraction, err := data.GetSystemInteraction(session)
	if err != nil {
		return nil, err
	}

	_, splitter, err := c.Options.DataPrepTextFactory(session)
	if err != nil {
		return nil, err
	}

	documentGroupID := strings.Replace(session.ID, "-", "", -1)[:10]
	newMeta := session.Metadata
	newMeta.DocumentGroupID = documentGroupID
	if newMeta.DocumentIDs == nil {
		newMeta.DocumentIDs = map[string]string{}
	}

	// add all the files to the splitter so we know what chunks we have
	for _, filename := range filesToConvert {
		fileContent, err := getFileContent(c.Ctx, c.Options.Filestore, filename)
		if err != nil {
			return nil, err
		}
		documentID, err := splitter.AddDocument(filename, fileContent, documentGroupID)
		if err != nil {
			return nil, err
		}
		newMeta.DocumentIDs[filename] = documentID
	}

	_, err = c.UpdateSessionMetadata(context.TODO(), session, &newMeta)
	if err != nil {
		return nil, err
	}

	// Some qapair generators expand each chunk into N chunks so they can be run
	// by our outer concurrency manager
	allChunks, err := dataprep.ExpandChunks(splitter.Chunks)
	if err != nil {
		return nil, err
	}

	chunksToProcess := []*text.DataPrepTextSplitterChunk{}
	for _, chunk := range allChunks {
		if !hasProcessedQAChunk(systemInteraction, chunk.Filename, chunk.Index, chunk.PromptName) {
			chunksToProcess = append(chunksToProcess, chunk)
		}
	}

	return chunksToProcess, nil
}

func (c *Controller) getRagChunksToProcess(session *types.Session) ([]*text.DataPrepTextSplitterChunk, error) {
	filesToConvert, err := c.getTextFilesToConvert(session)
	if err != nil {
		return nil, err
	}

	splitter, err := text.NewDataPrepSplitter(text.DataPrepTextSplitterOptions{
		ChunkSize: session.Metadata.RagSettings.ChunkSize,
		Overflow:  session.Metadata.RagSettings.ChunkOverflow,
	})
	if err != nil {
		return nil, err
	}

	documentGroupID := strings.Replace(session.ID, "-", "", -1)[:10]

	for _, file := range filesToConvert {
		fileContent, err := getFileContent(c.Ctx, c.Options.Filestore, file)
		if err != nil {
			return nil, err
		}
		_, err = splitter.AddDocument(file, fileContent, documentGroupID)
		if err != nil {
			return nil, err
		}
	}

	return splitter.Chunks, nil
}

func (c *Controller) indexChunksForRag(session *types.Session) (*types.Session, int, error) {
	systemInteraction, err := data.GetSystemInteraction(session)
	if err != nil {
		return nil, 0, err
	}

	chunksToProcess, err := c.getRagChunksToProcess(session)
	if err != nil {
		return nil, 0, err
	}

	if len(chunksToProcess) == 0 {
		return session, 0, nil
	}

	// get the progress bar to display
	initialMessage := fmt.Sprintf("indexing %d text chunks into vector database", len(chunksToProcess))
	systemInteraction.Status = initialMessage
	systemInteraction.Progress = 1
	systemInteraction.DataPrepStage = types.TextDataPrepStageIndexRag
	session = c.WriteInteraction(session, systemInteraction)
	c.BroadcastProgress(session, 1, initialMessage)

	var completedCounter int64
	var errorCounter int64

	for i, chunk := range chunksToProcess {
		log.Info().Msgf("🔵 rag index %d of %d", i+1, len(chunksToProcess))

		convertError := c.indexChunkForRag(session, systemInteraction, chunk)

		if convertError != nil {
			atomic.AddInt64(&errorCounter, 1)
		} else {
			atomic.AddInt64(&completedCounter, 1)
		}

		percentConverted := int((float64(completedCounter) + float64(errorCounter)) / float64(len(chunksToProcess)) * 100)
		message := fmt.Sprintf("%d total, %d indexed and %d errors", len(chunksToProcess), completedCounter, errorCounter)
		c.BroadcastProgress(session, percentConverted, message)
		systemInteraction.Status = message
		systemInteraction.Progress = percentConverted
		session = c.WriteInteraction(session, systemInteraction)

		if convertError != nil {
			log.Error().Msgf("🔴 rag index error %s", convertError.Error())
		} else {
			log.Info().Msgf("🟢 rag index complete %d of %d", i+1, len(chunksToProcess))
		}

	}

	finishedMessage := fmt.Sprintf("indexed %d text chunks into vector database", len(chunksToProcess))

	c.BroadcastProgress(session, 100, finishedMessage)

	systemInteraction.Status = finishedMessage
	systemInteraction.DataPrepStage = types.TextDataPrepStageGenerateQuestions
	systemInteraction.Progress = 0
	session = c.WriteInteraction(session, systemInteraction)

	return session, len(chunksToProcess), nil
}

func (c *Controller) indexChunkForRag(session *types.Session, interaction *types.Interaction, chunk *text.DataPrepTextSplitterChunk) error {
	_, err := system.PostRequest[types.SessionRagIndexChunk, types.SessionRagResult](
		system.ClientOptions{},
		c.Options.RAGIndexingURL,
		types.SessionRagIndexChunk{
			SessionID:       session.ID,
			InteractionID:   interaction.ID,
			Filename:        chunk.Filename,
			DocumentID:      chunk.DocumentID,
			DocumentGroupID: chunk.DocumentGroupID,
			ContentOffset:   chunk.Index,
			Content:         chunk.Text,
		},
	)
	if err != nil {
		return err
	}
	return nil
}

// given a user prompt and an existing session id
// let's load from the vector store
func (c *Controller) getRAGResults(session *types.Session) ([]types.SessionRagResult, error) {
	userInteraction, err := data.GetUserInteraction(session)
	if err != nil {
		return nil, err
	}
	result, err := system.PostRequest[types.SessionRagQuery, []types.SessionRagResult](
		system.ClientOptions{},
		c.Options.RAGQueryURL,
		types.SessionRagQuery{
			Prompt:            userInteraction.Message,
			SessionID:         session.ID,
			DistanceThreshold: session.Metadata.RagSettings.Threshold,
			DistanceFunction:  session.Metadata.RagSettings.DistanceFunction,
			MaxResults:        session.Metadata.RagSettings.ResultsCount,
		},
	)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (c *Controller) convertChunksToQuestions(session *types.Session) (*types.Session, int, error) {
	userInteraction, err := data.GetUserInteraction(session)
	if err != nil {
		return nil, 0, err
	}

	systemInteraction, err := data.GetSystemInteraction(session)
	if err != nil {
		return nil, 0, err
	}

	dataprep, _, err := c.Options.DataPrepTextFactory(session)
	if err != nil {
		return nil, 0, err
	}

	chunksToProcess, err := c.getQAChunksToProcess(session, dataprep)
	if err != nil {
		return nil, 0, err
	}

	if len(chunksToProcess) == 0 {
		return session, 0, nil
	}

	systemInteraction.DataPrepTotalChunks = len(chunksToProcess)

	// get the progress bar to display
	initialMessage := fmt.Sprintf("converting %d text chunks to question answer pairs", len(chunksToProcess))

	// Validate quotas
	if c.Options.Config.SubscriptionQuotas.Enabled {
		usermeta, err := c.Options.Store.GetUserMeta(context.Background(), session.Owner)
		if err != nil {
			return nil, 0, fmt.Errorf("error getting user '%s' meta: %s", session.Owner, err.Error())
		}

		// Check if the plan is pro
		if usermeta.Config.StripeSubscriptionActive {
			if len(chunksToProcess) > c.Options.Config.SubscriptionQuotas.Finetuning.Pro.MaxChunks {

				// Get the progress bar to display
				initialMessage = fmt.Sprintf("too many chunks to convert in pro tier (%d), reducing to %d text chunks to question answer pairs",
					len(chunksToProcess),
					c.Options.Config.SubscriptionQuotas.Finetuning.Pro.MaxChunks,
				)

				// Cut the chunks to the pro tier limit
				chunksToProcess = chunksToProcess[:c.Options.Config.SubscriptionQuotas.Finetuning.Pro.MaxChunks]

				// Marking the session as limited
				systemInteraction.DataPrepLimited = true
				systemInteraction.DataPrepLimit = c.Options.Config.SubscriptionQuotas.Finetuning.Pro.MaxChunks
			}
		} else {
			// Free tier
			if len(chunksToProcess) > c.Options.Config.SubscriptionQuotas.Finetuning.Free.MaxChunks {

				// Get the progress bar to display
				initialMessage = fmt.Sprintf("too much data to process on the free tier (%d), reducing to %d text chunks. Upgrade your plan to process more text.",
					len(chunksToProcess),
					c.Options.Config.SubscriptionQuotas.Finetuning.Free.MaxChunks,
				)

				// Cut the chunks to the free tier limit
				chunksToProcess = chunksToProcess[:c.Options.Config.SubscriptionQuotas.Finetuning.Free.MaxChunks]
				// Marking the session as limited
				systemInteraction.DataPrepLimited = true
				systemInteraction.DataPrepLimit = c.Options.Config.SubscriptionQuotas.Finetuning.Free.MaxChunks
			}
		}
	}

	if systemInteraction.DataPrepLimited {
		log.Info().
			Str("user_id", session.Owner).
			Str("session_id", session.ID).
			Int("limit", systemInteraction.DataPrepLimit).
			Int("total_chunks", systemInteraction.DataPrepTotalChunks).
			Msgf("chunks have been reduced to the tier limit of %d, total chunks before: %d",
				c.Options.Config.SubscriptionQuotas.Finetuning.Free.MaxChunks,
				len(chunksToProcess),
			)
	}

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
			log.Info().Msgf("🔵 question conversion start %d of %d", i+1, len(chunksToProcess))
			questions, convertError := dataprep.ConvertChunk(chunk.Text, chunk.Index, chunk.DocumentID, chunk.DocumentGroupID, chunk.PromptName)

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
				systemInteraction = updateProcessedQAChunk(systemInteraction, chunk.Filename, chunk.Index, chunk.PromptName, len(questions), convertError)

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
				log.Info().Msgf("🟢 question conversion complete %d of %d", i+1, len(chunksToProcess))
			}

			return nil
		},
	)

	// if this error is hit - it means something has actually gone wrong rather than a data prep error
	// we catch the data prep errors and present them to the user once all processing is done
	if outerError != nil {
		return nil, 0, outerError
	}

	finishedMessage := fmt.Sprintf("converted %d text chunks", len(chunksToProcess))

	c.BroadcastProgress(session, 100, finishedMessage)

	systemInteraction.Status = finishedMessage
	systemInteraction.DataPrepStage = types.TextDataPrepStageEditQuestions
	systemInteraction.Progress = 0
	systemInteraction.State = types.InteractionStateEditing
	session = c.WriteInteraction(session, systemInteraction)

	docIDs := []string{}
	// TODO: remove duplication wrt splitter
	docGroupID := strings.Replace(session.ID, "-", "", -1)[:10]
	uniqueMap := make(map[string]bool)
	for _, val := range session.Metadata.DocumentIDs {
		if !uniqueMap[val] {
			uniqueMap[val] = true
			docIDs = append(docIDs, val)
		}
	}

	systemPrompt, err := prompts.TextFinetuneSystemPrompt(docIDs, docGroupID)
	if err != nil {
		return nil, 0, err
	}
	session.Metadata.SystemPrompt = systemPrompt
	c.WriteSession(session)

	return session, len(chunksToProcess), nil
}

func (c *Controller) convertChunksToQuestionsErrorCount(session *types.Session) (int, error) {
	systemInteraction, err := data.GetSystemInteraction(session)
	if err != nil {
		return 0, err
	}
	return getQAChunkErrors(systemInteraction), nil
}
