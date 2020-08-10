package config

import (
	"fmt"
)

type MediaAPI struct {
	Matrix *Global `yaml:"-"`

	Listen Address `yaml:"listen"`
	Bind   Address `yaml:"bind"`

	// The MediaAPI database stores information about files uploaded and downloaded
	// by local users. It is only accessed by the MediaAPI.
	Database DatabaseOptions `yaml:"database"`

	// The base path to where the media files will be stored. May be relative or absolute.
	BasePath Path `yaml:"base_path"`

	// The absolute base path to where media files will be stored.
	AbsBasePath Path `yaml:"-"`

	// The maximum file size in bytes that is allowed to be stored on this server.
	// Note: if max_file_size_bytes is set to 0, the size is unlimited.
	// Note: if max_file_size_bytes is not set, it will default to 10485760 (10MB)
	MaxFileSizeBytes *FileSizeBytes `yaml:"max_file_size_bytes,omitempty"`

	// Whether to dynamically generate thumbnails on-the-fly if the requested resolution is not already generated
	DynamicThumbnails bool `yaml:"dynamic_thumbnails"`

	// The maximum number of simultaneous thumbnail generators. default: 10
	MaxThumbnailGenerators int `yaml:"max_thumbnail_generators"`

	// A list of thumbnail sizes to be pre-generated for downloaded remote / uploaded content
	ThumbnailSizes []ThumbnailSize `yaml:"thumbnail_sizes"`
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
	checkNotEmpty(configErrs, "media_api.listen", string(c.Listen))
	checkNotEmpty(configErrs, "media_api.bind", string(c.Bind))
	checkNotEmpty(configErrs, "media_api.database.connection_string", string(c.Database.ConnectionString))

	checkNotEmpty(configErrs, "media_api.base_path", string(c.BasePath))
	checkPositive(configErrs, "media_api.max_file_size_bytes", int64(*c.MaxFileSizeBytes))
	checkPositive(configErrs, "media_api.max_thumbnail_generators", int64(c.MaxThumbnailGenerators))

	for i, size := range c.ThumbnailSizes {
		checkPositive(configErrs, fmt.Sprintf("media_api.thumbnail_sizes[%d].width", i), int64(size.Width))
		checkPositive(configErrs, fmt.Sprintf("media_api.thumbnail_sizes[%d].height", i), int64(size.Height))
	}
}
