package desktop

import (
	"fmt"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

func openFileWithinRoot(rootPath, filename string) (*os.File, os.FileInfo, error) {
	if filename == "" {
		return nil, nil, fmt.Errorf("filename is required")
	}
	if filename != filepath.Base(filename) || strings.ContainsAny(filename, `/\\`) || filename == "." || filename == ".." {
		return nil, nil, fmt.Errorf("invalid attachment filename")
	}

	root, err := os.OpenRoot(rootPath)
	if err != nil {
		return nil, nil, fmt.Errorf("open attachment root: %w", err)
	}
	defer root.Close()

	file, err := root.Open(filename)
	if err != nil {
		return nil, nil, fmt.Errorf("open attachment: %w", err)
	}

	info, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, nil, fmt.Errorf("stat attachment: %w", err)
	}
	if !info.Mode().IsRegular() {
		file.Close()
		return nil, nil, fmt.Errorf("attachment is not a regular file")
	}
	return file, info, nil
}

func (s *Server) handleFile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	file, info, err := openFileWithinRoot(incomingDir, r.URL.Query().Get("name"))
	if err != nil {
		s.logger.Warn("workspace attachment rejected", "err", err)
		http.Error(w, "Attachment not found", http.StatusNotFound)
		return
	}
	defer file.Close()

	w.Header().Set("Cache-Control", "private, max-age=86400")
	w.Header().Set("Content-Disposition", mime.FormatMediaType("inline", map[string]string{"filename": info.Name()}))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	http.ServeContent(w, r, info.Name(), info.ModTime(), file)
}
