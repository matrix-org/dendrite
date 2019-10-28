// Copyright 2019 Sumukha PK
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

package routing

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/Shopify/sarama"
	"github.com/matrix-org/dendrite/clientapi/httputil"
	"github.com/matrix-org/dendrite/encryptoapi/storage"
	"github.com/matrix-org/dendrite/encryptoapi/types"
	"github.com/matrix-org/util"
	"github.com/pkg/errors"
)

// UploadPKeys enables the user to upload his device
// and one time keys with limit at 50 set as default
func UploadPKeys(
	req *http.Request,
	encryptionDB *storage.Database,
	userID, deviceID string,
) util.JSONResponse {
	var keybody types.UploadEncrypt
	if reqErr := httputil.UnmarshalJSONRequest(req, &keybody); reqErr != nil {
		return *reqErr
	}
	keySpecific := turnSpecific(keybody)
	// persist keys into encryptionDB
	err := persistKeys(
		req.Context(),
		encryptionDB,
		&keySpecific,
		userID, deviceID)
	// numMap is algorithm-num map
	// this gets the number of unclaimed OTkeys
	numMap, ok := (queryOneTimeKeys(
		req.Context(),
		TYPESUM,
		userID,
		deviceID,
		encryptionDB)).(map[string]int)
	if !ok {
		return util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: struct{}{},
		}
	}
	if err != nil {
		return util.JSONResponse{
			Code: http.StatusBadGateway,
			JSON: types.UploadResponse{
				Count: numMap,
			},
		}
	}
	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: types.UploadResponse{
			Count: numMap,
		},
	}
}

// make keys instantiated to specific struct from keybody interface{}
func turnSpecific(
	cont types.UploadEncrypt,
) (spec types.UploadEncryptSpecific) {
	// both device keys are coordinate
	spec.DeviceKeys = cont.DeviceKeys
	spec.OneTimeKey.KeyString = make(map[string]string)
	spec.OneTimeKey.KeyObject = make(map[string]types.KeyObject)
	mapStringInterface := cont.OneTimeKey
	for key, val := range mapStringInterface {
		value, ok := val.(string)
		if ok {
			spec.OneTimeKey.KeyString[key] = value
		} else {
			valueObject := types.KeyObject{}
			target, _ := json.Marshal(val)
			err := json.Unmarshal(target, &valueObject)
			if err != nil {
				continue
			}
			spec.OneTimeKey.KeyObject[key] = valueObject
		}
	}
	return
}

// persist both device keys and one time keys
func persistKeys(
	ctx context.Context,
	database *storage.Database,
	body *types.UploadEncryptSpecific,
	userID,
	deviceID string,
) (err error) {
	// in order to persist keys , a check,
	// filtering the duplicates should be processed.
	// true stands for counterparts are in request
	// situation 1: only device keys
	// situation 2: both device keys and one time keys
	// situation 3: only one time keys
	if checkUpload(body, BODYDEVICEKEY) {
		deviceKeys := body.DeviceKeys
		al := deviceKeys.Algorithm
		err = persistAlgo(ctx, *database, userID, deviceID, al)
		if err != nil {
			return
		}
		if checkUpload(body, BODYONETIMEKEY) {
			if err = persistBothKeys(ctx, body, userID, deviceID, database, deviceKeys); err != nil {
				return
			}
		} else {
			if err = persistDeviceKeys(ctx, userID, deviceID, database, deviceKeys); err != nil {
				return
			}
		}
		// notifier to sync server
		upnotify(userID)
	} else {
		if checkUpload(body, BODYONETIMEKEY) {
			if err = persistOneTimeKeys(ctx, body, userID, deviceID, database); err != nil {
				return
			}
		} else {
			return errors.New("failed to persist keys")
		}
	}
	return err
}

// todo: check through interface for duplicate and what type of request should it be
// whether device or one time or both of them
func checkUpload(req *types.UploadEncryptSpecific, typ int) bool {
	if typ == BODYDEVICEKEY {
		devicekey := req.DeviceKeys
		if devicekey.UserID == "" {
			return false
		}
	}
	if typ == BODYONETIMEKEY {
		if req.OneTimeKey.KeyString == nil || req.OneTimeKey.KeyObject == nil {
			return false
		}
	}
	return true
}

func persistAlgo(
	ctx context.Context,
	encryptDB storage.Database,
	uid, device string,
	al []string,
) (err error) {
	err = encryptDB.InsertAl(ctx, uid, device, al)
	return
}

// queryOneTimeKeys todo: complete this field through claim type
func queryOneTimeKeys(
	ctx context.Context,
	typ int,
	userID, deviceID string,
	encryptionDB *storage.Database,
) interface{} {
	if typ == TYPESUM {
		res, _ := encryptionDB.SelectOneTimeKeyCount(ctx, deviceID, userID)
		return res
	}
	return nil
}

func upnotify(userID string) {
	m := sarama.ProducerMessage{
		Topic: "keyUpdate",
		Key:   sarama.StringEncoder("key"),
		Value: sarama.StringEncoder(userID),
	}
	keyProducer.ch.Input() <- &m
}

func persistBothKeys(
	ctx context.Context,
	body *types.UploadEncryptSpecific,
	userID, deviceID string,
	database *storage.Database,
	deviceKeys types.DeviceKeys,
) (err error) {
	// insert one time keys
	err = persistOneTimeKeys(ctx, body, userID, deviceID, database)
	if err != nil {
		return
	}
	// insert device keys
	err = persistDeviceKeys(ctx, userID, deviceID, database, deviceKeys)
	if err != nil {
		return
	}
	return
}

func persistDeviceKeys(
	ctx context.Context,
	userID, deviceID string,
	database *storage.Database,
	deviceKeys types.DeviceKeys,
) (err error) {
	keys := deviceKeys.Keys
	sigs := deviceKeys.Signature
	for alDevice, key := range keys {
		al := (strings.Split(alDevice, ":"))[0]
		keyTyp := DEVICEKEYSTR
		keyInfo := key
		keyID := ""
		sig := sigs[userID][fmt.Sprintf("%s:%s", "ed25519", deviceID)]
		err = database.InsertKey(ctx, deviceID, userID, keyID, keyTyp, keyInfo, al, sig)
	}
	return
}

func persistOneTimeKeys(
	ctx context.Context,
	body *types.UploadEncryptSpecific,
	userID, deviceID string,
	database *storage.Database,
) (err error) {
	onetimeKeys := body.OneTimeKey
	for alKeyID, val := range onetimeKeys.KeyString {
		al := (strings.Split(alKeyID, ":"))[0]
		keyID := (strings.Split(alKeyID, ":"))[1]
		keyInfo := val
		oneTimeKeyStringTyp := ONETIMEKEYSTR
		sig := ""
		err = database.InsertKey(ctx, deviceID, userID, keyID, oneTimeKeyStringTyp, keyInfo, al, sig)
		if err != nil {
			return
		}
	}
	for alKeyID, val := range onetimeKeys.KeyObject {
		al := (strings.Split(alKeyID, ":"))[0]
		keyID := (strings.Split(alKeyID, ":"))[1]
		keyInfo := val.Key
		oneTimeKeyObjectTyp := ONETIMEKEYSTR
		sig := val.Signature[userID][fmt.Sprintf("%s:%s", "ed25519", deviceID)]
		err = database.InsertKey(ctx, deviceID, userID, keyID, oneTimeKeyObjectTyp, keyInfo, al, sig)
		if err != nil {
			return
		}
	}
	return
}
