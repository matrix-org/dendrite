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
	"mime"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"sync"

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
// Files from remote servers (i.e. origin != cfg.ServerName) are cached locally.
// If they are present in the cache, they are served directly.
// If they are not present in the cache, they are obtained from the remote server and
// simultaneously served back to the client and written into the cache.
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
		resErr := jsonerror.InternalServerError()
		return &resErr
	}
	if mediaMetadata == nil {
		if r.MediaMetadata.Origin == cfg.ServerName {
			// If we do not have a record and the origin is local, the file is not found
			return &util.JSONResponse{
				Code: 404,
				JSON: jsonerror.NotFound(fmt.Sprintf("File with media ID %q does not exist", r.MediaMetadata.MediaID)),
			}
		}
		// If we do not have a record and the origin is remote, we need to fetch it and respond with that file
		resErr := r.getRemoteFile(cfg, db, activeRemoteRequests)
		if resErr != nil {
			return resErr
		}
	} else {
		// If we have a record, we can respond from the local file
		r.MediaMetadata = mediaMetadata
	}
	return r.respondFromLocalFile(w, cfg.AbsBasePath)
}

// respondFromLocalFile reads a file from local storage and writes it to the http.ResponseWriter
// Returns a util.JSONResponse error in case of error
func (r *downloadRequest) respondFromLocalFile(w http.ResponseWriter, absBasePath types.Path) *util.JSONResponse {
	filePath, err := fileutils.GetPathFromBase64Hash(r.MediaMetadata.Base64Hash, absBasePath)
	if err != nil {
		r.Logger.WithError(err).Error("Failed to get file path from metadata")
		resErr := jsonerror.InternalServerError()
		return &resErr
	}
	file, err := os.Open(filePath)
	defer file.Close()
	if err != nil {
		r.Logger.WithError(err).Error("Failed to open file")
		resErr := jsonerror.InternalServerError()
		return &resErr
	}
	stat, err := file.Stat()
	if err != nil {
		r.Logger.WithError(err).Error("Failed to stat file")
		resErr := jsonerror.InternalServerError()
		return &resErr
	}

	if r.MediaMetadata.FileSizeBytes > 0 && int64(r.MediaMetadata.FileSizeBytes) != stat.Size() {
		r.Logger.WithFields(log.Fields{
			"fileSizeDatabase": r.MediaMetadata.FileSizeBytes,
			"fileSizeDisk":     stat.Size(),
		}).Warn("File size in database and on-disk differ.")
		resErr := jsonerror.InternalServerError()
		return &resErr
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
			resErr := jsonerror.InternalServerError()
			return &resErr
		}
		// If we have written any data then we have already responded with 200 OK and all we can do is close the connection
		return nil
	}
	return nil
}

// getRemoteFile fetches the remote file and caches it locally
// A hash map of active remote requests to a struct containing a sync.Cond is used to only download remote files once,
// regardless of how many download requests are received.
// Note: The named errorResponse return variable is used in a deferred broadcast of the metadata and error response to waiting goroutines.
// Returns a util.JSONResponse error in case of error
func (r *downloadRequest) getRemoteFile(cfg *config.MediaAPI, db *storage.Database, activeRemoteRequests *types.ActiveRemoteRequests) (errorResponse *util.JSONResponse) {
	// Note: getMediaMetadataFromActiveRequest uses mutexes and conditions from activeRemoteRequests
	mediaMetadata, resErr := r.getMediaMetadataFromActiveRequest(activeRemoteRequests)
	if resErr != nil {
		return resErr
	} else if mediaMetadata != nil {
		// If we got metadata from an active request, we can respond from the local file
		r.MediaMetadata = mediaMetadata
	} else {
		// Note: This is an active request that MUST broadcastMediaMetadata to wake up waiting goroutines!
		// Note: broadcastMediaMetadata uses mutexes and conditions from activeRemoteRequests
		defer func() {
			// Note: errorResponse is the named return variable so we wrap this in a closure to re-evaluate the arguments at defer-time
			if err := recover(); err != nil {
				resErr := jsonerror.InternalServerError()
				r.broadcastMediaMetadata(activeRemoteRequests, &resErr)
				panic(err)
			}
			r.broadcastMediaMetadata(activeRemoteRequests, errorResponse)
		}()

		// check if we have a record of the media in our database
		mediaMetadata, err := db.GetMediaMetadata(r.MediaMetadata.MediaID, r.MediaMetadata.Origin)
		if err != nil {
			r.Logger.WithError(err).Error("Error querying the database.")
			resErr := jsonerror.InternalServerError()
			return &resErr
		}

		if mediaMetadata == nil {
			// If we do not have a record, we need to fetch the remote file first and then respond from the local file
			resErr := r.fetchRemoteFileAndStoreMetadata(cfg.AbsBasePath, cfg.MaxFileSizeBytes, db)
			if resErr != nil {
				return resErr
			}
		} else {
			// If we have a record, we can respond from the local file
			r.MediaMetadata = mediaMetadata
		}
	}
	return
}

func (r *downloadRequest) getMediaMetadataFromActiveRequest(activeRemoteRequests *types.ActiveRemoteRequests) (*types.MediaMetadata, *util.JSONResponse) {
	// Check if there is an active remote request for the file
	mxcURL := "mxc://" + string(r.MediaMetadata.Origin) + "/" + string(r.MediaMetadata.MediaID)

	activeRemoteRequests.Lock()
	defer activeRemoteRequests.Unlock()

	if activeRemoteRequestResult, ok := activeRemoteRequests.MXCToResult[mxcURL]; ok {
		r.Logger.Info("Waiting for another goroutine to fetch the remote file.")

		// NOTE: Wait unlocks and locks again internally. There is still a deferred Unlock() that will unlock this.
		activeRemoteRequestResult.Cond.Wait()
		if activeRemoteRequestResult.ErrorResponse != nil {
			return nil, activeRemoteRequestResult.ErrorResponse
		}

		if activeRemoteRequestResult.MediaMetadata == nil {
			return nil, &util.JSONResponse{
				Code: 404,
				JSON: jsonerror.NotFound("File not found."),
			}
		}

		return activeRemoteRequestResult.MediaMetadata, nil
	}

	// No active remote request so create one
	activeRemoteRequests.MXCToResult[mxcURL] = &types.RemoteRequestResult{
		Cond: &sync.Cond{L: activeRemoteRequests},
	}

	return nil, nil
}

// broadcastMediaMetadata broadcasts the media metadata and error response to waiting goroutines
// Only the owner of the activeRemoteRequestResult for this origin and media ID should call this function.
func (r *downloadRequest) broadcastMediaMetadata(activeRemoteRequests *types.ActiveRemoteRequests, errorResponse *util.JSONResponse) {
	activeRemoteRequests.Lock()
	defer activeRemoteRequests.Unlock()
	mxcURL := "mxc://" + string(r.MediaMetadata.Origin) + "/" + string(r.MediaMetadata.MediaID)
	if activeRemoteRequestResult, ok := activeRemoteRequests.MXCToResult[mxcURL]; ok {
		r.Logger.Info("Signalling other goroutines waiting for this goroutine to fetch the file.")
		activeRemoteRequestResult.MediaMetadata = r.MediaMetadata
		activeRemoteRequestResult.ErrorResponse = errorResponse
		activeRemoteRequestResult.Cond.Broadcast()
	}
	delete(activeRemoteRequests.MXCToResult, mxcURL)
}

// fetchRemoteFileAndStoreMetadata fetches the file from the remote server and stores its metadata in the database
func (r *downloadRequest) fetchRemoteFileAndStoreMetadata(absBasePath types.Path, maxFileSizeBytes types.FileSizeBytes, db *storage.Database) *util.JSONResponse {
	finalPath, duplicate, resErr := r.fetchRemoteFile(absBasePath, maxFileSizeBytes)
	if resErr != nil {
		return resErr
	}

	r.Logger.WithFields(log.Fields{
		"Base64Hash":    r.MediaMetadata.Base64Hash,
		"UploadName":    r.MediaMetadata.UploadName,
		"FileSizeBytes": r.MediaMetadata.FileSizeBytes,
		"Content-Type":  r.MediaMetadata.ContentType,
	}).Info("Storing file metadata to media repository database")

	// FIXME: timeout db request
	if err := db.StoreMediaMetadata(r.MediaMetadata); err != nil {
		// If the file is a duplicate (has the same hash as an existing file) then
		// there is valid metadata in the database for that file. As such we only
		// remove the file if it is not a duplicate.
		if duplicate == false {
			finalDir := filepath.Dir(string(finalPath))
			fileutils.RemoveDir(types.Path(finalDir), r.Logger)
		}
		// NOTE: It should really not be possible to fail the uniqueness test here so
		// there is no need to handle that separately
		resErr := jsonerror.InternalServerError()
		return &resErr
	}

	// TODO: generate thumbnails

	r.Logger.WithFields(log.Fields{
		"UploadName":    r.MediaMetadata.UploadName,
		"Base64Hash":    r.MediaMetadata.Base64Hash,
		"FileSizeBytes": r.MediaMetadata.FileSizeBytes,
		"Content-Type":  r.MediaMetadata.ContentType,
	}).Infof("Remote file cached")

	return nil
}

func (r *downloadRequest) fetchRemoteFile(absBasePath types.Path, maxFileSizeBytes types.FileSizeBytes) (types.Path, bool, *util.JSONResponse) {
	r.Logger.Info("Fetching remote file")

	// create request for remote file
	resp, resErr := r.createRemoteRequest()
	if resErr != nil {
		return "", false, resErr
	}
	defer resp.Body.Close()

	// get metadata from request and set metadata on response
	contentLength, err := strconv.ParseInt(resp.Header.Get("Content-Length"), 10, 64)
	if err != nil {
		r.Logger.WithError(err).Warn("Failed to parse content length")
		return "", false, &util.JSONResponse{
			Code: 502,
			JSON: jsonerror.Unknown("Invalid response from remote server"),
		}
	}
	if contentLength > int64(maxFileSizeBytes) {
		return "", false, &util.JSONResponse{
			Code: 413,
			JSON: jsonerror.Unknown(fmt.Sprintf("Remote file is too large (%v > %v bytes)", contentLength, maxFileSizeBytes)),
		}
	}
	r.MediaMetadata.FileSizeBytes = types.FileSizeBytes(contentLength)
	r.MediaMetadata.ContentType = types.ContentType(resp.Header.Get("Content-Type"))
	_, params, err := mime.ParseMediaType(resp.Header.Get("Content-Disposition"))
	if err == nil && params["filename"] != "" {
		r.MediaMetadata.UploadName = types.Filename(params["filename"])
	}

	r.Logger.Info("Transferring remote file")

	// The file data is hashed but is NOT used as the MediaID, unlike in Upload. The hash is useful as a
	// method of deduplicating files to save storage, as well as a way to conduct
	// integrity checks on the file data in the repository.
	// Data is truncated to maxFileSizeBytes. Content-Length was reported as 0 < Content-Length <= maxFileSizeBytes so this is OK.
	hash, bytesWritten, tmpDir, err := fileutils.WriteTempFile(resp.Body, maxFileSizeBytes, absBasePath)
	if err != nil {
		r.Logger.WithError(err).WithFields(log.Fields{
			"MaxFileSizeBytes": maxFileSizeBytes,
		}).Warn("Error while downloading file from remote server")
		fileutils.RemoveDir(tmpDir, r.Logger)
		return "", false, &util.JSONResponse{
			Code: 502,
			JSON: jsonerror.Unknown("File could not be downloaded from remote server"),
		}
	}

	r.Logger.Info("Remote file transferred")

	// It's possible the bytesWritten to the temporary file is different to the reported Content-Length from the remote
	// request's response. bytesWritten is therefore used as it is what would be sent to clients when reading from the local
	// file.
	r.MediaMetadata.FileSizeBytes = types.FileSizeBytes(bytesWritten)
	r.MediaMetadata.Base64Hash = hash

	// The database is the source of truth so we need to have moved the file first
	finalPath, duplicate, err := fileutils.MoveFileWithHashCheck(tmpDir, r.MediaMetadata, absBasePath, r.Logger)
	if err != nil {
		r.Logger.WithError(err).Error("Failed to move file.")
		resErr := jsonerror.InternalServerError()
		return "", false, &resErr
	}
	if duplicate {
		r.Logger.WithField("dst", finalPath).Info("File was stored previously - discarding duplicate")
		// Continue on to store the metadata in the database
	}

	return types.Path(finalPath), duplicate, nil
}

func (r *downloadRequest) createRemoteRequest() (*http.Response, *util.JSONResponse) {
	dnsResult, err := gomatrixserverlib.LookupServer(r.MediaMetadata.Origin)
	if err != nil {
		if dnsErr, ok := err.(*net.DNSError); ok && dnsErr.Timeout() {
			return nil, &util.JSONResponse{
				Code: 504,
				JSON: jsonerror.Unknown(fmt.Sprintf("DNS look up for homeserver at %v timed out", r.MediaMetadata.Origin)),
			}
		}
		resErr := jsonerror.InternalServerError()
		return nil, &resErr
	}
	httpsURL := "https://" + dnsResult.Addrs[0]

	r.Logger.WithField("URL", httpsURL).Info("Connecting to remote")

	remoteReqAddr := httpsURL + "/_matrix/media/v1/download/" + string(r.MediaMetadata.Origin) + "/" + string(r.MediaMetadata.MediaID)
	remoteReq, err := http.NewRequest("GET", remoteReqAddr, nil)
	if err != nil {
		resErr := jsonerror.InternalServerError()
		return nil, &resErr
	}

	remoteReq.Header.Set("Host", string(r.MediaMetadata.Origin))

	client := http.Client{}
	resp, err := client.Do(remoteReq)
	if err != nil {
		r.Logger.Warn("Failed to execute request for remote file")
		return nil, &util.JSONResponse{
			Code: 502,
			JSON: jsonerror.Unknown(fmt.Sprintf("File with media ID %q could not be downloaded from %q", r.MediaMetadata.MediaID, r.MediaMetadata.Origin)),
		}
	}

	if resp.StatusCode != 200 {
		if resp.StatusCode == 404 {
			return nil, &util.JSONResponse{
				Code: 404,
				JSON: jsonerror.NotFound(fmt.Sprintf("File with media ID %q does not exist", r.MediaMetadata.MediaID)),
			}
		}
		r.Logger.WithFields(log.Fields{
			"StatusCode": resp.StatusCode,
		}).Warn("Received error response")
		return nil, &util.JSONResponse{
			Code: 502,
			JSON: jsonerror.Unknown(fmt.Sprintf("File with media ID %q could not be downloaded from %q", r.MediaMetadata.MediaID, r.MediaMetadata.Origin)),
		}
	}

	return resp, nil
}
