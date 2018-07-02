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

package routing

import (
	"github.com/matrix-org/util"
	"github.com/matrix-org/dendrite/encryptoapi/storage"
	"net/http"
	"github.com/matrix-org/dendrite/clientapi/httputil"
	"github.com/matrix-org/dendrite/encryptoapi/types"
	"context"
	"github.com/pkg/errors"
	"strings"
	"fmt"
	"encoding/json"
	"time"
	"github.com/matrix-org/dendrite/clientapi/auth/storage/devices"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/dendrite/common/basecomponent"
	"github.com/Shopify/sarama"
)

const (
	TYPESUM          = iota
	TYPECLAIM
	TYPEVAL
	BODYDEVICEKEY
	BODYONETIMEKEY
	ONETIMEKEYSTRING
	ONETIMEKEYOBJECT
)

type KeyNotifier struct {
	base *basecomponent.BaseDendrite
	ch   sarama.AsyncProducer
}

var keyProducer = &KeyNotifier{}

func UploadPKeys(req *http.Request, encryptionDB *storage.Database, userID, deviceID string) util.JSONResponse {
	var keybody types.UploadEncrypt
	if reqErr := httputil.UnmarshalJSONRequest(req, &keybody); reqErr != nil {
		return *reqErr
	}
	keySpecific := turnSpecific(keybody)
	err := persistKeys(encryptionDB, req.Context(), &keySpecific, userID, deviceID)
	numMap := (QueryOneTimeKeys(
		TYPESUM,
		userID,
		deviceID,
		encryptionDB,
		req.Context())).(map[string]int)
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

func QueryPKeys(req *http.Request, encryptionDB *storage.Database, userID, deviceID string, deviceDB *devices.Database) util.JSONResponse {
	var queryRq types.QueryRequest
	queryRp := types.QueryResponse{}
	queryRp.Failure = make(map[string]interface{})
	queryRp.DeviceKeys = make(map[string]map[string]types.DeviceKeysQuery)
	if reqErr := httputil.UnmarshalJSONRequest(req, &queryRq); reqErr != nil {
		return *reqErr
	}

	/*
		federation consideration: when user id is in federation, a query is needed to ask fed for keys
		domain --------+ fed (keys)
		domain +--tout-- timer
	*/
	// todo: Add federation processing at specific userID.
	if false /*federation judgement*/ {
		tout := queryRq.Timeout
		if tout == 0 {
			tout = int64(10 * time.Second)
		}
		stimuCh := make(chan int)
		go func() {
			time.Sleep(time.Duration(tout) * 1000 * 1000)
			close(stimuCh)
		}()
		select {
		case <-stimuCh:
			queryRp.Failure = make(map[string]interface{})
			// todo: key in this map is restricted to username at the end, yet a mocked one.
			queryRp.Failure["@alice:localhost"] = "ran out of offered time"
		}
	}

	for uid, arr := range queryRq.DeviceKeys {
		queryRp.DeviceKeys[uid] = make(map[string]types.DeviceKeysQuery)
		deviceKeysQueryMap := queryRp.DeviceKeys[uid]
		// backward compatible to old interface
		midArr := []string{}
		for device, _ := range arr.(map[string]interface{}) {
			midArr = append(midArr, device)
		}
		dkeys, _ := encryptionDB.QueryInRange(req.Context(), uid, midArr)
		for _, key := range dkeys {
			// preset for complicated nested map struct
			if _, ok := deviceKeysQueryMap[key.Device_id]; !ok {
				// make consistency
				deviceKeysQueryMap[key.Device_id] = types.DeviceKeysQuery{}
			}
			if deviceKeysQueryMap[key.Device_id].Signature == nil {
				mid := make(map[string]map[string]string)
				midmap := deviceKeysQueryMap[key.Device_id]
				midmap.Signature = mid
				deviceKeysQueryMap[key.Device_id] = midmap
			}
			if deviceKeysQueryMap[key.Device_id].Keys == nil {
				mid := make(map[string]string)
				midmap := deviceKeysQueryMap[key.Device_id]
				midmap.Keys = mid
				deviceKeysQueryMap[key.Device_id] = midmap
			}
			if _, ok := deviceKeysQueryMap[key.Device_id].Signature[uid]; !ok {
				// make consistency
				deviceKeysQueryMap[key.Device_id].Signature[uid] = make(map[string]string)
			}
			// load for accomplishment
			single := deviceKeysQueryMap[key.Device_id]

			resKey := fmt.Sprintf("@%s:%s", key.Key_algorithm, key.Device_id)
			resBody := key.Key
			if _, ok := single.Keys[resKey]; !ok {
			}
			single.Keys[resKey] = resBody
			single.DeviceId = key.Device_id
			single.UserId = key.User_id
			single.Signature[uid][fmt.Sprintf("@%s:%s", "ed25519", key.Device_id)] = key.Signature
			single.Algorithm, _ = takeAL(*encryptionDB, req.Context(), key.User_id, key.Device_id)
			localpart, _, _ := gomatrixserverlib.SplitID('@', uid)
			device, _ := deviceDB.GetDeviceByID(req.Context(), localpart, deviceID)
			single.Unsigned.Info = device.DisplayName
			deviceKeysQueryMap[key.Device_id] = single
		}
	}
	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: queryRp,
	}
}

func ClaimOneTimeKeys(req *http.Request, encryptionDB *storage.Database, userID, deviceID string, deviceDB *devices.Database) util.JSONResponse {
	var claimRq types.ClaimRequest
	claimRp := types.ClaimResponse{}
	claimRp.Failures = make(map[string]interface{})
	claimRp.ClaimBody = make(map[string]map[string]map[string]interface{})
	if reqErr := httputil.UnmarshalJSONRequest(req, &claimRq); reqErr != nil {
		return *reqErr
	}

	/*
		federation consideration: when user id is in federation, a query is needed to ask fed for keys
		domain --------+ fed (keys)
		domain +--tout-- timer
	*/
	// todo: Add federation processing at specific userID.
	if false /*federation judgement*/ {
		tout := claimRq.Timeout
		stimuCh := make(chan int)
		go func() {
			time.Sleep(time.Duration(tout) * 1000 * 1000)
			close(stimuCh)
		}()
		select {
		case <-stimuCh:
			claimRp.Failures = make(map[string]interface{})
			// todo: key in this map is restricted to username at the end, yet a mocked one.
			claimRp.Failures["@alice:localhost"] = "ran out of offered time"
		}
	}

	content := claimRq.ClaimDetail
	for uid, detail := range content {
		for deviceID, al := range detail {
			var alTyp int
			if strings.Contains(al, "signed") {
				alTyp = ONETIMEKEYOBJECT
			} else {
				alTyp = ONETIMEKEYSTRING
			}
			key, err := pickOne(*encryptionDB, req.Context(), uid, deviceID, al)
			if err != nil {
				claimRp.Failures[uid] = fmt.Sprintf("%s:%s", "fail to get keys for device ", deviceID)
			}
			claimRp.ClaimBody[uid] = make(map[string]map[string]interface{})
			keymap := claimRp.ClaimBody[uid][deviceID]
			keymap = make(map[string]interface{})
			switch alTyp {
			case ONETIMEKEYSTRING:
				keymap[fmt.Sprintf("%s:%s", al, key.Key_id)] = key.Key
			case ONETIMEKEYOBJECT:
				sig := make(map[string]map[string]string)
				sig[uid] = make(map[string]string)
				sig[uid][fmt.Sprintf("%s:%s", "ed25519", deviceID)] = key.Signature
				keymap[fmt.Sprintf("%s:%s", al, key.Key_id)] = types.KeyObject{Key: key.Key, Signature: sig}
			}
			claimRp.ClaimBody[uid][deviceID] = keymap
		}
	}
	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: claimRp,
	}
}

func LookUpChangedPKeys() util.JSONResponse {
	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: struct{}{},
	}
}

// todo: check through interface for duplicate
func checkUpload(req *types.UploadEncryptSpecific, typ int) bool {
	if typ == BODYDEVICEKEY {
		devicekey := req.DeviceKeys
		if devicekey.UserId == "" {
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

// todo: complete this field through claim type
func QueryOneTimeKeys(
	typ int,
	userID, deviceID string,
	encryptionDB *storage.Database,
	ctx context.Context,
) interface{} {
	if typ == TYPESUM {
		res, _ := encryptionDB.SelectOneTimeKeyCount(ctx, deviceID, userID)
		return res
	}
	return nil
}

// todo: complete this function and invoke through sign out extension or some scenarios else those matter
// when web client sign out, a clean should be processed, cause all keys would never been used from then on.
func ClearUnused() {}

func persistKeys(
	database *storage.Database,
	ctx context.Context,
	body *types.UploadEncryptSpecific,
	userID,
	deviceID string,
) (err error) {
	// in order to persist keys , a check filtering duplicate should be processed
	if checkUpload(body, BODYDEVICEKEY) {
		deviceKeys := body.DeviceKeys
		al := deviceKeys.Algorithm
		err = persistAl(*database, ctx, userID, deviceID, al)
		if checkUpload(body, BODYONETIMEKEY) {
			// insert one time keys firstly
			onetimeKeys := body.OneTimeKey
			for al_keyID, val := range onetimeKeys.KeyString {
				al := (strings.Split(al_keyID, ":"))[0]
				keyID := (strings.Split(al_keyID, ":"))[1]
				keyInfo := val
				keyTyp := "one_time_key"
				sig := ""
				database.InsertKey(ctx, deviceID, userID, keyID, keyTyp, keyInfo, al, sig)
			}
			for al_keyID, val := range onetimeKeys.KeyObject {
				al := (strings.Split(al_keyID, ":"))[0]
				keyID := (strings.Split(al_keyID, ":"))[1]
				keyInfo := val.Key
				keyTyp := "one_time_key"
				sig := val.Signature[userID][fmt.Sprintf("%s:%s", "ed25519", deviceID)]
				database.InsertKey(ctx, deviceID, userID, keyID, keyTyp, keyInfo, al, sig)
			}
			// insert device keys
			keys := deviceKeys.Keys
			sigs := deviceKeys.Signature
			for al_device, key := range keys {
				al := (strings.Split(al_device, ":"))[0]
				keyTyp := "device_key"
				keyInfo := key
				keyID := ""
				sig := sigs[userID][fmt.Sprintf("%s:%s", "ed25519", deviceID)]
				database.InsertKey(
					ctx, deviceID, userID, keyID, keyTyp, keyInfo, al, sig)
			}
		} else {
			keys := deviceKeys.Keys
			sigs := deviceKeys.Signature
			for al_device, key := range keys {
				al := (strings.Split(al_device, ":"))[0]
				keyTyp := "device_key"
				keyInfo := key
				keyID := ""
				sig := sigs[userID][fmt.Sprintf("%s:%s", "ed25519", deviceID)]
				database.InsertKey(ctx, deviceID, userID, keyID, keyTyp, keyInfo, al, sig)
			}
		}
		// notifier to sync server
		upnotify(userID)
	} else {
		if checkUpload(body, BODYONETIMEKEY) {
			onetimeKeys := body.OneTimeKey
			for al_keyID, val := range onetimeKeys.KeyString {
				al := (strings.Split(al_keyID, ":"))[0]
				keyID := (strings.Split(al_keyID, ":"))[1]
				keyInfo := val
				keyTyp := "one_time_key"
				sig := ""
				database.InsertKey(ctx, deviceID, userID, keyID, keyTyp, keyInfo, al, sig)
			}
			for al_keyID, val := range onetimeKeys.KeyObject {
				al := (strings.Split(al_keyID, ":"))[0]
				keyID := (strings.Split(al_keyID, ":"))[1]
				keyInfo := val.Key
				keyTyp := "one_time_key"
				sig := val.Signature[userID][fmt.Sprintf("%s:%s", "ed25519", deviceID)]
				database.InsertKey(ctx, deviceID, userID, keyID, keyTyp, keyInfo, al, sig)
			}
		} else {
			return errors.New("Fail to touch keys !")
		}
	}
	return err
}

func turnSpecific(cont types.UploadEncrypt) (spec types.UploadEncryptSpecific) {
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
			json.Unmarshal(target, &valueObject)
			spec.OneTimeKey.KeyObject[key] = valueObject
		}
	}
	return
}

func persistAl(encryptDB storage.Database, ctx context.Context, uid, device string, al []string) (err error) {
	err = encryptDB.InsertAl(ctx, uid, device, al)
	return
}

func takeAL(encryptDB storage.Database, ctx context.Context, uid, device string) (al []string, err error) {
	al, err = encryptDB.SelectAl(ctx, uid, device)
	return
}

func pickOne(encryptDB storage.Database, ctx context.Context, uid, device, al string) (key types.KeyHolder, err error) {
	key, err = encryptDB.SelectOneTimeKeySingle(ctx, uid, device, al)
	return
}

func upnotify(userID string) {
	m := sarama.ProducerMessage{
		Topic: "keyUpdate",
		Key:   sarama.StringEncoder("key"),
		Value: sarama.StringEncoder(userID),
	}
	keyProducer.ch.Input() <- &m
}

func InitNotifier(base *basecomponent.BaseDendrite) {
	keyProducer.base = base
	pro, _ := sarama.NewAsyncProducer(base.Cfg.Kafka.Addresses, nil)
	keyProducer.ch = pro
}
