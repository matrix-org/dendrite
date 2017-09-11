/* Copyright 2016-2017 Vector Creations Ltd
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package gomatrixserverlib

import (
	"bytes"
	"encoding/base64"
	"testing"

	"golang.org/x/crypto/ed25519"
)

func TestVerifyEventSignatureTestVectors(t *testing.T) {
	// Check JSON verification using the test vectors from https://matrix.org/docs/spec/appendices.html
	seed, err := base64.RawStdEncoding.DecodeString("YJDBA9Xnr2sVqXD9Vj7XVUnmFZcZrlw8Md7kMW+3XA1")
	if err != nil {
		t.Fatal(err)
	}
	random := bytes.NewBuffer(seed)
	entityName := "domain"
	keyID := KeyID("ed25519:1")

	publicKey, _, err := ed25519.GenerateKey(random)
	if err != nil {
		t.Fatal(err)
	}

	testVerifyOK := func(input string) {
		err := verifyEventSignature(entityName, keyID, publicKey, []byte(input))
		if err != nil {
			t.Fatal(err)
		}
	}

	testVerifyNotOK := func(reason, input string) {
		err := verifyEventSignature(entityName, keyID, publicKey, []byte(input))
		if err == nil {
			t.Fatalf("Expected VerifyJSON to fail for input %v because %v", input, reason)
		}
	}

	testVerifyOK(`{
		"event_id": "$0:domain",
		"hashes": {
		    "sha256": "6tJjLpXtggfke8UxFhAKg82QVkJzvKOVOOSjUDK4ZSI"
   		},
   		"origin": "domain",
   		"origin_server_ts": 1000000,
   		"signatures": {
   		    "domain": {
   		        "ed25519:1": "2Wptgo4CwmLo/Y8B8qinxApKaCkBG2fjTWB7AbP5Uy+aIbygsSdLOFzvdDjww8zUVKCmI02eP9xtyJxc/cLiBA"
   		    }
   		},
   		"type": "X",
   		"unsigned": {
   		    "age_ts": 1000000
   		}
	}`)

	// It should still pass signature checks, even if we remove the unsigned data.
	testVerifyOK(`{
		"event_id": "$0:domain",
		"hashes": {
		    "sha256": "6tJjLpXtggfke8UxFhAKg82QVkJzvKOVOOSjUDK4ZSI"
   		},
   		"origin": "domain",
   		"origin_server_ts": 1000000,
   		"signatures": {
   		    "domain": {
   		        "ed25519:1": "2Wptgo4CwmLo/Y8B8qinxApKaCkBG2fjTWB7AbP5Uy+aIbygsSdLOFzvdDjww8zUVKCmI02eP9xtyJxc/cLiBA"
   		    }
   		},
   		"type": "X",
   		"unsigned": {}
	}`)

	testVerifyOK(`{
		"content": {
			"body": "Here is the message content"
		},
		"event_id": "$0:domain",
		"hashes": {
   	 	    "sha256": "onLKD1bGljeBWQhWZ1kaP9SorVmRQNdN5aM2JYU2n/g"
   	 	},
   	 	"origin": "domain",
   	 	"origin_server_ts": 1000000,
   	 	"type": "m.room.message",
   	 	"room_id": "!r:domain",
   	 	"sender": "@u:domain",
   	 	"signatures": {
   	 	    "domain": {
   	 	        "ed25519:1": "Wm+VzmOUOz08Ds+0NTWb1d4CZrVsJSikkeRxh6aCcUwu6pNC78FunoD7KNWzqFn241eYHYMGCA5McEiVPdhzBA"
   	 	    }
   	 	},
   	 	"unsigned": {
   	 	    "age_ts": 1000000
   	 	}
	}`)

	// It should still pass signature checks, even if we redact the content.
	testVerifyOK(`{
		"content": {},
		"event_id": "$0:domain",
		"hashes": {
   	 	    "sha256": "onLKD1bGljeBWQhWZ1kaP9SorVmRQNdN5aM2JYU2n/g"
   	 	},
   	 	"origin": "domain",
   	 	"origin_server_ts": 1000000,
   	 	"type": "m.room.message",
   	 	"room_id": "!r:domain",
   	 	"sender": "@u:domain",
   	 	"signatures": {
   	 	    "domain": {
   	 	        "ed25519:1": "Wm+VzmOUOz08Ds+0NTWb1d4CZrVsJSikkeRxh6aCcUwu6pNC78FunoD7KNWzqFn241eYHYMGCA5McEiVPdhzBA"
   	 	    }
   	 	},
   	 	"unsigned": {}
	}`)

	testVerifyNotOK("The event is modified", `{
		"event_id": "$0:domain",
		"hashes": {
		    "sha256": "6tJjLpXtggfke8UxFhAKg82QVkJzvKOVOOSjUDK4ZSI"
   		},
   		"origin": "domain",
   		"origin_server_ts": 1000000,
   		"signatures": {
   		    "domain": {
   		        "ed25519:1": "2Wptgo4CwmLo/Y8B8qinxApKaCkBG2fjTWB7AbP5Uy+aIbygsSdLOFzvdDjww8zUVKCmI02eP9xtyJxc/cLiBA"
   		    }
   		},
   		"type": "modified",
   		"unsigned": {}
	}`)

	testVerifyNotOK("The content hash is modified", `{
		"content": {},
		"event_id": "$0:domain",
		"hashes": {
   	 	    "sha256": "adifferenthashvalueaP9SorVmRQNdN5aM2JYU2n/g"
   	 	},
   	 	"origin": "domain",
   	 	"origin_server_ts": 1000000,
   	 	"type": "m.room.message",
   	 	"room_id": "!r:domain",
   	 	"sender": "@u:domain",
   	 	"signatures": {
   	 	    "domain": {
   	 	        "ed25519:1": "Wm+VzmOUOz08Ds+0NTWb1d4CZrVsJSikkeRxh6aCcUwu6pNC78FunoD7KNWzqFn241eYHYMGCA5McEiVPdhzBA"
   	 	    }
   	 	},
   	 	"unsigned": {}
	}`)
}

func TestSignEventTestVectors(t *testing.T) {
	// Check matrix event signing using the test vectors from https://matrix.org/docs/spec/appendices.html
	seed, err := base64.RawStdEncoding.DecodeString("YJDBA9Xnr2sVqXD9Vj7XVUnmFZcZrlw8Md7kMW+3XA1")
	if err != nil {
		t.Fatal(err)
	}
	random := bytes.NewBuffer(seed)
	entityName := "domain"
	keyID := KeyID("ed25519:1")

	_, privateKey, err := ed25519.GenerateKey(random)
	if err != nil {
		t.Fatal(err)
	}

	testSign := func(input string, want string) {
		hashed, err := addContentHashesToEvent([]byte(input))
		if err != nil {
			t.Fatal(err)
		}
		signed, err := signEvent(entityName, keyID, privateKey, hashed)
		if err != nil {
			t.Fatal(err)
		}

		if !IsJSONEqual([]byte(want), signed) {
			t.Fatalf("SignEvent(%q): want %v got %v", input, want, string(signed))
		}
	}

	testSign(`{
		"event_id": "$0:domain",
		"origin": "domain",
		"origin_server_ts": 1000000,
		"type": "X",
		"unsigned": {
			"age_ts": 1000000
		}
	}`, `{
		"event_id": "$0:domain",
		"hashes": {
		    "sha256": "6tJjLpXtggfke8UxFhAKg82QVkJzvKOVOOSjUDK4ZSI"
   		},
   		"origin": "domain",
   		"origin_server_ts": 1000000,
   		"signatures": {
   		    "domain": {
   		        "ed25519:1": "2Wptgo4CwmLo/Y8B8qinxApKaCkBG2fjTWB7AbP5Uy+aIbygsSdLOFzvdDjww8zUVKCmI02eP9xtyJxc/cLiBA"
   		    }
   		},
   		"type": "X",
   		"unsigned": {
   		    "age_ts": 1000000
   		}
	}`)

	testSign(`{
		"content": {
			"body": "Here is the message content"
		},
		"event_id": "$0:domain",
		"origin": "domain",
		"origin_server_ts": 1000000,
		"type": "m.room.message",
		"room_id": "!r:domain",
		"sender": "@u:domain",
		"unsigned": {
			"age_ts": 1000000
		}
	}`, `{
		"content": {
			"body": "Here is the message content"
		},
		"event_id": "$0:domain",
		"hashes": {
   	 	    "sha256": "onLKD1bGljeBWQhWZ1kaP9SorVmRQNdN5aM2JYU2n/g"
   	 	},
   	 	"origin": "domain",
   	 	"origin_server_ts": 1000000,
   	 	"type": "m.room.message",
   	 	"room_id": "!r:domain",
   	 	"sender": "@u:domain",
   	 	"signatures": {
   	 	    "domain": {
   	 	        "ed25519:1": "Wm+VzmOUOz08Ds+0NTWb1d4CZrVsJSikkeRxh6aCcUwu6pNC78FunoD7KNWzqFn241eYHYMGCA5McEiVPdhzBA"
   	 	    }
   	 	},
   	 	"unsigned": {
   	 	    "age_ts": 1000000
   	 	}
	}`)
}
