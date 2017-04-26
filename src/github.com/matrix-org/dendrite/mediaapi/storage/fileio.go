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
	"fmt"
	"os"

	log "github.com/Sirupsen/logrus"
)

// LimitedFileWriter writes only a limited number of bytes to a file.
//
// If the callee attempts to write more bytes the file is deleted and further
// writes are silently discarded.
//
// This isn't thread safe.
type LimitedFileWriter struct {
	filePath     string
	file         *os.File
	writtenBytes uint64
	maxBytes     uint64
}

// NewLimitedFileWriter creates a new LimitedFileWriter at the given location.
//
// If a file already exists at the location it is immediately truncated.
//
// A maxBytes of 0 or negative is treated as no limit.
func NewLimitedFileWriter(filePath string, maxBytes uint64) (*LimitedFileWriter, error) {
	file, err := os.Create(filePath)
	if err != nil {
		return nil, err
	}

	writer := LimitedFileWriter{
		filePath: filePath,
		file:     file,
		maxBytes: maxBytes,
	}

	return &writer, nil
}

// Close closes the underlying file descriptor, if its open.
//
// Any error comes from File.Close
func (writer *LimitedFileWriter) Close() error {
	if writer.file != nil {
		file := writer.file
		writer.file = nil
		return file.Close()
	}
	return nil
}

func (writer *LimitedFileWriter) Write(p []byte) (n int, err error) {
	if writer.maxBytes > 0 && uint64(len(p))+writer.writtenBytes > writer.maxBytes {
		if writer.file != nil {
			writer.Close()
			err = os.Remove(writer.filePath)
			if err != nil {
				log.Printf("Failed to delete file %v\n", err)
			}
		}

		return 0, fmt.Errorf("Reached limit")
	}

	if writer.file != nil {
		n, err = writer.file.Write(p)
		writer.writtenBytes += uint64(n)

		if err != nil {
			log.Printf("Failed to write to file %v\n", err)
		}
	}

	return
}
