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
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/dendrite/common/config"
	"github.com/matrix-org/dendrite/mediaapi/fileutils"
	"github.com/matrix-org/dendrite/mediaapi/storage"
	"github.com/matrix-org/dendrite/mediaapi/thumbnailer"
	"github.com/matrix-org/dendrite/mediaapi/types"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
)

const mediaIDCharacters = "A-Za-z0-9_=-"

// Note: unfortunately regex.MustCompile() cannot be assigned to a const
var mediaIDRegex = regexp.MustCompile("[" + mediaIDCharacters + "]+")

// downloadRequest metadata included in or derivable from a download or thumbnail request
// https://matrix.org/docs/spec/client_server/r0.2.0.html#get-matrix-media-r0-download-servername-mediaid
// http://matrix.org/docs/spec/client_server/r0.2.0.html#get-matrix-media-r0-thumbnail-servername-mediaid
type downloadRequest struct {
	MediaMetadata      *types.MediaMetadata
	IsThumbnailRequest bool
	ThumbnailSize      types.ThumbnailSize
	Logger             *log.Entry
}

// Download implements /download amd /thumbnail
// Files from this server (i.e. origin == cfg.ServerName) are served directly
// Files from remote servers (i.e. origin != cfg.ServerName) are cached locally.
// If they are present in the cache, they are served directly.
// If they are not present in the cache, they are obtained from the remote server and
// simultaneously served back to the client and written into the cache.
func Download(
	w http.ResponseWriter,
	req *http.Request,
	origin gomatrixserverlib.ServerName,
	mediaID types.MediaID,
	cfg *config.Dendrite,
	db *storage.Database,
	client *gomatrixserverlib.Client,
	activeRemoteRequests *types.ActiveRemoteRequests,
	activeThumbnailGeneration *types.ActiveThumbnailGeneration,
	isThumbnailRequest bool,
) {
	dReq := &downloadRequest{
		MediaMetadata: &types.MediaMetadata{
			MediaID: mediaID,
			Origin:  origin,
		},
		IsThumbnailRequest: isThumbnailRequest,
		Logger: util.GetLogger(req.Context()).WithFields(log.Fields{
			"Origin":  origin,
			"MediaID": mediaID,
		}),
	}

	if dReq.IsThumbnailRequest {
		width, err := strconv.Atoi(req.FormValue("width"))
		if err != nil {
			width = -1
		}
		height, err := strconv.Atoi(req.FormValue("height"))
		if err != nil {
			height = -1
		}
		dReq.ThumbnailSize = types.ThumbnailSize{
			Width:        width,
			Height:       height,
			ResizeMethod: strings.ToLower(req.FormValue("method")),
		}
		dReq.Logger.WithFields(log.Fields{
			"RequestedWidth":        dReq.ThumbnailSize.Width,
			"RequestedHeight":       dReq.ThumbnailSize.Height,
			"RequestedResizeMethod": dReq.ThumbnailSize.ResizeMethod,
		})
	}

	// request validation
	if req.Method != http.MethodGet {
		dReq.jsonErrorResponse(w, util.JSONResponse{
			Code: http.StatusMethodNotAllowed,
			JSON: jsonerror.Unknown("request method must be GET"),
		})
		return
	}

	if resErr := dReq.Validate(); resErr != nil {
		dReq.jsonErrorResponse(w, *resErr)
		return
	}

	metadata, err := dReq.doDownload(
		req.Context(), w, cfg, db, client,
		activeRemoteRequests, activeThumbnailGeneration,
	)
	if err != nil {
		// TODO: Handle the fact we might have started writing the response
		dReq.jsonErrorResponse(w, util.ErrorResponse(err))
		return
	}

	if metadata == nil {
		dReq.jsonErrorResponse(w, util.JSONResponse{
			Code: http.StatusNotFound,
			JSON: jsonerror.NotFound("File not found"),
		})
		return
	}

}

func (r *downloadRequest) jsonErrorResponse(w http.ResponseWriter, res util.JSONResponse) {
	// Marshal JSON response into raw bytes to send as the HTTP body
	resBytes, err := json.Marshal(res.JSON)
	if err != nil {
		r.Logger.WithError(err).Error("Failed to marshal JSONResponse")
		// this should never fail to be marshalled so drop err to the floor
		res = util.MessageResponse(http.StatusInternalServerError, "Internal Server Error")
		resBytes, _ = json.Marshal(res.JSON)
	}

	// Set status code and write the body
	w.WriteHeader(res.Code)
	r.Logger.WithField("code", res.Code).Infof("Responding (%d bytes)", len(resBytes))

	// we don't really care that much if we fail to write the error response
	w.Write(resBytes) // nolint: errcheck
}

// Validate validates the downloadRequest fields
func (r *downloadRequest) Validate() *util.JSONResponse {
	if !mediaIDRegex.MatchString(string(r.MediaMetadata.MediaID)) {
		return &util.JSONResponse{
			Code: http.StatusNotFound,
			JSON: jsonerror.NotFound(fmt.Sprintf("mediaId must be a non-empty string using only characters in %v", mediaIDCharacters)),
		}
	}
	// Note: the origin will be validated either by comparison to the configured server name of this homeserver
	// or by a DNS SRV record lookup when creating a request for remote files
	if r.MediaMetadata.Origin == "" {
		return &util.JSONResponse{
			Code: http.StatusNotFound,
			JSON: jsonerror.NotFound("serverName must be a non-empty string"),
		}
	}

	if r.IsThumbnailRequest {
		if r.ThumbnailSize.Width <= 0 || r.ThumbnailSize.Height <= 0 {
			return &util.JSONResponse{
				Code: http.StatusBadRequest,
				JSON: jsonerror.Unknown("width and height must be greater than 0"),
			}
		}
		// Default method to scale if not set
		if r.ThumbnailSize.ResizeMethod == "" {
			r.ThumbnailSize.ResizeMethod = types.Scale
		}
		if r.ThumbnailSize.ResizeMethod != types.Crop && r.ThumbnailSize.ResizeMethod != types.Scale {
			return &util.JSONResponse{
				Code: http.StatusBadRequest,
				JSON: jsonerror.Unknown("method must be one of crop or scale"),
			}
		}
	}
	return nil
}

func (r *downloadRequest) doDownload(
	ctx context.Context,
	w http.ResponseWriter,
	cfg *config.Dendrite,
	db *storage.Database,
	client *gomatrixserverlib.Client,
	activeRemoteRequests *types.ActiveRemoteRequests,
	activeThumbnailGeneration *types.ActiveThumbnailGeneration,
) (*types.MediaMetadata, error) {
	// check if we have a record of the media in our database
	mediaMetadata, err := db.GetMediaMetadata(
		ctx, r.MediaMetadata.MediaID, r.MediaMetadata.Origin,
	)
	if err != nil {
		return nil, errors.Wrap(err, "error querying the database")
	}
	if mediaMetadata == nil {
		if r.MediaMetadata.Origin == cfg.Matrix.ServerName {
			// If we do not have a record and the origin is local, the file is not found
			return nil, nil
		}
		// If we do not have a record and the origin is remote, we need to fetch it and respond with that file
		resErr := r.getRemoteFile(
			ctx, client, cfg, db, activeRemoteRequests, activeThumbnailGeneration,
		)
		if resErr != nil {
			return nil, resErr
		}
	} else {
		// If we have a record, we can respond from the local file
		r.MediaMetadata = mediaMetadata
	}
	return r.respondFromLocalFile(
		ctx, w, cfg.Media.AbsBasePath, activeThumbnailGeneration,
		cfg.Media.MaxThumbnailGenerators, db,
		cfg.Media.DynamicThumbnails, cfg.Media.ThumbnailSizes,
	)
}

// respondFromLocalFile reads a file from local storage and writes it to the http.ResponseWriter
// If no file was found then returns nil, nil
func (r *downloadRequest) respondFromLocalFile(
	ctx context.Context,
	w http.ResponseWriter,
	absBasePath config.Path,
	activeThumbnailGeneration *types.ActiveThumbnailGeneration,
	maxThumbnailGenerators int,
	db *storage.Database,
	dynamicThumbnails bool,
	thumbnailSizes []config.ThumbnailSize,
) (*types.MediaMetadata, error) {
	filePath, err := fileutils.GetPathFromBase64Hash(r.MediaMetadata.Base64Hash, absBasePath)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get file path from metadata")
	}
	file, err := os.Open(filePath)
	defer file.Close() // nolint: errcheck, staticcheck, megacheck
	if err != nil {
		return nil, errors.Wrap(err, "failed to open file")
	}
	stat, err := file.Stat()
	if err != nil {
		return nil, errors.Wrap(err, "failed to stat file")
	}

	if r.MediaMetadata.FileSizeBytes > 0 && int64(r.MediaMetadata.FileSizeBytes) != stat.Size() {
		r.Logger.WithFields(log.Fields{
			"fileSizeDatabase": r.MediaMetadata.FileSizeBytes,
			"fileSizeDisk":     stat.Size(),
		}).Warn("File size in database and on-disk differ.")
		return nil, errors.New("file size in database and on-disk differ")
	}

	var responseFile *os.File
	var responseMetadata *types.MediaMetadata
	if r.IsThumbnailRequest {
		thumbFile, thumbMetadata, resErr := r.getThumbnailFile(
			ctx, types.Path(filePath), activeThumbnailGeneration, maxThumbnailGenerators,
			db, dynamicThumbnails, thumbnailSizes,
		)
		if thumbFile != nil {
			defer thumbFile.Close() // nolint: errcheck
		}
		if resErr != nil {
			return nil, resErr
		}
		if thumbFile == nil {
			r.Logger.WithFields(log.Fields{
				"UploadName":    r.MediaMetadata.UploadName,
				"Base64Hash":    r.MediaMetadata.Base64Hash,
				"FileSizeBytes": r.MediaMetadata.FileSizeBytes,
				"ContentType":   r.MediaMetadata.ContentType,
			}).Info("No good thumbnail found. Responding with original file.")
			responseFile = file
			responseMetadata = r.MediaMetadata
		} else {
			r.Logger.Info("Responding with thumbnail")
			responseFile = thumbFile
			responseMetadata = thumbMetadata.MediaMetadata
		}
	} else {
		r.Logger.WithFields(log.Fields{
			"UploadName":    r.MediaMetadata.UploadName,
			"Base64Hash":    r.MediaMetadata.Base64Hash,
			"FileSizeBytes": r.MediaMetadata.FileSizeBytes,
			"ContentType":   r.MediaMetadata.ContentType,
		}).Info("Responding with file")
		responseFile = file
		responseMetadata = r.MediaMetadata
	}

	w.Header().Set("Content-Type", string(responseMetadata.ContentType))
	w.Header().Set("Content-Length", strconv.FormatInt(int64(responseMetadata.FileSizeBytes), 10))
	contentSecurityPolicy := "default-src 'none';" +
		" script-src 'none';" +
		" plugin-types application/pdf;" +
		" style-src 'unsafe-inline';" +
		" object-src 'self';"
	w.Header().Set("Content-Security-Policy", contentSecurityPolicy)

	if _, err := io.Copy(w, responseFile); err != nil {
		return nil, errors.Wrap(err, "failed to copy from cache")
	}
	return responseMetadata, nil
}

// Note: Thumbnail generation may be ongoing asynchronously.
// If no thumbnail was found then returns nil, nil, nil
func (r *downloadRequest) getThumbnailFile(
	ctx context.Context,
	filePath types.Path,
	activeThumbnailGeneration *types.ActiveThumbnailGeneration,
	maxThumbnailGenerators int,
	db *storage.Database,
	dynamicThumbnails bool,
	thumbnailSizes []config.ThumbnailSize,
) (*os.File, *types.ThumbnailMetadata, error) {
	var thumbnail *types.ThumbnailMetadata
	var err error

	if dynamicThumbnails {
		thumbnail, err = r.generateThumbnail(
			ctx, filePath, r.ThumbnailSize, activeThumbnailGeneration,
			maxThumbnailGenerators, db,
		)
		if err != nil {
			return nil, nil, err
		}
	}
	// If dynamicThumbnails is true but there are too many thumbnails being actively generated, we can fall back
	// to trying to use a pre-generated thumbnail
	if thumbnail == nil {
		var thumbnails []*types.ThumbnailMetadata
		thumbnails, err = db.GetThumbnails(
			ctx, r.MediaMetadata.MediaID, r.MediaMetadata.Origin,
		)
		if err != nil {
			return nil, nil, errors.Wrap(err, "error looking up thumbnails")
		}

		// If we get a thumbnailSize, a pre-generated thumbnail would be best but it is not yet generated.
		// If we get a thumbnail, we're done.
		var thumbnailSize *types.ThumbnailSize
		thumbnail, thumbnailSize = thumbnailer.SelectThumbnail(r.ThumbnailSize, thumbnails, thumbnailSizes)
		// If dynamicThumbnails is true and we are not over-loaded then we would have generated what was requested above.
		// So we don't try to generate a pre-generated thumbnail here.
		if thumbnailSize != nil && !dynamicThumbnails {
			r.Logger.WithFields(log.Fields{
				"Width":        thumbnailSize.Width,
				"Height":       thumbnailSize.Height,
				"ResizeMethod": thumbnailSize.ResizeMethod,
			}).Info("Pre-generating thumbnail for immediate response.")
			thumbnail, err = r.generateThumbnail(
				ctx, filePath, *thumbnailSize, activeThumbnailGeneration,
				maxThumbnailGenerators, db,
			)
			if err != nil {
				return nil, nil, err
			}
		}
	}
	if thumbnail == nil {
		return nil, nil, nil
	}
	r.Logger = r.Logger.WithFields(log.Fields{
		"Width":         thumbnail.ThumbnailSize.Width,
		"Height":        thumbnail.ThumbnailSize.Height,
		"ResizeMethod":  thumbnail.ThumbnailSize.ResizeMethod,
		"FileSizeBytes": thumbnail.MediaMetadata.FileSizeBytes,
		"ContentType":   thumbnail.MediaMetadata.ContentType,
	})
	thumbPath := string(thumbnailer.GetThumbnailPath(types.Path(filePath), thumbnail.ThumbnailSize))
	thumbFile, err := os.Open(string(thumbPath))
	if err != nil {
		thumbFile.Close() // nolint: errcheck
		return nil, nil, errors.Wrap(err, "failed to open file")
	}
	thumbStat, err := thumbFile.Stat()
	if err != nil {
		thumbFile.Close() // nolint: errcheck
		return nil, nil, errors.Wrap(err, "failed to stat file")
	}
	if types.FileSizeBytes(thumbStat.Size()) != thumbnail.MediaMetadata.FileSizeBytes {
		thumbFile.Close() // nolint: errcheck
		return nil, nil, errors.New("thumbnail file sizes on disk and in database differ")
	}
	return thumbFile, thumbnail, nil
}

func (r *downloadRequest) generateThumbnail(
	ctx context.Context,
	filePath types.Path,
	thumbnailSize types.ThumbnailSize,
	activeThumbnailGeneration *types.ActiveThumbnailGeneration,
	maxThumbnailGenerators int,
	db *storage.Database,
) (*types.ThumbnailMetadata, error) {
	r.Logger.WithFields(log.Fields{
		"Width":        thumbnailSize.Width,
		"Height":       thumbnailSize.Height,
		"ResizeMethod": thumbnailSize.ResizeMethod,
	})
	busy, err := thumbnailer.GenerateThumbnail(
		ctx, filePath, thumbnailSize, r.MediaMetadata,
		activeThumbnailGeneration, maxThumbnailGenerators, db, r.Logger,
	)
	if err != nil {
		return nil, errors.Wrap(err, "error creating thumbnail")
	}
	if busy {
		return nil, nil
	}
	var thumbnail *types.ThumbnailMetadata
	thumbnail, err = db.GetThumbnail(
		ctx, r.MediaMetadata.MediaID, r.MediaMetadata.Origin,
		thumbnailSize.Width, thumbnailSize.Height, thumbnailSize.ResizeMethod,
	)
	if err != nil {
		return nil, errors.Wrap(err, "error looking up thumbnail")
	}
	return thumbnail, nil
}

// getRemoteFile fetches the remote file and caches it locally
// A hash map of active remote requests to a struct containing a sync.Cond is used to only download remote files once,
// regardless of how many download requests are received.
// Note: The named errorResponse return variable is used in a deferred broadcast of the metadata and error response to waiting goroutines.
func (r *downloadRequest) getRemoteFile(
	ctx context.Context,
	client *gomatrixserverlib.Client,
	cfg *config.Dendrite,
	db *storage.Database,
	activeRemoteRequests *types.ActiveRemoteRequests,
	activeThumbnailGeneration *types.ActiveThumbnailGeneration,
) (errorResponse error) {
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
				r.broadcastMediaMetadata(activeRemoteRequests, errors.New("paniced"))
				panic(err)
			}
			r.broadcastMediaMetadata(activeRemoteRequests, errorResponse)
		}()

		// check if we have a record of the media in our database
		mediaMetadata, err := db.GetMediaMetadata(
			ctx, r.MediaMetadata.MediaID, r.MediaMetadata.Origin,
		)
		if err != nil {
			return errors.Wrap(err, "error querying the database.")
		}

		if mediaMetadata == nil {
			// If we do not have a record, we need to fetch the remote file first and then respond from the local file
			err := r.fetchRemoteFileAndStoreMetadata(
				ctx, client,
				cfg.Media.AbsBasePath, *cfg.Media.MaxFileSizeBytes, db,
				cfg.Media.ThumbnailSizes, activeThumbnailGeneration,
				cfg.Media.MaxThumbnailGenerators,
			)
			if err != nil {
				return errors.Wrap(err, "error querying the database.")
			}
		} else {
			// If we have a record, we can respond from the local file
			r.MediaMetadata = mediaMetadata
		}
	}
	return nil
}

func (r *downloadRequest) getMediaMetadataFromActiveRequest(activeRemoteRequests *types.ActiveRemoteRequests) (*types.MediaMetadata, error) {
	// Check if there is an active remote request for the file
	mxcURL := "mxc://" + string(r.MediaMetadata.Origin) + "/" + string(r.MediaMetadata.MediaID)

	activeRemoteRequests.Lock()
	defer activeRemoteRequests.Unlock()

	if activeRemoteRequestResult, ok := activeRemoteRequests.MXCToResult[mxcURL]; ok {
		r.Logger.Info("Waiting for another goroutine to fetch the remote file.")

		// NOTE: Wait unlocks and locks again internally. There is still a deferred Unlock() that will unlock this.
		activeRemoteRequestResult.Cond.Wait()
		if activeRemoteRequestResult.Error != nil {
			return nil, activeRemoteRequestResult.Error
		}

		if activeRemoteRequestResult.MediaMetadata == nil {
			return nil, nil
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
func (r *downloadRequest) broadcastMediaMetadata(activeRemoteRequests *types.ActiveRemoteRequests, err error) {
	activeRemoteRequests.Lock()
	defer activeRemoteRequests.Unlock()
	mxcURL := "mxc://" + string(r.MediaMetadata.Origin) + "/" + string(r.MediaMetadata.MediaID)
	if activeRemoteRequestResult, ok := activeRemoteRequests.MXCToResult[mxcURL]; ok {
		r.Logger.Info("Signalling other goroutines waiting for this goroutine to fetch the file.")
		activeRemoteRequestResult.MediaMetadata = r.MediaMetadata
		activeRemoteRequestResult.Error = err
		activeRemoteRequestResult.Cond.Broadcast()
	}
	delete(activeRemoteRequests.MXCToResult, mxcURL)
}

// fetchRemoteFileAndStoreMetadata fetches the file from the remote server and stores its metadata in the database
func (r *downloadRequest) fetchRemoteFileAndStoreMetadata(
	ctx context.Context,
	client *gomatrixserverlib.Client,
	absBasePath config.Path,
	maxFileSizeBytes config.FileSizeBytes,
	db *storage.Database,
	thumbnailSizes []config.ThumbnailSize,
	activeThumbnailGeneration *types.ActiveThumbnailGeneration,
	maxThumbnailGenerators int,
) error {
	finalPath, duplicate, err := r.fetchRemoteFile(
		ctx, client, absBasePath, maxFileSizeBytes,
	)
	if err != nil {
		return err
	}

	r.Logger.WithFields(log.Fields{
		"Base64Hash":    r.MediaMetadata.Base64Hash,
		"UploadName":    r.MediaMetadata.UploadName,
		"FileSizeBytes": r.MediaMetadata.FileSizeBytes,
		"ContentType":   r.MediaMetadata.ContentType,
	}).Info("Storing file metadata to media repository database")

	// FIXME: timeout db request
	if err := db.StoreMediaMetadata(ctx, r.MediaMetadata); err != nil {
		// If the file is a duplicate (has the same hash as an existing file) then
		// there is valid metadata in the database for that file. As such we only
		// remove the file if it is not a duplicate.
		if !duplicate {
			finalDir := filepath.Dir(string(finalPath))
			fileutils.RemoveDir(types.Path(finalDir), r.Logger)
		}
		// NOTE: It should really not be possible to fail the uniqueness test here so
		// there is no need to handle that separately
		return errors.New("failed to store file metadata in DB")
	}

	go func() {
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

	r.Logger.WithFields(log.Fields{
		"UploadName":    r.MediaMetadata.UploadName,
		"Base64Hash":    r.MediaMetadata.Base64Hash,
		"FileSizeBytes": r.MediaMetadata.FileSizeBytes,
		"ContentType":   r.MediaMetadata.ContentType,
	}).Infof("Remote file cached")

	return nil
}

func (r *downloadRequest) fetchRemoteFile(
	ctx context.Context,
	client *gomatrixserverlib.Client,
	absBasePath config.Path,
	maxFileSizeBytes config.FileSizeBytes,
) (types.Path, bool, error) {
	r.Logger.Info("Fetching remote file")

	// create request for remote file
	resp, err := r.createRemoteRequest(ctx, client)
	if err != nil {
		return "", false, err
	}
	if resp == nil {
		// Remote file not found
		return "", false, nil
	}
	defer resp.Body.Close() // nolint: errcheck

	// get metadata from request and set metadata on response
	contentLength, err := strconv.ParseInt(resp.Header.Get("Content-Length"), 10, 64)
	if err != nil {
		r.Logger.WithError(err).Warn("Failed to parse content length")
		return "", false, errors.Wrap(err, "invalid response from remote server")
	}
	if contentLength > int64(maxFileSizeBytes) {
		// TODO: Bubble up this as a 413
		return "", false, fmt.Errorf("remote file is too large (%v > %v bytes)", contentLength, maxFileSizeBytes)
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
		return "", false, errors.New("file could not be downloaded from remote server")
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
		return "", false, errors.Wrap(err, "failed to move file")
	}
	if duplicate {
		r.Logger.WithField("dst", finalPath).Info("File was stored previously - discarding duplicate")
		// Continue on to store the metadata in the database
	}

	return types.Path(finalPath), duplicate, nil
}

func (r *downloadRequest) createRemoteRequest(
	ctx context.Context, matrixClient *gomatrixserverlib.Client,
) (*http.Response, error) {
	resp, err := matrixClient.CreateMediaDownloadRequest(ctx, r.MediaMetadata.Origin, string(r.MediaMetadata.MediaID))
	if err != nil {
		return nil, fmt.Errorf("file with media ID %q could not be downloaded from %q", r.MediaMetadata.MediaID, r.MediaMetadata.Origin)
	}

	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode == http.StatusNotFound {
			return nil, nil
		}
		r.Logger.WithFields(log.Fields{
			"StatusCode": resp.StatusCode,
		}).Warn("Received error response")
		return nil, fmt.Errorf("file with media ID %q could not be downloaded from %q", r.MediaMetadata.MediaID, r.MediaMetadata.Origin)
	}

	return resp, nil
}
