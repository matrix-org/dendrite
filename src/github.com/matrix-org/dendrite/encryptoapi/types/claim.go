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
	  "one_time_keys": {
		"@alice:example.com": {
		  "JLAFKJWSCS": "curve25519"
		}
	  }
	}
*/
type ClaimRequest struct {
	Timeout     int64                        `json:"timeout"`
	ClaimDetail map[string]map[string]string `json:"one_time_keys"`
}

/*
	{
	  "failures": {},
	  "one_time_keys": {
		"@alice:example.com": {
		  "JLAFKJWSCS": {
			"signed_curve25519:AAAAHg": {
			  "key": "zKbLg+NrIjpnagy+pIY6uPL4ZwEG2v+8F9lmgsnlZzs",
			  "signatures": {
				"@alice:example.com": {
				  "ed25519:JLAFKJWSCS": "FLWxXqGbwrb8SM3Y795eB6OA8bwBcoMZFXBqnTn58AYWZSqiD45tlBVcDa2L7RwdKXebW/VzDlnfVJ+9jok1Bw"
				}
			  }
			}
		  }
		}
	  }
	}
*/
type ClaimResponse struct {
	Failures map[string]interface{} `json:"failures"`
	ClaimBody map[string]map[string]map[string]interface{} `json:"one_time_keys"`
}
