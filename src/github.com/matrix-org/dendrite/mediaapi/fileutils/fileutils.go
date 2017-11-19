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

package fileutils

import (
	"bufio"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/matrix-org/dendrite/common/config"
	"github.com/matrix-org/dendrite/mediaapi/types"
	log "github.com/sirupsen/logrus"
)

// GetPathFromBase64Hash evaluates the path to a media file from its Base64Hash
// 3 subdirectories are created for more manageable browsing and use the remainder as the file name.
// For example, if Base64Hash is 'qwerty', the path will be 'q/w/erty/file'.
func GetPathFromBase64Hash(base64Hash types.Base64Hash, absBasePath config.Path) (string, error) {
	if len(base64Hash) < 3 {
		return "", fmt.Errorf("Invalid filePath (Base64Hash too short - min 3 characters): %q", base64Hash)
	}
	if len(base64Hash) > 255 {
		return "", fmt.Errorf("Invalid filePath (Base64Hash too long - max 255 characters): %q", base64Hash)
	}

	filePath, err := filepath.Abs(filepath.Join(
		string(absBasePath),
		string(base64Hash[0:1]),
		string(base64Hash[1:2]),
		string(base64Hash[2:]),
		"file",
	))
	if err != nil {
		return "", fmt.Errorf("Unable to construct filePath: %q", err)
	}

	// check if the absolute absBasePath is a prefix of the absolute filePath
	// if so, no directory escape has occurred and the filePath is valid
	// Note: absBasePath is already absolute
	if !strings.HasPrefix(filePath, string(absBasePath)) {
		return "", fmt.Errorf("Invalid filePath (not within absBasePath %v): %v", absBasePath, filePath)
	}

	return filePath, nil
}

// MoveFileWithHashCheck checks for hash collisions when moving a temporary file to its final path based on metadata
// The final path is based on the hash of the file.
// If the final path exists and the file size matches, the file does not need to be moved.
// In error cases where the file is not a duplicate, the caller may decide to remove the final path.
// Returns the final path of the file, whether it is a duplicate and an error.
func MoveFileWithHashCheck(tmpDir types.Path, mediaMetadata *types.MediaMetadata, absBasePath config.Path, logger *log.Entry) (types.Path, bool, error) {
	// Note: in all error and success cases, we need to remove the temporary directory
	defer RemoveDir(tmpDir, logger)
	duplicate := false
	finalPath, err := GetPathFromBase64Hash(mediaMetadata.Base64Hash, absBasePath)
	if err != nil {
		return "", duplicate, fmt.Errorf("failed to get file path from metadata: %q", err)
	}

	var stat os.FileInfo
	// Note: The double-negative is intentional as os.IsExist(err) != !os.IsNotExist(err).
	// The functions are error checkers to be used in different cases.
	if stat, err = os.Stat(finalPath); !os.IsNotExist(err) {
		duplicate = true
		if stat.Size() == int64(mediaMetadata.FileSizeBytes) {
			return types.Path(finalPath), duplicate, nil
		}
		return "", duplicate, fmt.Errorf("downloaded file with hash collision but different file size (%v)", finalPath)
	}
	err = moveFile(
		types.Path(filepath.Join(string(tmpDir), "content")),
		types.Path(finalPath),
	)
	if err != nil {
		return "", duplicate, fmt.Errorf("failed to move file to final destination (%v): %q", finalPath, err)
	}
	return types.Path(finalPath), duplicate, nil
}

// RemoveDir removes a directory and logs a warning in case of errors
func RemoveDir(dir types.Path, logger *log.Entry) {
	dirErr := os.RemoveAll(string(dir))
	if dirErr != nil {
		logger.WithError(dirErr).WithField("dir", dir).Warn("Failed to remove directory")
	}
}

// WriteTempFile writes to a new temporary file
func WriteTempFile(reqReader io.Reader, maxFileSizeBytes config.FileSizeBytes, absBasePath config.Path) (hash types.Base64Hash, size types.FileSizeBytes, path types.Path, err error) {
	size = -1

	tmpFileWriter, tmpFile, tmpDir, err := createTempFileWriter(absBasePath)
	if err != nil {
		return
	}
	defer (func() { err = tmpFile.Close() })()

	// The amount of data read is limited to maxFileSizeBytes. At this point, if there is more data it will be truncated.
	limitedReader := io.LimitReader(reqReader, int64(maxFileSizeBytes))
	// Hash the file data. The hash will be returned. The hash is useful as a
	// method of deduplicating files to save storage, as well as a way to conduct
	// integrity checks on the file data in the repository.
	hasher := sha256.New()
	teeReader := io.TeeReader(limitedReader, hasher)
	bytesWritten, err := io.Copy(tmpFileWriter, teeReader)
	if err != nil && err != io.EOF {
		return
	}

	err = tmpFileWriter.Flush()
	if err != nil {
		return
	}

	hash = types.Base64Hash(base64.RawURLEncoding.EncodeToString(hasher.Sum(nil)[:]))
	size = types.FileSizeBytes(bytesWritten)
	path = tmpDir
	return
}

// moveFile attempts to move the file src to dst
func moveFile(src types.Path, dst types.Path) error {
	dstDir := filepath.Dir(string(dst))

	err := os.MkdirAll(dstDir, 0770)
	if err != nil {
		return fmt.Errorf("Failed to make directory: %q", err)
	}
	err = os.Rename(string(src), string(dst))
	if err != nil {
		return fmt.Errorf("Failed to move directory: %q", err)
	}
	return nil
}

func createTempFileWriter(absBasePath config.Path) (*bufio.Writer, *os.File, types.Path, error) {
	tmpDir, err := createTempDir(absBasePath)
	if err != nil {
		return nil, nil, "", fmt.Errorf("Failed to create temp dir: %q", err)
	}
	writer, tmpFile, err := createFileWriter(tmpDir)
	if err != nil {
		return nil, nil, "", fmt.Errorf("Failed to create file writer: %q", err)
	}
	return writer, tmpFile, tmpDir, nil
}

// createTempDir creates a tmp/<random string> directory within baseDirectory and returns its path
func createTempDir(baseDirectory config.Path) (types.Path, error) {
	baseTmpDir := filepath.Join(string(baseDirectory), "tmp")
	if err := os.MkdirAll(baseTmpDir, 0770); err != nil {
		return "", fmt.Errorf("Failed to create base temp dir: %v", err)
	}
	tmpDir, err := ioutil.TempDir(baseTmpDir, "")
	if err != nil {
		return "", fmt.Errorf("Failed to create temp dir: %v", err)
	}
	return types.Path(tmpDir), nil
}

// createFileWriter creates a buffered file writer with a new file
// The caller should flush the writer before closing the file.
// Returns the file handle as it needs to be closed when writing is complete
func createFileWriter(directory types.Path) (*bufio.Writer, *os.File, error) {
	filePath := filepath.Join(string(directory), "content")
	file, err := os.Create(filePath)
	if err != nil {
		return nil, nil, fmt.Errorf("Failed to create file: %v", err)
	}

	return bufio.NewWriter(file), file, nil
}
