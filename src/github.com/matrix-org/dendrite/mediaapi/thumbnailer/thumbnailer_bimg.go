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

// +build bimg

package thumbnailer

import (
	"fmt"
	"os"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/matrix-org/dendrite/mediaapi/storage"
	"github.com/matrix-org/dendrite/mediaapi/types"
	"gopkg.in/h2non/bimg.v1"
)

// GenerateThumbnails generates the configured thumbnail sizes for the source file
func GenerateThumbnails(src types.Path, configs []types.ThumbnailSize, mediaMetadata *types.MediaMetadata, activeThumbnailGeneration *types.ActiveThumbnailGeneration, maxThumbnailGenerators int, db *storage.Database, logger *log.Entry) (busy bool, errorReturn error) {
	buffer, err := bimg.Read(string(src))
	if err != nil {
		logger.WithError(err).WithField("src", src).Error("Failed to read src file")
		return false, err
	}
	for _, config := range configs {
		// Note: createThumbnail does locking based on activeThumbnailGeneration
		busy, err = createThumbnail(src, buffer, config, mediaMetadata, activeThumbnailGeneration, maxThumbnailGenerators, db, logger)
		if err != nil {
			logger.WithError(err).WithField("src", src).Error("Failed to generate thumbnails")
			return false, err
		}
		if busy {
			return true, nil
		}
	}
	return false, nil
}

// GenerateThumbnail generates the configured thumbnail size for the source file
func GenerateThumbnail(src types.Path, config types.ThumbnailSize, mediaMetadata *types.MediaMetadata, activeThumbnailGeneration *types.ActiveThumbnailGeneration, maxThumbnailGenerators int, db *storage.Database, logger *log.Entry) (busy bool, errorReturn error) {
	buffer, err := bimg.Read(string(src))
	if err != nil {
		logger.WithError(err).WithFields(log.Fields{
			"src": src,
		}).Error("Failed to read src file")
		return false, err
	}
	// Note: createThumbnail does locking based on activeThumbnailGeneration
	busy, err = createThumbnail(src, buffer, config, mediaMetadata, activeThumbnailGeneration, maxThumbnailGenerators, db, logger)
	if err != nil {
		logger.WithError(err).WithFields(log.Fields{
			"src": src,
		}).Error("Failed to generate thumbnails")
		return false, err
	}
	if busy {
		return true, nil
	}
	return false, nil
}

// createThumbnail checks if the thumbnail exists, and if not, generates it
// Thumbnail generation is only done once for each non-existing thumbnail.
func createThumbnail(src types.Path, buffer []byte, config types.ThumbnailSize, mediaMetadata *types.MediaMetadata, activeThumbnailGeneration *types.ActiveThumbnailGeneration, maxThumbnailGenerators int, db *storage.Database, logger *log.Entry) (busy bool, errorReturn error) {
	logger = logger.WithFields(log.Fields{
		"Width":        config.Width,
		"Height":       config.Height,
		"ResizeMethod": config.ResizeMethod,
	})

	dst := GetThumbnailPath(src, config)

	// Note: getActiveThumbnailGeneration uses mutexes and conditions from activeThumbnailGeneration
	isActive, busy, err := getActiveThumbnailGeneration(dst, config, activeThumbnailGeneration, maxThumbnailGenerators, logger)
	if err != nil {
		return false, err
	}
	if busy {
		return true, nil
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
		logger.Error("Failed to query database for thumbnail.")
		return false, err
	}
	if thumbnailMetadata != nil {
		return false, nil
	}
	// Note: The double-negative is intentional as os.IsExist(err) != !os.IsNotExist(err).
	// The functions are error checkers to be used in different cases.
	if _, err = os.Stat(string(dst)); !os.IsNotExist(err) {
		// Thumbnail exists
		return false, nil
	}

	if isActive == false {
		// Note: This should not happen, but we check just in case.
		logger.Error("Failed to stat file but this is not the active thumbnail generator. This should not happen.")
		return false, fmt.Errorf("Not active thumbnail generator. Stat error: %q", err)
	}

	start := time.Now()
	width, height, err := resize(dst, buffer, config.Width, config.Height, config.ResizeMethod == "crop", logger)
	if err != nil {
		return false, err
	}
	logger.WithFields(log.Fields{
		"ActualWidth":  width,
		"ActualHeight": height,
		"processTime":  time.Now().Sub(start),
	}).Info("Generated thumbnail")

	stat, err := os.Stat(string(dst))
	if err != nil {
		return false, err
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
			"ActualWidth":  width,
			"ActualHeight": height,
		}).Error("Failed to store thumbnail metadata in database.")
		return false, err
	}

	return false, nil
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
