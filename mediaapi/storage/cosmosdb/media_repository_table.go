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

// const mediaSchema = `
// -- The media_repository table holds metadata for each media file stored and accessible to the local server,
// -- the actual file is stored separately.
// CREATE TABLE IF NOT EXISTS mediaapi_media_repository (
//     -- The id used to refer to the media.
//     -- For uploads to this server this is a base64-encoded sha256 hash of the file data
//     -- For media from remote servers, this can be any unique identifier string
//     media_id TEXT NOT NULL,
//     -- The origin of the media as requested by the client. Should be a homeserver domain.
//     media_origin TEXT NOT NULL,
//     -- The MIME-type of the media file as specified when uploading.
//     content_type TEXT NOT NULL,
//     -- Size of the media file in bytes.
//     file_size_bytes INTEGER NOT NULL,
//     -- When the content was uploaded in UNIX epoch ms.
//     creation_ts INTEGER NOT NULL,
//     -- The file name with which the media was uploaded.
//     upload_name TEXT NOT NULL,
//     -- Alternate RFC 4648 unpadded base64 encoding string representation of a SHA-256 hash sum of the file data.
//     base64hash TEXT NOT NULL,
//     -- The user who uploaded the file. Should be a Matrix user ID.
//     user_id TEXT NOT NULL
// );
// CREATE UNIQUE INDEX IF NOT EXISTS mediaapi_media_repository_index ON mediaapi_media_repository (media_id, media_origin);
// `

type mediaRepositoryCosmos struct {
	MediaID           string `json:"media_id"`
	MediaOrigin       string `json:"media_origin"`
	ContentType       string `json:"content_type"`
	FileSizeBytes     int64  `json:"file_size_bytes"`
	CreationTimestamp int64  `json:"creation_ts"`
	UploadName        string `json:"upload_name"`
	Base64hash        string `json:"base64hash"`
	UserID            string `json:"user_id"`
}

type mediaRepositoryCosmosData struct {
	cosmosdbapi.CosmosDocument
	MediaRepository mediaRepositoryCosmos `json:"mx_mediaapi_media_repository"`
}

// const insertMediaSQL = `
// INSERT INTO mediaapi_media_repository (media_id, media_origin, content_type, file_size_bytes, creation_ts, upload_name, base64hash, user_id)
//     VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
// `

// const selectMediaSQL = `
// SELECT content_type, file_size_bytes, creation_ts, upload_name, base64hash, user_id FROM mediaapi_media_repository WHERE media_id = $1 AND media_origin = $2
// `

// SELECT content_type, file_size_bytes, creation_ts, upload_name, media_id, user_id FROM mediaapi_media_repository WHERE base64hash = $1 AND media_origin = $2
const selectMediaByHashSQL = "" +
	"select * from c where c._cn = @x1 " +
	"and c.mx_mediaapi_media_repository.base64hash = @x2 " +
	"and c.mx_mediaapi_media_repository.media_origin = @x3 "

type mediaStatements struct {
	db     *Database
	writer cosmosdbutil.Writer
	// insertMediaStmt       *sql.Stmt
	// selectMediaStmt       *sql.Stmt
	selectMediaByHashStmt string
	tableName             string
}

func (s *mediaStatements) getCollectionName() string {
	return cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
}

func (s *mediaStatements) getPartitionKey() string {
	return cosmosdbapi.GetPartitionKeyByCollection(s.db.cosmosConfig.TenantName, s.getCollectionName())
}

func getMediaRepository(s *mediaStatements, ctx context.Context, pk string, docId string) (*mediaRepositoryCosmosData, error) {
	response := mediaRepositoryCosmosData{}
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

func (s *mediaStatements) prepare(db *Database, writer cosmosdbutil.Writer) (err error) {
	s.db = db
	s.writer = writer

	s.selectMediaByHashStmt = selectMediaByHashSQL
	s.tableName = "media_repository"
	return
}

func (s *mediaStatements) insertMedia(
	ctx context.Context, mediaMetadata *types.MediaMetadata,
) error {
	mediaMetadata.CreationTimestamp = types.UnixMs(time.Now().UnixNano() / 1000000)
	// return s.writer.Do(s.db, nil, func(txn *sql.Tx) error {

	// INSERT INTO mediaapi_media_repository (media_id, media_origin, content_type, file_size_bytes, creation_ts, upload_name, base64hash, user_id)
	// VALUES ($1, $2, $3, $4, $5, $6, $7, $8)

	// CREATE UNIQUE INDEX IF NOT EXISTS mediaapi_media_repository_index ON mediaapi_media_repository (media_id, media_origin);
	docId := fmt.Sprintf("%s_%s", mediaMetadata.MediaID, mediaMetadata.Origin)
	cosmosDocId := cosmosdbapi.GetDocumentId(s.db.cosmosConfig.TenantName, s.getCollectionName(), docId)

	data := mediaRepositoryCosmos{
		MediaID:           string(mediaMetadata.MediaID),
		MediaOrigin:       string(mediaMetadata.Origin),
		ContentType:       string(mediaMetadata.ContentType),
		FileSizeBytes:     int64(mediaMetadata.FileSizeBytes),
		CreationTimestamp: int64(mediaMetadata.CreationTimestamp),
		UploadName:        string(mediaMetadata.UploadName),
		Base64hash:        string(mediaMetadata.Base64Hash),
		UserID:            string(mediaMetadata.UserID),
	}

	dbData := &mediaRepositoryCosmosData{
		CosmosDocument:  cosmosdbapi.GenerateDocument(s.getCollectionName(), s.db.cosmosConfig.TenantName, s.getPartitionKey(), cosmosDocId),
		MediaRepository: data,
	}

	// stmt := sqlutil.TxStmt(txn, s.insertMediaStmt)
	// _, err := stmt.ExecContext(
	// 	ctx,
	// 	mediaMetadata.MediaID,
	// 	mediaMetadata.Origin,
	// 	mediaMetadata.ContentType,
	// 	mediaMetadata.FileSizeBytes,
	// 	mediaMetadata.CreationTimestamp,
	// 	mediaMetadata.UploadName,
	// 	mediaMetadata.Base64Hash,
	// 	mediaMetadata.UserID,
	// )

	var options = cosmosdbapi.GetCreateDocumentOptions(dbData.Pk)
	_, _, err := cosmosdbapi.GetClient(s.db.connection).CreateDocument(
		ctx,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		&dbData,
		options)

	return err
	// })
}

func (s *mediaStatements) selectMedia(
	ctx context.Context, mediaID types.MediaID, mediaOrigin gomatrixserverlib.ServerName,
) (*types.MediaMetadata, error) {
	mediaMetadata := types.MediaMetadata{
		MediaID: mediaID,
		Origin:  mediaOrigin,
	}

	// SELECT content_type, file_size_bytes, creation_ts, upload_name, base64hash, user_id FROM mediaapi_media_repository WHERE media_id = $1 AND media_origin = $2

	// CREATE UNIQUE INDEX IF NOT EXISTS mediaapi_media_repository_index ON mediaapi_media_repository (media_id, media_origin);
	docId := fmt.Sprintf("%s_%s", mediaMetadata.MediaID, mediaMetadata.Origin)
	cosmosDocId := cosmosdbapi.GetDocumentId(s.db.cosmosConfig.TenantName, s.getCollectionName(), docId)

	// err := s.selectMediaStmt.QueryRowContext(
	// ctx, mediaMetadata.MediaID, mediaMetadata.Origin,
	row, err := getMediaRepository(s, ctx, s.getPartitionKey(), cosmosDocId)

	if err != nil {
		return nil, err
	}

	if row == nil {
		return nil, nil
	}

	mediaMetadata.ContentType = types.ContentType(row.MediaRepository.ContentType)
	mediaMetadata.FileSizeBytes = types.FileSizeBytes(row.MediaRepository.FileSizeBytes)
	mediaMetadata.CreationTimestamp = types.UnixMs(row.MediaRepository.CreationTimestamp)
	mediaMetadata.UploadName = types.Filename(row.MediaRepository.UploadName)
	mediaMetadata.Base64Hash = types.Base64Hash(row.MediaRepository.Base64hash)
	mediaMetadata.UserID = types.MatrixUserID(row.MediaRepository.UserID)

	return &mediaMetadata, err
}

func (s *mediaStatements) selectMediaByHash(
	ctx context.Context, mediaHash types.Base64Hash, mediaOrigin gomatrixserverlib.ServerName,
) (*types.MediaMetadata, error) {
	mediaMetadata := types.MediaMetadata{
		Base64Hash: mediaHash,
		Origin:     mediaOrigin,
	}

	// SELECT content_type, file_size_bytes, creation_ts, upload_name, media_id, user_id FROM mediaapi_media_repository WHERE base64hash = $1 AND media_origin = $2

	params := map[string]interface{}{
		"@x1": s.getCollectionName(),
		"@x2": mediaHash,
		"@x3": mediaOrigin,
	}

	// err := s.selectMediaStmt.QueryRowContext(
	// 	ctx, mediaMetadata.Base64Hash, mediaMetadata.Origin,
	// ).Scan(
	var rows []mediaRepositoryCosmosData
	err := cosmosdbapi.PerformQuery(ctx,
		s.db.connection,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		s.getPartitionKey(), s.selectMediaByHashStmt, params, &rows)

	if err != nil {
		return nil, err
	}

	if len(rows) == 0 {
		return nil, nil
	}

	row := rows[0]

	mediaMetadata.ContentType = types.ContentType(row.MediaRepository.ContentType)
	mediaMetadata.FileSizeBytes = types.FileSizeBytes(row.MediaRepository.FileSizeBytes)
	mediaMetadata.CreationTimestamp = types.UnixMs(row.MediaRepository.CreationTimestamp)
	mediaMetadata.UploadName = types.Filename(row.MediaRepository.UploadName)
	mediaMetadata.MediaID = types.MediaID(row.MediaRepository.MediaID)
	mediaMetadata.UserID = types.MatrixUserID(row.MediaRepository.UserID)
	return &mediaMetadata, err
}
