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
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sync"

	"github.com/matrix-org/dendrite/mediaapi/storage"
	"github.com/matrix-org/dendrite/mediaapi/types"
	"github.com/matrix-org/dendrite/setup/config"
	log "github.com/sirupsen/logrus"
)

type thumbnailFitness struct {
	isSmaller      int
	aspect         float64
	size           float64
	methodMismatch int
	fileSize       types.FileSizeBytes
}

// thumbnailTemplate is the filename template for thumbnails
const thumbnailTemplate = "thumbnail-%vx%v-%v"

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
func SelectThumbnail(desired types.ThumbnailSize, thumbnails []*types.ThumbnailMetadata, thumbnailSizes []config.ThumbnailSize) (*types.ThumbnailMetadata, *types.ThumbnailSize) {
	var chosenThumbnail *types.ThumbnailMetadata
	var chosenThumbnailSize *types.ThumbnailSize
	bestFit := newThumbnailFitness()

	for _, thumbnail := range thumbnails {
		if desired.ResizeMethod == types.Scale && thumbnail.ThumbnailSize.ResizeMethod != types.Scale {
			continue
		}
		fitness := calcThumbnailFitness(thumbnail.ThumbnailSize, thumbnail.MediaMetadata, desired)
		if isBetter := fitness.betterThan(bestFit, desired.ResizeMethod == types.Crop); isBetter {
			bestFit = fitness
			chosenThumbnail = thumbnail
		}
	}

	for _, thumbnailSize := range thumbnailSizes {
		if desired.ResizeMethod == types.Scale && thumbnailSize.ResizeMethod != types.Scale {
			continue
		}
		fitness := calcThumbnailFitness(types.ThumbnailSize(thumbnailSize), nil, desired)
		if isBetter := fitness.betterThan(bestFit, desired.ResizeMethod == types.Crop); isBetter {
			bestFit = fitness
			chosenThumbnailSize = (*types.ThumbnailSize)(&thumbnailSize)
		}
	}

	return chosenThumbnail, chosenThumbnailSize
}

// getActiveThumbnailGeneration checks for active thumbnail generation
func getActiveThumbnailGeneration(dst types.Path, _ types.ThumbnailSize, activeThumbnailGeneration *types.ActiveThumbnailGeneration, maxThumbnailGenerators int, logger *log.Entry) (isActive bool, busy bool, errorReturn error) {
	// Check if there is active thumbnail generation.
	activeThumbnailGeneration.Lock()
	defer activeThumbnailGeneration.Unlock()
	if activeThumbnailGenerationResult, ok := activeThumbnailGeneration.PathToResult[string(dst)]; ok {
		logger.Info("Waiting for another goroutine to generate the thumbnail.")

		// NOTE: Wait unlocks and locks again internally. There is still a deferred Unlock() that will unlock this.
		activeThumbnailGenerationResult.Cond.Wait()
		// Note: either there is an error or it is nil, either way returning it is correct
		return false, false, activeThumbnailGenerationResult.Err
	}

	// Only allow thumbnail generation up to a maximum configured number. Above this we fall back to serving the
	// original. Or in the case of pre-generation, they maybe get generated on the first request for a thumbnail if
	// load has subsided.
	if len(activeThumbnailGeneration.PathToResult) >= maxThumbnailGenerators {
		return false, true, nil
	}

	// No active thumbnail generation so create one
	activeThumbnailGeneration.PathToResult[string(dst)] = &types.ThumbnailGenerationResult{
		Cond: &sync.Cond{L: activeThumbnailGeneration},
	}

	return true, false, nil
}

// broadcastGeneration broadcasts that thumbnail generation completed and the error to all waiting goroutines
// Note: This should only be called by the owner of the activeThumbnailGenerationResult
func broadcastGeneration(dst types.Path, activeThumbnailGeneration *types.ActiveThumbnailGeneration, _ types.ThumbnailSize, errorReturn error, logger *log.Entry) {
	activeThumbnailGeneration.Lock()
	defer activeThumbnailGeneration.Unlock()
	if activeThumbnailGenerationResult, ok := activeThumbnailGeneration.PathToResult[string(dst)]; ok {
		logger.Info("Signalling other goroutines waiting for this goroutine to generate the thumbnail.")
		// Note: errorReturn is a named return value error that is signalled from here to waiting goroutines
		activeThumbnailGenerationResult.Err = errorReturn
		activeThumbnailGenerationResult.Cond.Broadcast()
	}
	delete(activeThumbnailGeneration.PathToResult, string(dst))
}

func isThumbnailExists(
	ctx context.Context,
	dst types.Path,
	config types.ThumbnailSize,
	mediaMetadata *types.MediaMetadata,
	db storage.Database,
	logger *log.Entry,
) (bool, error) {
	thumbnailMetadata, err := db.GetThumbnail(
		ctx, mediaMetadata.MediaID, mediaMetadata.Origin,
		config.Width, config.Height, config.ResizeMethod,
	)
	if err != nil {
		logger.Error("Failed to query database for thumbnail.")
		return false, err
	}
	if thumbnailMetadata != nil {
		return true, nil
	}
	// Note: The double-negative is intentional as os.IsExist(err) != !os.IsNotExist(err).
	// The functions are error checkers to be used in different cases.
	if _, err = os.Stat(string(dst)); !os.IsNotExist(err) {
		// Thumbnail exists
		return true, nil
	}
	return false, nil
}

// init with worst values
func newThumbnailFitness() thumbnailFitness {
	return thumbnailFitness{
		isSmaller:      1,
		aspect:         math.Inf(1),
		size:           math.Inf(1),
		methodMismatch: 0,
		fileSize:       types.FileSizeBytes(math.MaxInt64),
	}
}

func calcThumbnailFitness(size types.ThumbnailSize, metadata *types.MediaMetadata, desired types.ThumbnailSize) thumbnailFitness {
	dW := desired.Width
	dH := desired.Height
	tW := size.Width
	tH := size.Height

	fitness := newThumbnailFitness()
	// In all cases, a larger metric value is a worse fit.
	// compare size: thumbnail smaller is true and gives 1, larger is false and gives 0
	fitness.isSmaller = boolToInt(tW < dW || tH < dH)
	// comparison of aspect ratios only makes sense for a request for desired cropped
	fitness.aspect = math.Abs(float64(dW*tH - dH*tW))
	// compare sizes
	fitness.size = math.Abs(float64((dW - tW) * (dH - tH)))
	// compare resize method
	fitness.methodMismatch = boolToInt(size.ResizeMethod != desired.ResizeMethod)
	if metadata != nil {
		// file size
		fitness.fileSize = metadata.FileSizeBytes
	}

	return fitness
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func (a thumbnailFitness) betterThan(b thumbnailFitness, desiredCrop bool) bool {
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
