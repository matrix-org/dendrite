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

package routing

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"

	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/dendrite/mediaapi/fileutils"
	"github.com/matrix-org/dendrite/mediaapi/storage"
	"github.com/matrix-org/dendrite/mediaapi/thumbnailer"
	"github.com/matrix-org/dendrite/mediaapi/types"
	"github.com/matrix-org/dendrite/setup/config"
	userapi "github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
	log "github.com/sirupsen/logrus"
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

// Upload implements POST /upload
// This endpoint involves uploading potentially significant amounts of data to the homeserver.
// This implementation supports a configurable maximum file size limit in bytes. If a user tries to upload more than this, they will receive an error that their upload is too large.
// Uploaded files are processed piece-wise to avoid DoS attacks which would starve the server of memory.
// TODO: We should time out requests if they have not received any data within a configured timeout period.
func Upload(req *http.Request, cfg *config.MediaAPI, dev *userapi.Device, db storage.Database, activeThumbnailGeneration *types.ActiveThumbnailGeneration) util.JSONResponse {
	r, resErr := parseAndValidateRequest(req, cfg, dev)
	if resErr != nil {
		return *resErr
	}

	if resErr = r.doUpload(req.Context(), req.Body, cfg, db, activeThumbnailGeneration); resErr != nil {
		return *resErr
	}

	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: uploadResponse{
			ContentURI: fmt.Sprintf("mxc://%s/%s", cfg.Matrix.ServerName, r.MediaMetadata.MediaID),
		},
	}
}

// parseAndValidateRequest parses the incoming upload request to validate and extract
// all the metadata about the media being uploaded.
// Returns either an uploadRequest or an error formatted as a util.JSONResponse
func parseAndValidateRequest(req *http.Request, cfg *config.MediaAPI, dev *userapi.Device) (*uploadRequest, *util.JSONResponse) {
	r := &uploadRequest{
		MediaMetadata: &types.MediaMetadata{
			Origin:        cfg.Matrix.ServerName,
			FileSizeBytes: types.FileSizeBytes(req.ContentLength),
			ContentType:   types.ContentType(req.Header.Get("Content-Type")),
			UploadName:    types.Filename(url.PathEscape(req.FormValue("filename"))),
			UserID:        types.MatrixUserID(dev.UserID),
		},
		Logger: util.GetLogger(req.Context()).WithField("Origin", cfg.Matrix.ServerName),
	}

	if resErr := r.Validate(*cfg.MaxFileSizeBytes); resErr != nil {
		return nil, resErr
	}

	return r, nil
}

func (r *uploadRequest) generateMediaID(ctx context.Context, db storage.Database) (types.MediaID, error) {
	for {
		// First try generating a meda ID. We'll do this by
		// generating some random bytes and then hex-encoding.
		mediaIDBytes := make([]byte, 32)
		_, err := rand.Read(mediaIDBytes)
		if err != nil {
			return "", fmt.Errorf("rand.Read: %w", err)
		}
		mediaID := types.MediaID(hex.EncodeToString(mediaIDBytes))
		// Then we will check if this media ID already exists in
		// our database. If it does then we had best generate a
		// new one.
		existingMetadata, err := db.GetMediaMetadata(ctx, mediaID, r.MediaMetadata.Origin)
		if err != nil {
			return "", fmt.Errorf("db.GetMediaMetadata: %w", err)
		}
		if existingMetadata != nil {
			// The media ID was already used - repeat the process
			// and generate a new one instead.
			continue
		}
		// The media ID was not already used - let's return that.
		return mediaID, nil
	}
}

func (r *uploadRequest) doUpload(
	ctx context.Context,
	reqReader io.Reader,
	cfg *config.MediaAPI,
	db storage.Database,
	activeThumbnailGeneration *types.ActiveThumbnailGeneration,
) *util.JSONResponse {
	r.Logger.WithFields(log.Fields{
		"UploadName":    r.MediaMetadata.UploadName,
		"FileSizeBytes": r.MediaMetadata.FileSizeBytes,
		"ContentType":   r.MediaMetadata.ContentType,
	}).Info("Uploading file")

	// The file data is hashed and the hash is used as the MediaID. The hash is useful as a
	// method of deduplicating files to save storage, as well as a way to conduct
	// integrity checks on the file data in the repository.
	// Data is truncated to maxFileSizeBytes. Content-Length was reported as 0 < Content-Length <= maxFileSizeBytes so this is OK.
	//
	// TODO: This has a bad API shape where you either need to call:
	//   fileutils.RemoveDir(tmpDir, r.Logger)
	// or call:
	//   r.storeFileAndMetadata(ctx, tmpDir, ...)
	// before you return from doUpload else we will leak a temp file. We could make this nicer with a `WithTransaction` style of
	// nested function to guarantee either storage or cleanup.
	if *cfg.MaxFileSizeBytes > 0 {
		if *cfg.MaxFileSizeBytes+1 <= 0 {
			r.Logger.WithFields(log.Fields{
				"MaxFileSizeBytes": *cfg.MaxFileSizeBytes,
			}).Warnf("Configured MaxFileSizeBytes overflows int64, defaulting to %d bytes", config.DefaultMaxFileSizeBytes)
			cfg.MaxFileSizeBytes = &config.DefaultMaxFileSizeBytes
		}
		reqReader = io.LimitReader(reqReader, int64(*cfg.MaxFileSizeBytes)+1)
	}

	hash, bytesWritten, tmpDir, err := fileutils.WriteTempFile(ctx, reqReader, cfg.AbsBasePath)
	if err != nil {
		r.Logger.WithError(err).WithFields(log.Fields{
			"MaxFileSizeBytes": *cfg.MaxFileSizeBytes,
		}).Warn("Error while transferring file")
		return &util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.Unknown("Failed to upload"),
		}
	}

	// Check if temp file size exceeds max file size configuration
	if *cfg.MaxFileSizeBytes > 0 && bytesWritten > types.FileSizeBytes(*cfg.MaxFileSizeBytes) {
		fileutils.RemoveDir(tmpDir, r.Logger) // delete temp file
		return requestEntityTooLargeJSONResponse(*cfg.MaxFileSizeBytes)
	}

	// Look up the media by the file hash. If we already have the file but under a
	// different media ID then we won't upload the file again - instead we'll just
	// add a new metadata entry that refers to the same file.
	existingMetadata, err := db.GetMediaMetadataByHash(
		ctx, hash, r.MediaMetadata.Origin,
	)
	if err != nil {
		fileutils.RemoveDir(tmpDir, r.Logger)
		r.Logger.WithError(err).Error("Error querying the database by hash.")
		resErr := jsonerror.InternalServerError()
		return &resErr
	}
	if existingMetadata != nil {
		// The file already exists, delete the uploaded temporary file.
		defer fileutils.RemoveDir(tmpDir, r.Logger)
		// The file already exists. Make a new media ID up for it.
		mediaID, merr := r.generateMediaID(ctx, db)
		if merr != nil {
			r.Logger.WithError(merr).Error("Failed to generate media ID for existing file")
			resErr := jsonerror.InternalServerError()
			return &resErr
		}

		// Then amend the upload metadata.
		r.MediaMetadata = &types.MediaMetadata{
			MediaID:           mediaID,
			Origin:            r.MediaMetadata.Origin,
			ContentType:       r.MediaMetadata.ContentType,
			FileSizeBytes:     r.MediaMetadata.FileSizeBytes,
			CreationTimestamp: r.MediaMetadata.CreationTimestamp,
			UploadName:        r.MediaMetadata.UploadName,
			Base64Hash:        hash,
			UserID:            r.MediaMetadata.UserID,
		}
	} else {
		// The file doesn't exist. Update the request metadata.
		r.MediaMetadata.FileSizeBytes = bytesWritten
		r.MediaMetadata.Base64Hash = hash
		r.MediaMetadata.MediaID, err = r.generateMediaID(ctx, db)
		if err != nil {
			fileutils.RemoveDir(tmpDir, r.Logger)
			r.Logger.WithError(err).Error("Failed to generate media ID for new upload")
			resErr := jsonerror.InternalServerError()
			return &resErr
		}
	}

	r.Logger = r.Logger.WithField("media_id", r.MediaMetadata.MediaID)
	r.Logger.WithFields(log.Fields{
		"Base64Hash":    r.MediaMetadata.Base64Hash,
		"UploadName":    r.MediaMetadata.UploadName,
		"FileSizeBytes": r.MediaMetadata.FileSizeBytes,
		"ContentType":   r.MediaMetadata.ContentType,
	}).Info("File uploaded")

	return r.storeFileAndMetadata(
		ctx, tmpDir, cfg.AbsBasePath, db, cfg.ThumbnailSizes,
		activeThumbnailGeneration, cfg.MaxThumbnailGenerators,
	)
}

func requestEntityTooLargeJSONResponse(maxFileSizeBytes config.FileSizeBytes) *util.JSONResponse {
	return &util.JSONResponse{
		Code: http.StatusRequestEntityTooLarge,
		JSON: jsonerror.Unknown(fmt.Sprintf("HTTP Content-Length is greater than the maximum allowed upload size (%v).", maxFileSizeBytes)),
	}
}

// Validate validates the uploadRequest fields
func (r *uploadRequest) Validate(maxFileSizeBytes config.FileSizeBytes) *util.JSONResponse {
	if maxFileSizeBytes > 0 && r.MediaMetadata.FileSizeBytes > types.FileSizeBytes(maxFileSizeBytes) {
		return requestEntityTooLargeJSONResponse(maxFileSizeBytes)
	}
	if strings.HasPrefix(string(r.MediaMetadata.UploadName), "~") {
		return &util.JSONResponse{
			Code: http.StatusBadRequest,
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
		if _, _, err := gomatrixserverlib.SplitID('@', string(r.MediaMetadata.UserID)); err != nil {
			return &util.JSONResponse{
				Code: http.StatusBadRequest,
				JSON: jsonerror.BadJSON("user id must be in the form @localpart:domain"),
			}
		}
	}
	return nil
}

// storeFileAndMetadata moves the temporary file to its final path based on metadata and stores the metadata in the database
// See getPathFromMediaMetadata in fileutils for details of the final path.
// The order of operations is important as it avoids metadata entering the database before the file
// is ready, and if we fail to move the file, it never gets added to the database.
// Returns a util.JSONResponse error and cleans up directories in case of error.
func (r *uploadRequest) storeFileAndMetadata(
	ctx context.Context,
	tmpDir types.Path,
	absBasePath config.Path,
	db storage.Database,
	thumbnailSizes []config.ThumbnailSize,
	activeThumbnailGeneration *types.ActiveThumbnailGeneration,
	maxThumbnailGenerators int,
) *util.JSONResponse {
	finalPath, duplicate, err := fileutils.MoveFileWithHashCheck(tmpDir, r.MediaMetadata, absBasePath, r.Logger)
	if err != nil {
		r.Logger.WithError(err).Error("Failed to move file.")
		return &util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.Unknown("Failed to upload"),
		}
	}
	if duplicate {
		r.Logger.WithField("dst", finalPath).Info("File was stored previously - discarding duplicate")
	}

	if err = db.StoreMediaMetadata(ctx, r.MediaMetadata); err != nil {
		r.Logger.WithError(err).Warn("Failed to store metadata")
		// If the file is a duplicate (has the same hash as an existing file) then
		// there is valid metadata in the database for that file. As such we only
		// remove the file if it is not a duplicate.
		if !duplicate {
			fileutils.RemoveDir(types.Path(path.Dir(string(finalPath))), r.Logger)
		}
		return &util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.Unknown("Failed to upload"),
		}
	}

	go func() {
		file, err := os.Open(string(finalPath))
		if err != nil {
			r.Logger.WithError(err).Error("unable to open file")
			return
		}
		defer file.Close() // nolint: errcheck
		// http.DetectContentType only needs 512 bytes
		buf := make([]byte, 512)
		_, err = file.Read(buf)
		if err != nil {
			r.Logger.WithError(err).Error("unable to read file")
			return
		}
		// Check if we need to generate thumbnails
		fileType := http.DetectContentType(buf)
		if !strings.HasPrefix(fileType, "image") {
			r.Logger.WithField("contentType", fileType).Debugf("uploaded file is not an image or can not be thumbnailed, not generating thumbnails")
			return
		}

		busy, err := thumbnailer.GenerateThumbnails(
			context.Background(), finalPath, thumbnailSizes, r.MediaMetadata,
			activeThumbnailGeneration, maxThumbnailGenerators, db, r.Logger,
		)
		if err != nil {
			r.Logger.WithError(err).Warn("Error generating thumbnails")
		}
		if busy {
			r.Logger.Warn("Maximum number of active thumbnail generators reached. Skipping pre-generation.")
		}
	}()

	return nil
}
