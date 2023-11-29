package controller

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	stdlog "log"
	"mime/multipart"
	"net/http"
	"time"

	"github.com/hashicorp/go-retryablehttp"
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
