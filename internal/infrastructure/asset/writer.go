package asset

import (
	"os"
	"path/filepath"
)

type FSWriter struct {
	AssetsDir string
}

func NewFSWriter(assetsDir string) *FSWriter {
	return &FSWriter{AssetsDir: assetsDir}
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
	return "/assets/" + taskID + "/" + color + ".jpg", nil
}
