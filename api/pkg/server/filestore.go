package server

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/gorilla/mux"
	"github.com/rs/zerolog/log"
)

func (apiServer *WaterlilyAPIServer) filestoreDownload(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	vars := mux.Vars(r)
	r.URL.Path = fmt.Sprintf("/%s", vars["path"])
	log.Info().Msgf("file: %s", vars["path"])
	fileserver := http.FileServer(http.Dir(apiServer.Options.FilestoreDirectory))
	fileserver.ServeHTTP(w, r)
}

func (apiServer *WaterlilyAPIServer) filestoreUpload(w http.ResponseWriter, r *http.Request) {
	// Parse the multipart form request
	err := r.ParseMultipartForm(32 << 20) // 32MB is the maximum memory allocated to store the file
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// extract the access_token query parameter and compare it against the apiServer.Options.FilestoreToken
	// if they don't match, return an error
	access_token := r.URL.Query().Get("access_token")
	if access_token != apiServer.Options.FilestoreToken {
		http.Error(w, "access_token does not match", http.StatusUnauthorized)
		return
	}

	path := r.URL.Query().Get("path")
	if path == "" {
		http.Error(w, "you must provide a path", http.StatusUnauthorized)
		return
	}
	// the folder we should put this image into
	uploadDir, err := apiServer.ensureFilestorePath(path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Get the file from the "uploads" field
	file, handler, err := r.FormFile("uploads")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	defer file.Close()

	// Create the file in the save directory
	f, err := os.OpenFile(filepath.Join(uploadDir, handler.Filename), os.O_WRONLY|os.O_CREATE, 0666)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer f.Close()

	// Write the file to the disk
	_, err = io.Copy(f, file)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Return success

	w.Header().Set("Content-Type", "text/plain") // set the content type to plain text
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, "OK")
}
