package desktop

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	maxUploadSize = 500 << 20 // 500MB
	incomingDir   = "/home/retro/work/incoming"
)

// handleUpload handles file uploads to the sandbox incoming folder.
func (s *Server) handleUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if err := r.ParseMultipartForm(maxUploadSize); err != nil {
		s.logger.Error("parse multipart form failed", "err", err)
		http.Error(w, "Failed to parse form: "+err.Error(), http.StatusBadRequest)
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		s.logger.Error("get file from form failed", "err", err)
		http.Error(w, "Failed to get file: "+err.Error(), http.StatusBadRequest)
		return
	}
	defer file.Close()

	// Ensure directory exists
	if err := os.MkdirAll(incomingDir, 0755); err != nil {
		s.logger.Error("create incoming directory failed", "err", err)
		http.Error(w, "Failed to create directory: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Sanitize filename to prevent path traversal
	filename := filepath.Base(header.Filename)
	destPath := filepath.Join(incomingDir, filename)

	// If file exists, add numeric suffix
	destPath, filename = uniqueFilePath(destPath, filename)

	// Create destination file
	dst, err := os.Create(destPath)
	if err != nil {
		s.logger.Error("create destination file failed", "path", destPath, "err", err)
		http.Error(w, "Failed to create file: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer dst.Close()

	// Copy file content
	written, err := io.Copy(dst, file)
	if err != nil {
		s.logger.Error("write file content failed", "err", err)
		http.Error(w, "Failed to write file: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"path":     destPath,
		"size":     written,
		"filename": filename,
	})

	s.logger.Info("file uploaded", "path", destPath, "size", written)
}

// uniqueFilePath returns a unique file path by adding numeric suffix if file exists.
func uniqueFilePath(destPath, filename string) (string, string) {
	if _, err := os.Stat(destPath); os.IsNotExist(err) {
		return destPath, filename
	}

	dir := filepath.Dir(destPath)
	ext := filepath.Ext(filename)
	base := strings.TrimSuffix(filename, ext)

	for i := 1; i < 1000; i++ {
		newFilename := fmt.Sprintf("%s (%d)%s", base, i, ext)
		newPath := filepath.Join(dir, newFilename)
		if _, err := os.Stat(newPath); os.IsNotExist(err) {
			return newPath, newFilename
		}
	}

	// Fallback: use timestamp
	newFilename := fmt.Sprintf("%s (%d)%s", base, time.Now().UnixNano(), ext)
	return filepath.Join(dir, newFilename), newFilename
}
