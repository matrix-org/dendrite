package routing

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/matrix-org/dendrite/mediaapi/fileutils"
	"github.com/matrix-org/dendrite/mediaapi/storage"
	"github.com/matrix-org/dendrite/mediaapi/types"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/util"
	log "github.com/sirupsen/logrus"
)

func Test_uploadRequest_doUpload(t *testing.T) {
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

	cfg := &config.MediaAPI{
		MaxFileSizeBytes:  maxSize,
		BasePath:          config.Path(testdataPath),
		AbsBasePath:       config.Path(testdataPath),
		DynamicThumbnails: false,
	}

	// create testdata folder and remove when done
	_ = os.Mkdir(testdataPath, os.ModePerm)
	defer fileutils.RemoveDir(types.Path(testdataPath), nil)

	db, err := storage.NewMediaAPIDatasource(nil, &config.DatabaseOptions{
		ConnectionString:       "file::memory:?cache=shared",
		MaxOpenConnections:     100,
		MaxIdleConnections:     2,
		ConnMaxLifetimeSeconds: -1,
	})
	if err != nil {
		t.Errorf("error opening mediaapi database: %v", err)
	}

	tests := []struct {
		name   string
		fields fields
		args   args
		want   *util.JSONResponse
	}{
		{
			name: "upload ok",
			args: args{
				ctx:       context.Background(),
				reqReader: strings.NewReader("test"),
				cfg:       cfg,
				db:        db,
			},
			fields: fields{
				Logger: logger,
				MediaMetadata: &types.MediaMetadata{
					MediaID:    "1337",
					UploadName: "test ok",
				},
			},
			want: nil,
		},
		{
			name: "upload ok (exact size)",
			args: args{
				ctx:       context.Background(),
				reqReader: strings.NewReader("testtest"),
				cfg:       cfg,
				db:        db,
			},
			fields: fields{
				Logger: logger,
				MediaMetadata: &types.MediaMetadata{
					MediaID:    "1338",
					UploadName: "test ok (exact size)",
				},
			},
			want: nil,
		},
		{
			name: "upload not ok",
			args: args{
				ctx:       context.Background(),
				reqReader: strings.NewReader("test test test"),
				cfg:       cfg,
				db:        db,
			},
			fields: fields{
				Logger: logger,
				MediaMetadata: &types.MediaMetadata{
					MediaID:    "1339",
					UploadName: "test fail",
				},
			},
			want: requestEntityTooLargeJSONResponse(maxSize),
		},
		{
			name: "upload ok with unlimited filesize",
			args: args{
				ctx:       context.Background(),
				reqReader: strings.NewReader("test test test"),
				cfg: &config.MediaAPI{
					MaxFileSizeBytes:  config.FileSizeBytes(0),
					BasePath:          config.Path(testdataPath),
					AbsBasePath:       config.Path(testdataPath),
					DynamicThumbnails: false,
				},
				db: db,
			},
			fields: fields{
				Logger: logger,
				MediaMetadata: &types.MediaMetadata{
					MediaID:    "1339",
					UploadName: "test fail",
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &uploadRequest{
				MediaMetadata: tt.fields.MediaMetadata,
				Logger:        tt.fields.Logger,
			}
			if got := r.doUpload(tt.args.ctx, tt.args.reqReader, tt.args.cfg, tt.args.db, tt.args.activeThumbnailGeneration); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("doUpload() = %+v, want %+v", got, tt.want)
			}
		})
	}
}
