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
	"database/sql"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"strings"

	log "github.com/Sirupsen/logrus"
	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/dendrite/mediaapi/config"
	"github.com/matrix-org/dendrite/mediaapi/fileutils"
	"github.com/matrix-org/dendrite/mediaapi/storage"
	"github.com/matrix-org/dendrite/mediaapi/types"
	"github.com/matrix-org/util"
)

// uploadRequest metadata included in or derivable from an upload request
// https://matrix.org/docs/spec/client_server/r0.2.0.html#post-matrix-media-r0-upload
// NOTE: The members come from HTTP request metadata such as headers, query parameters or can be derived from such
type uploadRequest struct {
	MediaMetadata *types.MediaMetadata
	Logger        *log.Entry
}

// uploadResponse defines the format of the JSON response
// https://matrix.org/docs/spec/client_server/r0.2.0.html#post-matrix-media-r0-upload
type uploadResponse struct {
	ContentURI string `json:"content_uri"`
}

// Upload implements /upload
// This endpoint involves uploading potentially significant amounts of data to the homeserver.
// This implementation supports a configurable maximum file size limit in bytes. If a user tries to upload more than this, they will receive an error that their upload is too large.
// Uploaded files are processed piece-wise to avoid DoS attacks which would starve the server of memory.
// TODO: We should time out requests if they have not received any data within a configured timeout period.
func Upload(req *http.Request, cfg *config.MediaAPI, db *storage.Database) util.JSONResponse {
	r, resErr := parseAndValidateRequest(req, cfg)
	if resErr != nil {
		return *resErr
	}

	r.Logger.WithFields(log.Fields{
		"Origin":              r.MediaMetadata.Origin,
		"UploadName":          r.MediaMetadata.UploadName,
		"FileSizeBytes":       r.MediaMetadata.FileSizeBytes,
		"Content-Type":        r.MediaMetadata.ContentType,
		"Content-Disposition": r.MediaMetadata.ContentDisposition,
	}).Info("Uploading file")

	// The file data is hashed and the hash is used as the MediaID. The hash is useful as a
	// method of deduplicating files to save storage, as well as a way to conduct
	// integrity checks on the file data in the repository.
	hash, bytesWritten, tmpDir, copyError := fileutils.WriteTempFile(req.Body, cfg.MaxFileSizeBytes, cfg.AbsBasePath)
	if copyError != nil {
		logFields := log.Fields{
			"Origin":  r.MediaMetadata.Origin,
			"MediaID": r.MediaMetadata.MediaID,
		}
		if copyError == fileutils.ErrFileIsTooLarge {
			logFields["MaxFileSizeBytes"] = cfg.MaxFileSizeBytes
		}
		r.Logger.WithError(copyError).WithFields(logFields).Warn("Error while transferring file")
		fileutils.RemoveDir(tmpDir, r.Logger)
		return util.JSONResponse{
			Code: 400,
			JSON: jsonerror.Unknown(fmt.Sprintf("Failed to upload")),
		}
	}

	r.MediaMetadata.FileSizeBytes = bytesWritten
	r.MediaMetadata.Base64Hash = hash
	r.MediaMetadata.MediaID = types.MediaID(hash)

	r.Logger.WithFields(log.Fields{
		"MediaID":             r.MediaMetadata.MediaID,
		"Origin":              r.MediaMetadata.Origin,
		"Base64Hash":          r.MediaMetadata.Base64Hash,
		"UploadName":          r.MediaMetadata.UploadName,
		"FileSizeBytes":       r.MediaMetadata.FileSizeBytes,
		"Content-Type":        r.MediaMetadata.ContentType,
		"Content-Disposition": r.MediaMetadata.ContentDisposition,
	}).Info("File uploaded")

	// check if we already have a record of the media in our database and if so, we can remove the temporary directory
	mediaMetadata, err := db.GetMediaMetadata(r.MediaMetadata.MediaID, r.MediaMetadata.Origin)
	if err == nil {
		r.MediaMetadata = mediaMetadata
		fileutils.RemoveDir(tmpDir, r.Logger)
		return util.JSONResponse{
			Code: 200,
			JSON: uploadResponse{
				ContentURI: fmt.Sprintf("mxc://%s/%s", cfg.ServerName, r.MediaMetadata.MediaID),
			},
		}
	} else if err != sql.ErrNoRows {
		r.Logger.WithError(err).WithField("MediaID", r.MediaMetadata.MediaID).Warn("Failed to query database")
	}

	// TODO: generate thumbnails

	resErr = r.storeFileAndMetadata(tmpDir, cfg.AbsBasePath, db)
	if resErr != nil {
		return *resErr
	}

	return util.JSONResponse{
		Code: 200,
		JSON: uploadResponse{
			ContentURI: fmt.Sprintf("mxc://%s/%s", cfg.ServerName, r.MediaMetadata.MediaID),
		},
	}
}

// parseAndValidateRequest parses the incoming upload request to validate and extract
// all the metadata about the media being uploaded.
// Returns either an uploadRequest or an error formatted as a util.JSONResponse
func parseAndValidateRequest(req *http.Request, cfg *config.MediaAPI) (*uploadRequest, *util.JSONResponse) {
	if req.Method != "POST" {
		return nil, &util.JSONResponse{
			Code: 400,
			JSON: jsonerror.Unknown("HTTP request method must be POST."),
		}
	}

	r := &uploadRequest{
		MediaMetadata: &types.MediaMetadata{
			Origin:             cfg.ServerName,
			ContentDisposition: types.ContentDisposition(req.Header.Get("Content-Disposition")),
			FileSizeBytes:      types.FileSizeBytes(req.ContentLength),
			ContentType:        types.ContentType(req.Header.Get("Content-Type")),
			UploadName:         types.Filename(url.PathEscape(req.FormValue("filename"))),
		},
		Logger: util.GetLogger(req.Context()),
	}

	if resErr := r.Validate(cfg.MaxFileSizeBytes); resErr != nil {
		return nil, resErr
	}

	if len(r.MediaMetadata.UploadName) > 0 {
		r.MediaMetadata.ContentDisposition = types.ContentDisposition(
			"inline; filename*=utf-8''" + string(r.MediaMetadata.UploadName),
		)
	}

	return r, nil
}

// Validate validates the uploadRequest fields
func (r *uploadRequest) Validate(maxFileSizeBytes types.FileSizeBytes) *util.JSONResponse {
	if r.MediaMetadata.FileSizeBytes < 1 {
		return &util.JSONResponse{
			Code: 400,
			JSON: jsonerror.Unknown("HTTP Content-Length request header must be greater than zero."),
		}
	}
	if maxFileSizeBytes > 0 && r.MediaMetadata.FileSizeBytes > maxFileSizeBytes {
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
	if r.MediaMetadata.UploadName[0] == '~' {
		return &util.JSONResponse{
			Code: 400,
			JSON: jsonerror.Unknown("File name must not begin with '~'."),
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

// storeFileAndMetadata first moves a temporary file named content from tmpDir to its
// final path (see getPathFromMediaMetadata for details.) Once the file is moved, the
// metadata about the file is written into the media repository database. This order
// of operations is important as it avoids metadata entering the database before the file
// is ready and if we fail to move the file, it never gets added to the database.
// In case of any error, appropriate files and directories are cleaned up a
// util.JSONResponse error is returned.
func (r *uploadRequest) storeFileAndMetadata(tmpDir types.Path, absBasePath types.Path, db *storage.Database) *util.JSONResponse {
	finalPath, duplicate, err := fileutils.MoveFileWithHashCheck(tmpDir, r.MediaMetadata, absBasePath, r.Logger)
	if err != nil {
		r.Logger.WithError(err).Error("Failed to move file.")
		return &util.JSONResponse{
			Code: 400,
			JSON: jsonerror.Unknown(fmt.Sprintf("Failed to upload")),
		}
	}
	if duplicate {
		r.Logger.WithField("dst", finalPath).Info("File was stored previously - discarding duplicate")
	}

	if err = db.StoreMediaMetadata(r.MediaMetadata); err != nil {
		r.Logger.WithError(err).Warn("Failed to store metadata")
		// If the file is a duplicate (has the same hash as an existing file) then
		// there is valid metadata in the database for that file. As such we only
		// remove the file if it is not a duplicate.
		if duplicate == false {
			fileutils.RemoveDir(types.Path(path.Dir(finalPath)), r.Logger)
		}
		return &util.JSONResponse{
			Code: 400,
			JSON: jsonerror.Unknown(fmt.Sprintf("Failed to upload")),
		}
	}

	return nil
}
