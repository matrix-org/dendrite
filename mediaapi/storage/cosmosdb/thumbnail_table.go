// Copyright 2017-2018 New Vector Ltd
// Copyright 2019-2020 The Matrix.org Foundation C.I.C.
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

package cosmosdb

import (
	"context"
	"fmt"
	"time"

	"github.com/matrix-org/dendrite/internal/cosmosdbapi"
	"github.com/matrix-org/dendrite/internal/cosmosdbutil"

	"github.com/matrix-org/dendrite/mediaapi/types"
	"github.com/matrix-org/gomatrixserverlib"
)

// const thumbnailSchema = `
// -- The mediaapi_thumbnail table holds metadata for each thumbnail file stored and accessible to the local server,
// -- the actual file is stored separately.
// CREATE TABLE IF NOT EXISTS mediaapi_thumbnail (
//     media_id TEXT NOT NULL,
//     media_origin TEXT NOT NULL,
//     content_type TEXT NOT NULL,
//     file_size_bytes INTEGER NOT NULL,
//     creation_ts INTEGER NOT NULL,
//     width INTEGER NOT NULL,
//     height INTEGER NOT NULL,
//     resize_method TEXT NOT NULL
// );
// CREATE UNIQUE INDEX IF NOT EXISTS mediaapi_thumbnail_index ON mediaapi_thumbnail (media_id, media_origin, width, height, resize_method);
// `

type ThumbnailCosmos struct {
	MediaID           string `json:"media_id"`
	MediaOrigin       string `json:"media_origin"`
	ContentType       string `json:"content_type"`
	FileSizeBytes     int64  `json:"file_size_bytes"`
	CreationTimestamp int64  `json:"creation_ts"`
	Width             int64  `json:"width"`
	Height            int64  `json:"height"`
	ResizeMethod      string `json:"resize_method"`
}

type ThumbnailCosmosData struct {
	Id        string          `json:"id"`
	Pk        string          `json:"_pk"`
	Tn        string          `json:"_sid"`
	Cn        string          `json:"_cn"`
	ETag      string          `json:"_etag"`
	Timestamp int64           `json:"_ts"`
	Thumbnail ThumbnailCosmos `json:"mx_mediaapi_thumbnail"`
}

// const insertThumbnailSQL = `
// INSERT INTO mediaapi_thumbnail (media_id, media_origin, content_type, file_size_bytes, creation_ts, width, height, resize_method)
//     VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
// `

// Note: this selects one specific thumbnail
// const selectThumbnailSQL = `
// SELECT content_type, file_size_bytes, creation_ts FROM mediaapi_thumbnail WHERE media_id = $1 AND media_origin = $2 AND width = $3 AND height = $4 AND resize_method = $5
// `

// Note: this selects all thumbnails for a media_origin and media_id
// SELECT content_type, file_size_bytes, creation_ts, width, height, resize_method FROM mediaapi_thumbnail WHERE media_id = $1 AND media_origin = $2
const selectThumbnailsSQL = "" +
	"select * from c where c._cn = @x1 " +
	"and c.mx_mediaapi_thumbnail.media_id = @x2" +
	"and c.mx_mediaapi_thumbnail.media_origin = @x3"

type thumbnailStatements struct {
	db     *Database
	writer cosmosdbutil.Writer
	// insertThumbnailStmt  *sql.Stmt
	// selectThumbnailStmt  *sql.Stmt
	selectThumbnailsStmt string
	tableName            string
}

func queryThumbnail(s *thumbnailStatements, ctx context.Context, qry string, params map[string]interface{}) ([]ThumbnailCosmosData, error) {
	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
	var pk = cosmosdbapi.GetPartitionKey(s.db.cosmosConfig.TenantName, dbCollectionName)
	var response []ThumbnailCosmosData

	var optionsQry = cosmosdbapi.GetQueryDocumentsOptions(pk)
	var query = cosmosdbapi.GetQuery(qry, params)
	_, err := cosmosdbapi.GetClient(s.db.connection).QueryDocuments(
		ctx,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		query,
		&response,
		optionsQry)

	if err != nil {
		return nil, err
	}
	return response, nil
}

func getThumbnail(s *thumbnailStatements, ctx context.Context, pk string, docId string) (*ThumbnailCosmosData, error) {
	response := ThumbnailCosmosData{}
	err := cosmosdbapi.GetDocumentOrNil(
		s.db.connection,
		s.db.cosmosConfig,
		ctx,
		pk,
		docId,
		&response)

	if response.Id == "" {
		return nil, nil
	}

	return &response, err
}

func (s *thumbnailStatements) prepare(db *Database, writer cosmosdbutil.Writer) (err error) {
	s.db = db
	s.writer = writer
	s.tableName = "thumbnail"
	s.selectThumbnailsStmt = selectThumbnailsSQL
	return
}

func (s *thumbnailStatements) insertThumbnail(
	ctx context.Context, thumbnailMetadata *types.ThumbnailMetadata,
) error {
	thumbnailMetadata.MediaMetadata.CreationTimestamp = types.UnixMs(time.Now().UnixNano() / 1000000)

	// INSERT INTO mediaapi_thumbnail (media_id, media_origin, content_type, file_size_bytes, creation_ts, width, height, resize_method)
	// VALUES ($1, $2, $3, $4, $5, $6, $7, $8)

	// return s.writer.Do(s.db, nil, func(txn *sql.Tx) error {
	// stmt := sqlutil.TxStmt(txn, s.insertThumbnailStmt)

	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
	// CREATE UNIQUE INDEX IF NOT EXISTS mediaapi_thumbnail_index ON mediaapi_thumbnail (media_id, media_origin, width, height, resize_method);
	docId := fmt.Sprintf("%s_%s_%d_%d_s",
		thumbnailMetadata.MediaMetadata.MediaID,
		thumbnailMetadata.MediaMetadata.Origin,
		thumbnailMetadata.ThumbnailSize.Width,
		thumbnailMetadata.ThumbnailSize.Height,
		thumbnailMetadata.ThumbnailSize.ResizeMethod,
	)
	cosmosDocId := cosmosdbapi.GetDocumentId(s.db.cosmosConfig.TenantName, dbCollectionName, docId)
	pk := cosmosdbapi.GetPartitionKey(s.db.cosmosConfig.TenantName, dbCollectionName)

	// _, err := stmt.ExecContext(
	// 	ctx,
	// 	thumbnailMetadata.MediaMetadata.MediaID,
	// 	thumbnailMetadata.MediaMetadata.Origin,
	// 	thumbnailMetadata.MediaMetadata.ContentType,
	// 	thumbnailMetadata.MediaMetadata.FileSizeBytes,
	// 	thumbnailMetadata.MediaMetadata.CreationTimestamp,
	// 	thumbnailMetadata.ThumbnailSize.Width,
	// 	thumbnailMetadata.ThumbnailSize.Height,
	// 	thumbnailMetadata.ThumbnailSize.ResizeMethod,
	// )

	data := ThumbnailCosmos{
		MediaID:           string(thumbnailMetadata.MediaMetadata.MediaID),
		MediaOrigin:       string(thumbnailMetadata.MediaMetadata.Origin),
		ContentType:       string(thumbnailMetadata.MediaMetadata.ContentType),
		FileSizeBytes:     int64(thumbnailMetadata.MediaMetadata.FileSizeBytes),
		CreationTimestamp: int64(thumbnailMetadata.MediaMetadata.CreationTimestamp),
		Width:             int64(thumbnailMetadata.ThumbnailSize.Width),
		Height:            int64(thumbnailMetadata.ThumbnailSize.Height),
		ResizeMethod:      string(thumbnailMetadata.ThumbnailSize.ResizeMethod),
	}

	dbData := &ThumbnailCosmosData{
		Id:        cosmosDocId,
		Tn:        s.db.cosmosConfig.TenantName,
		Cn:        dbCollectionName,
		Pk:        pk,
		Timestamp: time.Now().Unix(),
		Thumbnail: data,
	}

	var options = cosmosdbapi.GetUpsertDocumentOptions(dbData.Pk)
	_, _, err := cosmosdbapi.GetClient(s.db.connection).CreateDocument(
		ctx,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		&dbData,
		options)

	return err
}

func (s *thumbnailStatements) selectThumbnail(
	ctx context.Context,
	mediaID types.MediaID,
	mediaOrigin gomatrixserverlib.ServerName,
	width, height int,
	resizeMethod string,
) (*types.ThumbnailMetadata, error) {
	thumbnailMetadata := types.ThumbnailMetadata{
		MediaMetadata: &types.MediaMetadata{
			MediaID: mediaID,
			Origin:  mediaOrigin,
		},
		ThumbnailSize: types.ThumbnailSize{
			Width:        width,
			Height:       height,
			ResizeMethod: resizeMethod,
		},
	}

	// SELECT content_type, file_size_bytes, creation_ts FROM mediaapi_thumbnail WHERE media_id = $1 AND media_origin = $2 AND width = $3 AND height = $4 AND resize_method = $5

	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
	// CREATE UNIQUE INDEX IF NOT EXISTS mediaapi_thumbnail_index ON mediaapi_thumbnail (media_id, media_origin, width, height, resize_method);
	docId := fmt.Sprintf("%s_%s_%d_%d_s",
		mediaID,
		mediaOrigin,
		width,
		height,
		resizeMethod,
	)
	cosmosDocId := cosmosdbapi.GetDocumentId(s.db.cosmosConfig.TenantName, dbCollectionName, docId)
	pk := cosmosdbapi.GetPartitionKey(s.db.cosmosConfig.TenantName, dbCollectionName)

	// row := sqlutil.TxStmt(txn, s.selectOutboundPeeksStmt).QueryRowContext(ctx, roomID)
	row, err := getThumbnail(s, ctx, pk, cosmosDocId)

	if err != nil {
		return nil, err
	}

	if row == nil {
		return nil, nil
	}

	thumbnailMetadata.MediaMetadata.MediaID = types.MediaID(row.Thumbnail.MediaID)
	thumbnailMetadata.MediaMetadata.Origin = gomatrixserverlib.ServerName(row.Thumbnail.MediaOrigin)
	thumbnailMetadata.ThumbnailSize.Width = int(row.Thumbnail.Width)
	thumbnailMetadata.ThumbnailSize.Height = int(row.Thumbnail.Height)
	thumbnailMetadata.ThumbnailSize.ResizeMethod = row.Thumbnail.ResizeMethod
	thumbnailMetadata.MediaMetadata.ContentType = types.ContentType(row.Thumbnail.ContentType)
	thumbnailMetadata.MediaMetadata.FileSizeBytes = types.FileSizeBytes(row.Thumbnail.FileSizeBytes)
	thumbnailMetadata.MediaMetadata.CreationTimestamp = types.UnixMs(row.Thumbnail.CreationTimestamp)
	return &thumbnailMetadata, nil
}

func (s *thumbnailStatements) selectThumbnails(
	ctx context.Context, mediaID types.MediaID, mediaOrigin gomatrixserverlib.ServerName,
) ([]*types.ThumbnailMetadata, error) {

	// SELECT content_type, file_size_bytes, creation_ts, width, height, resize_method FROM mediaapi_thumbnail WHERE media_id = $1 AND media_origin = $2

	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
	params := map[string]interface{}{
		"@x1": dbCollectionName,
		"@x2": mediaID,
		"@x3": mediaOrigin,
	}

	// rows, err := s.selectThumbnailsStmt.QueryContext(
	// 	ctx, mediaID, mediaOrigin,
	// )
	rows, err := queryThumbnail(s, ctx, s.selectThumbnailsStmt, params)

	if err != nil {
		return nil, err
	}

	var thumbnails []*types.ThumbnailMetadata
	for _, item := range rows {
		thumbnailMetadata := types.ThumbnailMetadata{
			MediaMetadata: &types.MediaMetadata{
				MediaID: mediaID,
				Origin:  mediaOrigin,
			},
		}
		thumbnailMetadata.MediaMetadata.ContentType = types.ContentType(item.Thumbnail.ContentType)
		thumbnailMetadata.MediaMetadata.FileSizeBytes = types.FileSizeBytes(item.Thumbnail.FileSizeBytes)
		thumbnailMetadata.MediaMetadata.CreationTimestamp = types.UnixMs(item.Thumbnail.CreationTimestamp)
		thumbnailMetadata.ThumbnailSize.Width = int(item.Thumbnail.Width)
		thumbnailMetadata.ThumbnailSize.Height = int(item.Thumbnail.Height)
		thumbnailMetadata.ThumbnailSize.ResizeMethod = item.Thumbnail.ResizeMethod

		thumbnails = append(thumbnails, &thumbnailMetadata)
	}

	return thumbnails, err
}
