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

package cosmosdb

import (
	"context"
	"fmt"
	"time"

	"github.com/matrix-org/dendrite/internal/cosmosdbutil"

	"github.com/matrix-org/dendrite/internal/cosmosdbapi"

	"github.com/matrix-org/dendrite/userapi/api"

	"github.com/matrix-org/dendrite/clientapi/userutil"
	"github.com/matrix-org/gomatrixserverlib"
)

// const devicesSchema = `
// -- This sequence is used for automatic allocation of session_id.
// -- CREATE SEQUENCE IF NOT EXISTS device_session_id_seq START 1;

// -- Stores data about devices.
// CREATE TABLE IF NOT EXISTS device_devices (
//     access_token TEXT PRIMARY KEY,
//     session_id INTEGER,
//     device_id TEXT ,
//     localpart TEXT ,
//     created_ts BIGINT,
//     display_name TEXT,
//     last_seen_ts BIGINT,
//     ip TEXT,
//     user_agent TEXT,

// 		UNIQUE (localpart, device_id)
// );
// `

type deviceCosmos struct {
	ID     string `json:"device_id"`
	UserID string `json:"user_id"`
	// The access_token granted to this device.
	// This uniquely identifies the device from all other devices and clients.
	AccessToken string `json:"access_token"`
	// The unique ID of the session identified by the access token.
	// Can be used as a secure substitution in places where data needs to be
	// associated with access tokens.
	SessionID   int64  `json:"session_id"`
	DisplayName string `json:"display_name"`
	LastSeenTS  int64  `json:"last_seen_ts"`
	LastSeenIP  string `json:"last_seen_ip"`
	Localpart   string `json:"local_part"`
	UserAgent   string `json:"user_agent"`
	// If the device is for an appservice user,
	// this is the appservice ID.
	AppserviceID string `json:"app_service_id"`
}

type deviceCosmosData struct {
	cosmosdbapi.CosmosDocument
	Device deviceCosmos `json:"mx_userapi_device"`
}

type deviceCosmosSessionCount struct {
	SessionCount int64 `json:"sessioncount"`
}

type devicesStatements struct {
	db                      *Database
	selectDevicesCountStmt  string
	selectDeviceByTokenStmt string
	// selectDeviceByIDStmt         *sql.Stmt
	selectDevicesByIDStmt                string
	selectDevicesByLocalpartStmt         string
	selectDevicesByLocalpartExceptIDStmt string
	serverName                           gomatrixserverlib.ServerName
	tableName                            string
}

func (s *devicesStatements) getCollectionName() string {
	return cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
}

func (s *devicesStatements) getPartitionKey() string {
	//No easy PK, so just use the collection
	return cosmosdbapi.GetPartitionKeyByCollection(s.db.cosmosConfig.TenantName, s.getCollectionName())
}

func mapFromDevice(db deviceCosmos) api.Device {
	return api.Device{
		AccessToken:  db.AccessToken,
		AppserviceID: db.AppserviceID,
		ID:           db.ID,
		LastSeenIP:   db.LastSeenIP,
		LastSeenTS:   db.LastSeenTS,
		SessionID:    db.SessionID,
		UserAgent:    db.UserAgent,
		UserID:       db.UserID,
	}
}

func mapTodevice(api api.Device, s *devicesStatements) deviceCosmos {
	localPart, _ := userutil.ParseUsernameParam(api.UserID, &s.serverName)
	return deviceCosmos{
		AccessToken:  api.AccessToken,
		AppserviceID: api.AppserviceID,
		ID:           api.ID,
		LastSeenIP:   api.LastSeenIP,
		LastSeenTS:   api.LastSeenTS,
		Localpart:    localPart,
		SessionID:    api.SessionID,
		UserAgent:    api.UserAgent,
		UserID:       api.UserID,
	}
}

func getDevice(s *devicesStatements, ctx context.Context, pk string, docId string) (*deviceCosmosData, error) {
	response := deviceCosmosData{}
	err := cosmosdbapi.GetDocumentOrNil(
		s.db.connection,
		s.db.cosmosConfig,
		ctx,
		pk,
		docId,
		&response)

	if response.Id == "" {
		return nil, cosmosdbutil.ErrNoRows
	}

	return &response, err
}

func setDevice(s *devicesStatements, ctx context.Context, device deviceCosmosData) (*deviceCosmosData, error) {
	var optionsReplace = cosmosdbapi.GetReplaceDocumentOptions(device.Pk, device.ETag)
	var _, _, ex = cosmosdbapi.GetClient(s.db.connection).ReplaceDocument(
		ctx,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		device.Id,
		&device,
		optionsReplace)
	return &device, ex
}

func (s *devicesStatements) prepare(db *Database, server gomatrixserverlib.ServerName) (err error) {
	s.db = db
	s.selectDevicesCountStmt = "select count(c._ts) as sessioncount from c where c._cn = @x1"
	s.selectDevicesByLocalpartStmt = "select * from c where c._cn = @x1 and c.mx_userapi_device.local_part = @x2 and ARRAY_CONTAINS(@x3, c.mx_userapi_device.device_id)"
	s.selectDevicesByLocalpartExceptIDStmt = "select * from c where c._cn = @x1 and c.mx_userapi_device.local_part = @x2 and c.mx_userapi_device.device_id != @x3"
	s.selectDeviceByTokenStmt = "select * from c where c._cn = @x1 and c.mx_userapi_device.access_token = @x2"
	s.selectDevicesByIDStmt = "select * from c where c._cn = @x1 and ARRAY_CONTAINS(@x2, c.mx_userapi_device.device_id)"
	s.serverName = server
	s.tableName = "device_devices"
	return
}

// insertDevice creates a new device. Returns an error if any device with the same access token already exists.
// Returns an error if the user already has a device with the given device ID.
// Returns the device on success.
func (s *devicesStatements) insertDevice(
	ctx context.Context, id, localpart, accessToken string,
	displayName *string, ipAddr, userAgent string,
) (*api.Device, error) {
	createdTimeMS := time.Now().UnixNano() / 1000000
	var sessionID int64
	// "SELECT COUNT(access_token) FROM device_devices"
	// HACK: Do we need a Cosmos Table for the sequence?
	params := map[string]interface{}{
		"@x1": s.getCollectionName(),
	}

	var rows []deviceCosmosSessionCount
	err := cosmosdbapi.PerformQuery(ctx,
		s.db.connection,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		s.getPartitionKey(), s.selectDevicesCountStmt, params, &rows)
	if err != nil {
		return nil, err
	}
	sessionID = rows[0].SessionCount
	sessionID++
	// "INSERT INTO device_devices (device_id, localpart, access_token, created_ts, display_name, session_id, last_seen_ts, ip, user_agent)" +
	// 	" VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)"

	//     access_token TEXT PRIMARY KEY,
	// 		UNIQUE (localpart, device_id)
	// HACK: check for duplicate PK as we are using the UNIQUE key for the DocId
	docId := fmt.Sprintf("%s_%s", localpart, id)
	cosmosDocId := cosmosdbapi.GetDocumentId(s.db.cosmosConfig.TenantName, s.getCollectionName(), docId)

	data := deviceCosmos{
		ID:          id,
		UserID:      userutil.MakeUserID(localpart, s.serverName),
		AccessToken: accessToken,
		SessionID:   sessionID,
		LastSeenTS:  createdTimeMS,
		LastSeenIP:  ipAddr,
		Localpart:   localpart,
		UserAgent:   userAgent,
	}

	var dbData = deviceCosmosData{
		CosmosDocument: cosmosdbapi.GenerateDocument(s.getCollectionName(), s.db.cosmosConfig.TenantName, s.getPartitionKey(), cosmosDocId),
		Device:         data,
	}

	var optionsCreate = cosmosdbapi.GetCreateDocumentOptions(dbData.Pk)
	var _, _, errCreate = cosmosdbapi.GetClient(s.db.connection).CreateDocument(
		ctx,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		dbData,
		optionsCreate)

	if errCreate != nil {
		return nil, errCreate
	}

	var result = mapFromDevice(dbData.Device)
	return &result, nil
}

func (s *devicesStatements) deleteDevice(
	ctx context.Context, id, localpart string,
) error {
	// "DELETE FROM device_devices WHERE device_id = $1 AND localpart = $2"
	docId := fmt.Sprintf("%s_%s", localpart, id)
	cosmosDocId := cosmosdbapi.GetDocumentId(s.db.cosmosConfig.TenantName, s.getCollectionName(), docId)
	var options = cosmosdbapi.GetDeleteDocumentOptions(s.getPartitionKey())
	var _, err = cosmosdbapi.GetClient(s.db.connection).DeleteDocument(
		ctx,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		cosmosDocId,
		options)

	if err != nil {
		return err
	}
	return err
}

func (s *devicesStatements) deleteDevices(
	ctx context.Context, localpart string, devices []string,
) error {
	// "DELETE FROM device_devices WHERE localpart = $1 AND device_id IN ($2)"
	params := map[string]interface{}{
		"@x1": s.getCollectionName(),
		"@x2": localpart,
		"@x3": devices,
	}

	var rows []deviceCosmosData
	err := cosmosdbapi.PerformQuery(ctx,
		s.db.connection,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		s.getPartitionKey(), s.selectDevicesByLocalpartStmt, params, &rows)

	if err != nil {
		return err
	}
	for _, item := range rows {
		s.deleteDevice(ctx, item.Device.ID, item.Device.Localpart)
	}
	return err
}

func (s *devicesStatements) deleteDevicesByLocalpart(
	ctx context.Context, localpart, exceptDeviceID string,
) error {
	// "DELETE FROM device_devices WHERE localpart = $1 AND device_id != $2"
	exceptDevices := []string{
		exceptDeviceID,
	}
	params := map[string]interface{}{
		"@x1": s.getCollectionName(),
		"@x2": localpart,
		"@x3": exceptDevices,
	}
	var rows []deviceCosmosData
	err := cosmosdbapi.PerformQuery(ctx,
		s.db.connection,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		s.getPartitionKey(), s.selectDevicesByLocalpartStmt, params, &rows)

	if err != nil {
		return err
	}
	for _, item := range rows {
		s.deleteDevice(ctx, item.Device.ID, item.Device.Localpart)
	}
	return err
}

func (s *devicesStatements) updateDeviceName(
	ctx context.Context, localpart, deviceID string, displayName *string,
) error {
	// "UPDATE device_devices SET display_name = $1 WHERE localpart = $2 AND device_id = $3"
	docId := fmt.Sprintf("%s_%s", localpart, deviceID)
	cosmosDocId := cosmosdbapi.GetDocumentId(s.db.cosmosConfig.TenantName, s.getCollectionName(), docId)
	var response, exGet = getDevice(s, ctx, s.getPartitionKey(), cosmosDocId)
	if exGet != nil {
		return exGet
	}

	response.Device.DisplayName = *displayName

	var _, exReplace = setDevice(s, ctx, *response)
	if exReplace != nil {
		return exReplace
	}
	return exReplace
}

func (s *devicesStatements) selectDeviceByToken(
	ctx context.Context, accessToken string,
) (*api.Device, error) {
	// "SELECT session_id, device_id, localpart FROM device_devices WHERE access_token = $1"
	params := map[string]interface{}{
		"@x1": s.getCollectionName(),
		"@x2": accessToken,
	}

	var rows []deviceCosmosData
	err := cosmosdbapi.PerformQuery(ctx,
		s.db.connection,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		s.getPartitionKey(), s.selectDeviceByTokenStmt, params, &rows)

	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, cosmosdbutil.ErrNoRows
	}

	if err == nil {
		result := mapFromDevice(rows[0].Device)
		return &result, nil
	}
	return nil, err
}

// selectDeviceByID retrieves a device from the database with the given user
// localpart and deviceID
func (s *devicesStatements) selectDeviceByID(
	ctx context.Context, localpart, deviceID string,
) (*api.Device, error) {
	// "SELECT display_name FROM device_devices WHERE localpart = $1 and device_id = $2"
	docId := fmt.Sprintf("%s_%s", localpart, deviceID)
	cosmosDocId := cosmosdbapi.GetDocumentId(s.db.cosmosConfig.TenantName, s.getCollectionName(), docId)
	var response, exGet = getDevice(s, ctx, s.getPartitionKey(), cosmosDocId)
	if exGet != nil {
		return nil, exGet
	}
	result := mapFromDevice(response.Device)
	return &result, nil
}

func (s *devicesStatements) selectDevicesByLocalpart(
	ctx context.Context, localpart, exceptDeviceID string,
) ([]api.Device, error) {
	devices := []api.Device{}
	// "SELECT device_id, display_name, last_seen_ts, ip, user_agent FROM device_devices WHERE localpart = $1 AND device_id != $2"
	params := map[string]interface{}{
		"@x1": s.getCollectionName(),
		"@x2": localpart,
		"@x3": exceptDeviceID,
	}
	var rows []deviceCosmosData
	err := cosmosdbapi.PerformQuery(ctx,
		s.db.connection,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		s.getPartitionKey(), s.selectDevicesByLocalpartExceptIDStmt, params, &rows)

	if err != nil {
		return nil, err
	}

	for _, item := range rows {
		dev := mapFromDevice(item.Device)
		dev.UserID = userutil.MakeUserID(localpart, s.serverName)
		devices = append(devices, dev)
	}

	return devices, nil
}

func (s *devicesStatements) selectDevicesByID(ctx context.Context, deviceIDs []string) ([]api.Device, error) {
	// "SELECT device_id, localpart, display_name FROM device_devices WHERE device_id IN ($1)"
	var devices []api.Device
	params := map[string]interface{}{
		"@x1": s.getCollectionName(),
		"@x2": deviceIDs,
	}
	var rows []deviceCosmosData
	err := cosmosdbapi.PerformQuery(ctx,
		s.db.connection,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		s.getPartitionKey(), s.selectDevicesByIDStmt, params, &rows)

	if err != nil {
		return nil, err
	}
	for _, item := range rows {
		dev := mapFromDevice(item.Device)
		devices = append(devices, dev)
	}
	return devices, nil
}

func (s *devicesStatements) updateDeviceLastSeen(ctx context.Context, localpart, deviceID, ipAddr string) error {
	lastSeenTs := time.Now().UnixNano() / 1000000

	// "UPDATE device_devices SET last_seen_ts = $1, ip = $2 WHERE localpart = $3 AND device_id = $4"
	docId := fmt.Sprintf("%s_%s", localpart, deviceID)
	cosmosDocId := cosmosdbapi.GetDocumentId(s.db.cosmosConfig.TenantName, s.getCollectionName(), docId)
	var response, exGet = getDevice(s, ctx, s.getPartitionKey(), cosmosDocId)
	if exGet != nil {
		return exGet
	}

	response.Device.LastSeenTS = lastSeenTs
	response.Device.LastSeenIP = ipAddr

	var _, exReplace = setDevice(s, ctx, *response)
	if exReplace != nil {
		return exReplace
	}
	return exReplace
}
