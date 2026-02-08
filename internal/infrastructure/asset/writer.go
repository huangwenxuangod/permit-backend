package asset

import (
	"os"
	"path/filepath"
	"strings"
)

type FSWriter struct {
	AssetsDir     string
	PublicBaseURL string
}

func NewFSWriter(assetsDir string, publicBaseURL string) *FSWriter {
	return &FSWriter{AssetsDir: assetsDir, PublicBaseURL: strings.TrimRight(publicBaseURL, "/")}
}

func (w *FSWriter) Write(taskID, color string, data []byte) (string, error) {
	dir := filepath.Join(w.AssetsDir, taskID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	out := filepath.Join(dir, color+".jpg")
	if err := os.WriteFile(out, data, 0o644); err != nil {
		return "", err
	}
	return w.buildURL("/assets/" + taskID + "/" + color + ".jpg"), nil
}

func (w *FSWriter) WriteFile(taskID, filename string, data []byte) (string, error) {
	dir := filepath.Join(w.AssetsDir, taskID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	out := filepath.Join(dir, filename)
	if err := os.WriteFile(out, data, 0o644); err != nil {
		return "", err
	}
	return w.buildURL("/assets/" + taskID + "/" + filename), nil
}

func (w *FSWriter) buildURL(path string) string {
	if w.PublicBaseURL == "" {
		return path
	}
	return w.PublicBaseURL + path
}
