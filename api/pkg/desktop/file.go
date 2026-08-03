package desktop

import (
	"fmt"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

func resolveFileWithinRoot(root, filename string) (string, error) {
	if filename == "" {
		return "", fmt.Errorf("filename is required")
	}
	if filename != filepath.Base(filename) || strings.ContainsAny(filename, `/\\`) || filename == "." || filename == ".." {
		return "", fmt.Errorf("invalid attachment filename")
	}

	resolvedRoot, err := filepath.EvalSymlinks(filepath.Clean(root))
	if err != nil {
		return "", fmt.Errorf("resolve attachment root: %w", err)
	}
	resolvedPath, err := filepath.EvalSymlinks(filepath.Join(resolvedRoot, filename))
	if err != nil {
		return "", fmt.Errorf("resolve attachment: %w", err)
	}
	relativePath, err := filepath.Rel(resolvedRoot, resolvedPath)
	if err != nil || relativePath == "." || relativePath == ".." || strings.HasPrefix(relativePath, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("attachment path is outside the incoming directory")
	}

	info, err := os.Stat(resolvedPath)
	if err != nil {
		return "", fmt.Errorf("stat attachment: %w", err)
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("attachment is not a regular file")
	}
	return resolvedPath, nil
}

func (s *Server) handleFile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	filePath, err := resolveFileWithinRoot(incomingDir, r.URL.Query().Get("name"))
	if err != nil {
		s.logger.Warn("workspace attachment rejected", "err", err)
		http.Error(w, "Attachment not found", http.StatusNotFound)
		return
	}

	file, err := os.Open(filePath)
	if err != nil {
		http.Error(w, "Attachment not found", http.StatusNotFound)
		return
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		http.Error(w, "Attachment not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Cache-Control", "private, max-age=86400")
	w.Header().Set("Content-Disposition", mime.FormatMediaType("inline", map[string]string{"filename": filepath.Base(filePath)}))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	http.ServeContent(w, r, filepath.Base(filePath), info.ModTime(), file)
}
