package routing

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/matrix-org/dendrite/internal/httputil"
	"github.com/matrix-org/dendrite/mediaapi/fileutils"
	"github.com/matrix-org/dendrite/mediaapi/storage"
	"github.com/matrix-org/dendrite/mediaapi/thumbnailer"
	"github.com/matrix-org/dendrite/mediaapi/types"
	"github.com/matrix-org/dendrite/setup/config"
	userapi "github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/gomatrixserverlib/spec"
	"github.com/matrix-org/util"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"golang.org/x/net/html"
)

var (
	ErrorMissingUrl                = errors.New("missing url")
	ErrorUnsupportedContentType    = errors.New("unsupported content type")
	ErrorFileTooLarge              = errors.New("file too large")
	ErrorTimeoutThumbnailGenerator = errors.New("timeout waiting for thumbnail generator")
)

func makeUrlPreviewHandler(
	cfg *config.MediaAPI,
	rateLimits *httputil.RateLimits,
	db storage.Database,
	activeThumbnailGeneration *types.ActiveThumbnailGeneration,
) func(req *http.Request, device *userapi.Device) util.JSONResponse {

	activeUrlPreviewRequests := &types.ActiveUrlPreviewRequests{Url: map[string]*types.UrlPreviewResult{}}
	urlPreviewCache := &types.UrlPreviewCache{Records: map[string]*types.UrlPreviewCacheRecord{}}

	go func() {
		for {
			t := time.Now().Unix()
			for k, record := range urlPreviewCache.Records {
				if record.Created < (t - int64(cfg.UrlPreviewCacheTime)) {
					urlPreviewCache.Lock.Lock()
					delete(urlPreviewCache.Records, k)
					urlPreviewCache.Lock.Unlock()
				}
			}
			time.Sleep(time.Duration(16) * time.Second)
		}
	}()

	httpHandler := func(req *http.Request, device *userapi.Device) util.JSONResponse {
		req = util.RequestWithLogging(req)

		// log := util.GetLogger(req.Context())
		// Here be call to the url preview handler
		pUrl := req.URL.Query().Get("url")
		ts := req.URL.Query().Get("ts")
		if pUrl == "" {
			return util.ErrorResponse(ErrorMissingUrl)
		}
		_ = ts

		logger := util.GetLogger(req.Context()).WithFields(log.Fields{
			"url": pUrl,
		})
		// Check rate limits
		if r := rateLimits.Limit(req, device); r != nil {
			return *r
		}

		// Get url preview from cache
		if cacheRecord, ok := urlPreviewCache.Records[pUrl]; ok {
			if cacheRecord.Error != nil {
				return util.ErrorResponse(cacheRecord.Error)
			}
			return util.JSONResponse{
				Code: http.StatusOK,
				JSON: cacheRecord.Preview,
			}
		}

		// Check if there is an active request
		activeUrlPreviewRequests.Lock()
		if activeUrlPreviewRequest, ok := activeUrlPreviewRequests.Url[pUrl]; ok {
			activeUrlPreviewRequests.Unlock()
			// Wait for it to complete
			activeUrlPreviewRequest.Cond.L.Lock()
			defer activeUrlPreviewRequest.Cond.L.Unlock()
			activeUrlPreviewRequest.Cond.Wait()

			if activeUrlPreviewRequest.Error != nil {
				return util.ErrorResponse(activeUrlPreviewRequest.Error)
			}
			return util.JSONResponse{
				Code: http.StatusOK,
				JSON: activeUrlPreviewRequest.Preview,
			}
		}

		// Start new url preview request
		activeUrlPreviewRequest := &types.UrlPreviewResult{Cond: sync.NewCond(&sync.Mutex{})}
		activeUrlPreviewRequests.Url[pUrl] = activeUrlPreviewRequest
		activeUrlPreviewRequests.Unlock()

		// we defer caching the url preview response as well as signalling the waiting goroutines
		// about the completion of the request
		defer func() {
			urlPreviewCacheItem := &types.UrlPreviewCacheRecord{
				Created: time.Now().Unix(),
			}
			if activeUrlPreviewRequest.Error != nil {
				urlPreviewCacheItem.Error = activeUrlPreviewRequest.Error
			} else {
				urlPreviewCacheItem.Preview = activeUrlPreviewRequest.Preview
			}

			urlPreviewCache.Lock.Lock()
			urlPreviewCache.Records[pUrl] = urlPreviewCacheItem
			defer urlPreviewCache.Lock.Unlock()

			activeUrlPreviewRequests.Lock()
			activeUrlPreviewRequests.Url[pUrl].Cond.Broadcast()
			delete(activeUrlPreviewRequests.Url, pUrl)
			defer activeUrlPreviewRequests.Unlock()
		}()

		resp, err := downloadUrl(pUrl, time.Duration(cfg.UrlPreviewTimeout)*time.Second)
		if err != nil {
			activeUrlPreviewRequest.Error = err
		} else {
			defer resp.Body.Close()

			var result *types.UrlPreview
			var err error
			var imgUrl *url.URL
			var imgReader *http.Response
			var mediaData *types.MediaMetadata

			if strings.HasPrefix(resp.Header.Get("Content-Type"), "text/html") {
				result, err = getPreviewFromHTML(resp, pUrl)
				if err == nil && result.ImageUrl != "" {
					if imgUrl, err = url.Parse(result.ImageUrl); err == nil {
						imgReader, err = downloadUrl(result.ImageUrl, time.Duration(cfg.UrlPreviewTimeout)*time.Second)
						if err == nil {
							mediaData, err = downloadAndStoreImage(imgUrl.Path, req.Context(), imgReader, cfg, device, db, activeThumbnailGeneration, logger)
							if err == nil {
								result.ImageUrl = fmt.Sprintf("mxc://%s/%s", mediaData.Origin, mediaData.MediaID)
							} else {
								// We don't show the orginal URL as it is insecure for the room users
								result.ImageUrl = ""
							}
						}
					}
				}
			} else if strings.HasPrefix(resp.Header.Get("Content-Type"), "image/") {
				mediaData, err := downloadAndStoreImage("somefile", req.Context(), resp, cfg, device, db, activeThumbnailGeneration, logger)
				if err == nil {
					result = &types.UrlPreview{ImageUrl: fmt.Sprintf("mxc://%s/%s", mediaData.Origin, mediaData.MediaID)}
				}
			} else {
				return util.ErrorResponse(errors.New("Unsupported content type"))
			}

			if err != nil {
				activeUrlPreviewRequest.Error = err
			} else {
				activeUrlPreviewRequest.Preview = result
			}
		}

		// choose the answer based on the result
		if activeUrlPreviewRequest.Error != nil {
			return util.ErrorResponse(activeUrlPreviewRequest.Error)
		} else {
			return util.JSONResponse{
				Code: http.StatusOK,
				JSON: activeUrlPreviewRequest.Preview,
			}
		}
	}

	return httpHandler

}

func downloadUrl(url string, t time.Duration) (*http.Response, error) {
	client := http.Client{Timeout: t}
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, errors.New("HTTP status code: " + strconv.Itoa(resp.StatusCode))
	}

	return resp, nil
}

func getPreviewFromHTML(resp *http.Response, url string) (*types.UrlPreview, error) {
	fields := getMetaFieldsFromHTML(resp)
	preview := &types.UrlPreview{
		Title:       fields["og:title"],
		Description: fields["og:description"],
	}

	if fields["og:title"] == "" {
		preview.Title = url
	}
	if fields["og:image"] != "" {
		preview.ImageUrl = fields["og:image"]
	} else if fields["og:image:url"] != "" {
		preview.ImageUrl = fields["og:image:url"]
	} else if fields["og:image:secure_url"] != "" {
		preview.ImageUrl = fields["og:image:secure_url"]
	}

	if fields["og:image:width"] != "" {
		if width, err := strconv.Atoi(fields["og:image:width"]); err == nil {
			preview.ImageWidth = width
		}
	}
	if fields["og:image:height"] != "" {
		if height, err := strconv.Atoi(fields["og:image:height"]); err == nil {
			preview.ImageHeight = height
		}
	}

	return preview, nil
}

func downloadAndStoreImage(
	filename string,
	ctx context.Context,
	req *http.Response,
	cfg *config.MediaAPI,
	dev *userapi.Device,
	db storage.Database,
	activeThumbnailGeneration *types.ActiveThumbnailGeneration,
	logger *log.Entry,

) (*types.MediaMetadata, error) {

	userid := types.MatrixUserID("user")
	if dev != nil {
		userid = types.MatrixUserID(dev.UserID)
	}

	reqReader := req.Body.(io.Reader)
	if cfg.MaxFileSizeBytes > 0 {
		reqReader = io.LimitReader(reqReader, int64(cfg.MaxFileSizeBytes)+1)
	}
	hash, bytesWritten, tmpDir, err := fileutils.WriteTempFile(ctx, reqReader, cfg.AbsBasePath)
	if err != nil {
		logger.WithError(err).WithFields(log.Fields{
			"MaxFileSizeBytes": cfg.MaxFileSizeBytes,
		}).Warn("Error while transferring file")
		return nil, err
	}
	defer fileutils.RemoveDir(tmpDir, logger)

	// Check if temp file size exceeds max file size configuration
	if cfg.MaxFileSizeBytes > 0 && bytesWritten > types.FileSizeBytes(cfg.MaxFileSizeBytes) {
		return nil, ErrorFileTooLarge
	}

	// Check if we already have this file
	existingMetadata, err := db.GetMediaMetadataByHash(
		ctx, hash, cfg.Matrix.ServerName,
	)

	if err != nil {
		logger.WithError(err).Error("unable to get media metadata by hash")
		return nil, err
	}

	if existingMetadata != nil {

		logger.WithField("mediaID", existingMetadata.MediaID).Debug("media already exists")
		return existingMetadata, nil
	}

	tmpFileName := filepath.Join(string(tmpDir), "content")
	// Check if the file is an image.
	// Otherwise return an error
	file, err := os.Open(string(tmpFileName))
	if err != nil {
		logger.WithError(err).Error("unable to open file")
		return nil, err
	}
	defer file.Close()

	buf := make([]byte, 512)

	_, err = file.Read(buf)
	if err != nil {
		logger.WithError(err).Error("unable to read file")
		return nil, err
	}

	fileType := http.DetectContentType(buf)
	if !strings.HasPrefix(fileType, "image") {
		logger.WithField("contentType", fileType).Debugf("uploaded file is not an image or can not be thumbnailed, not generating thumbnails")
		return nil, ErrorUnsupportedContentType
	}
	logger.WithField("contentType", fileType).Debug("uploaded file is an image")

	// Create a thumbnail from the image
	thumbnailPath := tmpFileName + ".thumbnail"

	// Check if we have too many thumbnail generators running
	// If so, wait up to 30 seconds for one to finish
	timeout := time.After(30 * time.Second)
	for {
		if len(activeThumbnailGeneration.PathToResult) < cfg.MaxThumbnailGenerators {
			activeThumbnailGeneration.Lock()
			activeThumbnailGeneration.PathToResult[string(hash)] = nil
			activeThumbnailGeneration.Unlock()

			defer func() {
				activeThumbnailGeneration.Lock()
				delete(activeThumbnailGeneration.PathToResult, string(hash))
				activeThumbnailGeneration.Unlock()
			}()

			err = thumbnailer.CreateThumbnailFromFile(types.Path(tmpFileName), types.Path(thumbnailPath), types.ThumbnailSize(cfg.UrlPreviewThumbnailSize), logger)
			if err != nil {
				if errors.Is(err, thumbnailer.ErrThumbnailTooLarge) {
					thumbnailPath = tmpFileName
				} else {
					logger.WithError(err).Error("unable to create thumbnail")
					return nil, err
				}
			}
			break
		}

		select {
		case <-timeout:
			logger.Error("timed out waiting for thumbnail generator")
			return nil, ErrorTimeoutThumbnailGenerator
		default:
			time.Sleep(time.Second)
		}
	}

	thumbnailFileInfo, err := os.Stat(string(thumbnailPath))
	if err != nil {
		logger.WithError(err).Error("unable to get thumbnail file info")
		return nil, err
	}

	r := &uploadRequest{
		MediaMetadata: &types.MediaMetadata{
			Origin: cfg.Matrix.ServerName,
		},
		Logger: logger,
	}

	// Move the thumbnail to the media store
	mediaID, err := r.generateMediaID(ctx, db)
	if err != nil {
		logger.WithError(err).Error("unable to generate media ID")
		return nil, err
	}
	mediaMetaData := &types.MediaMetadata{
		MediaID:           mediaID,
		Origin:            cfg.Matrix.ServerName,
		ContentType:       types.ContentType(fileType),
		FileSizeBytes:     types.FileSizeBytes(thumbnailFileInfo.Size()),
		UploadName:        types.Filename(filename),
		CreationTimestamp: spec.Timestamp(time.Now().Unix()),
		Base64Hash:        hash,
		UserID:            userid,
	}

	finalPath, err := fileutils.GetPathFromBase64Hash(mediaMetaData.Base64Hash, cfg.AbsBasePath)
	if err != nil {
		logger.WithError(err).Error("unable to get path from base64 hash")
		return nil, err
	}
	err = fileutils.MoveFile(types.Path(thumbnailPath), types.Path(finalPath))
	if err != nil {
		logger.WithError(err).Error("unable to move thumbnail file")
		return nil, err
	}
	// Store the metadata in the database
	err = db.StoreMediaMetadata(ctx, mediaMetaData)
	if err != nil {
		logger.WithError(err).Error("unable to store media metadata")
		return nil, err
	}

	return mediaMetaData, nil
}

func getMetaFieldsFromHTML(resp *http.Response) map[string]string {
	htmlTokens := html.NewTokenizer(resp.Body)
	ogValues := map[string]string{}
	fieldsToGet := []string{
		"og:title",
		"og:description",
		"og:image",
		"og:image:url",
		"og:image:secure_url",
		"og:image:width",
		"og:image:height",
		"og:image:type",
	}
	fieldsMap := make(map[string]bool, len(fieldsToGet))
	for _, field := range fieldsToGet {
		fieldsMap[field] = true
		ogValues[field] = ""
	}

	headTagOpened := false
	for {
		tokenType := htmlTokens.Next()
		if tokenType == html.ErrorToken {
			break
		}
		token := htmlTokens.Token()

		// Check if there was opened a head tag
		if tokenType == html.StartTagToken && token.Data == "head" {
			headTagOpened = true
		}
		// We search for meta tags only inside the head tag if it exists
		if headTagOpened && tokenType == html.EndTagToken && token.Data == "head" {
			break
		}
		if (tokenType == html.SelfClosingTagToken || tokenType == html.StartTagToken) && token.Data == "meta" {
			var propertyName string
			var propertyContent string
			for _, attr := range token.Attr {
				if attr.Key == "property" {
					propertyName = attr.Val
				}
				if attr.Key == "content" {
					propertyContent = attr.Val
				}
				if propertyName != "" && propertyContent != "" {
					break
				}
			}
			// Push the values to the map if they are in the required fields list
			if propertyName != "" && propertyContent != "" {
				if _, ok := fieldsMap[propertyName]; ok {
					ogValues[propertyName] = propertyContent
				}
			}
		}
	}
	return ogValues
}
