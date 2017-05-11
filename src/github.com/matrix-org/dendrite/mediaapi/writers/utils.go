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

	log "github.com/Sirupsen/logrus"
	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/dendrite/mediaapi/types"
	"github.com/matrix-org/util"
)

// createTempDir creates a tmp/<random string> directory within baseDirectory and returns its path
func createTempDir(baseDirectory types.Path) (types.Path, error) {
	baseTmpDir := path.Join(string(baseDirectory), "tmp")
	err := os.MkdirAll(baseTmpDir, 0770)
	if err != nil {
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

func createTempFileWriter(basePath types.Path, logger *log.Entry) (*bufio.Writer, *os.File, types.Path, *util.JSONResponse) {
	tmpDir, err := createTempDir(basePath)
	if err != nil {
		logger.Infof("Failed to create temp dir %q\n", err)
		return nil, nil, "", &util.JSONResponse{
			Code: 400,
			JSON: jsonerror.Unknown(fmt.Sprintf("Failed to upload: %q", err)),
		}
	}
	writer, tmpFile, err := createFileWriter(tmpDir, "content")
	if err != nil {
		logger.Infof("Failed to create file writer %q\n", err)
		return nil, nil, "", &util.JSONResponse{
			Code: 400,
			JSON: jsonerror.Unknown(fmt.Sprintf("Failed to upload: %q", err)),
		}
	}
	return writer, tmpFile, tmpDir, nil
}

func getPathFromMediaMetadata(m *types.MediaMetadata, basePath types.Path) string {
	return path.Join(
		string(basePath),
		string(m.Origin),
		string(m.MediaID[:3]),
		string(m.MediaID[3:]),
	)
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
