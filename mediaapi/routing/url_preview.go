package routing

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
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
	ErrNoMetadataFound             = errors.New("no metadata found")
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

		hash := getHashFromString(pUrl)
		// Check if we have a previously stored response
		if urlPreviewCached, err := loadUrlPreviewResponse(req.Context(), cfg, db, hash); err == nil {
			logger.Debug("Loaded url preview from the cache")
			// Put in into the cache for further usage
			defer func() {
				if _, ok := urlPreviewCache.Records[pUrl]; !ok {

					urlPreviewCacheItem := &types.UrlPreviewCacheRecord{
						Created: time.Now().Unix(),
						Preview: urlPreviewCached,
					}
					urlPreviewCache.Lock.Lock()
					urlPreviewCache.Records[pUrl] = urlPreviewCacheItem
					defer urlPreviewCache.Lock.Unlock()
				}
			}()

			return util.JSONResponse{
				Code: http.StatusOK,
				JSON: urlPreviewCached,
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
				// Store the response file for further usage
				err := storeUrlPreviewResponse(req.Context(), cfg, db, *device, hash, activeUrlPreviewRequest.Preview, logger)
				if err != nil {
					logger.WithError(err).Error("unable to store url preview response")
				}
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
			defer func() {
				err := resp.Body.Close()
				if err != nil {
					logger.WithError(err).Error("unable to close response body")
				}
			}()

			var result *types.UrlPreview
			var err, err2 error
			var imgUrl *url.URL
			var imgReader *http.Response
			var mediaData *types.MediaMetadata
			var width, height int

			if strings.HasPrefix(resp.Header.Get("Content-Type"), "text/html") {
				// The url is a webpage - get data from the meta tags
				result = getPreviewFromHTML(resp, pUrl)
				if result.ImageUrl != "" {
					// The page has an og:image link
					if imgUrl, err2 = url.Parse(result.ImageUrl); err2 == nil {
						imgReader, err2 = downloadUrl(result.ImageUrl, time.Duration(cfg.UrlPreviewTimeout)*time.Second)
						if err2 == nil {
							// Download image and store it as a thumbnail
							mediaData, width, height, err2 = downloadAndStoreImage(imgUrl.Path, req.Context(), imgReader, cfg, device, db, activeThumbnailGeneration, logger)
						}
					}
					// In case of any error in image download
					// we don't show the original URL as it is insecure for the room users
					if err2 != nil {
						result.ImageUrl = ""
					}

				}
			} else if strings.HasPrefix(resp.Header.Get("Content-Type"), "image/") {
				// The url is an image link
				mediaData, width, height, err = downloadAndStoreImage("somefile", req.Context(), resp, cfg, device, db, activeThumbnailGeneration, logger)
				if err == nil {
					result = &types.UrlPreview{}
				}
			} else {
				return util.ErrorResponse(errors.New("Unsupported content type"))
			}

			// In case of any error happened during the page/image download
			// we store the error instead of the preview
			if err != nil {
				activeUrlPreviewRequest.Error = err
			} else {
				// We have a mediadata so we have an image in the preview
				if mediaData != nil {
					result.ImageUrl = fmt.Sprintf("mxc://%s/%s", mediaData.Origin, mediaData.MediaID)
					result.ImageWidth = width
					result.ImageHeight = height
					result.ImageType = mediaData.ContentType
					result.ImageSize = mediaData.FileSizeBytes
				}

				activeUrlPreviewRequest.Preview = result
			}
		}

		// Return eather the error or the preview
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

func getPreviewFromHTML(resp *http.Response, url string) *types.UrlPreview {
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

	return preview
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

) (*types.MediaMetadata, int, int, error) {

	var width, height int

	userid := types.MatrixUserID(dev.UserID)

	reqReader := req.Body.(io.Reader)
	if cfg.MaxFileSizeBytes > 0 {
		reqReader = io.LimitReader(reqReader, int64(cfg.MaxFileSizeBytes)+1)
	}
	hash, bytesWritten, tmpDir, err := fileutils.WriteTempFile(ctx, reqReader, cfg.AbsBasePath)
	if err != nil {
		logger.WithError(err).WithFields(log.Fields{
			"MaxFileSizeBytes": cfg.MaxFileSizeBytes,
		}).Warn("Error while transferring file")
		return nil, width, height, err
	}
	defer fileutils.RemoveDir(tmpDir, logger)

	// Check if temp file size exceeds max file size configuration
	if cfg.MaxFileSizeBytes > 0 && bytesWritten > types.FileSizeBytes(cfg.MaxFileSizeBytes) {
		return nil, 0, 0, ErrorFileTooLarge
	}

	// Check if we already have this file
	existingMetadata, err := db.GetMediaMetadataByHash(
		ctx, hash, cfg.Matrix.ServerName,
	)

	if err != nil {
		logger.WithError(err).Error("unable to get media metadata by hash")
		return nil, width, height, err
	}

	if existingMetadata != nil {

		logger.WithField("mediaID", existingMetadata.MediaID).Debug("media already exists")
		// Here we have to read the image to get it's size
		filePath, err := fileutils.GetPathFromBase64Hash(existingMetadata.Base64Hash, cfg.AbsBasePath)
		if err != nil {
			return nil, width, height, err
		}
		img, err := thumbnailer.ReadFile(string(filePath))
		if err != nil {
			return nil, width, height, err
		}
		return existingMetadata, img.Bounds().Dx(), img.Bounds().Dy(), nil
	}

	tmpFileName := filepath.Join(string(tmpDir), "content")
	fileType, err := detectFileType(tmpFileName, logger)
	if err != nil {
		logger.WithError(err).Error("unable to detect file type")
		return nil, width, height, err
	}
	logger.WithField("contentType", fileType).Debug("uploaded file is an image")

	// Create a thumbnail from the image
	thumbnailPath := tmpFileName + ".thumbnail"

	width, height, err = createThumbnail(types.Path(tmpFileName), types.Path(thumbnailPath), types.ThumbnailSize(cfg.UrlPreviewThumbnailSize),
		hash, activeThumbnailGeneration, cfg.MaxThumbnailGenerators, logger)
	if err != nil {
		if errors.Is(err, thumbnailer.ErrThumbnailTooLarge) {
			thumbnailPath = tmpFileName
		} else {
			return nil, width, height, err
		}
	}

	thumbnailFileInfo, err := os.Stat(string(thumbnailPath))
	if err != nil {
		logger.WithError(err).Error("unable to get thumbnail file info")
		return nil, width, height, err
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
		return nil, width, height, err
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
		return nil, width, height, err
	}
	err = fileutils.MoveFile(types.Path(thumbnailPath), types.Path(finalPath))
	if err != nil {
		logger.WithError(err).Error("unable to move thumbnail file")
		return nil, width, height, err
	}
	// Store the metadata in the database
	err = db.StoreMediaMetadata(ctx, mediaMetaData)
	if err != nil {
		logger.WithError(err).Error("unable to store media metadata")
		return nil, width, height, err
	}

	return mediaMetaData, width, height, nil
}

func createThumbnail(src types.Path, dst types.Path, size types.ThumbnailSize, hash types.Base64Hash, activeThumbnailGeneration *types.ActiveThumbnailGeneration, maxThumbnailGenerators int, logger *log.Entry) (int, int, error) {
	// Check if we have too many thumbnail generators running
	// If so, wait up to 30 seconds for one to finish
	timeout := time.After(30 * time.Second)
	for {
		if len(activeThumbnailGeneration.PathToResult) < maxThumbnailGenerators {

			activeThumbnailGeneration.Lock()
			activeThumbnailGeneration.PathToResult[string(hash)] = nil
			activeThumbnailGeneration.Unlock()

			defer func() {
				activeThumbnailGeneration.Lock()
				delete(activeThumbnailGeneration.PathToResult, string(hash))
				activeThumbnailGeneration.Unlock()
			}()

			width, height, err := thumbnailer.CreateThumbnailFromFile(src, dst, size, logger)
			if err != nil {
				logger.WithError(err).Error("unable to create thumbnail")
				return 0, 0, err
			}
			return width, height, nil
		}

		select {
		case <-timeout:
			logger.Error("timed out waiting for thumbnail generator")
			return 0, 0, ErrorTimeoutThumbnailGenerator
		default:
			time.Sleep(time.Second)
		}
	}
}

func storeUrlPreviewResponse(ctx context.Context, cfg *config.MediaAPI, db storage.Database, user userapi.Device, hash types.Base64Hash, preview *types.UrlPreview, logger *log.Entry) error {

	jsonPreview, err := json.Marshal(preview)
	if err != nil {
		return err
	}

	_, bytesWritten, tmpDir, err := fileutils.WriteTempFile(ctx, bytes.NewReader(jsonPreview), cfg.AbsBasePath)
	if err != nil {
		return err
	}
	defer fileutils.RemoveDir(tmpDir, logger)

	r := &uploadRequest{
		MediaMetadata: &types.MediaMetadata{
			Origin: cfg.Matrix.ServerName,
		},
		Logger: logger,
	}

	mediaID, err := r.generateMediaID(ctx, db)
	if err != nil {
		return err
	}

	mediaMetaData := &types.MediaMetadata{
		MediaID:           mediaID,
		Origin:            cfg.Matrix.ServerName,
		ContentType:       "application/json",
		FileSizeBytes:     types.FileSizeBytes(bytesWritten),
		UploadName:        types.Filename("url_preview.json"),
		CreationTimestamp: spec.Timestamp(time.Now().Unix()),
		Base64Hash:        hash,
		UserID:            types.MatrixUserID(user.UserID),
	}

	_, _, err = fileutils.MoveFileWithHashCheck(tmpDir, mediaMetaData, cfg.AbsBasePath, logger)
	if err != nil {
		return err
	}

	err = db.StoreMediaMetadata(ctx, mediaMetaData)
	if err != nil {
		logger.WithError(err).Error("unable to store media metadata")
		return err
	}
	return nil
}

func loadUrlPreviewResponse(ctx context.Context, cfg *config.MediaAPI, db storage.Database, hash types.Base64Hash) (*types.UrlPreview, error) {
	if mediaMetadata, err := db.GetMediaMetadataByHash(ctx, hash, cfg.Matrix.ServerName); err == nil && mediaMetadata != nil {
		// Get the response file
		filePath, err := fileutils.GetPathFromBase64Hash(mediaMetadata.Base64Hash, cfg.AbsBasePath)
		if err != nil {
			return nil, err
		}
		data, err := os.ReadFile(string(filePath))
		if err != nil {
			return nil, err
		}
		var preview types.UrlPreview
		err = json.Unmarshal(data, &preview)
		if err != nil {
			return nil, err
		}
		return &preview, nil
	}
	return nil, ErrNoMetadataFound
}

func detectFileType(filePath string, logger *log.Entry) (string, error) {
	// Check if the file is an image.
	// Otherwise return an error
	file, err := os.Open(string(filePath))
	if err != nil {
		logger.WithError(err).Error("unable to open image file")
		return "", err
	}
	defer func() {
		err := file.Close()
		if err != nil {
			logger.WithError(err).Error("unable to close image file")
		}
	}()

	buf := make([]byte, 512)

	_, err = file.Read(buf)
	if err != nil {
		logger.WithError(err).Error("unable to read file")
		return "", err
	}

	fileType := http.DetectContentType(buf)
	if !strings.HasPrefix(fileType, "image") {
		logger.WithField("contentType", fileType).Debugf("uploaded file is not an image")
		return "", ErrorUnsupportedContentType
	}
	return fileType, nil
}

func getHashFromString(s string) types.Base64Hash {
	hasher := sha256.New()
	hasher.Write([]byte(s))
	return types.Base64Hash(base64.RawURLEncoding.EncodeToString(hasher.Sum(nil)))
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
