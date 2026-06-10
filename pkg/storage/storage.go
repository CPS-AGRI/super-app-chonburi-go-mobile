package storage

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

type StorageProvider interface {
	Upload(file io.Reader, filename string) (string, error)
	Delete(filename string) error
	GetURL(filename string) string
}

type localStorage struct {
	baseDir string
	baseURL string
}

func NewLocalStorage(baseDir, baseURL string) StorageProvider {
	_ = os.MkdirAll(baseDir, 0755)
	return &localStorage{baseDir: baseDir, baseURL: baseURL}
}

func (s *localStorage) Upload(file io.Reader, filename string) (string, error) {
	dstPath := filepath.Join(s.baseDir, filename)
	dst, err := os.Create(dstPath)
	if err != nil {
		return "", err
	}
	defer dst.Close()

	if _, err := io.Copy(dst, file); err != nil {
		return "", err
	}

	return fmt.Sprintf("%s/%s", s.baseURL, filename), nil
}

func (s *localStorage) Delete(filename string) error {
	return os.Remove(filepath.Join(s.baseDir, filename))
}

func (s *localStorage) GetURL(filename string) string {
	return fmt.Sprintf("%s/%s", s.baseURL, filename)
}
