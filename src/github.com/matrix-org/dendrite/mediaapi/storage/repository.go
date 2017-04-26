// Copyright 2017 Vector Creations Ltd
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package storage

import (
	"crypto/sha256"
	"encoding/base64"
	"hash"
	"io"
	"io/ioutil"
	"os"
	"path"

	log "github.com/Sirupsen/logrus"
)

// Description contains various attributes for an image.
type Description struct {
	Type   string
	Length int64
}

type repositoryPaths struct {
	contentPath string
	typePath    string
}

// Repository stores locally uploaded media, and caches remote media that has
// been requested.
type Repository struct {
	StorePrefix string
	MaxBytes    uint64
}

// ReaderFromRemoteCache returns a io.ReadCloser with the cached remote content,
// if it exists. Use IsNotExist to check if the error was due to it not existing
// in the cache
func (repo Repository) ReaderFromRemoteCache(host, name string) (io.ReadCloser, *Description, error) {
	mediaDir := repo.getDirForRemoteMedia(host, name)
	repoPaths := getPathsForMedia(mediaDir)

	return repo.readerFromRepository(repoPaths)
}

// ReaderFromLocalRepo returns a io.ReadCloser with the locally uploaded content,
// if it exists. Use IsNotExist to check if the error was due to it not existing
// in the cache
func (repo Repository) ReaderFromLocalRepo(name string) (io.ReadCloser, *Description, error) {
	mediaDir := repo.getDirForLocalMedia(name)
	repoPaths := getPathsForMedia(mediaDir)

	return repo.readerFromRepository(repoPaths)
}

func (repo Repository) readerFromRepository(repoPaths repositoryPaths) (io.ReadCloser, *Description, error) {
	contentTypeBytes, err := ioutil.ReadFile(repoPaths.typePath)
	if err != nil {
		return nil, nil, err
	}

	contentType := string(contentTypeBytes)

	file, err := os.Open(repoPaths.contentPath)
	if err != nil {
		return nil, nil, err
	}

	stat, err := file.Stat()
	if err != nil {
		return nil, nil, err
	}

	descr := Description{
		Type:   contentType,
		Length: stat.Size(),
	}

	return file, &descr, nil
}

// WriterToLocalRepository returns a RepositoryWriter for writing newly uploaded
// content into the repository.
//
// The returned RepositoryWriter will fail if more than MaxBytes tries to be
// written.
func (repo Repository) WriterToLocalRepository(descr Description) (RepositoryWriter, error) {
	return newLocalRepositoryWriter(repo, descr)
}

// WriterToRemoteCache returns a RepositoryWriter for caching newly downloaded
// remote content.
//
// The returned RepositoryWriter will silently stop writing if more than MaxBytes
// tries to be written and does *not* return an error.
func (repo Repository) WriterToRemoteCache(host, name string, descr Description) (RepositoryWriter, error) {
	return newRemoteRepositoryWriter(repo, host, name, descr)
}

func (repo *Repository) makeTempDir() (string, error) {
	tmpDir := path.Join(repo.StorePrefix, "tmp")
	os.MkdirAll(tmpDir, 0770)
	return ioutil.TempDir(tmpDir, "")
}

func (repo *Repository) getDirForLocalMedia(name string) string {
	return path.Join(repo.StorePrefix, "local", name[:3], name[3:])
}

func (repo *Repository) getDirForRemoteMedia(host, sanitizedName string) string {
	return path.Join(repo.StorePrefix, "remote", host, sanitizedName[:3], sanitizedName[3:])
}

// Get the actual paths for the data and metadata associated with remote media.
func getPathsForMedia(dir string) repositoryPaths {
	contentPath := path.Join(dir, "content")
	typePath := path.Join(dir, "type")
	return repositoryPaths{
		contentPath: contentPath,
		typePath:    typePath,
	}
}

// IsNotExists check if error was due to content not existing in cache.
func IsNotExists(err error) bool { return os.IsNotExist(err) }

// RepositoryWriter is used to either store into the repository newly uploaded
// media or to cache recently fetched remote media.
type RepositoryWriter interface {
	io.WriteCloser

	// Finished should be called when successfully finished writing; otherwise
	// the written content will not be committed to the repository.
	Finished() (string, error)
}

type remoteRepositoryWriter struct {
	tmpDir   string
	finalDir string
	name     string
	file     io.WriteCloser
	erred    bool
}

func newRemoteRepositoryWriter(repo Repository, host, name string, descr Description) (*remoteRepositoryWriter, error) {
	tmpFile, tmpDir, err := getTempWriter(repo, descr)
	if err != nil {
		log.Printf("Failed to create writer: %v\n", err)
		return nil, err
	}

	return &remoteRepositoryWriter{
		tmpDir:   tmpDir,
		finalDir: repo.getDirForRemoteMedia(host, name),
		name:     name,
		file:     tmpFile,
		erred:    false,
	}, nil
}

func (writer remoteRepositoryWriter) Write(p []byte) (int, error) {
	// Its OK to fail when writing to the remote repo. We just hide the error
	// from the layers above
	if !writer.erred {
		if _, err := writer.file.Write(p); err != nil {
			writer.erred = true
		}
	}
	return len(p), nil
}

func (writer remoteRepositoryWriter) Close() error {
	os.RemoveAll(writer.tmpDir)
	writer.file.Close()
	return nil
}

func (writer remoteRepositoryWriter) Finished() (string, error) {
	var err error
	if !writer.erred {
		os.MkdirAll(path.Dir(writer.finalDir), 0770)
		err = os.Rename(writer.tmpDir, writer.finalDir)
		if err != nil {
			return "", err
		}
	}
	err = writer.Close()
	return writer.name, err
}

type localRepositoryWriter struct {
	repo     Repository
	tmpDir   string
	hasher   hash.Hash
	file     io.WriteCloser
	finished bool
}

func newLocalRepositoryWriter(repo Repository, descr Description) (*localRepositoryWriter, error) {
	tmpFile, tmpDir, err := getTempWriter(repo, descr)
	if err != nil {
		return nil, err
	}

	return &localRepositoryWriter{
		repo:     repo,
		tmpDir:   tmpDir,
		hasher:   sha256.New(),
		file:     tmpFile,
		finished: false,
	}, nil
}

func (writer localRepositoryWriter) Write(p []byte) (int, error) {
	writer.hasher.Write(p) // Never errors.
	n, err := writer.file.Write(p)
	if err != nil {
		writer.Close()
	}
	return n, err
}

func (writer localRepositoryWriter) Close() error {
	var err error
	if !writer.finished {
		err = os.RemoveAll(writer.tmpDir)
		if err != nil {
			return err
		}
	}

	err = writer.file.Close()
	return err
}

func (writer localRepositoryWriter) Finished() (string, error) {
	hash := writer.hasher.Sum(nil)
	name := base64.URLEncoding.EncodeToString(hash[:])
	finalDir := writer.repo.getDirForLocalMedia(name)
	os.MkdirAll(path.Dir(finalDir), 0770)
	err := os.Rename(writer.tmpDir, finalDir)
	if err != nil {
		log.Println("Failed to move temp directory:", writer.tmpDir, finalDir, err)
		return "", err
	}
	writer.finished = true
	writer.Close()
	return name, nil
}

func getTempWriter(repo Repository, descr Description) (io.WriteCloser, string, error) {
	tmpDir, err := repo.makeTempDir()
	if err != nil {
		log.Printf("Failed to create temp dir: %v\n", err)
		return nil, "", err
	}

	repoPaths := getPathsForMedia(tmpDir)

	if err = ioutil.WriteFile(repoPaths.typePath, []byte(descr.Type), 0660); err != nil {
		log.Printf("Failed to create typeFile: %q\n", err)
		return nil, "", err
	}

	tmpFile, err := NewLimitedFileWriter(repoPaths.contentPath, repo.MaxBytes)
	if err != nil {
		log.Printf("Failed to create limited file: %v\n", err)
		return nil, "", err
	}

	return tmpFile, tmpDir, nil
}
