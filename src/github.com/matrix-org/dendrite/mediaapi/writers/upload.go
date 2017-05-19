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
		logger.WithError(dirErr).WithField("dir", dir).Warn("Failed to remove directory")
	}
}

// parseAndValidateRequest parses the incoming upload request to validate and extract
// all the metadata about the media being uploaded. Returns either an uploadRequest or
// an error formatted as a util.JSONResponse
func parseAndValidateRequest(req *http.Request, cfg *config.MediaAPI) (*uploadRequest, *util.JSONResponse) {
	if req.Method != "POST" {
		return nil, &util.JSONResponse{
			Code: 400,
			JSON: jsonerror.Unknown("HTTP request method must be POST."),
		}
	}

	// FIXME: This will require querying some other component/db but currently
	// just accepts a user id for auth
	userID, resErr := auth.VerifyAccessToken(req)
	if resErr != nil {
		return nil, resErr
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
		return nil, resErr
	}

	if len(r.MediaMetadata.UploadName) > 0 {
		r.MediaMetadata.ContentDisposition = types.ContentDisposition(
			"inline; filename*=utf-8''" + url.PathEscape(string(r.MediaMetadata.UploadName)),
		)
	}

	return r, nil
}

// writeFileWithLimitAndHash reads data from an io.Reader and writes it to a temporary
// file named 'content' in the returned temporary directory. It only reads up to a limit of
// cfg.MaxFileSizeBytes from the io.Reader. The data written is hashed and the hashsum is
// returned. If any errors occur, a util.JSONResponse error is returned.
func writeFileWithLimitAndHash(r io.Reader, cfg *config.MediaAPI, logger *log.Entry, contentLength types.ContentLength) ([]byte, types.Path, *util.JSONResponse) {
	writer, file, tmpDir, errorResponse := createTempFileWriter(cfg.AbsBasePath, logger)
	if errorResponse != nil {
		return nil, "", errorResponse
	}
	defer file.Close()

	// The limited reader restricts how many bytes are read from the body to the specified maximum bytes
	// Note: the golang HTTP server closes the request body
	limitedBody := io.LimitReader(r, int64(cfg.MaxFileSizeBytes))
	// The file data is hashed and the hash is returned. The hash is useful as a
	// method of deduplicating files to save storage, as well as a way to conduct
	// integrity checks on the file data in the repository. The hash gets used as
	// the MediaID.
	hasher := sha256.New()
	// A TeeReader is used to allow us to read from the limitedBody and simultaneously
	// write to the hasher here and to the http.ResponseWriter via the io.Copy call below.
	reader := io.TeeReader(limitedBody, hasher)

	bytesWritten, err := io.Copy(writer, reader)
	if err != nil {
		logger.WithError(err).Warn("Failed to copy")
		removeDir(tmpDir, logger)
		return nil, "", &util.JSONResponse{
			Code: 400,
			JSON: jsonerror.Unknown(fmt.Sprintf("Failed to upload")),
		}
	}

	writer.Flush()

	if bytesWritten != int64(contentLength) {
		logger.WithFields(log.Fields{
			"bytesWritten":  bytesWritten,
			"contentLength": contentLength,
		}).Warn("Fewer bytes written than expected")
	}

	return hasher.Sum(nil), tmpDir, nil
}

// storeFileAndMetadata first moves a temporary file named content from tmpDir to its
// final path (see getPathFromMediaMetadata for details.) Once the file is moved, the
// metadata about the file is written into the media repository database. This order
// of operations is important as it avoids metadata entering the database before the file
// is ready and if we fail to move the file, it never gets added to the database.
// In case of any error, appropriate files and directories are cleaned up a
// util.JSONResponse error is returned.
func storeFileAndMetadata(tmpDir types.Path, absBasePath types.Path, mediaMetadata *types.MediaMetadata, db *storage.Database, logger *log.Entry) *util.JSONResponse {
	finalPath, err := getPathFromMediaMetadata(mediaMetadata, absBasePath)
	if err != nil {
		logger.WithError(err).Warn("Failed to get file path from metadata")
		removeDir(tmpDir, logger)
		return &util.JSONResponse{
			Code: 400,
			JSON: jsonerror.Unknown(fmt.Sprintf("Failed to upload")),
		}
	}

	err = moveFile(
		types.Path(path.Join(string(tmpDir), "content")),
		types.Path(finalPath),
	)
	if err != nil {
		logger.WithError(err).WithField("dst", finalPath).Warn("Failed to move file to final destination")
		removeDir(tmpDir, logger)
		return &util.JSONResponse{
			Code: 400,
			JSON: jsonerror.Unknown(fmt.Sprintf("Failed to upload")),
		}
	}

	if err = db.StoreMediaMetadata(mediaMetadata); err != nil {
		logger.WithError(err).Warn("Failed to store metadata")
		removeDir(types.Path(path.Dir(finalPath)), logger)
		return &util.JSONResponse{
			Code: 400,
			JSON: jsonerror.Unknown(fmt.Sprintf("Failed to upload")),
		}
	}

	return nil
}

// Upload implements /upload
//
// This endpoint involves uploading potentially significant amounts of data to the homeserver.
// This implementation supports a configurable maximum file size limit in bytes. If a user tries to upload more than this, they will receive an error that their upload is too large.
// Uploaded files are processed piece-wise to avoid DoS attacks which would starve the server of memory.
// TODO: We should time out requests if they have not received any data within a configured timeout period.
func Upload(req *http.Request, cfg *config.MediaAPI, db *storage.Database) util.JSONResponse {
	logger := util.GetLogger(req.Context())

	r, resErr := parseAndValidateRequest(req, cfg)
	if resErr != nil {
		return *resErr
	}

	logger.WithFields(log.Fields{
		"Origin":              r.MediaMetadata.Origin,
		"UploadName":          r.MediaMetadata.UploadName,
		"Content-Length":      r.MediaMetadata.ContentLength,
		"Content-Type":        r.MediaMetadata.ContentType,
		"Content-Disposition": r.MediaMetadata.ContentDisposition,
	}).Info("Uploading file")

	// The file data is hashed and the hash is used as the MediaID. The hash is useful as a
	// method of deduplicating files to save storage, as well as a way to conduct
	// integrity checks on the file data in the repository.
	hash, tmpDir, resErr := writeFileWithLimitAndHash(req.Body, cfg, logger, r.MediaMetadata.ContentLength)
	if resErr != nil {
		return *resErr
	}
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
		logger.WithError(err).WithField("MediaID", r.MediaMetadata.MediaID).Warn("Failed to query database")
	}

	// TODO: generate thumbnails

	resErr = storeFileAndMetadata(tmpDir, cfg.AbsBasePath, r.MediaMetadata, db, logger)
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
