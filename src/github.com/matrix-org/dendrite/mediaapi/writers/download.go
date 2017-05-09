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

	log "github.com/Sirupsen/logrus"
	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/dendrite/mediaapi/config"
	"github.com/matrix-org/dendrite/mediaapi/storage"
	"github.com/matrix-org/dendrite/mediaapi/types"
	"github.com/matrix-org/util"
)

// downloadRequest metadata included in or derivable from an download request
// https://matrix.org/docs/spec/client_server/r0.2.0.html#post-matrix-media-r0-download
type downloadRequest struct {
	MediaMetadata *types.MediaMetadata
}

// Validate validates the downloadRequest fields
func (r downloadRequest) Validate() *util.JSONResponse {
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

func jsonErrorResponse(w http.ResponseWriter, res util.JSONResponse, logger *log.Entry) {
	// Marshal JSON response into raw bytes to send as the HTTP body
	resBytes, err := json.Marshal(res.JSON)
	if err != nil {
		logger.WithError(err).Error("Failed to marshal JSONResponse")
		// this should never fail to be marshalled so drop err to the floor
		res = util.MessageResponse(500, "Internal Server Error")
		resBytes, _ = json.Marshal(res.JSON)
	}

	// Set status code and write the body
	w.WriteHeader(res.Code)
	logger.WithField("code", res.Code).Infof("Responding (%d bytes)", len(resBytes))
	w.Write(resBytes)
}

var errFileIsTooLarge = fmt.Errorf("file is too large")
var errRead = fmt.Errorf("failed to read response from remote server")
var errResponse = fmt.Errorf("failed to write file data to response body")
var errWrite = fmt.Errorf("failed to write file to disk")

// Download implements /download
// Files from this server (i.e. origin == cfg.ServerName) are served directly
// Files from remote servers (i.e. origin != cfg.ServerName) are cached locally.
// If they are present in the cache, they are served directly.
// If they are not present in the cache, they are obtained from the remote server and
// simultaneously served back to the client and written into the cache.
func Download(w http.ResponseWriter, req *http.Request, origin types.ServerName, mediaID types.MediaID, cfg config.MediaAPI, db *storage.Database) {
	logger := util.GetLogger(req.Context())

	// request validation
	if req.Method != "GET" {
		jsonErrorResponse(w, util.JSONResponse{
			Code: 405,
			JSON: jsonerror.Unknown("request method must be GET"),
		}, logger)
		return
	}

	r := &downloadRequest{
		MediaMetadata: &types.MediaMetadata{
			MediaID: mediaID,
			Origin:  origin,
		},
	}

	if resErr := r.Validate(); resErr != nil {
		jsonErrorResponse(w, *resErr, logger)
		return
	}

	// check if we have a record of the media in our database
	err := db.GetMediaMetadata(r.MediaMetadata.MediaID, r.MediaMetadata.Origin, r.MediaMetadata)

	if err == nil {
		// If we have a record, we can respond from the local file
		logger.WithFields(log.Fields{
			"MediaID":             r.MediaMetadata.MediaID,
			"Origin":              r.MediaMetadata.Origin,
			"UploadName":          r.MediaMetadata.UploadName,
			"Content-Length":      r.MediaMetadata.ContentLength,
			"Content-Type":        r.MediaMetadata.ContentType,
			"Content-Disposition": r.MediaMetadata.ContentDisposition,
		}).Infof("Downloading file")

		filePath := getPathFromMediaMetadata(r.MediaMetadata, cfg.BasePath)
		file, err := os.Open(filePath)
		if err != nil {
			// FIXME: Remove erroneous file from database?
			jsonErrorResponse(w, util.JSONResponse{
				Code: 404,
				JSON: jsonerror.NotFound(fmt.Sprintf("File with media ID %q does not exist", r.MediaMetadata.MediaID)),
			}, logger)
			return
		}

		stat, err := file.Stat()
		if err != nil {
			// FIXME: Remove erroneous file from database?
			jsonErrorResponse(w, util.JSONResponse{
				Code: 404,
				JSON: jsonerror.NotFound(fmt.Sprintf("File with media ID %q does not exist", r.MediaMetadata.MediaID)),
			}, logger)
			return
		}

		if r.MediaMetadata.ContentLength > 0 && int64(r.MediaMetadata.ContentLength) != stat.Size() {
			logger.Warnf("File size in database (%v) and on disk (%v) differ.", r.MediaMetadata.ContentLength, stat.Size())
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
			logger.Warnf("Failed to copy from cache %v\n", err)
			if bytesResponded == 0 {
				jsonErrorResponse(w, util.JSONResponse{
					Code: 500,
					JSON: jsonerror.NotFound(fmt.Sprintf("Failed to respond with file with media ID %q", r.MediaMetadata.MediaID)),
				}, logger)
			}
			// If we have written any data then we have already responded with 200 OK and all we can do is close the connection
			return
		}
	} else if err == sql.ErrNoRows && r.MediaMetadata.Origin != cfg.ServerName {
		// If we do not have a record and the origin is remote, we need to fetch it and respond with that file
		logger.WithFields(log.Fields{
			"MediaID": r.MediaMetadata.MediaID,
			"Origin":  r.MediaMetadata.Origin,
		}).Infof("Fetching remote file")

		// TODO: lock request in hash set

		// FIXME: Only request once (would race if multiple requests for the same remote file)
		// Use a hash set based on the origin and media ID (the request URL should be fine...) and synchronise adding / removing members
		urls := getMatrixUrls(r.MediaMetadata.Origin)

		logger.Printf("Connecting to remote %q\n", urls[0])

		remoteReqAddr := urls[0] + "/_matrix/media/v1/download/" + string(r.MediaMetadata.Origin) + "/" + string(r.MediaMetadata.MediaID)
		remoteReq, err := http.NewRequest("GET", remoteReqAddr, nil)
		if err != nil {
			jsonErrorResponse(w, util.JSONResponse{
				Code: 500,
				JSON: jsonerror.Unknown(fmt.Sprintf("File with media ID %q could not be downloaded from %q", r.MediaMetadata.MediaID, r.MediaMetadata.Origin)),
			}, logger)
			return
		}

		remoteReq.Header.Set("Host", string(r.MediaMetadata.Origin))

		client := http.Client{}
		resp, err := client.Do(remoteReq)
		if err != nil {
			jsonErrorResponse(w, util.JSONResponse{
				Code: 502,
				JSON: jsonerror.Unknown(fmt.Sprintf("File with media ID %q could not be downloaded from %q", r.MediaMetadata.MediaID, r.MediaMetadata.Origin)),
			}, logger)
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode != 200 {
			logger.Printf("Server responded with %d\n", resp.StatusCode)
			if resp.StatusCode == 404 {
				jsonErrorResponse(w, util.JSONResponse{
					Code: 404,
					JSON: jsonerror.NotFound(fmt.Sprintf("File with media ID %q does not exist", r.MediaMetadata.MediaID)),
				}, logger)
				return
			}
			jsonErrorResponse(w, util.JSONResponse{
				Code: 502,
				JSON: jsonerror.Unknown(fmt.Sprintf("File with media ID %q could not be downloaded from %q", r.MediaMetadata.MediaID, r.MediaMetadata.Origin)),
			}, logger)
			return
		}

		contentLength, err := strconv.ParseInt(resp.Header.Get("Content-Length"), 10, 64)
		if err != nil {
			logger.Warn("Failed to parse content length")
		}
		r.MediaMetadata.ContentLength = types.ContentLength(contentLength)

		logger.WithFields(log.Fields{
			"MediaID": r.MediaMetadata.MediaID,
			"Origin":  r.MediaMetadata.Origin,
		}).Infof("Connected to remote")

		w.Header().Set("Content-Type", string(r.MediaMetadata.ContentType))
		w.Header().Set("Content-Length", strconv.FormatInt(int64(r.MediaMetadata.ContentLength), 10))
		contentSecurityPolicy := "default-src 'none';" +
			" script-src 'none';" +
			" plugin-types application/pdf;" +
			" style-src 'unsafe-inline';" +
			" object-src 'self';"
		w.Header().Set("Content-Security-Policy", contentSecurityPolicy)

		tmpDir, err := createTempDir(cfg.BasePath)
		if err != nil {
			logger.Infof("Failed to create temp dir %q\n", err)
			jsonErrorResponse(w, util.JSONResponse{
				Code: 400,
				JSON: jsonerror.Unknown(fmt.Sprintf("Failed to upload: %q", err)),
			}, logger)
			return
		}
		tmpFile, writer, err := createFileWriter(tmpDir, types.Filename(r.MediaMetadata.MediaID[3:]))
		if err != nil {
			logger.Infof("Failed to create file writer %q\n", err)
			jsonErrorResponse(w, util.JSONResponse{
				Code: 400,
				JSON: jsonerror.Unknown(fmt.Sprintf("Failed to upload: %q", err)),
			}, logger)
			return
		}
		defer tmpFile.Close()

		logger.WithFields(log.Fields{
			"MediaID": r.MediaMetadata.MediaID,
			"Origin":  r.MediaMetadata.Origin,
		}).Infof("Proxying and caching remote file")

		// bytesResponded is the total number of bytes written to the response to the client request
		// bytesWritten is the total number of bytes written to disk
		var bytesResponded, bytesWritten int64 = 0, 0
		var fetchError error
		// Note: the buffer size is the same as is used in io.Copy()
		buffer := make([]byte, 32*1024)
		for {
			// read from remote request's response body
			bytesRead, readErr := resp.Body.Read(buffer)
			if bytesRead > 0 {
				// write to client request's response body
				bytesTemp, respErr := w.Write(buffer[:bytesRead])
				if bytesTemp != bytesRead || (respErr != nil && respErr != io.EOF) {
					// TODO: BORKEN
					logger.Errorf("bytesTemp %v != bytesRead %v : %v", bytesTemp, bytesRead, respErr)
					fetchError = errResponse
					break
				}
				bytesResponded += int64(bytesTemp)
				if fetchError == nil || (fetchError != errFileIsTooLarge && fetchError != errWrite) {
					// if larger than cfg.MaxFileSize then stop writing to disk and discard cached file
					if bytesWritten+int64(len(buffer)) > int64(cfg.MaxFileSize) {
						// TODO: WAAAAHNING and clean up temp files
						fetchError = errFileIsTooLarge
					} else {
						// write to disk
						bytesTemp, writeErr := writer.Write(buffer)
						if writeErr != nil && writeErr != io.EOF {
							// TODO: WAAAAHNING and clean up temp files
							fetchError = errWrite
						} else {
							bytesWritten += int64(bytesTemp)
						}
					}
				}
			}
			if readErr != nil {
				if readErr != io.EOF {
					fetchError = errRead
				}
				break
			}
		}

		writer.Flush()

		if fetchError != nil {
			logFields := log.Fields{
				"MediaID": r.MediaMetadata.MediaID,
				"Origin":  r.MediaMetadata.Origin,
			}
			if fetchError == errFileIsTooLarge {
				logFields["MaxFileSize"] = cfg.MaxFileSize
			}
			logger.WithFields(logFields).Warnln(fetchError)
			tmpDirErr := os.RemoveAll(string(tmpDir))
			if tmpDirErr != nil {
				logger.Warnf("Failed to remove tmpDir (%v): %q\n", tmpDir, tmpDirErr)
			}
			// Note: if we have responded with any data in the body at all then we have already sent 200 OK and we can only abort at this point
			if bytesResponded < 1 {
				jsonErrorResponse(w, util.JSONResponse{
					Code: 502,
					JSON: jsonerror.Unknown(fmt.Sprintf("File with media ID %q could not be downloaded from %q", r.MediaMetadata.MediaID, r.MediaMetadata.Origin)),
				}, logger)
			} else {
				// We attempt to bluntly close the connection because that is the
				// best thing we can do after we've sent a 200 OK
				logger.Println("Attempting to close the connection.")
				hijacker, ok := w.(http.Hijacker)
				if ok {
					connection, _, hijackErr := hijacker.Hijack()
					if hijackErr == nil {
						logger.Println("Closing")
						connection.Close()
					} else {
						logger.Printf("Error trying to hijack: %v", hijackErr)
					}
				}
			}
			return
		}

		// Note: After this point we have responded to the client's request and are just dealing with local caching.
		// As we have responded with 200 OK, any errors are ineffectual to the client request and so we just log and return.

		logger.WithFields(log.Fields{
			"MediaID":             r.MediaMetadata.MediaID,
			"Origin":              r.MediaMetadata.Origin,
			"UploadName":          r.MediaMetadata.UploadName,
			"Content-Length":      r.MediaMetadata.ContentLength,
			"Content-Type":        r.MediaMetadata.ContentType,
			"Content-Disposition": r.MediaMetadata.ContentDisposition,
		}).Infof("Storing file metadata to media repository database")

		// if written to disk, add to db
		err = db.StoreMediaMetadata(r.MediaMetadata)
		if err != nil {
			tmpDirErr := os.RemoveAll(string(tmpDir))
			if tmpDirErr != nil {
				logger.Warnf("Failed to remove tmpDir (%v): %q\n", tmpDir, tmpDirErr)
			}
			return
		}

		// TODO: unlock request in hash set

		// TODO: generate thumbnails

		err = moveFile(
			types.Path(path.Join(string(tmpDir), "content")),
			types.Path(getPathFromMediaMetadata(r.MediaMetadata, cfg.BasePath)),
		)
		if err != nil {
			tmpDirErr := os.RemoveAll(string(tmpDir))
			if tmpDirErr != nil {
				logger.Warnf("Failed to remove tmpDir (%v): %q\n", tmpDir, tmpDirErr)
			}
			return
		}

		logger.WithFields(log.Fields{
			"MediaID":             r.MediaMetadata.MediaID,
			"Origin":              r.MediaMetadata.Origin,
			"UploadName":          r.MediaMetadata.UploadName,
			"Content-Length":      r.MediaMetadata.ContentLength,
			"Content-Type":        r.MediaMetadata.ContentType,
			"Content-Disposition": r.MediaMetadata.ContentDisposition,
		}).Infof("Remote file cached")
	} else {
		// TODO: If we do not have a record and the origin is local, or if we have another error from the database, the file is not found
		jsonErrorResponse(w, util.JSONResponse{
			Code: 404,
			JSON: jsonerror.NotFound(fmt.Sprintf("File with media ID %q does not exist", r.MediaMetadata.MediaID)),
		}, logger)
	}
}

// Given a matrix server name, attempt to discover URLs to contact the server
// on.
func getMatrixUrls(serverName types.ServerName) []string {
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
