package ingestion

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/google/uuid"
)

type Storage interface {
	Save(filename string, r io.Reader) (string, int64, error)
	Open(path string) (io.ReadCloser, error)
	Delete(path string) error
}

type LocalStorage struct {
	baseDir string
}

func NewLocalStorage(baseDir string) (*LocalStorage, error) {
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create upload directory %s: %w", baseDir, err)
	}
	return &LocalStorage{baseDir: baseDir}, nil
}

func (s *LocalStorage) Save(filename string, r io.Reader) (string, int64, error) {
	ext := filepath.Ext(filename)
	safeID := uuid.New().String()
	relPath := fmt.Sprintf("%s%s", safeID, ext)
	fullPath := filepath.Join(s.baseDir, relPath)

	out, err := os.Create(fullPath)
	if err != nil {
		return "", 0, fmt.Errorf("failed to create file on disk: %w", err)
	}
	defer out.Close()

	n, err := io.Copy(out, r)
	if err != nil {
		return "", 0, fmt.Errorf("failed to write file content: %w", err)
	}

	return fullPath, n, nil
}

func (s *LocalStorage) Open(path string) (io.ReadCloser, error) {
	return os.Open(path)
}

func (s *LocalStorage) Delete(path string) error {
	return os.Remove(path)
}
