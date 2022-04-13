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

package types

import (
	"sync"

	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/gomatrixserverlib"
)

// FileSizeBytes is a file size in bytes
type FileSizeBytes int64

// ContentType is an HTTP Content-Type header string representing the MIME type of a request body
type ContentType string

// Filename is a string representing the name of a file
type Filename string

// Base64Hash is a base64 URLEncoding string representation of a SHA-256 hash sum
type Base64Hash string

// Path is an absolute or relative UNIX filesystem path
type Path string

// MediaID is a string representing the unique identifier for a file (could be a hash but does not have to be)
type MediaID string

// RequestMethod is an HTTP request method i.e. GET, POST, etc
type RequestMethod string

// MatrixUserID is a Matrix user ID string in the form @user:domain e.g. @alice:matrix.org
type MatrixUserID string

// MediaMetadata is metadata associated with a media file
type MediaMetadata struct {
	MediaID           MediaID
	Origin            gomatrixserverlib.ServerName
	ContentType       ContentType
	FileSizeBytes     FileSizeBytes
	CreationTimestamp gomatrixserverlib.Timestamp
	UploadName        Filename
	Base64Hash        Base64Hash
	UserID            MatrixUserID
}

// RemoteRequestResult is used for broadcasting the result of a request for a remote file to routines waiting on the condition
type RemoteRequestResult struct {
	// Condition used for the requester to signal the result to all other routines waiting on this condition
	Cond *sync.Cond
	// MediaMetadata of the requested file to avoid querying the database for every waiting routine
	MediaMetadata *MediaMetadata
	// An error, nil in case of no error.
	Error error
}

// ActiveRemoteRequests is a lockable map of media URIs requested from remote homeservers
// It is used for ensuring multiple requests for the same file do not clobber each other.
type ActiveRemoteRequests struct {
	sync.Mutex
	// The string key is an mxc:// URL
	MXCToResult map[string]*RemoteRequestResult
}

// ThumbnailSize contains a single thumbnail size configuration
type ThumbnailSize config.ThumbnailSize

// ThumbnailMetadata contains the metadata about an individual thumbnail
type ThumbnailMetadata struct {
	MediaMetadata *MediaMetadata
	ThumbnailSize ThumbnailSize
}

// ThumbnailGenerationResult is used for broadcasting the result of thumbnail generation to routines waiting on the condition
type ThumbnailGenerationResult struct {
	// Condition used for the generator to signal the result to all other routines waiting on this condition
	Cond *sync.Cond
	// Resulting error from the generation attempt
	Err error
}

// ActiveThumbnailGeneration is a lockable map of file paths being thumbnailed
// It is used to ensure thumbnails are only generated once.
type ActiveThumbnailGeneration struct {
	sync.Mutex
	// The string key is a thumbnail file path
	PathToResult map[string]*ThumbnailGenerationResult
}

// Crop indicates we should crop the thumbnail on resize
const Crop = "crop"

// Scale indicates we should scale the thumbnail on resize
const Scale = "scale"
