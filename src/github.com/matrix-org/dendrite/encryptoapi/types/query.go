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

package types

/*
	{
	  "timeout": 10000,
	  "device_keys": {
		"@alice:example.com": ["DISYYYX","XYIISONM"]
	  }
	}
*/
type QueryRequest struct {
	Timeout    int64               `json:"timeout"`
	DeviceKeys map[string]interface{} `json:"device_keys"`
	Token      string              `json:"token"`
}

/*
	{
	  "failures": {},
	  "device_keys": {
		"@alice:example.com": {
		  "JLAFKJWSCS": {
			"user_id": "@alice:example.com",
			"device_id": "JLAFKJWSCS",
			"algorithms": [
			  "m.olm.curve25519-aes-sha256",
			  "m.megolm.v1.aes-sha"
			],
			"keys": {
			  "curve25519:JLAFKJWSCS": "3C5BFWi2Y8MaVvjM8M22DBmh24PmgR0nPvJOIArzgyI",
			  "ed25519:JLAFKJWSCS": "lEuiRJBit0IG6nUf5pUzWTUEsRVVe/HJkoKuEww9ULI"
			},
			"signatures": {
			  "@alice:example.com": {
				"ed25519:JLAFKJWSCS": "dSO80A01XiigH3uBiDVx/EjzaoycHcjq9lfQX0uWsqxl2giMIiSPR8a4d291W1ihKJL/a+myXS367WT6NAIcBA"
			  }
			},
			"unsigned": {
			  "device_display_name": "Alice's mobile phone"
			}
		  }
		}
	  }
	}
*/
type QueryResponse struct {
	Failure    map[string]interface{}                `json:"failures"`
	DeviceKeys map[string]map[string]DeviceKeysQuery `json:"device_keys"`
}

type DeviceKeysQuery struct {
	UserId    string                       `json:"user_id"`
	DeviceId  string                       `json:"device_id"`
	Algorithm []string                     `json:"algorithms"`
	Keys      map[string]string            `json:"keys"`
	Signature map[string]map[string]string `json:"signatures"`
	Unsigned  UnsignedDeviceInfo           `json:"unsigned"`
}

type UnsignedDeviceInfo struct {
	Info string `json:"device_display_name"`
}
