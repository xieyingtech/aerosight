package media

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type LocalObjectStorage struct {
	root string
}

func NewLocalObjectStorage(root string) (*LocalObjectStorage, error) {
	if strings.TrimSpace(root) == "" {
		return nil, errors.New("local object storage root is required")
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	return &LocalObjectStorage{root: absolute}, nil
}

func (storage *LocalObjectStorage) GetObject(_ context.Context, key string) (Object, error) {
	path, err := storage.resolve(key)
	if err != nil {
		return Object{}, err
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return Object{}, err
	}
	return Object{Key: key, Body: body, ChecksumSHA256: checksum(body), VersionID: checksum(body)}, nil
}

func (storage *LocalObjectStorage) PutObject(_ context.Context, key string, reader io.Reader, contentType string) (Object, error) {
	path, err := storage.resolve(key)
	if err != nil {
		return Object{}, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return Object{}, err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".aerosight-object-*")
	if err != nil {
		return Object{}, err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	var body bytes.Buffer
	if _, err := io.Copy(io.MultiWriter(temporary, &body), reader); err != nil {
		temporary.Close()
		return Object{}, err
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return Object{}, err
	}
	if err := temporary.Close(); err != nil {
		return Object{}, err
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return Object{}, err
	}
	digest := checksum(body.Bytes())
	return Object{Key: key, Body: body.Bytes(), ContentType: contentType, ChecksumSHA256: digest, VersionID: digest}, nil
}

func (storage *LocalObjectStorage) resolve(key string) (string, error) {
	if strings.Contains(key, "\\") || strings.HasPrefix(key, "/") || !strings.HasPrefix(key, "projects/") {
		return "", errors.New("invalid project object key")
	}
	clean := filepath.Clean(filepath.FromSlash(key))
	if clean != filepath.FromSlash(key) || clean == "." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", errors.New("invalid project object key")
	}
	path := filepath.Join(storage.root, clean)
	relative, err := filepath.Rel(storage.root, path)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", errors.New("object key escapes storage root")
	}
	return path, nil
}
