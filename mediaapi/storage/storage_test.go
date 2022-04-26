package storage_test

import (
	"context"
	"reflect"
	"testing"

	"github.com/matrix-org/dendrite/mediaapi/storage"
	"github.com/matrix-org/dendrite/mediaapi/types"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/dendrite/test"
)

func mustCreateDatabase(t *testing.T, dbType test.DBType) (storage.Database, func()) {
	connStr, close := test.PrepareDBConnectionString(t, dbType)
	db, err := storage.NewMediaAPIDatasource(&config.DatabaseOptions{
		ConnectionString: config.DataSource(connStr),
	})
	if err != nil {
		t.Fatalf("NewSyncServerDatasource returned %s", err)
	}
	return db, close
}
func TestMediaRepository(t *testing.T) {
	test.WithAllDatabases(t, func(t *testing.T, dbType test.DBType) {
		db, close := mustCreateDatabase(t, dbType)
		defer close()
		ctx := context.Background()
		t.Run("can insert media & query media", func(t *testing.T) {
			metadata := &types.MediaMetadata{
				MediaID:       "testing",
				Origin:        "localhost",
				ContentType:   "image/png",
				FileSizeBytes: 10,
				UploadName:    "upload test",
				Base64Hash:    "dGVzdGluZw==",
				UserID:        "@alice:localhost",
			}
			if err := db.StoreMediaMetadata(ctx, metadata); err != nil {
				t.Fatalf("unable to store media metadata: %v", err)
			}
			// query by media id
			gotMetadata, err := db.GetMediaMetadata(ctx, metadata.MediaID, metadata.Origin)
			if err != nil {
				t.Fatalf("unable to query media metadata: %v", err)
			}
			if !reflect.DeepEqual(metadata, gotMetadata) {
				t.Fatalf("expected metadata %+v, got %v", metadata, gotMetadata)
			}
			// query by media hash
			gotMetadata, err = db.GetMediaMetadataByHash(ctx, metadata.Base64Hash, metadata.Origin)
			if err != nil {
				t.Fatalf("unable to query media metadata by hash: %v", err)
			}
			if !reflect.DeepEqual(metadata, gotMetadata) {
				t.Fatalf("expected metadata %+v, got %v", metadata, gotMetadata)
			}
		})
	})
}

func TestThumbnailsStorage(t *testing.T) {
	test.WithAllDatabases(t, func(t *testing.T, dbType test.DBType) {
		db, close := mustCreateDatabase(t, dbType)
		defer close()
		ctx := context.Background()
		t.Run("can insert thumbnails & query media", func(t *testing.T) {
			thumbnails := []*types.ThumbnailMetadata{
				{
					MediaMetadata: &types.MediaMetadata{
						MediaID:       "testing",
						Origin:        "localhost",
						ContentType:   "image/png",
						FileSizeBytes: 6,
					},
					ThumbnailSize: types.ThumbnailSize{
						Width:        5,
						Height:       5,
						ResizeMethod: types.Crop,
					},
				},
				{
					MediaMetadata: &types.MediaMetadata{
						MediaID:       "testing",
						Origin:        "localhost",
						ContentType:   "image/png",
						FileSizeBytes: 7,
					},
					ThumbnailSize: types.ThumbnailSize{
						Width:        1,
						Height:       1,
						ResizeMethod: types.Scale,
					},
				},
			}
			for i := range thumbnails {
				if err := db.StoreThumbnail(ctx, thumbnails[i]); err != nil {
					t.Fatalf("unable to store thumbnail metadata: %v", err)
				}
			}
			// query by single thumbnail
			gotMetadata, err := db.GetThumbnail(ctx,
				thumbnails[0].MediaMetadata.MediaID,
				thumbnails[0].MediaMetadata.Origin,
				thumbnails[0].ThumbnailSize.Width, thumbnails[0].ThumbnailSize.Height,
				thumbnails[0].ThumbnailSize.ResizeMethod,
			)
			if err != nil {
				t.Fatalf("unable to query thumbnail metadata: %v", err)
			}
			if !reflect.DeepEqual(thumbnails[0].MediaMetadata, gotMetadata.MediaMetadata) {
				t.Fatalf("expected metadata %+v, got %+v", thumbnails[0].MediaMetadata, gotMetadata.MediaMetadata)
			}
			if !reflect.DeepEqual(thumbnails[0].ThumbnailSize, gotMetadata.ThumbnailSize) {
				t.Fatalf("expected metadata %+v, got %+v", thumbnails[0].MediaMetadata, gotMetadata.MediaMetadata)
			}
			// query by all thumbnails
			gotMediadatas, err := db.GetThumbnails(ctx, thumbnails[0].MediaMetadata.MediaID, thumbnails[0].MediaMetadata.Origin)
			if err != nil {
				t.Fatalf("unable to query media metadata by hash: %v", err)
			}
			if len(gotMediadatas) != len(thumbnails) {
				t.Fatalf("expected %d stored thumbnail metadata, got %d", len(thumbnails), len(gotMediadatas))
			}
			for i := range gotMediadatas {
				if !reflect.DeepEqual(thumbnails[i].MediaMetadata, gotMediadatas[i].MediaMetadata) {
					t.Fatalf("expected metadata %+v, got %v", thumbnails[i].MediaMetadata, gotMediadatas[i].MediaMetadata)
				}
				if !reflect.DeepEqual(thumbnails[i].ThumbnailSize, gotMediadatas[i].ThumbnailSize) {
					t.Fatalf("expected metadata %+v, got %v", thumbnails[i].ThumbnailSize, gotMediadatas[i].ThumbnailSize)
				}
			}
		})
	})
}
