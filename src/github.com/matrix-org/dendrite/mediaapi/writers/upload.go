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
	"fmt"
	"io"
	"net/http"
	"strings"

	log "github.com/Sirupsen/logrus"
	"github.com/matrix-org/dendrite/clientapi/auth"
	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/dendrite/mediaapi/config"
	"github.com/matrix-org/dendrite/mediaapi/storage"
	"github.com/matrix-org/util"
)

// UploadRequest metadata included in or derivable from an upload request
// https://matrix.org/docs/spec/client_server/r0.2.0.html#post-matrix-media-r0-upload
// NOTE: ContentType is an HTTP request header and Filename is passed as a query parameter
type UploadRequest struct {
	ContentDisposition string
	ContentLength      int64
	ContentType        string
	Filename           string
	Base64FileHash     string
	Method             string
	UserID             string
}

// Validate validates the UploadRequest fields
func (r UploadRequest) Validate() *util.JSONResponse {
	// TODO: Any validation to be done on ContentDisposition?
	if r.ContentLength < 1 {
		return &util.JSONResponse{
			Code: 400,
			JSON: jsonerror.BadJSON("HTTP Content-Length request header must be greater than zero."),
		}
	}
	// TODO: Check if the Content-Type is a valid type?
	if r.ContentType == "" {
		return &util.JSONResponse{
			Code: 400,
			JSON: jsonerror.BadJSON("HTTP Content-Type request header must be set."),
		}
	}
	// TODO: Validate filename - what are the valid characters?
	if r.Method != "POST" {
		return &util.JSONResponse{
			Code: 400,
			JSON: jsonerror.BadJSON("HTTP request method must be POST."),
		}
	}
	if r.UserID != "" {
		// TODO: We should put user ID parsing code into gomatrixserverlib and use that instead
		//       (see https://github.com/matrix-org/gomatrixserverlib/blob/3394e7c7003312043208aa73727d2256eea3d1f6/eventcontent.go#L347 )
		//       It should be a struct (with pointers into a single string to avoid copying) and
		//       we should update all refs to use UserID types rather than strings.
		// https://github.com/matrix-org/synapse/blob/v0.19.2/synapse/types.py#L92
		if len(r.UserID) == 0 || r.UserID[0] != '@' {
			return &util.JSONResponse{
				Code: 400,
				JSON: jsonerror.BadJSON("user id must start with '@'"),
			}
		}
		parts := strings.SplitN(r.UserID[1:], ":", 2)
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

// Upload implements /upload
func Upload(req *http.Request, cfg config.MediaAPI, db *storage.Database, repo *storage.Repository) util.JSONResponse {
	logger := util.GetLogger(req.Context())

	// FIXME: This will require querying some other component/db but currently
	// just accepts a user id for auth
	userID, resErr := auth.VerifyAccessToken(req)
	if resErr != nil {
		return *resErr
	}

	r := &UploadRequest{
		ContentDisposition: req.Header.Get("Content-Disposition"),
		ContentLength:      req.ContentLength,
		ContentType:        req.Header.Get("Content-Type"),
		Filename:           req.FormValue("filename"),
		Method:             req.Method,
		UserID:             userID,
	}

	if resErr = r.Validate(); resErr != nil {
		return *resErr
	}

	logger.WithFields(log.Fields{
		"ContentType": r.ContentType,
		"Filename":    r.Filename,
		"UserID":      r.UserID,
	}).Info("Uploading file")

	// TODO: Store file to disk
	//       - make path to file
	//       - progressive writing (could support Content-Length 0 and cut off
	//         after some max upload size is exceeded)
	//       - generate id (ideally a hash but a random string to start with)
	writer, err := repo.WriterToLocalRepository(storage.Description{
		Type: r.ContentType,
	})
	if err != nil {
		logger.Infof("Failed to get cache writer %q\n", err)
		return util.JSONResponse{
			Code: 400,
			JSON: jsonerror.BadJSON(fmt.Sprintf("Failed to upload: %q", err)),
		}
	}

	defer writer.Close()

	if _, err = io.Copy(writer, req.Body); err != nil {
		logger.Infof("Failed to copy %q\n", err)
		return util.JSONResponse{
			Code: 400,
			JSON: jsonerror.BadJSON(fmt.Sprintf("Failed to upload: %q", err)),
		}
	}

	r.Base64FileHash, err = writer.Finished()
	if err != nil {
		return util.JSONResponse{
			Code: 400,
			JSON: jsonerror.BadJSON(fmt.Sprintf("Failed to upload: %q", err)),
		}
	}
	// TODO: check if file with hash already exists

	// TODO: generate thumbnails

	err = db.CreateMedia(r.Base64FileHash, cfg.ServerName, r.ContentType, r.ContentDisposition, r.ContentLength, r.Filename, r.UserID)
	if err != nil {
		return util.JSONResponse{
			Code: 400,
			JSON: jsonerror.BadJSON(fmt.Sprintf("Failed to upload: %q", err)),
		}
	}

	return util.JSONResponse{
		Code: 200,
		JSON: uploadResponse{
			ContentURI: fmt.Sprintf("mxc://%s/%s", cfg.ServerName, r.Base64FileHash),
		},
	}
}
