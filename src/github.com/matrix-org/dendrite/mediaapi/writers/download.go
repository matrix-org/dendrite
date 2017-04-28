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
	"net"
	"net/http"
	"strconv"
	"strings"

	log "github.com/Sirupsen/logrus"
	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/dendrite/mediaapi/config"
	"github.com/matrix-org/dendrite/mediaapi/storage"
	"github.com/matrix-org/util"
)

// DownloadRequest metadata included in or derivable from an upload request
// https://matrix.org/docs/spec/client_server/r0.2.0.html#post-matrix-media-r0-download
type DownloadRequest struct {
	MediaID    string
	ServerName string
}

// Validate validates the DownloadRequest fields
func (r DownloadRequest) Validate() *util.JSONResponse {
	// FIXME: the following errors aren't bad JSON, rather just a bad request path
	// maybe give the URL pattern in the routing, these are not even possible as the handler would not be hit...?
	if r.MediaID == "" {
		return &util.JSONResponse{
			Code: 400,
			JSON: jsonerror.BadJSON("mediaId must be a non-empty string"),
		}
	}
	if r.ServerName == "" {
		return &util.JSONResponse{
			Code: 400,
			JSON: jsonerror.BadJSON("serverName must be a non-empty string"),
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

// Download implements /download
func Download(w http.ResponseWriter, req *http.Request, serverName string, mediaID string, cfg config.MediaAPI, db *storage.Database, downloadServer DownloadServer) {
	logger := util.GetLogger(req.Context())

	if req.Method != "GET" {
		jsonErrorResponse(w, util.JSONResponse{
			Code: 405,
			JSON: jsonerror.Unknown("request method must be GET"),
		}, logger)
		return
	}

	r := &DownloadRequest{
		MediaID:    mediaID,
		ServerName: serverName,
	}

	if resErr := r.Validate(); resErr != nil {
		jsonErrorResponse(w, *resErr, logger)
		return
	}

	contentType, contentDisposition, fileSize, filename, err := db.GetMedia(r.MediaID, r.ServerName)
	if err != nil {
		if strings.Compare(r.ServerName, cfg.ServerName) != 0 {
			// TODO: get remote file from remote server
			jsonErrorResponse(w, util.JSONResponse{
				Code: 404,
				JSON: jsonerror.NotFound(fmt.Sprintf("NOT YET IMPLEMENTED")),
			}, logger)
			return
		}
		jsonErrorResponse(w, util.JSONResponse{
			Code: 404,
			JSON: jsonerror.NotFound(fmt.Sprintf("File %q does not exist", r.MediaID)),
		}, logger)
		return
	}

	// - read file and respond
	logger.WithFields(log.Fields{
		"MediaID":             r.MediaID,
		"ServerName":          r.ServerName,
		"Filename":            filename,
		"Content-Type":        contentType,
		"Content-Disposition": contentDisposition,
	}).Infof("Downloading file")

	logger.WithField("code", 200).Infof("Responding (%d bytes)", fileSize)

	respWriter := httpResponseWriter{resp: w}
	if err = downloadServer.getImage(respWriter, r.ServerName, r.MediaID); err != nil {
		if respWriter.haveWritten() {
			closeConnection(w)
			return
		}

		errStatus := 500
		switch err {
		case errNotFound:
			errStatus = 404
		case errProxy:
			errStatus = 502
		}
		http.Error(w, err.Error(), errStatus)
		return
	}

	return
}

// DownloadServer serves and caches remote media.
type DownloadServer struct {
	Client          http.Client
	Repository      storage.Repository
	LocalServerName string
}

func (handler *DownloadServer) getImage(w responseWriter, host, name string) error {
	var file io.ReadCloser
	var descr *storage.Description
	var err error
	if host == handler.LocalServerName {
		file, descr, err = handler.Repository.ReaderFromLocalRepo(name)
	} else {
		file, descr, err = handler.Repository.ReaderFromRemoteCache(host, name)
	}

	if err == nil {
		log.Println("Found in Cache")
		w.setContentType(descr.Type)

		size := strconv.FormatInt(descr.Length, 10)
		w.setContentLength(size)
		w.setContentSecurityPolicy()
		if _, err = io.Copy(w, file); err != nil {
			log.Printf("Failed to copy from cache %v\n", err)
			return err
		}
		w.Close()
		return nil
	} else if !storage.IsNotExists(err) {
		log.Printf("Error looking in cache: %v\n", err)
		return err
	}

	if host == handler.LocalServerName {
		// Its fatal if we can't find local files in our cache.
		return errNotFound
	}

	respBody, desc, err := handler.fetchRemoteMedia(host, name)
	if err != nil {
		return err
	}

	defer respBody.Close()

	w.setContentType(desc.Type)
	if desc.Length > 0 {
		w.setContentLength(strconv.FormatInt(desc.Length, 10))
	}

	writer, err := handler.Repository.WriterToRemoteCache(host, name, *desc)
	if err != nil {
		log.Printf("Failed to get cache writer %q\n", err)
		return err
	}

	defer writer.Close()

	reader := io.TeeReader(respBody, w)
	if _, err := io.Copy(writer, reader); err != nil {
		log.Printf("Failed to copy %q\n", err)
		return err
	}

	writer.Finished()

	log.Println("Finished conn")

	return nil
}

func (handler *DownloadServer) fetchRemoteMedia(host, name string) (io.ReadCloser, *storage.Description, error) {
	urls := getMatrixUrls(host)

	log.Printf("Connecting to remote %q\n", urls[0])

	remoteReq, err := http.NewRequest("GET", urls[0]+"/_matrix/media/v1/download/"+host+"/"+name, nil)
	if err != nil {
		log.Printf("Failed to connect to remote: %q\n", err)
		return nil, nil, err
	}

	remoteReq.Header.Set("Host", host)

	resp, err := handler.Client.Do(remoteReq)
	if err != nil {
		log.Printf("Failed to connect to remote: %q\n", err)
		return nil, nil, errProxy
	}

	if resp.StatusCode != 200 {
		resp.Body.Close()
		log.Printf("Server responded with %d\n", resp.StatusCode)
		if resp.StatusCode == 404 {
			return nil, nil, errNotFound
		}
		return nil, nil, errProxy
	}

	desc := storage.Description{
		Type:   resp.Header.Get("Content-Type"),
		Length: -1,
	}

	length, err := strconv.ParseInt(resp.Header.Get("Content-Length"), 10, 64)
	if err == nil {
		desc.Length = length
	}

	return resp.Body, &desc, nil
}

// Given a http.ResponseWriter, attempt to force close the connection.
//
// This is useful if you get a fatal error after sending the initial 200 OK
// response.
func closeConnection(w http.ResponseWriter) {
	log.Println("Attempting to close connection")

	// We attempt to bluntly close the connection because that is the
	// best thing we can do after we've sent a 200 OK
	hijack, ok := w.(http.Hijacker)
	if ok {
		conn, _, err := hijack.Hijack()
		if err != nil {
			fmt.Printf("Err trying to hijack: %v", err)
			return
		}
		log.Println("Closing")
		conn.Close()
		return
	}
	log.Println("Not hijacker")
}

// Given a matrix server name, attempt to discover URLs to contact the server
// on.
func getMatrixUrls(host string) []string {
	_, srvs, err := net.LookupSRV("matrix", "tcp", host)
	if err != nil {
		return []string{"https://" + host + ":8448"}
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

// Given a path of the form '/<host>/<name>' extract the host and name.
func getMediaIDFromPath(path string) (host, name string, err error) {
	parts := strings.Split(path, "/")
	if len(parts) != 3 {
		err = fmt.Errorf("Invalid path %q", path)
		return
	}

	host, name = parts[1], parts[2]

	if host == "" || name == "" {
		err = fmt.Errorf("Invalid path %q", path)
		return
	}

	return
}

type responseWriter interface {
	io.WriteCloser
	setContentLength(string)
	setContentSecurityPolicy()
	setContentType(string)
	haveWritten() bool
}

type httpResponseWriter struct {
	resp    http.ResponseWriter
	written bool
}

func (writer httpResponseWriter) haveWritten() bool {
	return writer.written
}

func (writer httpResponseWriter) Write(p []byte) (n int, err error) {
	writer.written = true
	return writer.resp.Write(p)
}

func (writer httpResponseWriter) Close() error { return nil }

func (writer httpResponseWriter) setContentType(contentType string) {
	writer.resp.Header().Set("Content-Type", contentType)
}

func (writer httpResponseWriter) setContentLength(length string) {
	writer.resp.Header().Set("Content-Length", length)
}

func (writer httpResponseWriter) setContentSecurityPolicy() {
	contentSecurityPolicy := "default-src 'none';" +
		" script-src 'none';" +
		" plugin-types application/pdf;" +
		" style-src 'unsafe-inline';" +
		" object-src 'self';"
	writer.resp.Header().Set("Content-Security-Policy", contentSecurityPolicy)
}

var errProxy = fmt.Errorf("Failed to contact remote")
var errNotFound = fmt.Errorf("Image not found")
