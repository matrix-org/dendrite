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
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strconv"

	log "github.com/Sirupsen/logrus"
	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/dendrite/mediaapi/config"
	"github.com/matrix-org/dendrite/mediaapi/fileutils"
	"github.com/matrix-org/dendrite/mediaapi/storage"
	"github.com/matrix-org/dendrite/mediaapi/types"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
)

const mediaIDCharacters = "A-Za-z0-9_=-"

// Note: unfortunately regex.MustCompile() cannot be assigned to a const
var mediaIDRegex = regexp.MustCompile("[" + mediaIDCharacters + "]+")

// downloadRequest metadata included in or derivable from an download request
// https://matrix.org/docs/spec/client_server/r0.2.0.html#get-matrix-media-r0-download-servername-mediaid
type downloadRequest struct {
	MediaMetadata *types.MediaMetadata
	Logger        *log.Entry
}

// Download implements /download
// Files from this server (i.e. origin == cfg.ServerName) are served directly
func Download(w http.ResponseWriter, req *http.Request, origin gomatrixserverlib.ServerName, mediaID types.MediaID, cfg *config.MediaAPI, db *storage.Database, activeRemoteRequests *types.ActiveRemoteRequests) {
	r := &downloadRequest{
		MediaMetadata: &types.MediaMetadata{
			MediaID: mediaID,
			Origin:  origin,
		},
		Logger: util.GetLogger(req.Context()).WithFields(log.Fields{
			"Origin":  origin,
			"MediaID": mediaID,
		}),
	}

	// request validation
	if req.Method != "GET" {
		r.jsonErrorResponse(w, util.JSONResponse{
			Code: 405,
			JSON: jsonerror.Unknown("request method must be GET"),
		})
		return
	}

	if resErr := r.Validate(); resErr != nil {
		r.jsonErrorResponse(w, *resErr)
		return
	}

	if resErr := r.doDownload(w, cfg, db, activeRemoteRequests); resErr != nil {
		r.jsonErrorResponse(w, *resErr)
		return
	}
}

func (r *downloadRequest) jsonErrorResponse(w http.ResponseWriter, res util.JSONResponse) {
	// Marshal JSON response into raw bytes to send as the HTTP body
	resBytes, err := json.Marshal(res.JSON)
	if err != nil {
		r.Logger.WithError(err).Error("Failed to marshal JSONResponse")
		// this should never fail to be marshalled so drop err to the floor
		res = util.MessageResponse(500, "Internal Server Error")
		resBytes, _ = json.Marshal(res.JSON)
	}

	// Set status code and write the body
	w.WriteHeader(res.Code)
	r.Logger.WithField("code", res.Code).Infof("Responding (%d bytes)", len(resBytes))
	w.Write(resBytes)
}

// Validate validates the downloadRequest fields
func (r *downloadRequest) Validate() *util.JSONResponse {
	if mediaIDRegex.MatchString(string(r.MediaMetadata.MediaID)) == false {
		return &util.JSONResponse{
			Code: 404,
			JSON: jsonerror.NotFound(fmt.Sprintf("mediaId must be a non-empty string using only characters in %v", mediaIDCharacters)),
		}
	}
	// Note: the origin will be validated either by comparison to the configured server name of this homeserver
	// or by a DNS SRV record lookup when creating a request for remote files
	if r.MediaMetadata.Origin == "" {
		return &util.JSONResponse{
			Code: 404,
			JSON: jsonerror.NotFound("serverName must be a non-empty string"),
		}
	}
	return nil
}

func (r *downloadRequest) doDownload(w http.ResponseWriter, cfg *config.MediaAPI, db *storage.Database, activeRemoteRequests *types.ActiveRemoteRequests) *util.JSONResponse {
	// check if we have a record of the media in our database
	mediaMetadata, err := db.GetMediaMetadata(r.MediaMetadata.MediaID, r.MediaMetadata.Origin)
	if err != nil {
		r.Logger.WithError(err).Error("Error querying the database.")
		return &util.JSONResponse{
			Code: 500,
			JSON: jsonerror.InternalServerError(),
		}
	}
	if mediaMetadata == nil {
		if r.MediaMetadata.Origin == cfg.ServerName {
			// If we do not have a record and the origin is local, the file is not found
			return &util.JSONResponse{
				Code: 404,
				JSON: jsonerror.NotFound(fmt.Sprintf("File with media ID %q does not exist", r.MediaMetadata.MediaID)),
			}
		}
		// TODO: If we do not have a record and the origin is remote, we need to fetch it and respond with that file
		return &util.JSONResponse{
			Code: 404,
			JSON: jsonerror.NotFound(fmt.Sprintf("File with media ID %q does not exist", r.MediaMetadata.MediaID)),
		}
	}
	// If we have a record, we can respond from the local file
	r.MediaMetadata = mediaMetadata
	return r.respondFromLocalFile(w, cfg.AbsBasePath)
}

// respondFromLocalFile reads a file from local storage and writes it to the http.ResponseWriter
// Returns a util.JSONResponse error in case of error
func (r *downloadRequest) respondFromLocalFile(w http.ResponseWriter, absBasePath types.Path) *util.JSONResponse {
	filePath, err := fileutils.GetPathFromBase64Hash(r.MediaMetadata.Base64Hash, absBasePath)
	if err != nil {
		r.Logger.WithError(err).Error("Failed to get file path from metadata")
		return &util.JSONResponse{
			Code: 500,
			JSON: jsonerror.InternalServerError(),
		}
	}
	file, err := os.Open(filePath)
	defer file.Close()
	if err != nil {
		r.Logger.WithError(err).Error("Failed to open file")
		return &util.JSONResponse{
			Code: 500,
			JSON: jsonerror.InternalServerError(),
		}
	}
	stat, err := file.Stat()
	if err != nil {
		r.Logger.WithError(err).Error("Failed to stat file")
		return &util.JSONResponse{
			Code: 500,
			JSON: jsonerror.InternalServerError(),
		}
	}

	if r.MediaMetadata.FileSizeBytes > 0 && int64(r.MediaMetadata.FileSizeBytes) != stat.Size() {
		r.Logger.WithFields(log.Fields{
			"fileSizeDatabase": r.MediaMetadata.FileSizeBytes,
			"fileSizeDisk":     stat.Size(),
		}).Warn("File size in database and on-disk differ.")
		return &util.JSONResponse{
			Code: 500,
			JSON: jsonerror.InternalServerError(),
		}
	}

	r.Logger.WithFields(log.Fields{
		"UploadName":    r.MediaMetadata.UploadName,
		"Base64Hash":    r.MediaMetadata.Base64Hash,
		"FileSizeBytes": r.MediaMetadata.FileSizeBytes,
		"Content-Type":  r.MediaMetadata.ContentType,
	}).Info("Responding with file")

	w.Header().Set("Content-Type", string(r.MediaMetadata.ContentType))
	w.Header().Set("Content-Length", strconv.FormatInt(int64(r.MediaMetadata.FileSizeBytes), 10))
	contentSecurityPolicy := "default-src 'none';" +
		" script-src 'none';" +
		" plugin-types application/pdf;" +
		" style-src 'unsafe-inline';" +
		" object-src 'self';"
	w.Header().Set("Content-Security-Policy", contentSecurityPolicy)

	if bytesResponded, err := io.Copy(w, file); err != nil {
		r.Logger.WithError(err).Warn("Failed to copy from cache")
		if bytesResponded == 0 {
			return &util.JSONResponse{
				Code: 500,
				JSON: jsonerror.NotFound(fmt.Sprintf("Failed to respond with file with media ID %q", r.MediaMetadata.MediaID)),
			}
		}
		// If we have written any data then we have already responded with 200 OK and all we can do is close the connection
		// FIXME: close the connection here or just return?
		r.closeConnection(w)
	}
	return nil
}

func (r *downloadRequest) closeConnection(w http.ResponseWriter) {
	r.Logger.Info("Attempting to close the connection.")
	hijacker, ok := w.(http.Hijacker)
	if ok {
		connection, _, hijackErr := hijacker.Hijack()
		if hijackErr == nil {
			r.Logger.Info("Closing")
			connection.Close()
		} else {
			r.Logger.WithError(hijackErr).Warn("Error trying to hijack and close connection")
		}
	}
}
