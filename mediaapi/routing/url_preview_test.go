package routing

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/dendrite/mediaapi/fileutils"
	"github.com/matrix-org/dendrite/mediaapi/storage"
	"github.com/matrix-org/dendrite/mediaapi/types"
	"github.com/matrix-org/dendrite/setup/config"
	userapi "github.com/matrix-org/dendrite/userapi/api"
	log "github.com/sirupsen/logrus"
)

var tests = []map[string]interface{}{
	{
		"test": `<html>
	<head>
	<title>Title</title>
	<meta property="og:title" content="test_title"/>
	<meta property="og:description" content="test_description" ></meta>
	<meta property="og:image" content="test.png">
	<meta property="og:image:url" content="test2.png"/><meta>
	<meta property="og:image:secure_url" content="test3.png">
	<meta property="og:type" content="image/jpeg" />
	<meta property="og:url" content="/image.jpg" />
	</head>
	</html>
	`,
		"expected": map[string]string{
			"og:title":            "test_title",
			"og:description":      "test_description",
			"og:image":            "test.png",
			"og:image:url":        "test2.png",
			"og:image:secure_url": "test3.png",
			"og:type":             "image/jpeg",
			"og:url":              "/image.jpg",
		},
	},
}

func Test_getMetaFieldsFromHTML(t *testing.T) {
	for _, test := range tests {
		r := &http.Response{Body: io.NopCloser(strings.NewReader(test["test"].(string)))}
		result := getMetaFieldsFromHTML(r)
		fmt.Println(result)
		for k, v := range test["expected"].(map[string]string) {
			if val, ok := result[k]; ok {
				if val != v {
					t.Errorf("Values don't match: expected %s, got %s", v, val)
				}
			} else {
				t.Errorf("Not found %s in the test HTML", k)
			}
		}
	}
}

func Test_LoadStorePreview(t *testing.T) {
	type fields struct {
		MediaMetadata *types.MediaMetadata
		Logger        *log.Entry
	}
	type args struct {
		ctx                       context.Context
		reqReader                 io.Reader
		cfg                       *config.MediaAPI
		db                        storage.Database
		activeThumbnailGeneration *types.ActiveThumbnailGeneration
	}

	wd, err := os.Getwd()
	if err != nil {
		t.Errorf("failed to get current working directory: %v", err)
	}

	maxSize := config.FileSizeBytes(8)
	logger := log.New().WithField("mediaapi", "test")
	testdataPath := filepath.Join(wd, "./testdata")

	g := &config.Global{}
	g.Defaults(config.DefaultOpts{Generate: true})
	cfg := &config.MediaAPI{
		Matrix:            g,
		MaxFileSizeBytes:  maxSize,
		BasePath:          config.Path(testdataPath),
		AbsBasePath:       config.Path(testdataPath),
		DynamicThumbnails: false,
	}

	// create testdata folder and remove when done
	_ = os.Mkdir(testdataPath, os.ModePerm)
	defer fileutils.RemoveDir(types.Path(testdataPath), nil)
	cm := sqlutil.NewConnectionManager(nil, config.DatabaseOptions{})
	db, err := storage.NewMediaAPIDatasource(cm, &config.DatabaseOptions{
		ConnectionString:       "file::memory:?cache=shared",
		MaxOpenConnections:     100,
		MaxIdleConnections:     2,
		ConnMaxLifetimeSeconds: -1,
	})
	if err != nil {
		t.Errorf("error opening mediaapi database: %v", err)
	}

	testPreview := &types.UrlPreview{
		Title:       "test_title",
		Description: "test_description",
		ImageUrl:    "test_url.png",
		ImageType:   "image/png",
		ImageSize:   types.FileSizeBytes(100),
		ImageHeight: 100,
		ImageWidth:  100,
		Type:        "video",
		Url:         "video.avi",
	}

	hash := getHashFromString("testhash")
	device := userapi.Device{
		ID:     "1",
		UserID: "user",
	}
	err = storeUrlPreviewResponse(context.Background(), cfg, db, device, hash, testPreview, logger)
	if err != nil {
		t.Errorf("Can't store urel preview response: %v", err)
	}

	filePath, err := fileutils.GetPathFromBase64Hash(hash, cfg.AbsBasePath)
	if err != nil {
		t.Errorf("Can't get stored file path: %v", err)
	}
	_, err = os.Stat(filePath)
	if err != nil {
		t.Errorf("Can't get stored file info: %v", err)

	}

	loadedPreview, err := loadUrlPreviewResponse(context.Background(), cfg, db, hash)
	if err != nil {
		t.Errorf("Can't load the preview: %v", err)
	}

	if !reflect.DeepEqual(loadedPreview, testPreview) {
		t.Errorf("Stored and loaded previews not equal: stored=%v, loaded=%v", testPreview, loadedPreview)
	}
}
