package config

import (
	"fmt"
)

type MediaAPI struct {
	Matrix *Global `yaml:"-"`

	// The MediaAPI database stores information about files uploaded and downloaded
	// by local users. It is only accessed by the MediaAPI.
	Database DatabaseOptions `yaml:"database,omitempty"`

	// The base path to where the media files will be stored. May be relative or absolute.
	BasePath Path `yaml:"base_path"`

	// The absolute base path to where media files will be stored.
	AbsBasePath Path `yaml:"-"`

	// The maximum file size in bytes that is allowed to be stored on this server.
	// Note: if max_file_size_bytes is set to 0, the size is unlimited.
	// Note: if max_file_size_bytes is not set, it will default to 10485760 (10MB)
	MaxFileSizeBytes FileSizeBytes `yaml:"max_file_size_bytes,omitempty"`

	// Whether to dynamically generate thumbnails on-the-fly if the requested resolution is not already generated
	DynamicThumbnails bool `yaml:"dynamic_thumbnails"`

	// The maximum number of simultaneous thumbnail generators. default: 10
	MaxThumbnailGenerators int `yaml:"max_thumbnail_generators"`

	// A list of thumbnail sizes to be pre-generated for downloaded remote / uploaded content
	ThumbnailSizes []ThumbnailSize `yaml:"thumbnail_sizes"`

	// The time in seconds to cache URL previews for
	UrlPreviewCacheTime int `yaml:"url_preview_cache_time"`

	// The timeout in milliseconds for fetching URL previews
	UrlPreviewTimeout int `yaml:"url_preview_timeout"`

	UrlPreviewThumbnailSize ThumbnailSize `yaml:"url_preview_thumbnail_size"`
}

// DefaultMaxFileSizeBytes defines the default file size allowed in transfers
var DefaultMaxFileSizeBytes = FileSizeBytes(10485760)

func (c *MediaAPI) Defaults(opts DefaultOpts) {
	c.MaxFileSizeBytes = DefaultMaxFileSizeBytes
	c.MaxThumbnailGenerators = 10
	c.UrlPreviewCacheTime = 10
	c.UrlPreviewTimeout = 10000

	if opts.Generate {
		c.ThumbnailSizes = []ThumbnailSize{
			{
				Width:        32,
				Height:       32,
				ResizeMethod: "crop",
			},
			{
				Width:        96,
				Height:       96,
				ResizeMethod: "crop",
			},
			{
				Width:        640,
				Height:       480,
				ResizeMethod: "scale",
			},
		}
		if !opts.SingleDatabase {
			c.Database.ConnectionString = "file:mediaapi.db"
		}
		c.BasePath = "./media_store"
	}

	c.UrlPreviewThumbnailSize = ThumbnailSize{
		Width:        200,
		Height:       200,
		ResizeMethod: "scale",
	}
}

func (c *MediaAPI) Verify(configErrs *ConfigErrors) {
	checkNotEmpty(configErrs, "media_api.base_path", string(c.BasePath))
	checkPositive(configErrs, "media_api.max_file_size_bytes", int64(c.MaxFileSizeBytes))
	checkPositive(configErrs, "media_api.max_thumbnail_generators", int64(c.MaxThumbnailGenerators))

	for i, size := range c.ThumbnailSizes {
		checkPositive(configErrs, fmt.Sprintf("media_api.thumbnail_sizes[%d].width", i), int64(size.Width))
		checkPositive(configErrs, fmt.Sprintf("media_api.thumbnail_sizes[%d].height", i), int64(size.Height))
	}

	if c.Matrix.DatabaseOptions.ConnectionString == "" {
		checkNotEmpty(configErrs, "media_api.database.connection_string", string(c.Database.ConnectionString))
	}

	// If MaxFileSizeBytes overflows int64, default to DefaultMaxFileSizeBytes
	if c.MaxFileSizeBytes+1 <= 0 {
		c.MaxFileSizeBytes = DefaultMaxFileSizeBytes
		fmt.Printf("Configured MediaApi.MaxFileSizeBytes overflows int64, defaulting to %d bytes", DefaultMaxFileSizeBytes)
	}

}
