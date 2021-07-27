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

type DeviceCosmos struct {
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

type DeviceCosmosData struct {
	Id        string       `json:"id"`
	Pk        string       `json:"_pk"`
	Tn        string       `json:"_sid"`
	Cn        string       `json:"_cn"`
	ETag      string       `json:"_etag"`
	Timestamp int64        `json:"_ts"`
	Device    DeviceCosmos `json:"mx_userapi_device"`
}

type DeviceCosmosSessionCount struct {
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

func mapFromDevice(db DeviceCosmos) api.Device {
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

func mapTodevice(api api.Device, s *devicesStatements) DeviceCosmos {
	localPart, _ := userutil.ParseUsernameParam(api.UserID, &s.serverName)
	return DeviceCosmos{
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

func queryDevice(s *devicesStatements, ctx context.Context, qry string, params map[string]interface{}) ([]DeviceCosmosData, error) {
	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
	var pk = cosmosdbapi.GetPartitionKey(s.db.cosmosConfig.TenantName, dbCollectionName)
	var response []DeviceCosmosData

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

func getDevice(s *devicesStatements, ctx context.Context, pk string, docId string) (*DeviceCosmosData, error) {
	response := DeviceCosmosData{}
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

func setDevice(s *devicesStatements, ctx context.Context, device DeviceCosmosData) (*DeviceCosmosData, error) {
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
	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.db.devices.tableName)
	var pk = cosmosdbapi.GetPartitionKey(s.db.cosmosConfig.TenantName, dbCollectionName)
	var response []DeviceCosmosSessionCount
	params := map[string]interface{}{
		"@x1": dbCollectionName,
	}

	var optionsQry = cosmosdbapi.GetQueryDocumentsOptions(pk)
	var query = cosmosdbapi.GetQuery(s.selectDevicesCountStmt, params)
	var _, err = cosmosdbapi.GetClient(s.db.connection).QueryDocuments(
		ctx,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		query,
		&response,
		optionsQry)
	if err != nil {
		return nil, err
	}
	sessionID = response[0].SessionCount
	sessionID++
	// "INSERT INTO device_devices (device_id, localpart, access_token, created_ts, display_name, session_id, last_seen_ts, ip, user_agent)" +
	// 	" VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)"

	data := DeviceCosmos{
		ID:          id,
		UserID:      userutil.MakeUserID(localpart, s.serverName),
		AccessToken: accessToken,
		SessionID:   sessionID,
		LastSeenTS:  createdTimeMS,
		LastSeenIP:  ipAddr,
		Localpart:   localpart,
		UserAgent:   userAgent,
	}

	//     access_token TEXT PRIMARY KEY,
	// 		UNIQUE (localpart, device_id)
	// HACK: check for duplicate PK as we are using the UNIQUE key for the DocId
	docId := fmt.Sprintf("%s_%s", localpart, id)
	cosmosDocId := cosmosdbapi.GetDocumentId(s.db.cosmosConfig.TenantName, dbCollectionName, docId)

	var dbData = DeviceCosmosData{
		Id:        cosmosDocId,
		Tn:        s.db.cosmosConfig.TenantName,
		Cn:        dbCollectionName,
		Pk:        pk,
		Timestamp: time.Now().Unix(),
		Device:    data,
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
	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.db.devices.tableName)
	docId := fmt.Sprintf("%s_%s", localpart, id)
	cosmosDocId := cosmosdbapi.GetDocumentId(s.db.cosmosConfig.TenantName, dbCollectionName, docId)
	pk := cosmosdbapi.GetPartitionKey(s.db.cosmosConfig.TenantName, dbCollectionName)
	var options = cosmosdbapi.GetDeleteDocumentOptions(pk)
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
	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.db.devices.tableName)
	var response []DeviceCosmosData
	params := map[string]interface{}{
		"@x1": dbCollectionName,
		"@x2": localpart,
		"@x3": devices,
	}

	response, err := queryDevice(s, ctx, s.selectDevicesByLocalpartStmt, params)

	if err != nil {
		return err
	}
	for _, item := range response {
		s.deleteDevice(ctx, item.Device.ID, item.Device.Localpart)
	}
	return err
}

func (s *devicesStatements) deleteDevicesByLocalpart(
	ctx context.Context, localpart, exceptDeviceID string,
) error {
	// "DELETE FROM device_devices WHERE localpart = $1 AND device_id != $2"
	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.db.devices.tableName)
	exceptDevices := []string{
		exceptDeviceID,
	}
	params := map[string]interface{}{
		"@x1": dbCollectionName,
		"@x2": localpart,
		"@x3": exceptDevices,
	}

	response, err := queryDevice(s, ctx, s.selectDevicesByLocalpartStmt, params)

	if err != nil {
		return err
	}
	for _, item := range response {
		s.deleteDevice(ctx, item.Device.ID, item.Device.Localpart)
	}
	return err
}

func (s *devicesStatements) updateDeviceName(
	ctx context.Context, localpart, deviceID string, displayName *string,
) error {
	// "UPDATE device_devices SET display_name = $1 WHERE localpart = $2 AND device_id = $3"
	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.db.devices.tableName)
	docId := fmt.Sprintf("%s_%s", localpart, deviceID)
	cosmosDocId := cosmosdbapi.GetDocumentId(s.db.cosmosConfig.TenantName, dbCollectionName, docId)
	pk := cosmosdbapi.GetPartitionKey(s.db.cosmosConfig.TenantName, dbCollectionName)
	var response, exGet = getDevice(s, ctx, pk, cosmosDocId)
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
	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.db.devices.tableName)
	var response []DeviceCosmosData
	params := map[string]interface{}{
		"@x1": dbCollectionName,
		"@x2": accessToken,
	}

	response, err := queryDevice(s, ctx, s.selectDeviceByTokenStmt, params)

	if err != nil {
		return nil, err
	}
	if len(response) == 0 {
		return nil, cosmosdbutil.ErrNoRows
	}

	if err == nil {
		result := mapFromDevice(response[0].Device)
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
	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.db.devices.tableName)
	docId := fmt.Sprintf("%s_%s", localpart, deviceID)
	cosmosDocId := cosmosdbapi.GetDocumentId(s.db.cosmosConfig.TenantName, dbCollectionName, docId)
	pk := cosmosdbapi.GetPartitionKey(s.db.cosmosConfig.TenantName, dbCollectionName)
	var response, exGet = getDevice(s, ctx, pk, cosmosDocId)
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
	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.db.devices.tableName)
	params := map[string]interface{}{
		"@x1": dbCollectionName,
		"@x2": localpart,
		"@x3": exceptDeviceID,
	}

	response, err := queryDevice(s, ctx, s.selectDevicesByLocalpartExceptIDStmt, params)

	if err != nil {
		return nil, err
	}

	for _, item := range response {
		dev := mapFromDevice(item.Device)
		dev.UserID = userutil.MakeUserID(localpart, s.serverName)
		devices = append(devices, dev)
	}

	return devices, nil
}

func (s *devicesStatements) selectDevicesByID(ctx context.Context, deviceIDs []string) ([]api.Device, error) {
	// "SELECT device_id, localpart, display_name FROM device_devices WHERE device_id IN ($1)"
	var devices []api.Device
	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.db.devices.tableName)
	var response []DeviceCosmosData
	params := map[string]interface{}{
		"@x1": dbCollectionName,
		"@x2": deviceIDs,
	}

	response, err := queryDevice(s, ctx, s.selectDevicesByIDStmt, params)

	if err != nil {
		return nil, err
	}
	for _, item := range response {
		dev := mapFromDevice(item.Device)
		devices = append(devices, dev)
	}
	return devices, nil
}

func (s *devicesStatements) updateDeviceLastSeen(ctx context.Context, localpart, deviceID, ipAddr string) error {
	lastSeenTs := time.Now().UnixNano() / 1000000

	// "UPDATE device_devices SET last_seen_ts = $1, ip = $2 WHERE localpart = $3 AND device_id = $4"
	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.db.devices.tableName)
	docId := fmt.Sprintf("%s_%s", localpart, deviceID)
	cosmosDocId := cosmosdbapi.GetDocumentId(s.db.cosmosConfig.TenantName, dbCollectionName, docId)
	pk := cosmosdbapi.GetPartitionKey(s.db.cosmosConfig.TenantName, dbCollectionName)
	var response, exGet = getDevice(s, ctx, pk, cosmosDocId)
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
