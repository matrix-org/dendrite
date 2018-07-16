// Copyright 2018 Vector Creations Ltd
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
	"github.com/Shopify/sarama"
	"github.com/matrix-org/dendrite/clientapi/auth/storage/devices"
	"github.com/matrix-org/dendrite/clientapi/httputil"
	"github.com/matrix-org/dendrite/common/basecomponent"
	"github.com/matrix-org/dendrite/encryptoapi/storage"
	"github.com/matrix-org/dendrite/encryptoapi/types"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
	"github.com/pkg/errors"
	"net/http"
	"strings"
	"time"
)

const (
	// TYPESUM sum type
	TYPESUM = iota
	// BODYDEVICEKEY device key body
	BODYDEVICEKEY
	// BODYONETIMEKEY one time key
	BODYONETIMEKEY
	// ONETIMEKEYSTRING key string
	ONETIMEKEYSTRING
	// ONETIMEKEYOBJECT key object
	ONETIMEKEYOBJECT
)

// ONETIMEKEYSTR stands for storage string property
const ONETIMEKEYSTR = "one_time_key"

// DEVICEKEYSTR stands for storage string property
const DEVICEKEYSTR = "device_key"

// KeyNotifier kafka notifier
type KeyNotifier struct {
	base *basecomponent.BaseDendrite
	ch   sarama.AsyncProducer
}

var keyProducer = &KeyNotifier{}

// UploadPKeys this function is for user upload his device key, and one-time-key to a limit at 50 set as default
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
	numMap := (QueryOneTimeKeys(
		req.Context(),
		TYPESUM,
		userID,
		deviceID,
		encryptionDB)).(map[string]int)
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

// QueryPKeys this function is for user query other's device key
func QueryPKeys(
	req *http.Request,
	encryptionDB *storage.Database,
	deviceID string,
	deviceDB *devices.Database,
) util.JSONResponse {
	var err error
	var queryRq types.QueryRequest
	queryRp := types.QueryResponse{}
	queryRp.Failure = make(map[string]interface{})
	queryRp.DeviceKeys = make(map[string]map[string]types.DeviceKeysQuery)
	if reqErr := httputil.UnmarshalJSONRequest(req, &queryRq); reqErr != nil {
		return *reqErr
	}

	/*
		federation consideration: when user id is in federation, a
		query is needed to ask fed for keys.
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
		case <-make(chan interface{}):
			// todo : here goes federation chan , still a mocked one
		}
	}

	// query one's device key from user corresponding to uid
	for uid, arr := range queryRq.DeviceKeys {
		queryRp.DeviceKeys[uid] = make(map[string]types.DeviceKeysQuery)
		deviceKeysQueryMap := queryRp.DeviceKeys[uid]
		// backward compatible to old interface
		midArr := []string{}
		// figure out device list from devices described as device which is actually deviceID
		for device := range arr.(map[string]interface{}) {
			midArr = append(midArr, device)
		}
		// all device keys
		dkeys, _ := encryptionDB.QueryInRange(req.Context(), uid, midArr)
		// build response for them

		for _, key := range dkeys {

			deviceKeysQueryMap = presetDeviceKeysQueryMap(deviceKeysQueryMap, uid, key)
			// load for accomplishment
			single := deviceKeysQueryMap[key.DeviceID]
			resKey := fmt.Sprintf("%s:%s", key.KeyAlgorithm, key.DeviceID)
			resBody := key.Key
			single.Keys[resKey] = resBody
			single.DeviceID = key.DeviceID
			single.UserID = key.UserID
			single.Signature[uid][fmt.Sprintf("%s:%s", "ed25519", key.DeviceID)] = key.Signature
			single.Algorithm, err = takeAL(req.Context(), *encryptionDB, key.UserID, key.DeviceID)
			localpart, _, _ := gomatrixserverlib.SplitID('@', uid)
			device, _ := deviceDB.GetDeviceByID(req.Context(), localpart, deviceID)
			single.Unsigned.Info = device.DisplayName
			deviceKeysQueryMap[key.DeviceID] = single
		}
	}
	if err != nil {
		return util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: queryRp,
		}
	}
	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: queryRp,
	}
}

// ClaimOneTimeKeys claim for one time key that may be used in session exchange in olm encryption
func ClaimOneTimeKeys(
	req *http.Request,
	encryptionDB *storage.Database,
) util.JSONResponse {
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
		case <-make(chan interface{}):
			// todo : here goes federation chan , still a mocked one
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
			key, err := pickOne(req.Context(), *encryptionDB, uid, deviceID, al)
			if err != nil {
				claimRp.Failures[uid] = fmt.Sprintf("%s:%s", "fail to get keys for device ", deviceID)
			}
			claimRp.ClaimBody[uid] = make(map[string]map[string]interface{})
			keyPreMap := claimRp.ClaimBody[uid]
			keymap := keyPreMap[deviceID]
			if keymap == nil {
				keymap = make(map[string]interface{})
			}
			switch alTyp {
			case ONETIMEKEYSTRING:
				keymap[fmt.Sprintf("%s:%s", al, key.KeyID)] = key.Key
			case ONETIMEKEYOBJECT:
				sig := make(map[string]map[string]string)
				sig[uid] = make(map[string]string)
				sig[uid][fmt.Sprintf("%s:%s", "ed25519", deviceID)] = key.Signature
				keymap[fmt.Sprintf("%s:%s", al, key.KeyID)] = types.KeyObject{Key: key.Key, Signature: sig}
			}
			claimRp.ClaimBody[uid][deviceID] = keymap
		}
	}
	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: claimRp,
	}
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

// QueryOneTimeKeys todo: complete this field through claim type
func QueryOneTimeKeys(
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

// ClearUnused when web client sign out, a clean should be processed, cause all keys would never been used from then on.
// todo: complete this function and invoke through sign out extension or some scenarios else those matter
func ClearUnused() {}

// persist both device keys and one time keys
func persistKeys(
	ctx context.Context,
	database *storage.Database,
	body *types.UploadEncryptSpecific,
	userID,
	deviceID string,
) (err error) {
	// in order to persist keys , a check filtering duplicate should be processed
	// true stands for counterparts are in request
	// situation 1: only device keys
	// situation 2: both device keys and one time keys
	// situation 3: only one time keys
	if checkUpload(body, BODYDEVICEKEY) {
		deviceKeys := body.DeviceKeys
		al := deviceKeys.Algorithm
		err = persistAl(ctx, *database, userID, deviceID, al)
		if err != nil {
			return
		}
		if checkUpload(body, BODYONETIMEKEY) {
			if err = bothKeyProcess(ctx, body, userID, deviceID, database, deviceKeys); err != nil {
				return
			}
		} else {
			if err = dkeyProcess(ctx, userID, deviceID, database, deviceKeys); err != nil {
				return
			}
		}
		// notifier to sync server
		upnotify(userID)
	} else {
		if checkUpload(body, BODYONETIMEKEY) {
			if err = otmKeyProcess(ctx, body, userID, deviceID, database); err != nil {
				return
			}
		} else {
			return errors.New("failed to touch keys")
		}
	}
	return err
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

func persistAl(
	ctx context.Context,
	encryptDB storage.Database,
	uid, device string,
	al []string,
) (err error) {
	err = encryptDB.InsertAl(ctx, uid, device, al)
	return
}

func takeAL(
	ctx context.Context,
	encryptDB storage.Database,
	uid, device string,
) (al []string, err error) {
	al, err = encryptDB.SelectAl(ctx, uid, device)
	return
}

func pickOne(
	ctx context.Context,
	encryptDB storage.Database,
	uid, device, al string,
) (key types.KeyHolder, err error) {
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

// InitNotifier initialize kafka notifier
func InitNotifier(base *basecomponent.BaseDendrite) {
	keyProducer.base = base
	pro, _ := sarama.NewAsyncProducer(base.Cfg.Kafka.Addresses, nil)
	keyProducer.ch = pro
}

func presetDeviceKeysQueryMap(
	deviceKeysQueryMap map[string]types.DeviceKeysQuery,
	uid string,
	key types.KeyHolder,
) map[string]types.DeviceKeysQuery {
	// preset for complicated nested map struct
	if _, ok := deviceKeysQueryMap[key.DeviceID]; !ok {
		// make consistency
		deviceKeysQueryMap[key.DeviceID] = types.DeviceKeysQuery{}
	}
	if deviceKeysQueryMap[key.DeviceID].Signature == nil {
		mid := make(map[string]map[string]string)
		midmap := deviceKeysQueryMap[key.DeviceID]
		midmap.Signature = mid
		deviceKeysQueryMap[key.DeviceID] = midmap
	}
	if deviceKeysQueryMap[key.DeviceID].Keys == nil {
		mid := make(map[string]string)
		midmap := deviceKeysQueryMap[key.DeviceID]
		midmap.Keys = mid
		deviceKeysQueryMap[key.DeviceID] = midmap
	}
	if _, ok := deviceKeysQueryMap[key.DeviceID].Signature[uid]; !ok {
		// make consistency
		deviceKeysQueryMap[key.DeviceID].Signature[uid] = make(map[string]string)
	}
	return deviceKeysQueryMap
}

func bothKeyProcess(
	ctx context.Context,
	body *types.UploadEncryptSpecific,
	userID, deviceID string,
	database *storage.Database,
	deviceKeys types.DeviceKeys,
) (err error) {
	// insert one time keys firstly
	onetimeKeys := body.OneTimeKey
	for alKeyID, val := range onetimeKeys.KeyString {
		al := (strings.Split(alKeyID, ":"))[0]
		keyID := (strings.Split(alKeyID, ":"))[1]
		keyInfo := val
		keyStringTyp := ONETIMEKEYSTR
		sig := ""
		err = database.InsertKey(ctx, deviceID, userID, keyID, keyStringTyp, keyInfo, al, sig)
		if err != nil {
			return
		}
	}
	for alKeyID, val := range onetimeKeys.KeyObject {
		al := (strings.Split(alKeyID, ":"))[0]
		keyID := (strings.Split(alKeyID, ":"))[1]
		keyInfo := val.Key
		keyObjectTyp := ONETIMEKEYSTR
		sig := val.Signature[userID][fmt.Sprintf("%s:%s", "ed25519", deviceID)]
		err = database.InsertKey(ctx, deviceID, userID, keyID, keyObjectTyp, keyInfo, al, sig)
		if err != nil {
			return
		}
	}
	// insert device keys
	keys := deviceKeys.Keys
	sigs := deviceKeys.Signature
	for alDevice, key := range keys {
		al := (strings.Split(alDevice, ":"))[0]
		keyTyp := DEVICEKEYSTR
		keyInfo := key
		keyID := ""
		sig := sigs[userID][fmt.Sprintf("%s:%s", "ed25519", deviceID)]
		err = database.InsertKey(
			ctx, deviceID, userID, keyID, keyTyp, keyInfo, al, sig)
		if err != nil {
			return
		}
	}
	return
}

func dkeyProcess(
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

func otmKeyProcess(
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
