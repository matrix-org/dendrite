package config

import (
	"fmt"
)

type MediaAPI struct {
	Matrix *Global `json:"-"`

	Listen                 Address         `json:"Listen" comment:"Listen address for this component."`
	Bind                   Address         `json:"Bind" comment:"Bind address for this component."`
	Database               DatabaseOptions `json:"Database" comment:"Database configuration for this component."`
	BasePath               Path            `json:"BasePath" comment:"Storage path for uploaded media. May be relative or absolute."`
	AbsBasePath            Path            `json:"-"`
	MaxFileSizeBytes       *FileSizeBytes  `json:"MaxFileSizeBytes" comment:"The maximum allowed file size (in bytes) for media uploads to this homeserver\n(0 = unlimited)."`
	DynamicThumbnails      bool            `json:"DynamicThumbnails" comment:"Whether to dynamically generate thumbnails if needed."`
	MaxThumbnailGenerators int             `json:"MaxThumbnailGenerators" comment:"The maximum number of simultaneous thumbnail generators to run."`
	ThumbnailSizes         []ThumbnailSize `json:"ThumbnailSizes" comment:"A list of thumbnail sizes to be generated for media content."`
}

func (c *MediaAPI) Defaults() {
	c.Listen = "localhost:7774"
	c.Bind = "localhost:7774"
	c.Database.Defaults()
	c.Database.ConnectionString = "file:mediaapi.db"

	defaultMaxFileSizeBytes := FileSizeBytes(10485760)
	c.MaxFileSizeBytes = &defaultMaxFileSizeBytes
	c.MaxThumbnailGenerators = 10
	c.BasePath = "./media_store"
}

func (c *MediaAPI) Verify(configErrs *ConfigErrors, isMonolith bool) {
	checkNotEmpty(configErrs, "MediaAPI.Listen", string(c.Listen))
	checkNotEmpty(configErrs, "MediaAPI.Bind", string(c.Bind))
	checkNotEmpty(configErrs, "MediaAPI.Database.ConnectionString", string(c.Database.ConnectionString))

	checkNotEmpty(configErrs, "MediaAPI.BasePath", string(c.BasePath))
	checkPositive(configErrs, "MediaAPI.MaxFileSizeBytes", int64(*c.MaxFileSizeBytes))
	checkPositive(configErrs, "MediaAPI.MaxThumbnailGenerators", int64(c.MaxThumbnailGenerators))

	for i, size := range c.ThumbnailSizes {
		checkPositive(configErrs, fmt.Sprintf("MediaAPI.ThumbnailSizes[%d].Width", i), int64(size.Width))
		checkPositive(configErrs, fmt.Sprintf("MediaAPI.ThumbnailSizes[%d].Height", i), int64(size.Height))
	}
}
