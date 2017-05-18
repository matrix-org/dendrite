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

package writers

import (
	"bufio"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"strings"

	log "github.com/Sirupsen/logrus"
	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/dendrite/mediaapi/types"
	"github.com/matrix-org/util"
)

// createTempDir creates a tmp/<random string> directory within baseDirectory and returns its path
func createTempDir(baseDirectory types.Path) (types.Path, error) {
	baseTmpDir := path.Join(string(baseDirectory), "tmp")
	if err := os.MkdirAll(baseTmpDir, 0770); err != nil {
		log.Printf("Failed to create base temp dir: %v\n", err)
		return "", err
	}
	tmpDir, err := ioutil.TempDir(baseTmpDir, "")
	if err != nil {
		log.Printf("Failed to create temp dir: %v\n", err)
		return "", err
	}
	return types.Path(tmpDir), nil
}

// createFileWriter creates a buffered file writer with a new file at directory/filename
// Returns the file handle as it needs to be closed when writing is complete
func createFileWriter(directory types.Path, filename types.Filename) (*bufio.Writer, *os.File, error) {
	filePath := path.Join(string(directory), string(filename))
	file, err := os.Create(filePath)
	if err != nil {
		log.Printf("Failed to create file: %v\n", err)
		return nil, nil, err
	}

	return bufio.NewWriter(file), file, nil
}

func createTempFileWriter(absBasePath types.Path, logger *log.Entry) (*bufio.Writer, *os.File, types.Path, *util.JSONResponse) {
	tmpDir, err := createTempDir(absBasePath)
	if err != nil {
		logger.Infof("Failed to create temp dir %q\n", err)
		return nil, nil, "", &util.JSONResponse{
			Code: 400,
			JSON: jsonerror.Unknown(fmt.Sprintf("Failed to upload")),
		}
	}
	writer, tmpFile, err := createFileWriter(tmpDir, "content")
	if err != nil {
		logger.Infof("Failed to create file writer %q\n", err)
		return nil, nil, "", &util.JSONResponse{
			Code: 400,
			JSON: jsonerror.Unknown(fmt.Sprintf("Failed to upload")),
		}
	}
	return writer, tmpFile, tmpDir, nil
}

// getPathFromMediaMetadata validates and constructs the on-disk path to the media
// based on its origin and mediaID
// If a mediaID is too short, which could happen for other homeserver implementations,
// place it into a short-id subdirectory of the origin directory
// If the mediaID is long enough, we split it in two using one part as a subdirectory
// name and the other part as the file name. This is to allow storage of more files due
// to filesystem limitations on the number of files in a directory. For example, if
// mediaID is 'qwerty', we create subdirectory called 'qwe' and place the file in 'qwe'
// and call it 'rty'.
func getPathFromMediaMetadata(m *types.MediaMetadata, absBasePath types.Path) (string, error) {
	var subDir string
	var fileName string

	if len(m.MediaID) > 3 {
		subDir = string(m.MediaID[:3])
		fileName = string(m.MediaID[3:])
	} else {
		subDir = "short-id"
		fileName = string(m.MediaID)
	}

	filePath, err := filepath.Abs(path.Join(
		string(absBasePath),
		string(m.Origin),
		subDir,
		fileName,
	))

	// FIXME:
	// - validate origin
	// - sanitize mediaID (e.g. '/' characters and such)
	// - validate length of origin and mediaID according to common filesystem limitations

	// check if the absolute absBasePath is a prefix of the absolute filePath
	// if so, no directory escape has occurred and the filePath is valid
	// Note: absBasePath is already absolute
	if err != nil || strings.HasPrefix(filePath, string(absBasePath)) == false {
		return "", fmt.Errorf("Invalid filePath (not within absBasePath %v): %v", absBasePath, filePath)
	}

	return filePath, nil
}

// moveFile attempts to move the file src to dst
func moveFile(src types.Path, dst types.Path) error {
	dstDir := path.Dir(string(dst))

	err := os.MkdirAll(dstDir, 0770)
	if err != nil {
		log.Printf("Failed to make directory: %q", err)
		return err
	}
	err = os.Rename(string(src), string(dst))
	if err != nil {
		log.Printf("Failed to move directory: %q", err)
		return err
	}
	return nil
}
