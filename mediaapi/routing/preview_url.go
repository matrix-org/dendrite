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
	"io/ioutil"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/html"

	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/dendrite/mediaapi/fileutils"
	"github.com/matrix-org/dendrite/mediaapi/storage"
	"github.com/matrix-org/dendrite/mediaapi/types"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/util"
	log "github.com/sirupsen/logrus"
)

// the parameters included in the incoming preview_url request
type PreviewUrlRequest struct {
	url url.URL
	ts  types.UnixMs
}

// metadata of the url (media/html) to be previewed
type mediaInfo struct {
	MediaMetadata *types.MediaMetadata
	Logger        *log.Entry
}

// PreviewUrlResponse defines the format of the JSON response
// https://matrix.org/docs/spec/client_server/latest#get-matrix-media-r0-preview-url
// TODO: add more fields to the response
type PreviewUrlResponse struct {
	MatrixImageSize int64  `json:"matrix:image:size"`
	OgImage         string `json:"og:image"`
	OgSiteName      string `json:"og:site_name"`
	OgType          string `json:"og:type"`
	OgTitle         string `json:"og:title"`
	OgUrl           string `json:"og:url"`
	OgDescription   string `json:"og:description"`
}

// PreviewUrl implements GET /preview_url.
// Current implementation gets the url, parses the meta tags to obtain OG
// data and returns a JSONResponse for the client to process.
// TODO: avoid this endpoint in encrypted rooms
func PreviewUrl(
	req *http.Request, cfg *config.MediaAPI, db storage.Database) util.JSONResponse {

	// parsing the request
	r, err := parseRequest(req)
	if err != nil {
		return *err
	}

	// downloading metadata for the url
	mediaInfo, err := r.downloadUrl(cfg, req.Context(), db)
	if err != nil {
		return *err
	}

	// preparing the response
	response, err := mediaInfo.prepareResponse(cfg)
	if err != nil {
		return *err
	}

	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: response,
	}
}

// parseRequest parses the incoming preview request to extract the url and ts.
// Returns either a parsed PreviewUrlRequest or error formatted as
// util.JSONResponse
func parseRequest(req *http.Request) (*PreviewUrlRequest, *util.JSONResponse) {

	// get the url to be previewed from the request
	urlToPreview := req.URL.Query().Get("url")
	if len(urlToPreview) == 0 {
		return nil, &util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.MissingArgument("Missing URL"),
		}
	}

	// parse the url
	parsedUrl, err := url.Parse(urlToPreview)
	if err != nil {
		return nil, &util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.InvalidArgumentValue(
				"Unable to parse url " + err.Error()),
		}
	}

	// get the ts if provided in the request
	if tsStr := req.URL.Query().Get("ts"); len(tsStr) > 0 {
		ts, err := strconv.ParseInt(tsStr, 10, 64)
		if err != nil {
			return nil, &util.JSONResponse{
				Code: http.StatusBadRequest,
				JSON: jsonerror.InvalidArgumentValue(
					"Couldn't parse 'ts' to a valid integer" + err.Error()),
			}
		}

		request := &PreviewUrlRequest{
			url: *parsedUrl,
			// Convert timestamp to ms
			ts: types.UnixMs(ts / 1000000),
		}

		return request, nil
	}

	// set ts to current time if none provided
	ts := time.Now().UnixNano()

	request := &PreviewUrlRequest{
		url: *parsedUrl,
		ts:  types.UnixMs(ts / 1000000),
	}

	return request, nil
}

// downloadUrl downloads the url and saves the response body.
// Returns either mediaInfo (metadata about the file to preview) or error
// formatted as util.JSONResponse.
// Current implementation for saving response is heavily similar to /upload,
// need to work on this.
// TODO: better logging
func (previewReq *PreviewUrlRequest) downloadUrl(cfg *config.MediaAPI, ctx context.Context, db storage.Database) (*mediaInfo, *util.JSONResponse) {

	urlString := previewReq.url.String()

	// Get the URL
	response, err := http.Get(urlString)
	if err != nil {
		return nil, &util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.InvalidArgumentValue("Couldn't get the URL " + err.Error()),
		}
	}

	// TODO: check for other status codes too
	if response.StatusCode == 404 {
		return nil, &util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.NotFound("Given url returned 404"),
		}
	}

	// Save the response body to the temporary dir
	// TODO: should be a new directory like url_cache maybe and
	hash, bytesWritten, tmpDir, err := fileutils.WriteTempFile(ctx, response.Body, cfg.AbsBasePath)
	if err != nil {
		return nil, &util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.Unknown("Failed to store URL response " + err.Error()),
		}
	}

	// metadata of the media/webpage to preview
	urlMetadata := &mediaInfo{
		MediaMetadata: &types.MediaMetadata{
			Origin:        cfg.Matrix.ServerName,
			FileSizeBytes: bytesWritten,
			Base64Hash:    hash,
			ContentType:   types.ContentType(response.Header["Content-Type"][0]),
		},
		Logger: util.GetLogger(ctx).WithField("Origin", cfg.Matrix.ServerName),
	}

	// :-\
	// Look up the media by the file hash. If we already have the file but under a
	// different media ID then we won't upload the file again - instead we'll just
	// add a new metadata entry that refers to the same file.
	existingMetadata, err := db.GetMediaMetadataByHash(
		ctx, hash, urlMetadata.MediaMetadata.Origin,
	)
	if err != nil {
		fileutils.RemoveDir(tmpDir, urlMetadata.Logger)
		urlMetadata.Logger.WithError(err).Error("Error querying the database by hash.")
		resErr := jsonerror.InternalServerError()
		return nil, &resErr
	}
	if existingMetadata != nil {
		// The file already exists, delete the uploaded temporary file.
		defer fileutils.RemoveDir(tmpDir, urlMetadata.Logger)
		// The file already exists. Make a new media ID up for it.
		mediaID, merr := urlMetadata.generateMediaID(ctx, db)
		if merr != nil {
			urlMetadata.Logger.WithError(merr).Error("Failed to generate media ID for existing file")
			resErr := jsonerror.InternalServerError()
			return nil, &resErr
		}

		// Then amend the upload metadata.
		urlMetadata.MediaMetadata = &types.MediaMetadata{
			MediaID:           mediaID,
			Origin:            urlMetadata.MediaMetadata.Origin,
			ContentType:       urlMetadata.MediaMetadata.ContentType,
			FileSizeBytes:     urlMetadata.MediaMetadata.FileSizeBytes,
			CreationTimestamp: urlMetadata.MediaMetadata.CreationTimestamp,
			UploadName:        urlMetadata.MediaMetadata.UploadName,
			Base64Hash:        hash,
			UserID:            urlMetadata.MediaMetadata.UserID,
		}
	} else {
		// The file doesn't exist. Update the request metadata.
		urlMetadata.MediaMetadata.FileSizeBytes = bytesWritten
		urlMetadata.MediaMetadata.Base64Hash = hash
		urlMetadata.MediaMetadata.MediaID, err = urlMetadata.generateMediaID(ctx, db)
		if err != nil {
			fileutils.RemoveDir(tmpDir, urlMetadata.Logger)
			urlMetadata.Logger.WithError(err).Error("Failed to generate media ID for new download")
			resErr := jsonerror.InternalServerError()
			return nil, &resErr
		}
	}

	urlMetadata.Logger = urlMetadata.Logger.WithField("media_id", urlMetadata.MediaMetadata.MediaID)
	urlMetadata.Logger.WithFields(log.Fields{
		"Base64Hash":    urlMetadata.MediaMetadata.Base64Hash,
		"UploadName":    urlMetadata.MediaMetadata.UploadName,
		"FileSizeBytes": urlMetadata.MediaMetadata.FileSizeBytes,
		"ContentType":   urlMetadata.MediaMetadata.ContentType,
	}).Info("File downloaded")

	if resErr := urlMetadata.storeFileAndMetadata(ctx, tmpDir, cfg.AbsBasePath, db); resErr != nil {
		return nil, resErr
	}

	return urlMetadata, nil
}

func (m *mediaInfo) generateMediaID(ctx context.Context, db storage.Database) (types.MediaID, error) {
	for {
		// First try generating a media ID. We'll do this by
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
		existingMetadata, err := db.GetMediaMetadata(ctx, mediaID, m.MediaMetadata.Origin)
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

func (r *mediaInfo) storeFileAndMetadata(
	ctx context.Context,
	tmpDir types.Path,
	absBasePath config.Path,
	db storage.Database,
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

	return nil
}

// parseContent returns the data stored in content attributes
func parseContent(node *html.Node) string {
	// iterating the attributes of the tag to get content
	for _, attr := range node.Attr {
		if attr.Key == "content" {
			content := attr.Val
			return content
		}
	}
	return ""
}

// prepareResponse prepares the response to be returned to the client
func (mediaInfo *mediaInfo) prepareResponse(cfg *config.MediaAPI) (*PreviewUrlResponse, *util.JSONResponse) {

	// Reading the file in which the response body was stored and parsing the html.
	pathToFile, err := fileutils.GetPathFromBase64Hash(mediaInfo.MediaMetadata.Base64Hash, cfg.AbsBasePath)
	if err != nil {
		return nil, &util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: jsonerror.Unknown("Couldn't get the path to the stored response" + err.Error()),
		}
	}

	fileString, err := ioutil.ReadFile(pathToFile)
	if err != nil {
		return nil, &util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: jsonerror.Unknown("Couldn't read the stored response body" + err.Error()),
		}
	}

	file := strings.NewReader(string(fileString))

	tree, err := html.Parse(file)
	if err != nil {
		return nil, &util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: jsonerror.Unknown("Couldn't get the path to the stored file " + err.Error()),
		}
	}

	// map for storing the content values
	m := make(map[string]string)

	// Iterating the *html.Node, looking for og data
	var f func(*html.Node)
	f = func(n *html.Node) {
		//check if meta tag
		if n.Type == html.ElementNode && n.Data == "meta" {
			//parse attributes of the tag
			for _, a := range n.Attr {
				if a.Key == "property" && strings.HasPrefix(a.Val, "og:") {
					slice := strings.Split(a.Val, ":")
					m[slice[1]] = parseContent(n)
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			f(c)
		}
	}

	f(tree)

	response := &PreviewUrlResponse{
		OgImage:       m["image"],
		OgSiteName:    m["site_name"],
		OgType:        m["type"],
		OgTitle:       m["title"],
		OgUrl:         m["url"],
		OgDescription: m["description"],
	}

	return response, nil
}
