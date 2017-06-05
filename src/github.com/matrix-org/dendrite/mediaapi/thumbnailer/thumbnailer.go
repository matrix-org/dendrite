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

package thumbnailer

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sync"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/matrix-org/dendrite/mediaapi/storage"
	"github.com/matrix-org/dendrite/mediaapi/types"
	"gopkg.in/h2non/bimg.v1"
)

type thumbnailMetrics struct {
	isSmaller      int
	aspect         float64
	size           float64
	methodMismatch int
	fileSize       types.FileSizeBytes
}

// thumbnailTemplate is the filename template for thumbnails
const thumbnailTemplate = "thumbnail-%vx%v-%v"

// GenerateThumbnails generates the configured thumbnail sizes for the source file
func GenerateThumbnails(src types.Path, configs []types.ThumbnailSize, mediaMetadata *types.MediaMetadata, activeThumbnailGeneration *types.ActiveThumbnailGeneration, db *storage.Database, logger *log.Entry) error {
	buffer, err := bimg.Read(string(src))
	if err != nil {
		logger.WithError(err).WithField("src", src).Error("Failed to read src file")
		return err
	}
	for _, config := range configs {
		// Note: createThumbnail does locking based on activeThumbnailGeneration
		if err = createThumbnail(src, buffer, config, mediaMetadata, activeThumbnailGeneration, db, logger); err != nil {
			logger.WithError(err).WithField("src", src).Error("Failed to generate thumbnails")
			return err
		}
	}
	return nil
}

// GenerateThumbnail generates the configured thumbnail size for the source file
func GenerateThumbnail(src types.Path, config types.ThumbnailSize, mediaMetadata *types.MediaMetadata, activeThumbnailGeneration *types.ActiveThumbnailGeneration, db *storage.Database, logger *log.Entry) error {
	buffer, err := bimg.Read(string(src))
	if err != nil {
		logger.WithError(err).WithFields(log.Fields{
			"src": src,
		}).Error("Failed to read src file")
		return err
	}
	// Note: createThumbnail does locking based on activeThumbnailGeneration
	if err = createThumbnail(src, buffer, config, mediaMetadata, activeThumbnailGeneration, db, logger); err != nil {
		logger.WithError(err).WithFields(log.Fields{
			"src": src,
		}).Error("Failed to generate thumbnails")
		return err
	}
	return nil
}

// GetThumbnailPath returns the path to a thumbnail given the absolute src path and thumbnail size configuration
func GetThumbnailPath(src types.Path, config types.ThumbnailSize) types.Path {
	srcDir := filepath.Dir(string(src))
	return types.Path(filepath.Join(
		srcDir,
		fmt.Sprintf(thumbnailTemplate, config.Width, config.Height, config.ResizeMethod),
	))
}

// SelectThumbnail compares the (potentially) available thumbnails with the desired thumbnail and returns the best match
// The algorithm is very similar to what was implemented in Synapse
// In order of priority unless absolute, the following metrics are compared; the image is:
// * the same size or larger than requested
// * if a cropped image is desired, has an aspect ratio close to requested
// * has a size close to requested
// * if a cropped image is desired, prefer the same method, if scaled is desired, absolutely require scaled
// * has a small file size
// If a pre-generated thumbnail size is the best match, but it has not been generated yet, the caller can use the returned size to generate it.
// Returns nil if no thumbnail matches the criteria
func SelectThumbnail(desired types.ThumbnailSize, thumbnails []*types.ThumbnailMetadata, thumbnailSizes []types.ThumbnailSize) (*types.ThumbnailMetadata, *types.ThumbnailSize) {
	var chosenThumbnail *types.ThumbnailMetadata
	var chosenThumbnailSize *types.ThumbnailSize
	chosenMetrics := newThumbnailMetrics()

	for _, thumbnail := range thumbnails {
		if desired.ResizeMethod == "scale" && thumbnail.ThumbnailSize.ResizeMethod != "scale" {
			continue
		}
		metrics := calcThumbnailMetrics(thumbnail.ThumbnailSize, thumbnail.MediaMetadata, desired)
		if isBetter := metrics.betterThan(chosenMetrics, desired.ResizeMethod == "crop"); isBetter {
			chosenMetrics = metrics
			chosenThumbnail = thumbnail
		}
	}

	for _, thumbnailSize := range thumbnailSizes {
		if desired.ResizeMethod == "scale" && thumbnailSize.ResizeMethod != "scale" {
			continue
		}
		metrics := calcThumbnailMetrics(thumbnailSize, nil, desired)
		if isBetter := metrics.betterThan(chosenMetrics, desired.ResizeMethod == "crop"); isBetter {
			chosenMetrics = metrics
			chosenThumbnailSize = &types.ThumbnailSize{
				Width:        thumbnailSize.Width,
				Height:       thumbnailSize.Height,
				ResizeMethod: thumbnailSize.ResizeMethod,
			}
		}
	}

	return chosenThumbnail, chosenThumbnailSize
}

// createThumbnail checks if the thumbnail exists, and if not, generates it
// Thumbnail generation is only done once for each non-existing thumbnail.
func createThumbnail(src types.Path, buffer []byte, config types.ThumbnailSize, mediaMetadata *types.MediaMetadata, activeThumbnailGeneration *types.ActiveThumbnailGeneration, db *storage.Database, logger *log.Entry) (errorReturn error) {
	dst := GetThumbnailPath(src, config)

	// Note: getActiveThumbnailGeneration uses mutexes and conditions from activeThumbnailGeneration
	isActive, err := getActiveThumbnailGeneration(dst, config, activeThumbnailGeneration, logger)
	if err != nil {
		return err
	}

	if isActive {
		// Note: This is an active request that MUST broadcastGeneration to wake up waiting goroutines!
		// Note: broadcastGeneration uses mutexes and conditions from activeThumbnailGeneration
		defer func() {
			// Note: errorReturn is the named return variable so we wrap this in a closure to re-evaluate the arguments at defer-time
			if err := recover(); err != nil {
				broadcastGeneration(dst, activeThumbnailGeneration, config, err.(error), logger)
				panic(err)
			}
			broadcastGeneration(dst, activeThumbnailGeneration, config, errorReturn, logger)
		}()
	}

	// Check if the thumbnail exists.
	thumbnailMetadata, err := db.GetThumbnail(mediaMetadata.MediaID, mediaMetadata.Origin, config.Width, config.Height, config.ResizeMethod)
	if err != nil {
		logger.WithFields(log.Fields{
			"Width":        config.Width,
			"Height":       config.Height,
			"ResizeMethod": config.ResizeMethod,
		}).Error("Failed to query database for thumbnail.")
		return err
	}
	if thumbnailMetadata != nil {
		return nil
	}
	// Note: The double-negative is intentional as os.IsExist(err) != !os.IsNotExist(err).
	// The functions are error checkers to be used in different cases.
	if _, err = os.Stat(string(dst)); !os.IsNotExist(err) {
		// Thumbnail exists
		return nil
	}

	if isActive == false {
		// Note: This should not happen, but we check just in case.
		logger.WithFields(log.Fields{
			"Width":        config.Width,
			"Height":       config.Height,
			"ResizeMethod": config.ResizeMethod,
		}).Error("Failed to stat file but this is not the active thumbnail generator. This should not happen.")
		return fmt.Errorf("Not active thumbnail generator. Stat error: %q", err)
	}

	start := time.Now()
	width, height, err := resize(dst, buffer, config.Width, config.Height, config.ResizeMethod == "crop", logger)
	if err != nil {
		return err
	}
	logger.WithFields(log.Fields{
		"Width":        config.Width,
		"Height":       config.Height,
		"ActualWidth":  width,
		"ActualHeight": height,
		"ResizeMethod": config.ResizeMethod,
		"processTime":  time.Now().Sub(start),
	}).Info("Generated thumbnail")

	stat, err := os.Stat(string(dst))
	if err != nil {
		return err
	}

	thumbnailMetadata = &types.ThumbnailMetadata{
		MediaMetadata: &types.MediaMetadata{
			MediaID: mediaMetadata.MediaID,
			Origin:  mediaMetadata.Origin,
			// Note: the code currently always creates a JPEG thumbnail
			ContentType:   types.ContentType("image/jpeg"),
			FileSizeBytes: types.FileSizeBytes(stat.Size()),
		},
		ThumbnailSize: types.ThumbnailSize{
			Width:        width,
			Height:       height,
			ResizeMethod: config.ResizeMethod,
		},
	}

	err = db.StoreThumbnail(thumbnailMetadata)
	if err != nil {
		logger.WithError(err).WithFields(log.Fields{
			"Width":        config.Width,
			"Height":       config.Height,
			"ActualWidth":  width,
			"ActualHeight": height,
			"ResizeMethod": config.ResizeMethod,
		}).Error("Failed to store thumbnail metadata in database.")
		return err
	}

	return nil
}

// getActiveThumbnailGeneration checks for active thumbnail generation
func getActiveThumbnailGeneration(dst types.Path, config types.ThumbnailSize, activeThumbnailGeneration *types.ActiveThumbnailGeneration, logger *log.Entry) (bool, error) {
	// Check if there is active thumbnail generation.
	activeThumbnailGeneration.Lock()
	defer activeThumbnailGeneration.Unlock()
	if activeThumbnailGenerationResult, ok := activeThumbnailGeneration.PathToResult[string(dst)]; ok {
		logger.WithFields(log.Fields{
			"Width":        config.Width,
			"Height":       config.Height,
			"ResizeMethod": config.ResizeMethod,
		}).Info("Waiting for another goroutine to generate the thumbnail.")

		// NOTE: Wait unlocks and locks again internally. There is still a deferred Unlock() that will unlock this.
		activeThumbnailGenerationResult.Cond.Wait()
		// Note: either there is an error or it is nil, either way returning it is correct
		return false, activeThumbnailGenerationResult.Err
	}

	// No active thumbnail generation so create one
	activeThumbnailGeneration.PathToResult[string(dst)] = &types.ThumbnailGenerationResult{
		Cond: &sync.Cond{L: activeThumbnailGeneration},
	}

	return true, nil
}

// broadcastGeneration broadcasts that thumbnail generation completed and the error to all waiting goroutines
// Note: This should only be called by the owner of the activeThumbnailGenerationResult
func broadcastGeneration(dst types.Path, activeThumbnailGeneration *types.ActiveThumbnailGeneration, config types.ThumbnailSize, errorReturn error, logger *log.Entry) {
	activeThumbnailGeneration.Lock()
	defer activeThumbnailGeneration.Unlock()
	if activeThumbnailGenerationResult, ok := activeThumbnailGeneration.PathToResult[string(dst)]; ok {
		logger.WithFields(log.Fields{
			"Width":        config.Width,
			"Height":       config.Height,
			"ResizeMethod": config.ResizeMethod,
		}).Info("Signalling other goroutines waiting for this goroutine to generate the thumbnail.")
		// Note: retErr is a named return value error that is signalled from here to waiting goroutines
		activeThumbnailGenerationResult.Err = errorReturn
		activeThumbnailGenerationResult.Cond.Broadcast()
	}
	delete(activeThumbnailGeneration.PathToResult, string(dst))
}

// resize scales an image to fit within the provided width and height
// If the source aspect ratio is different to the target dimensions, one edge will be smaller than requested
// If crop is set to true, the image will be scaled to fill the width and height with any excess being cropped off
func resize(dst types.Path, buffer []byte, w, h int, crop bool, logger *log.Entry) (int, int, error) {
	inImage := bimg.NewImage(buffer)

	inSize, err := inImage.Size()
	if err != nil {
		return -1, -1, err
	}

	options := bimg.Options{
		Type:    bimg.JPEG,
		Quality: 85,
	}
	if crop {
		options.Width = w
		options.Height = h
		options.Crop = true
	} else {
		inAR := float64(inSize.Width) / float64(inSize.Height)
		outAR := float64(w) / float64(h)

		if inAR > outAR {
			// input has wider AR than requested output so use requested width and calculate height to match input AR
			options.Width = w
			options.Height = int(float64(w) / inAR)
		} else {
			// input has narrower AR than requested output so use requested height and calculate width to match input AR
			options.Width = int(float64(h) * inAR)
			options.Height = h
		}
	}

	newImage, err := inImage.Process(options)
	if err != nil {
		return -1, -1, err
	}

	if err = bimg.Write(string(dst), newImage); err != nil {
		logger.WithError(err).Error("Failed to resize image")
		return -1, -1, err
	}

	return options.Width, options.Height, nil
}

// init with worst values
func newThumbnailMetrics() thumbnailMetrics {
	return thumbnailMetrics{
		isSmaller:      1,
		aspect:         float64(16384 * 16384),
		size:           float64(16384 * 16384),
		methodMismatch: 0,
		fileSize:       types.FileSizeBytes(math.MaxInt64),
	}
}

func calcThumbnailMetrics(size types.ThumbnailSize, metadata *types.MediaMetadata, desired types.ThumbnailSize) thumbnailMetrics {
	dW := desired.Width
	dH := desired.Height
	tW := size.Width
	tH := size.Height

	metrics := newThumbnailMetrics()
	// In all cases, a larger metric value is a worse fit.
	// compare size: thumbnail smaller is true and gives 1, larger is false and gives 0
	metrics.isSmaller = boolToInt(tW < dW || tH < dH)
	// comparison of aspect ratios only makes sense for a request for desired cropped
	metrics.aspect = math.Abs(float64(dW*tH - dH*tW))
	// compare sizes
	metrics.size = math.Abs(float64((dW - tW) * (dH - tH)))
	// compare resize method
	metrics.methodMismatch = boolToInt(size.ResizeMethod != desired.ResizeMethod)
	if metadata != nil {
		// file size
		metrics.fileSize = metadata.FileSizeBytes
	}

	return metrics
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func (a thumbnailMetrics) betterThan(b thumbnailMetrics, desiredCrop bool) bool {
	// preference means returning -1

	// prefer images that are not smaller
	// e.g. isSmallerDiff > 0 means b is smaller than desired and a is not smaller
	if a.isSmaller > b.isSmaller {
		return false
	} else if a.isSmaller < b.isSmaller {
		return true
	}

	// prefer aspect ratios closer to desired only if desired cropped
	// only cropped images have differing aspect ratios
	// desired scaled only accepts scaled images
	if desiredCrop {
		if a.aspect > b.aspect {
			return false
		} else if a.aspect < b.aspect {
			return true
		}
	}

	// prefer closer in size
	if a.size > b.size {
		return false
	} else if a.size < b.size {
		return true
	}

	// prefer images using the same method
	// e.g. methodMismatchDiff > 0 means b's method is different from desired and a's matches the desired method
	if a.methodMismatch > b.methodMismatch {
		return false
	} else if a.methodMismatch < b.methodMismatch {
		return true
	}

	// prefer smaller files
	if a.fileSize > b.fileSize {
		return false
	} else if a.fileSize < b.fileSize {
		return true
	}

	return false
}
