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

package types

/*
	{
	  "device_keys": {
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
		}
	  },
	  "one_time_keys": {
		"curve25519:AAAAAQ": "/qyvZvwjiTxGdGU0RCguDCLeR+nmsb3FfNG3/Ve4vU8",
		"signed_curve25519:AAAAHg": {
		  "key": "zKbLg+NrIjpnagy+pIY6uPL4ZwEG2v+8F9lmgsnlZzs",
		  "signatures": {
			"@alice:example.com": {
			  "ed25519:JLAFKJWSCS": "FLWxXqGbwrb8SM3Y795eB6OA8bwBcoMZFXBqnTn58AYWZSqiD45tlBVcDa2L7RwdKXebW/VzDlnfVJ+9jok1Bw"
			}
		  }
		},
		"signed_curve25519:AAAAHQ": {
		  "key": "j3fR3HemM16M7CWhoI4Sk5ZsdmdfQHsKL1xuSft6MSw",
		  "signatures": {
			"@alice:example.com": {
			  "ed25519:JLAFKJWSCS": "IQeCEPb9HFk217cU9kw9EOiusC6kMIkoIRnbnfOh5Oc63S1ghgyjShBGpu34blQomoalCyXWyhaaT3MrLZYQAA"
			}
		  }
		}
	  }
	}
*/
type UploadEncrypt struct {
	DeviceKeys DeviceKeys             `json:"device_keys"`
	OneTimeKey map[string]interface{} `json:"one_time_keys"`
}
type UploadEncryptSpecific struct {
	DeviceKeys DeviceKeys         `json:"device_keys"`
	OneTimeKey OneTimeKeySpecific `json:"one_time_keys"`
}

/*
	{
	  "one_time_key_counts": {
		"curve25519": 10,
		"signed_curve25519": 20
	  }
	}
*/
type UploadResponse struct {
	Count map[string]int `json:"one_time_key_counts"`
}

/*
  "device_keys": {
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
    }
  }
*/
type DeviceKeys struct {
	UserId    string                       `json:"user_id"`
	DeviceId  string                       `json:"device_id"`
	Algorithm []string                     `json:"algorithms"`
	Keys      map[string]string            `json:"keys"`
	Signature map[string]map[string]string `json:"signatures"`
}

/*
    "signed_curve25519:AAAAHg": {
      "key": "zKbLg+NrIjpnagy+pIY6uPL4ZwEG2v+8F9lmgsnlZzs",
      "signatures": {
        "@alice:example.com": {
          "ed25519:JLAFKJWSCS": "FLWxXqGbwrb8SM3Y795eB6OA8bwBcoMZFXBqnTn58AYWZSqiD45tlBVcDa2L7RwdKXebW/VzDlnfVJ+9jok1Bw"
        }
      }
	}
*/
type KeyObject struct {
	Key       string                       `json:"key"`
	Signature map[string]map[string]string `json:"signatures"`
}

/*
    "one_time_keys": {
		"curve25519:AAAAAQ": "/qyvZvwjiTxGdGU0RCguDCLeR+nmsb3FfNG3/Ve4vU8",
		"signed_curve25519:AAAAHg": {
		  "key": "zKbLg+NrIjpnagy+pIY6uPL4ZwEG2v+8F9lmgsnlZzs",
		  "signatures": {
			"@alice:example.com": {
			  "ed25519:JLAFKJWSCS": "FLWxXqGbwrb8SM3Y795eB6OA8bwBcoMZFXBqnTn58AYWZSqiD45tlBVcDa2L7RwdKXebW/VzDlnfVJ+9jok1Bw"
			}
		  }
		},
		"signed_curve25519:AAAAHQ": {
		  "key": "j3fR3HemM16M7CWhoI4Sk5ZsdmdfQHsKL1xuSft6MSw",
		  "signatures": {
			"@alice:example.com": {
			  "ed25519:JLAFKJWSCS": "IQeCEPb9HFk217cU9kw9EOiusC6kMIkoIRnbnfOh5Oc63S1ghgyjShBGpu34blQomoalCyXWyhaaT3MrLZYQAA"
			}
		  }
		}
	}
*/
type OneTimeKey struct {
	//KeyString map[string]string
	//KeyObject map[string]KeyObject
	KeySth map[string]interface{}
}
type OneTimeKeySpecific struct {
	KeyString map[string]string
	KeyObject map[string]KeyObject
	//KeySth map[string]interface{}
}
