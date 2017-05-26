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
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path"
	"strconv"
	"strings"
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

// downloadRequest metadata included in or derivable from an download request
// https://matrix.org/docs/spec/client_server/r0.2.0.html#get-matrix-media-r0-download-servername-mediaid
type downloadRequest struct {
	MediaMetadata *types.MediaMetadata
	Logger        *log.Entry
}

// Validate validates the downloadRequest fields
func (r *downloadRequest) Validate() *util.JSONResponse {
	// FIXME: the following errors aren't bad JSON, rather just a bad request path
	// maybe give the URL pattern in the routing, these are not even possible as the handler would not be hit...?
	if r.MediaMetadata.MediaID == "" {
		return &util.JSONResponse{
			Code: 404,
			JSON: jsonerror.NotFound("mediaId must be a non-empty string"),
		}
	}
	if r.MediaMetadata.Origin == "" {
		return &util.JSONResponse{
			Code: 404,
			JSON: jsonerror.NotFound("serverName must be a non-empty string"),
		}
	}
	return nil
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
		Logger: util.GetLogger(req.Context()),
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

	// check if we have a record of the media in our database
	mediaMetadata, err := db.GetMediaMetadata(r.MediaMetadata.MediaID, r.MediaMetadata.Origin)

	if err == nil {
		// If we have a record, we can respond from the local file
		r.MediaMetadata = mediaMetadata
		r.respondFromLocalFile(w, cfg.AbsBasePath)
		return
	} else if err == sql.ErrNoRows && r.MediaMetadata.Origin != cfg.ServerName {
		// If we do not have a record and the origin is remote, we need to fetch it and respond with that file

		mxcURL := "mxc://" + string(r.MediaMetadata.Origin) + "/" + string(r.MediaMetadata.MediaID)

		// The following code using activeRemoteRequests is avoiding duplication of fetches from the remote server in the case
		// of multiple simultaneous incoming requests for the same remote file - it will be downloaded once, cached and served
		// to all clients.
		activeRemoteRequests.Lock()
		mediaMetadata, err = db.GetMediaMetadata(r.MediaMetadata.MediaID, r.MediaMetadata.Origin)
		if err == nil {
			// If we have a record, we can respond from the local file
			r.MediaMetadata = mediaMetadata
			r.respondFromLocalFile(w, cfg.AbsBasePath)
			activeRemoteRequests.Unlock()
			return
		}
		if activeRemoteRequestCondition, ok := activeRemoteRequests.Set[mxcURL]; ok {
			r.Logger.WithFields(log.Fields{
				"Origin":  r.MediaMetadata.Origin,
				"MediaID": r.MediaMetadata.MediaID,
			}).Info("Waiting for another goroutine to fetch the remote file.")
			activeRemoteRequestCondition.Wait()
			activeRemoteRequests.Unlock()
			activeRemoteRequests.Lock()
			mediaMetadata, err = db.GetMediaMetadata(r.MediaMetadata.MediaID, r.MediaMetadata.Origin)
			if err == nil {
				// If we have a record, we can respond from the local file
				r.MediaMetadata = mediaMetadata
				r.respondFromLocalFile(w, cfg.AbsBasePath)
			} else {
				r.Logger.WithFields(log.Fields{
					"MediaID": r.MediaMetadata.MediaID,
					"Origin":  r.MediaMetadata.Origin,
				}).Warn("Other goroutine failed to fetch remote file.")
				r.jsonErrorResponse(w, util.JSONResponse{
					Code: 500,
					JSON: jsonerror.Unknown(fmt.Sprintf("File with media ID %q could not be downloaded from %q", r.MediaMetadata.MediaID, r.MediaMetadata.Origin)),
				})
			}
			activeRemoteRequests.Unlock()
			return
		}
		activeRemoteRequests.Set[mxcURL] = &sync.Cond{L: activeRemoteRequests}
		activeRemoteRequests.Unlock()

		r.respondFromRemoteFile(w, cfg.AbsBasePath, cfg.MaxFileSizeBytes, db, activeRemoteRequests)
	} else if err == sql.ErrNoRows && r.MediaMetadata.Origin == cfg.ServerName {
		// If we do not have a record and the origin is local, the file is not found
		r.Logger.WithError(err).Warn("Failed to look up file in database")
		r.jsonErrorResponse(w, util.JSONResponse{
			Code: 404,
			JSON: jsonerror.NotFound(fmt.Sprintf("File with media ID %q does not exist", r.MediaMetadata.MediaID)),
		})
	} else {
		// Another error from the database
		r.Logger.WithError(err).WithFields(log.Fields{
			"MediaID": r.MediaMetadata.MediaID,
			"Origin":  r.MediaMetadata.Origin,
		}).Error("Error querying the database.")
		r.jsonErrorResponse(w, util.JSONResponse{
			Code: 500,
			JSON: jsonerror.Unknown("Internal server error"),
		})
	}
}

func (r *downloadRequest) respondFromLocalFile(w http.ResponseWriter, absBasePath types.Path) {
	r.Logger.WithFields(log.Fields{
		"MediaID":             r.MediaMetadata.MediaID,
		"Origin":              r.MediaMetadata.Origin,
		"UploadName":          r.MediaMetadata.UploadName,
		"Base64Hash":          r.MediaMetadata.Base64Hash,
		"FileSizeBytes":       r.MediaMetadata.FileSizeBytes,
		"Content-Type":        r.MediaMetadata.ContentType,
		"Content-Disposition": r.MediaMetadata.ContentDisposition,
	}).Infof("Downloading file")

	filePath, err := fileutils.GetPathFromBase64Hash(r.MediaMetadata.Base64Hash, absBasePath)
	if err != nil {
		// FIXME: Remove erroneous file from database?
		r.Logger.WithError(err).Warn("Failed to get file path from metadata")
		r.jsonErrorResponse(w, util.JSONResponse{
			Code: 404,
			JSON: jsonerror.NotFound(fmt.Sprintf("File with media ID %q does not exist", r.MediaMetadata.MediaID)),
		})
		return
	}
	file, err := os.Open(filePath)
	if err != nil {
		// FIXME: Remove erroneous file from database?
		r.Logger.WithError(err).Warn("Failed to open file")
		r.jsonErrorResponse(w, util.JSONResponse{
			Code: 404,
			JSON: jsonerror.NotFound(fmt.Sprintf("File with media ID %q does not exist", r.MediaMetadata.MediaID)),
		})
		return
	}

	stat, err := file.Stat()
	if err != nil {
		// FIXME: Remove erroneous file from database?
		r.Logger.WithError(err).Warn("Failed to stat file")
		r.jsonErrorResponse(w, util.JSONResponse{
			Code: 404,
			JSON: jsonerror.NotFound(fmt.Sprintf("File with media ID %q does not exist", r.MediaMetadata.MediaID)),
		})
		return
	}

	if r.MediaMetadata.FileSizeBytes > 0 && int64(r.MediaMetadata.FileSizeBytes) != stat.Size() {
		r.Logger.WithFields(log.Fields{
			"fileSizeDatabase": r.MediaMetadata.FileSizeBytes,
			"fileSizeDisk":     stat.Size(),
		}).Warn("File size in database and on-disk differ.")
		// FIXME: Remove erroneous file from database?
	}

	w.Header().Set("Content-Type", string(r.MediaMetadata.ContentType))
	w.Header().Set("Content-Length", strconv.FormatInt(stat.Size(), 10))
	contentSecurityPolicy := "default-src 'none';" +
		" script-src 'none';" +
		" plugin-types application/pdf;" +
		" style-src 'unsafe-inline';" +
		" object-src 'self';"
	w.Header().Set("Content-Security-Policy", contentSecurityPolicy)

	if bytesResponded, err := io.Copy(w, file); err != nil {
		r.Logger.WithError(err).Warn("Failed to copy from cache")
		if bytesResponded == 0 {
			r.jsonErrorResponse(w, util.JSONResponse{
				Code: 500,
				JSON: jsonerror.NotFound(fmt.Sprintf("Failed to respond with file with media ID %q", r.MediaMetadata.MediaID)),
			})
		}
		// If we have written any data then we have already responded with 200 OK and all we can do is close the connection
		return
	}
}

func (r *downloadRequest) createRemoteRequest() (*http.Response, *util.JSONResponse) {
	urls := getMatrixURLs(r.MediaMetadata.Origin)

	r.Logger.WithField("URL", urls[0]).Info("Connecting to remote")

	remoteReqAddr := urls[0] + "/_matrix/media/v1/download/" + string(r.MediaMetadata.Origin) + "/" + string(r.MediaMetadata.MediaID)
	remoteReq, err := http.NewRequest("GET", remoteReqAddr, nil)
	if err != nil {
		return nil, &util.JSONResponse{
			Code: 500,
			JSON: jsonerror.Unknown(fmt.Sprintf("File with media ID %q could not be downloaded from %q", r.MediaMetadata.MediaID, r.MediaMetadata.Origin)),
		}
	}

	remoteReq.Header.Set("Host", string(r.MediaMetadata.Origin))

	client := http.Client{}
	resp, err := client.Do(remoteReq)
	if err != nil {
		return nil, &util.JSONResponse{
			Code: 502,
			JSON: jsonerror.Unknown(fmt.Sprintf("File with media ID %q could not be downloaded from %q", r.MediaMetadata.MediaID, r.MediaMetadata.Origin)),
		}
	}

	if resp.StatusCode != 200 {
		r.Logger.WithFields(log.Fields{
			"Origin":     r.MediaMetadata.Origin,
			"MediaID":    r.MediaMetadata.MediaID,
			"StatusCode": resp.StatusCode,
		}).Info("Received error response")
		if resp.StatusCode == 404 {
			r.Logger.WithFields(log.Fields{
				"Origin":     r.MediaMetadata.Origin,
				"MediaID":    r.MediaMetadata.MediaID,
				"StatusCode": resp.StatusCode,
			}).Warn("Remote server says file does not exist")
			return nil, &util.JSONResponse{
				Code: 404,
				JSON: jsonerror.NotFound(fmt.Sprintf("File with media ID %q does not exist", r.MediaMetadata.MediaID)),
			}
		}
		return nil, &util.JSONResponse{
			Code: 502,
			JSON: jsonerror.Unknown(fmt.Sprintf("File with media ID %q could not be downloaded from %q", r.MediaMetadata.MediaID, r.MediaMetadata.Origin)),
		}
	}

	return resp, nil
}

func (r *downloadRequest) closeConnection(w http.ResponseWriter) {
	r.Logger.WithFields(log.Fields{
		"Origin":  r.MediaMetadata.Origin,
		"MediaID": r.MediaMetadata.MediaID,
	}).Info("Attempting to close the connection.")
	hijacker, ok := w.(http.Hijacker)
	if ok {
		connection, _, hijackErr := hijacker.Hijack()
		if hijackErr == nil {
			r.Logger.WithFields(log.Fields{
				"Origin":  r.MediaMetadata.Origin,
				"MediaID": r.MediaMetadata.MediaID,
			}).Info("Closing")
			connection.Close()
		} else {
			r.Logger.WithError(hijackErr).WithFields(log.Fields{
				"Origin":  r.MediaMetadata.Origin,
				"MediaID": r.MediaMetadata.MediaID,
			}).Warn("Error trying to hijack and close connection")
		}
	}
}

func completeRemoteRequest(activeRemoteRequests *types.ActiveRemoteRequests, mxcURL string) {
	if activeRemoteRequestCondition, ok := activeRemoteRequests.Set[mxcURL]; ok {
		activeRemoteRequestCondition.Broadcast()
	}
	delete(activeRemoteRequests.Set, mxcURL)
	activeRemoteRequests.Unlock()
}

func (r *downloadRequest) commitFileAndMetadata(tmpDir types.Path, absBasePath types.Path, activeRemoteRequests *types.ActiveRemoteRequests, db *storage.Database, mxcURL string) bool {
	updateActiveRemoteRequests := true

	r.Logger.WithFields(log.Fields{
		"MediaID":             r.MediaMetadata.MediaID,
		"Origin":              r.MediaMetadata.Origin,
		"Base64Hash":          r.MediaMetadata.Base64Hash,
		"UploadName":          r.MediaMetadata.UploadName,
		"FileSizeBytes":       r.MediaMetadata.FileSizeBytes,
		"Content-Type":        r.MediaMetadata.ContentType,
		"Content-Disposition": r.MediaMetadata.ContentDisposition,
	}).Info("Storing file metadata to media repository database")

	// The database is the source of truth so we need to have moved the file first
	finalPath, duplicate, err := fileutils.MoveFileWithHashCheck(tmpDir, r.MediaMetadata, absBasePath, r.Logger)
	if err != nil {
		r.Logger.WithError(err).Error("Failed to move file.")
		return updateActiveRemoteRequests
	}
	if duplicate {
		r.Logger.WithField("dst", finalPath).Info("File was stored previously - discarding duplicate")
		// Continue on to store the metadata in the database
	}

	// Writing the metadata to the media repository database and removing the mxcURL from activeRemoteRequests needs to be atomic.
	// If it were not atomic, a new request for the same file could come in in routine A and check the database before the INSERT.
	// Routine B which was fetching could then have its INSERT complete and remove the mxcURL from the activeRemoteRequests.
	// If routine A then checked the activeRemoteRequests it would think it needed to fetch the file when it's already in the database.
	// The locking below mitigates this situation.
	updateActiveRemoteRequests = false
	activeRemoteRequests.Lock()
	// FIXME: unlock after timeout of db request
	// if written to disk, add to db
	err = db.StoreMediaMetadata(r.MediaMetadata)
	if err != nil {
		// If the file is a duplicate (has the same hash as an existing file) then
		// there is valid metadata in the database for that file. As such we only
		// remove the file if it is not a duplicate.
		if duplicate == false {
			finalDir := path.Dir(finalPath)
			fileutils.RemoveDir(types.Path(finalDir), r.Logger)
		}
		completeRemoteRequest(activeRemoteRequests, mxcURL)
		return updateActiveRemoteRequests
	}
	r.Logger.WithFields(log.Fields{
		"Origin":  r.MediaMetadata.Origin,
		"MediaID": r.MediaMetadata.MediaID,
	}).Info("Signalling other goroutines waiting for us to fetch the file.")
	completeRemoteRequest(activeRemoteRequests, mxcURL)
	return updateActiveRemoteRequests
}

func (r *downloadRequest) respondFromRemoteFile(w http.ResponseWriter, absBasePath types.Path, maxFileSizeBytes types.FileSizeBytes, db *storage.Database, activeRemoteRequests *types.ActiveRemoteRequests) {
	r.Logger.WithFields(log.Fields{
		"Origin":  r.MediaMetadata.Origin,
		"MediaID": r.MediaMetadata.MediaID,
	}).Infof("Fetching remote file")

	mxcURL := "mxc://" + string(r.MediaMetadata.Origin) + "/" + string(r.MediaMetadata.MediaID)

	// If we hit an error and we return early, we need to lock, broadcast on the condition, delete the condition and unlock.
	// If we return normally we have slightly different locking around the storage of metadata to the database and deletion of the condition.
	// As such, this deferred cleanup of the sync.Cond is conditional.
	// This approach seems safer than potentially missing this cleanup in error cases.
	updateActiveRemoteRequests := true
	defer func() {
		if updateActiveRemoteRequests {
			activeRemoteRequests.Lock()
			// Note that completeRemoteRequest unlocks activeRemoteRequests
			completeRemoteRequest(activeRemoteRequests, mxcURL)
		}
	}()

	// create request for remote file
	resp, errorResponse := r.createRemoteRequest()
	if errorResponse != nil {
		r.jsonErrorResponse(w, *errorResponse)
		return
	}
	defer resp.Body.Close()

	// get metadata from request and set metadata on response
	contentLength, err := strconv.ParseInt(resp.Header.Get("Content-Length"), 10, 64)
	if err != nil {
		r.Logger.WithError(err).Warn("Failed to parse content length")
	}
	r.MediaMetadata.FileSizeBytes = types.FileSizeBytes(contentLength)

	r.MediaMetadata.ContentType = types.ContentType(resp.Header.Get("Content-Type"))
	r.MediaMetadata.ContentDisposition = types.ContentDisposition(resp.Header.Get("Content-Disposition"))
	// FIXME: parse from Content-Disposition header if possible, else fall back
	//r.MediaMetadata.UploadName          = types.Filename()

	r.Logger.WithFields(log.Fields{
		"MediaID": r.MediaMetadata.MediaID,
		"Origin":  r.MediaMetadata.Origin,
	}).Infof("Connected to remote")

	w.Header().Set("Content-Type", string(r.MediaMetadata.ContentType))
	w.Header().Set("Content-Length", strconv.FormatInt(int64(r.MediaMetadata.FileSizeBytes), 10))
	contentSecurityPolicy := "default-src 'none';" +
		" script-src 'none';" +
		" plugin-types application/pdf;" +
		" style-src 'unsafe-inline';" +
		" object-src 'self';"
	w.Header().Set("Content-Security-Policy", contentSecurityPolicy)

	// read the remote request's response body
	// simultaneously write it to the incoming request's response body and the temporary file
	r.Logger.WithFields(log.Fields{
		"MediaID": r.MediaMetadata.MediaID,
		"Origin":  r.MediaMetadata.Origin,
	}).Info("Proxying and caching remote file")

	// The file data is hashed but is NOT used as the MediaID, unlike in Upload. The hash is useful as a
	// method of deduplicating files to save storage, as well as a way to conduct
	// integrity checks on the file data in the repository.
	// bytesResponded is the total number of bytes written to the response to the client request
	// bytesWritten is the total number of bytes written to disk
	hash, bytesResponded, bytesWritten, tmpDir, copyError := fileutils.WriteTempFile(resp.Body, maxFileSizeBytes, absBasePath, w)

	if copyError != nil {
		logFields := log.Fields{
			"MediaID": r.MediaMetadata.MediaID,
			"Origin":  r.MediaMetadata.Origin,
		}
		if copyError == fileutils.ErrFileIsTooLarge {
			logFields["MaxFileSizeBytes"] = maxFileSizeBytes
		}
		r.Logger.WithError(copyError).WithFields(logFields).Warn("Error while transferring file")
		fileutils.RemoveDir(tmpDir, r.Logger)
		// Note: if we have responded with any data in the body at all then we have already sent 200 OK and we can only abort at this point
		if bytesResponded < 1 {
			r.jsonErrorResponse(w, util.JSONResponse{
				Code: 502,
				JSON: jsonerror.Unknown(fmt.Sprintf("File could not be downloaded from remote server")),
			})
		} else {
			// We attempt to bluntly close the connection because that is the
			// best thing we can do after we've sent a 200 OK
			r.closeConnection(w)
		}
		return
	}

	// The file has been fetched. It is moved to its final destination and its metadata is inserted into the database.

	// Note: After this point we have responded to the client's request and are just dealing with local caching.
	// As we have responded with 200 OK, any errors are ineffectual to the client request and so we just log and return.
	// FIXME: Does continuing to do work here that is ineffectual to the client have any bad side effects? Could we fire off the remainder in a separate goroutine to mitigate that?

	// It's possible the bytesWritten to the temporary file is different to the reported Content-Length from the remote
	// request's response. bytesWritten is therefore used as it is what would be sent to clients when reading from the local
	// file.
	r.MediaMetadata.FileSizeBytes = types.FileSizeBytes(bytesWritten)
	r.MediaMetadata.Base64Hash = hash
	r.MediaMetadata.UserID = types.MatrixUserID("@:" + string(r.MediaMetadata.Origin))

	updateActiveRemoteRequests = r.commitFileAndMetadata(tmpDir, absBasePath, activeRemoteRequests, db, mxcURL)

	// TODO: generate thumbnails

	r.Logger.WithFields(log.Fields{
		"MediaID":             r.MediaMetadata.MediaID,
		"Origin":              r.MediaMetadata.Origin,
		"UploadName":          r.MediaMetadata.UploadName,
		"Base64Hash":          r.MediaMetadata.Base64Hash,
		"FileSizeBytes":       r.MediaMetadata.FileSizeBytes,
		"Content-Type":        r.MediaMetadata.ContentType,
		"Content-Disposition": r.MediaMetadata.ContentDisposition,
	}).Infof("Remote file cached")
}

// Given a matrix server name, attempt to discover URLs to contact the server
// on.
func getMatrixURLs(serverName gomatrixserverlib.ServerName) []string {
	_, srvs, err := net.LookupSRV("matrix", "tcp", string(serverName))
	if err != nil {
		return []string{"https://" + string(serverName) + ":8448"}
	}

	results := make([]string, 0, len(srvs))
	for _, srv := range srvs {
		if srv == nil {
			continue
		}

		url := []string{"https://", strings.Trim(srv.Target, "."), ":", strconv.Itoa(int(srv.Port))}
		results = append(results, strings.Join(url, ""))
	}

	// TODO: Order based on priority and weight.

	return results
}
