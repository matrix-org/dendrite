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
	"hash"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"

	log "github.com/Sirupsen/logrus"
	"github.com/matrix-org/dendrite/mediaapi/types"
)

// RemoveDir removes a directory and logs a warning in case of errors
func RemoveDir(dir types.Path, logger *log.Entry) {
	dirErr := os.RemoveAll(string(dir))
	if dirErr != nil {
		logger.WithError(dirErr).WithField("dir", dir).Warn("Failed to remove directory")
	}
}

// createTempDir creates a tmp/<random string> directory within baseDirectory and returns its path
func createTempDir(baseDirectory types.Path) (types.Path, error) {
	baseTmpDir := path.Join(string(baseDirectory), "tmp")
	if err := os.MkdirAll(baseTmpDir, 0770); err != nil {
		return "", fmt.Errorf("Failed to create base temp dir: %v", err)
	}
	tmpDir, err := ioutil.TempDir(baseTmpDir, "")
	if err != nil {
		return "", fmt.Errorf("Failed to create temp dir: %v", err)
	}
	return types.Path(tmpDir), nil
}

// createFileWriter creates a buffered file writer with a new file at directory/filename
// Returns the file handle as it needs to be closed when writing is complete
func createFileWriter(directory types.Path, filename types.Filename) (*bufio.Writer, *os.File, error) {
	filePath := path.Join(string(directory), string(filename))
	file, err := os.Create(filePath)
	if err != nil {
		return nil, nil, fmt.Errorf("Failed to create file: %v", err)
	}

	return bufio.NewWriter(file), file, nil
}

func createTempFileWriter(absBasePath types.Path) (*bufio.Writer, *os.File, types.Path, error) {
	tmpDir, err := createTempDir(absBasePath)
	if err != nil {
		return nil, nil, "", fmt.Errorf("Failed to create temp dir: %q", err)
	}
	writer, tmpFile, err := createFileWriter(tmpDir, "content")
	if err != nil {
		return nil, nil, "", fmt.Errorf("Failed to create file writer: %q", err)
	}
	return writer, tmpFile, tmpDir, nil
}

var (
	// ErrFileIsTooLarge indicates that the uploaded file is larger than the configured maximum file size
	ErrFileIsTooLarge = fmt.Errorf("file is too large")
	errRead           = fmt.Errorf("failed to read response from remote server")
	errResponse       = fmt.Errorf("failed to write file data to response body")
	errHash           = fmt.Errorf("failed to hash file data")
	errWrite          = fmt.Errorf("failed to write file to disk")
)

// writeToResponse takes bytesToWrite bytes from buffer and writes them to respWriter
// Returns bytes written and an error. In case of error, or if there is no respWriter,
// the number of bytes written will be 0.
func writeToResponse(respWriter http.ResponseWriter, buffer []byte, bytesToWrite int) (int64, error) {
	if respWriter != nil {
		bytesWritten, respErr := respWriter.Write(buffer[:bytesToWrite])
		if bytesWritten != bytesToWrite || (respErr != nil && respErr != io.EOF) {
			return 0, errResponse
		}
		return int64(bytesWritten), nil
	}
	return 0, nil
}

// writeToDiskAndHasher takes bytesToWrite bytes from buffer and writes them to tmpFileWriter and hasher.
// Returns bytes written and an error. In case of error, including if writing would exceed maxFileSizeBytes,
// the number of bytes written will be 0.
func writeToDiskAndHasher(tmpFileWriter *bufio.Writer, hasher hash.Hash, bytesWritten int64, maxFileSizeBytes types.ContentLength, buffer []byte, bytesToWrite int) (int64, error) {
	// if larger than maxFileSizeBytes then stop writing to disk and discard cached file
	if bytesWritten+int64(bytesToWrite) > int64(maxFileSizeBytes) {
		return 0, ErrFileIsTooLarge
	}
	// write to hasher and to disk
	bytesTemp, writeErr := tmpFileWriter.Write(buffer[:bytesToWrite])
	bytesHashed, hashErr := hasher.Write(buffer[:bytesToWrite])
	if writeErr != nil && writeErr != io.EOF || bytesTemp != bytesToWrite || bytesTemp != bytesHashed {
		return 0, errWrite
	} else if hashErr != nil && hashErr != io.EOF {
		return 0, errHash
	}
	return int64(bytesTemp), nil
}

// ReadAndHashAndWriteWithLimit works like io.Copy except it copies from the reqReader to the
// optionally-supplied respWriter and a temporary file named 'content' using a bufio.Writer.
// The data written to disk is hashed using the SHA-256 algorithm.
// If there is an error with the reqReader or the respWriter, that is considered an error.
// If there is an error with the hasher or tmpFileWriter, that is non-critical and copying
// to the respWriter continues.
// maxFileSizeBytes limits the amount of data written to disk and the hasher.
// If a respWriter is provided, all the data will be proxied from the reqReader to
// the respWriter, regardless of errors or limits on writing to disk.
// Returns all of the hash sum, bytes written to disk, and temporary directory path, or an error.
func ReadAndHashAndWriteWithLimit(reqReader io.Reader, maxFileSizeBytes types.ContentLength, absBasePath types.Path, respWriter http.ResponseWriter) (types.Base64Hash, types.ContentLength, types.ContentLength, types.Path, error) {
	// create the temporary file writer
	tmpFileWriter, tmpFile, tmpDir, err := createTempFileWriter(absBasePath)
	if err != nil {
		return "", -1, -1, "", err
	}
	defer tmpFile.Close()

	// The file data is hashed and the hash is returned. The hash is useful as a
	// method of deduplicating files to save storage, as well as a way to conduct
	// integrity checks on the file data in the repository. The hash gets used as
	// the MediaID.
	hasher := sha256.New()

	// bytesResponded is the total number of bytes written to the response to the client request
	// bytesWritten is the total number of bytes written to disk
	var bytesResponded, bytesWritten int64 = 0, 0
	var bytesTemp int64
	var copyError error
	// Note: the buffer size is the same as is used in io.Copy()
	buffer := make([]byte, 32*1024)
	for {
		// read from remote request's response body
		bytesRead, readErr := reqReader.Read(buffer)
		if bytesRead > 0 {
			// Note: This code allows proxying files larger than maxFileSizeBytes!
			// write to client request's response body
			bytesTemp, copyError = writeToResponse(respWriter, buffer, bytesRead)
			bytesResponded += bytesTemp
			if copyError == nil {
				// Note: if we get here then copyError != ErrFileIsTooLarge && copyError != errWrite
				//   as if copyError == errResponse || copyError == errWrite then we would have broken
				//   out of the loop and there are no other cases
				bytesTemp, copyError = writeToDiskAndHasher(tmpFileWriter, hasher, bytesWritten, maxFileSizeBytes, buffer, (bytesRead))
				bytesWritten += bytesTemp
				// If we do not have a respWriter then we are only writing to the hasher and tmpFileWriter. In that case, if we get an error, we need to break.
				if respWriter == nil && copyError != nil {
					break
				}
			}
		}
		if readErr != nil {
			if readErr != io.EOF {
				copyError = errRead
			}
			break
		}
	}

	if copyError != nil {
		return "", -1, -1, "", copyError
	}

	tmpFileWriter.Flush()

	hash := hasher.Sum(nil)
	return types.Base64Hash(base64.URLEncoding.EncodeToString(hash[:])), types.ContentLength(bytesResponded), types.ContentLength(bytesWritten), tmpDir, nil
}

// GetPathFromMediaMetadata validates and constructs the on-disk path to the media
// based on its origin and mediaID
// If a mediaID is too short, which could happen for other homeserver implementations,
// place it into a short-id subdirectory of the origin directory
// If the mediaID is long enough, we split it into pieces, creating up to 2 subdirectories
// for more manageable browsing and use the remainder as the file name. For example, if
// mediaID is 'qwerty', we create subdirectories 'q', 'w' within 'q' and place the file
// in 'q/w' calling it 'erty'. If the mediaID is shorter than 3 characters, the last
// character is the file name and the preceding character, if any, is a subdirectory name.
func GetPathFromMediaMetadata(m *types.MediaMetadata, absBasePath types.Path) (string, error) {
	var subPath, fileName string

	hashLen := len(m.Base64Hash)

	switch {
	case hashLen < 1:
		return "", fmt.Errorf("Invalid filePath (Base64Hash too short): %q", m.Base64Hash)
	case hashLen > 255:
		return "", fmt.Errorf("Invalid filePath (Base64Hash too long - max 255 characters): %q", m.Base64Hash)
	case hashLen < 2:
		subPath = ""
		fileName = string(m.Base64Hash)
	case hashLen < 3:
		subPath = string(m.Base64Hash[0:1])
		fileName = string(m.Base64Hash[1:])
	default:
		subPath = path.Join(
			string(m.Base64Hash[0:1]),
			string(m.Base64Hash[1:2]),
		)
		fileName = string(m.Base64Hash[2:])
	}

	filePath, err := filepath.Abs(path.Join(
		string(absBasePath),
		subPath,
		fileName,
	))
	if err != nil {
		return "", fmt.Errorf("Unable to construct filePath: %q", err)
	}

	// check if the absolute absBasePath is a prefix of the absolute filePath
	// if so, no directory escape has occurred and the filePath is valid
	// Note: absBasePath is already absolute
	if strings.HasPrefix(filePath, string(absBasePath)) == false {
		return "", fmt.Errorf("Invalid filePath (not within absBasePath %v): %v", absBasePath, filePath)
	}

	return filePath, nil
}

// moveFile attempts to move the file src to dst
func moveFile(src types.Path, dst types.Path) error {
	dstDir := path.Dir(string(dst))

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

// MoveFileWithHashCheck attempts to move the file src to dst and checks for hash collisions based on metadata
// Check if destination file exists. As the destination is based on a hash of the file data,
// if it exists and the content length does not match then there is a hash collision for two different files. If
// it exists and the content length matches, it is believable that it is the same file and we can just
// discard the temporary file.
func MoveFileWithHashCheck(tmpDir types.Path, mediaMetadata *types.MediaMetadata, absBasePath types.Path, logger *log.Entry) (string, bool, error) {
	duplicate := false
	finalPath, err := GetPathFromMediaMetadata(mediaMetadata, absBasePath)
	if err != nil {
		RemoveDir(tmpDir, logger)
		return "", duplicate, fmt.Errorf("failed to get file path from metadata: %q", err)
	}

	var stat os.FileInfo
	if stat, err = os.Stat(finalPath); os.IsExist(err) {
		duplicate = true
		if stat.Size() == int64(mediaMetadata.ContentLength) {
			RemoveDir(tmpDir, logger)
			return finalPath, duplicate, nil
		}
		// Remove the tmpDir as we anyway cannot cache the file on disk due to the hash collision
		RemoveDir(tmpDir, logger)
		return "", duplicate, fmt.Errorf("downloaded file with hash collision but different file size (%v)", finalPath)
	}
	err = moveFile(
		types.Path(path.Join(string(tmpDir), "content")),
		types.Path(finalPath),
	)
	if err != nil {
		RemoveDir(tmpDir, logger)
		return "", duplicate, fmt.Errorf("failed to move file to final destination (%v): %q", finalPath, err)
	}
	return finalPath, duplicate, nil
}
