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
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"

	log "github.com/Sirupsen/logrus"
	"github.com/matrix-org/dendrite/clientapi/auth"
	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/dendrite/mediaapi/config"
	"github.com/matrix-org/dendrite/mediaapi/storage"
	"github.com/matrix-org/dendrite/mediaapi/types"
	"github.com/matrix-org/util"
)

// uploadRequest metadata included in or derivable from an upload request
// https://matrix.org/docs/spec/client_server/r0.2.0.html#post-matrix-media-r0-upload
// NOTE: The members come from HTTP request metadata such as headers, query parameters or can be derived from such
type uploadRequest struct {
	MediaMetadata *types.MediaMetadata
}

// Validate validates the uploadRequest fields
func (r uploadRequest) Validate(maxFileSizeBytes types.ContentLength) *util.JSONResponse {
	// TODO: Any validation to be done on ContentDisposition?

	if r.MediaMetadata.ContentLength < 1 {
		return &util.JSONResponse{
			Code: 400,
			JSON: jsonerror.Unknown("HTTP Content-Length request header must be greater than zero."),
		}
	}
	if maxFileSizeBytes > 0 && r.MediaMetadata.ContentLength > maxFileSizeBytes {
		return &util.JSONResponse{
			Code: 400,
			JSON: jsonerror.Unknown(fmt.Sprintf("HTTP Content-Length is greater than the maximum allowed upload size (%v).", maxFileSizeBytes)),
		}
	}
	// TODO: Check if the Content-Type is a valid type?
	if r.MediaMetadata.ContentType == "" {
		return &util.JSONResponse{
			Code: 400,
			JSON: jsonerror.Unknown("HTTP Content-Type request header must be set."),
		}
	}
	// TODO: Validate filename - what are the valid characters?
	if r.MediaMetadata.UserID != "" {
		// TODO: We should put user ID parsing code into gomatrixserverlib and use that instead
		//       (see https://github.com/matrix-org/gomatrixserverlib/blob/3394e7c7003312043208aa73727d2256eea3d1f6/eventcontent.go#L347 )
		//       It should be a struct (with pointers into a single string to avoid copying) and
		//       we should update all refs to use UserID types rather than strings.
		// https://github.com/matrix-org/synapse/blob/v0.19.2/synapse/types.py#L92
		if len(r.MediaMetadata.UserID) == 0 || r.MediaMetadata.UserID[0] != '@' {
			return &util.JSONResponse{
				Code: 400,
				JSON: jsonerror.Unknown("user id must start with '@'"),
			}
		}
		parts := strings.SplitN(string(r.MediaMetadata.UserID[1:]), ":", 2)
		if len(parts) != 2 {
			return &util.JSONResponse{
				Code: 400,
				JSON: jsonerror.BadJSON("user id must be in the form @localpart:domain"),
			}
		}
	}
	return nil
}

// https://matrix.org/docs/spec/client_server/r0.2.0.html#post-matrix-media-r0-upload
type uploadResponse struct {
	ContentURI string `json:"content_uri"`
}

func removeDir(dir types.Path, logger *log.Entry) {
	dirErr := os.RemoveAll(string(dir))
	if dirErr != nil {
		logger.Warnf("Failed to remove directory (%v): %q\n", dir, dirErr)
	}
}

// Upload implements /upload
//
// This endpoint involves uploading potentially significant amounts of data to the homeserver.
// This implementation supports a configurable maximum file size limit in bytes. If a user tries to upload more than this, they will receive an error that their upload is too large.
// Uploaded files are processed piece-wise to avoid DoS attacks which would starve the server of memory.
// TODO: Requests time out if they have not received any data within the configured timeout period.
func Upload(req *http.Request, cfg *config.MediaAPI, db *storage.Database) util.JSONResponse {
	logger := util.GetLogger(req.Context())

	if req.Method != "POST" {
		return util.JSONResponse{
			Code: 400,
			JSON: jsonerror.Unknown("HTTP request method must be POST."),
		}
	}

	// FIXME: This will require querying some other component/db but currently
	// just accepts a user id for auth
	userID, resErr := auth.VerifyAccessToken(req)
	if resErr != nil {
		return *resErr
	}

	r := &uploadRequest{
		MediaMetadata: &types.MediaMetadata{
			Origin:             cfg.ServerName,
			ContentDisposition: types.ContentDisposition(req.Header.Get("Content-Disposition")),
			ContentLength:      types.ContentLength(req.ContentLength),
			ContentType:        types.ContentType(req.Header.Get("Content-Type")),
			UploadName:         types.Filename(req.FormValue("filename")),
			UserID:             types.MatrixUserID(userID),
		},
	}

	if resErr = r.Validate(cfg.MaxFileSizeBytes); resErr != nil {
		return *resErr
	}

	if len(r.MediaMetadata.UploadName) > 0 {
		r.MediaMetadata.ContentDisposition = types.ContentDisposition(
			"inline; filename*=utf-8''" + url.PathEscape(string(r.MediaMetadata.UploadName)),
		)
	}

	logger.WithFields(log.Fields{
		"Origin":              r.MediaMetadata.Origin,
		"UploadName":          r.MediaMetadata.UploadName,
		"Content-Length":      r.MediaMetadata.ContentLength,
		"Content-Type":        r.MediaMetadata.ContentType,
		"Content-Disposition": r.MediaMetadata.ContentDisposition,
	}).Info("Uploading file")

	writer, file, tmpDir, errorResponse := createTempFileWriter(cfg.BasePath, logger)
	if errorResponse != nil {
		return *errorResponse
	}
	defer file.Close()

	// The limited reader restricts how many bytes are read from the body to the specified maximum bytes
	// Note: the golang HTTP server closes the request body
	limitedBody := io.LimitReader(req.Body, int64(cfg.MaxFileSizeBytes))
	hasher := sha256.New()
	reader := io.TeeReader(limitedBody, hasher)

	bytesWritten, err := io.Copy(writer, reader)
	if err != nil {
		logger.Warnf("Failed to copy %q\n", err)
		removeDir(tmpDir, logger)
		return util.JSONResponse{
			Code: 400,
			JSON: jsonerror.Unknown(fmt.Sprintf("Failed to upload")),
		}
	}

	writer.Flush()

	if bytesWritten != int64(r.MediaMetadata.ContentLength) {
		logger.Warnf("Bytes uploaded (%v) != claimed Content-Length (%v)", bytesWritten, r.MediaMetadata.ContentLength)
	}

	hash := hasher.Sum(nil)
	r.MediaMetadata.MediaID = types.MediaID(base64.URLEncoding.EncodeToString(hash[:]))

	logger.WithFields(log.Fields{
		"MediaID":             r.MediaMetadata.MediaID,
		"Origin":              r.MediaMetadata.Origin,
		"UploadName":          r.MediaMetadata.UploadName,
		"Content-Length":      r.MediaMetadata.ContentLength,
		"Content-Type":        r.MediaMetadata.ContentType,
		"Content-Disposition": r.MediaMetadata.ContentDisposition,
	}).Info("File uploaded")

	// check if we already have a record of the media in our database and if so, we can remove the temporary directory
	mediaMetadata, err := db.GetMediaMetadata(r.MediaMetadata.MediaID, r.MediaMetadata.Origin)
	if err == nil {
		r.MediaMetadata = mediaMetadata
		removeDir(tmpDir, logger)
		return util.JSONResponse{
			Code: 200,
			JSON: uploadResponse{
				ContentURI: fmt.Sprintf("mxc://%s/%s", cfg.ServerName, r.MediaMetadata.MediaID),
			},
		}
	} else if err != sql.ErrNoRows {
		logger.Warnf("Failed to query database for %v: %q", r.MediaMetadata.MediaID, err)
	}

	// TODO: generate thumbnails

	finalPath := getPathFromMediaMetadata(r.MediaMetadata, cfg.BasePath)

	err = moveFile(
		types.Path(path.Join(string(tmpDir), "content")),
		types.Path(finalPath),
	)
	if err != nil {
		logger.Warnf("Failed to move file to final destination: %q\n", err)
		removeDir(tmpDir, logger)
		return util.JSONResponse{
			Code: 400,
			JSON: jsonerror.Unknown(fmt.Sprintf("Failed to upload")),
		}
	}

	err = db.StoreMediaMetadata(r.MediaMetadata)
	if err != nil {
		logger.Warnf("Failed to store metadata: %q\n", err)
		removeDir(types.Path(path.Dir(finalPath)), logger)
		return util.JSONResponse{
			Code: 400,
			JSON: jsonerror.Unknown(fmt.Sprintf("Failed to upload")),
		}
	}

	return util.JSONResponse{
		Code: 200,
		JSON: uploadResponse{
			ContentURI: fmt.Sprintf("mxc://%s/%s", cfg.ServerName, r.MediaMetadata.MediaID),
		},
	}
}
