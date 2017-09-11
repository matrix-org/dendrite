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
	"encoding/json"
	"testing"
)

func stateNeededEquals(a, b StateNeeded) bool {
	if a.Create != b.Create {
		return false
	}
	if a.JoinRules != b.JoinRules {
		return false
	}
	if a.PowerLevels != b.PowerLevels {
		return false
	}
	if len(a.Member) != len(b.Member) {
		return false
	}
	if len(a.ThirdPartyInvite) != len(b.ThirdPartyInvite) {
		return false
	}
	for i := range a.Member {
		if a.Member[i] != b.Member[i] {
			return false
		}
	}
	for i := range a.ThirdPartyInvite {
		if a.ThirdPartyInvite[i] != b.ThirdPartyInvite[i] {
			return false
		}
	}
	return true
}

type testEventList []Event

func (tel *testEventList) UnmarshalJSON(data []byte) error {
	var eventJSONs []rawJSON
	var events []Event
	if err := json.Unmarshal([]byte(data), &eventJSONs); err != nil {
		return err
	}
	for _, eventJSON := range eventJSONs {
		event, err := NewEventFromTrustedJSON([]byte(eventJSON), false)
		if err != nil {
			return err
		}
		events = append(events, event)
	}
	*tel = testEventList(events)
	return nil
}

func testStateNeededForAuth(t *testing.T, eventdata string, builder *EventBuilder, want StateNeeded) {
	var events testEventList
	if err := json.Unmarshal([]byte(eventdata), &events); err != nil {
		panic(err)
	}
	got := StateNeededForAuth(events)
	if !stateNeededEquals(got, want) {
		t.Errorf("Testing StateNeededForAuth(%#v), wanted %#v got %#v", events, want, got)
	}
	if builder != nil {
		got, err := StateNeededForEventBuilder(builder)
		if !stateNeededEquals(got, want) {
			t.Errorf("Testing StateNeededForEventBuilder(%#v), wanted %#v got %#v", events, want, got)
		}
		if err != nil {
			panic(err)
		}
	}
}

func TestStateNeededForCreate(t *testing.T) {
	// Create events don't need anything.
	skey := ""
	testStateNeededForAuth(t, `[{"type": "m.room.create"}]`, &EventBuilder{
		Type:     "m.room.create",
		StateKey: &skey,
	}, StateNeeded{})
}

func TestStateNeededForMessage(t *testing.T) {
	// Message events need the create event, the sender and the power_levels.
	testStateNeededForAuth(t, `[{
		"type": "m.room.message",
		"sender": "@u1:a"
	}]`, &EventBuilder{
		Type:   "m.room.message",
		Sender: "@u1:a",
	}, StateNeeded{
		Create:      true,
		PowerLevels: true,
		Member:      []string{"@u1:a"},
	})
}

func TestStateNeededForAlias(t *testing.T) {
	// Alias events need only the create event.
	testStateNeededForAuth(t, `[{"type": "m.room.aliases"}]`, &EventBuilder{
		Type: "m.room.aliases",
	}, StateNeeded{
		Create: true,
	})
}

func TestStateNeededForJoin(t *testing.T) {
	skey := "@u1:a"
	b := EventBuilder{
		Type:     "m.room.member",
		StateKey: &skey,
		Sender:   "@u1:a",
	}
	if err := b.SetContent(memberContent{"join", nil}); err != nil {
		t.Fatal(err)
	}
	testStateNeededForAuth(t, `[{
		"type": "m.room.member",
		"state_key": "@u1:a",
		"sender": "@u1:a",
		"content": {"membership": "join"}
	}]`, &b, StateNeeded{
		Create:      true,
		JoinRules:   true,
		PowerLevels: true,
		Member:      []string{"@u1:a"},
	})
}

func TestStateNeededForInvite(t *testing.T) {
	skey := "@u2:b"
	b := EventBuilder{
		Type:     "m.room.member",
		StateKey: &skey,
		Sender:   "@u1:a",
	}
	if err := b.SetContent(memberContent{"invite", nil}); err != nil {
		t.Fatal(err)
	}
	testStateNeededForAuth(t, `[{
		"type": "m.room.member",
		"state_key": "@u2:b",
		"sender": "@u1:a",
		"content": {"membership": "invite"}
	}]`, &b, StateNeeded{
		Create:      true,
		PowerLevels: true,
		Member:      []string{"@u1:a", "@u2:b"},
	})
}

func TestStateNeededForInvite3PID(t *testing.T) {
	skey := "@u2:b"
	b := EventBuilder{
		Type:     "m.room.member",
		StateKey: &skey,
		Sender:   "@u1:a",
	}

	if err := b.SetContent(memberContent{"invite", &memberThirdPartyInvite{
		Signed: memberThirdPartyInviteSigned{
			Token: "my_token",
		},
	}}); err != nil {
		t.Fatal(err)
	}
	testStateNeededForAuth(t, `[{
		"type": "m.room.member",
		"state_key": "@u2:b",
		"sender": "@u1:a",
		"content": {
			"membership": "invite",
			"third_party_invite": {
				"signed": {
					"token": "my_token"
				}
			}
		}
	}]`, &b, StateNeeded{
		Create:           true,
		PowerLevels:      true,
		Member:           []string{"@u1:a", "@u2:b"},
		ThirdPartyInvite: []string{"my_token"},
	})
}

type testAuthEvents struct {
	CreateJSON           json.RawMessage            `json:"create"`
	JoinRulesJSON        json.RawMessage            `json:"join_rules"`
	PowerLevelsJSON      json.RawMessage            `json:"power_levels"`
	MemberJSON           map[string]json.RawMessage `json:"member"`
	ThirdPartyInviteJSON map[string]json.RawMessage `json:"third_party_invite"`
}

func (tae *testAuthEvents) Create() (*Event, error) {
	if len(tae.CreateJSON) == 0 {
		return nil, nil
	}
	var event Event
	event, err := NewEventFromTrustedJSON(tae.CreateJSON, false)
	if err != nil {
		return nil, err
	}
	return &event, nil
}

func (tae *testAuthEvents) JoinRules() (*Event, error) {
	if len(tae.JoinRulesJSON) == 0 {
		return nil, nil
	}
	event, err := NewEventFromTrustedJSON(tae.JoinRulesJSON, false)
	if err != nil {
		return nil, err
	}
	return &event, nil
}

func (tae *testAuthEvents) PowerLevels() (*Event, error) {
	if len(tae.PowerLevelsJSON) == 0 {
		return nil, nil
	}
	event, err := NewEventFromTrustedJSON(tae.PowerLevelsJSON, false)
	if err != nil {
		return nil, err
	}
	return &event, nil
}

func (tae *testAuthEvents) Member(stateKey string) (*Event, error) {
	if len(tae.MemberJSON[stateKey]) == 0 {
		return nil, nil
	}
	event, err := NewEventFromTrustedJSON(tae.MemberJSON[stateKey], false)
	if err != nil {
		return nil, err
	}
	return &event, nil
}

func (tae *testAuthEvents) ThirdPartyInvite(stateKey string) (*Event, error) {
	if len(tae.ThirdPartyInviteJSON[stateKey]) == 0 {
		return nil, nil
	}
	event, err := NewEventFromTrustedJSON(tae.ThirdPartyInviteJSON[stateKey], false)
	if err != nil {
		return nil, err
	}
	return &event, nil
}

type testCase struct {
	AuthEvents testAuthEvents    `json:"auth_events"`
	Allowed    []json.RawMessage `json:"allowed"`
	NotAllowed []json.RawMessage `json:"not_allowed"`
}

func testEventAllowed(t *testing.T, testCaseJSON string) {
	var tc testCase
	if err := json.Unmarshal([]byte(testCaseJSON), &tc); err != nil {
		panic(err)
	}
	for _, data := range tc.Allowed {
		event, err := NewEventFromTrustedJSON(data, false)
		if err != nil {
			panic(err)
		}
		if err = Allowed(event, &tc.AuthEvents); err != nil {
			t.Fatalf("Expected %q to be allowed but it was not: %q", string(data), err)
		}
	}
	for _, data := range tc.NotAllowed {
		event, err := NewEventFromTrustedJSON(data, false)
		if err != nil {
			panic(err)
		}
		if err := Allowed(event, &tc.AuthEvents); err == nil {
			t.Fatalf("Expected %q to not be allowed but it was", string(data))
		}
	}
}

func TestAllowedEmptyRoom(t *testing.T) {
	// Test that only m.room.create events can be sent without auth events.
	// TODO: Test the events that aren't m.room.create
	testEventAllowed(t, `{
		"auth_events": {},
		"allowed": [{
			"type": "m.room.create",
			"state_key": "",
			"sender": "@u1:a",
			"room_id": "!r1:a",
			"event_id": "$e1:a",
			"content": {"creator": "@u1:a"}
		}],
		"not_allowed": [{
			"type": "m.room.create",
			"state_key": "",
			"sender": "@u1:b",
			"room_id": "!r1:a",
			"event_id": "$e2:a",
			"content": {"creator": "@u1:b"},
			"unsigned": {
				"not_allowed": "Sent by a different server than the one which made the room_id"
			}
		}, {
			"type": "m.room.create",
			"state_key": "",
			"sender": "@u1:a",
			"room_id": "!r1:a",
			"event_id": "$e3:a",
			"prev_events": [["$e1", {}]],
			"content": {"creator": "@u1:a"},
			"unsigned": {
				"not_allowed": "Was not the first event in the room"
			}
		}, {
			"type": "m.room.message",
			"sender": "@u1:a",
			"room_id": "!r1:a",
			"event_id": "$e4:a",
			"content": {"body": "Test"},
			"unsigned": {
				"not_allowed": "No create event"
			}
		}, {
			"type": "m.room.member",
			"state_key": "@u1:a",
			"sender": "@u1:a",
			"room_id": "!r1:a",
			"event_id": "$e4:a",
			"content": {"membership": "join"},
			"unsigned": {
				"not_allowed": "No create event"
			}
		}, {
			"type": "m.room.create",
			"state_key": "",
			"sender": "not_a_user_id",
			"room_id": "!r1:a",
			"event_id": "$e5:a",
			"content": {"creator": "@u1:a"},
			"unsigned": {
				"not_allowed": "Sender is not a valid user ID"
			}
		}, {
			"type": "m.room.create",
			"state_key": "",
			"sender": "@u1:a",
			"room_id": "not_a_room_id",
			"event_id": "$e6:a",
			"content": {"creator": "@u1:a"},
			"unsigned": {
				"not_allowed": "Room is not a valid room ID"
			}
		}, {
			"type": "m.room.create",
			"sender": "@u1:a",
			"room_id": "!r1:a",
			"event_id": "$e7:a",
			"content": {"creator": "@u1:a"},
			"unsigned": {
				"not_allowed": "Missing state_key"
			}
		}, {
			"type": "m.room.create",
			"state_key": "not_empty",
			"sender": "@u1:a",
			"room_id": "!r1:a",
			"event_id": "$e7:a",
			"content": {"creator": "@u1:a"},
			"unsigned": {
				"not_allowed": "The state_key is not empty"
			}
		}]
	}`)
}

func TestAllowedFirstJoin(t *testing.T) {
	testEventAllowed(t, `{
		"auth_events": {
			"create": {
				"type": "m.room.create",
				"state_key": "",
				"sender": "@u1:a",
				"room_id": "!r1:a",
				"event_id": "$e1:a",
				"content": {"creator": "@u1:a"}
			}
		},
		"allowed": [{
			"type": "m.room.member",
			"state_key": "@u1:a",
			"sender": "@u1:a",
			"room_id": "!r1:a",
			"event_id": "$e2:a",
			"prev_events": [["$e1:a", {}]],
			"content": {"membership": "join"}
		}],
		"not_allowed": [{
			"type": "m.room.message",
			"sender": "@u1:a",
			"room_id": "!r1:a",
			"event_id": "$e3:a",
			"content": {"body": "test"},
			"unsigned": {
				"not_allowed": "Sender is not in the room"
			}
		}, {
			"type": "m.room.member",
			"state_key": "@u2:a",
			"sender": "@u2:a",
			"room_id": "!r1:a",
			"event_id": "$e4:a",
			"prev_events": [["$e1:a", {}]],
			"content": {"membership": "join"},
			"unsigned": {
				"not_allowed": "Only the creator can join the room"
			}
		}, {
			"type": "m.room.member",
			"sender": "@u1:a",
			"room_id": "!r1:a",
			"event_id": "$e4:a",
			"prev_events": [["$e1:a", {}]],
			"content": {"membership": "join"},
			"unsigned": {
				"not_allowed": "Missing state_key"
			}
		}, {
			"type": "m.room.member",
			"state_key": "@u1:a",
			"sender": "@u1:a",
			"room_id": "!r1:a",
			"event_id": "$e4:a",
			"prev_events": [["$e2:a", {}]],
			"content": {"membership": "join"},
			"unsigned": {
				"not_allowed": "The prev_event is not the create event"
			}
		}, {
			"type": "m.room.member",
			"state_key": "@u1:a",
			"sender": "@u1:a",
			"room_id": "!r1:a",
			"event_id": "$e4:a",
			"content": {"membership": "join"},
			"unsigned": {
				"not_allowed": "There are no prev_events"
			}
		}, {
			"type": "m.room.member",
			"state_key": "@u1:a",
			"sender": "@u1:a",
			"room_id": "!r1:a",
			"event_id": "$e4:a",
			"content": {"membership": "join"},
			"prev_events": [["$e1:a", {}], ["$e2:a", {}]],
			"unsigned": {
				"not_allowed": "There are too many prev_events"
			}
		}, {
			"type": "m.room.member",
			"state_key": "@u1:a",
			"sender": "@u2:a",
			"room_id": "!r1:a",
			"event_id": "$e4:a",
			"content": {"membership": "join"},
			"prev_events": [["$e1:a", {}]],
			"unsigned": {
				"not_allowed": "The sender doesn't match the joining user"
			}
		}, {
			"type": "m.room.member",
			"state_key": "@u1:a",
			"sender": "@u1:a",
			"room_id": "!r1:a",
			"event_id": "$e4:a",
			"content": {"membership": "invite"},
			"prev_events": [["$e1:a", {}]],
			"unsigned": {
				"not_allowed": "The membership is not 'join'"
			}
		}]
	}`)
}

func TestAllowedWithNoPowerLevels(t *testing.T) {
	testEventAllowed(t, `{
		"auth_events": {
			"create": {
				"type": "m.room.create",
				"state_key": "",
				"sender": "@u1:a",
				"room_id": "!r1:a",
				"event_id": "$e1:a",
				"content": {"creator": "@u1:a"}
			},
			"member": {
				"@u1:a": {
					"type": "m.room.member",
					"sender": "@u1:a",
					"room_id": "!r1:a",
					"state_key": "@u1:a",
					"event_id": "$e2:a",
					"content": {"membership": "join"}
				}
			}
		},
		"allowed": [{
			"type": "m.room.message",
			"sender": "@u1:a",
			"room_id": "!r1:a",
			"event_id": "$e3:a",
			"content": {"body": "Test"}
		}],
		"not_allowed": [{
			"type": "m.room.message",
			"sender": "@u2:a",
			"room_id": "!r1:a",
			"event_id": "$e4:a",
			"content": {"body": "Test"},
			"unsigned": {
				"not_allowed": "Sender is not in room"
			}
		}]
	}`)
}

func TestAllowedInviteFrom3PID(t *testing.T) {
	testEventAllowed(t, `{
		"auth_events": {
			"create": {
				"type": "m.room.create",
				"state_key": "",
				"sender": "@u1:a",
				"room_id": "!r1:a",
				"event_id": "$e1:a",
				"content": {"creator": "@u1:a"}
			},
			"member": {
				"@u1:a": {
					"type": "m.room.member",
					"sender": "@u1:a",
					"room_id": "!r1:a",
					"state_key": "@u1:a",
					"event_id": "$e2:a",
					"content": {"membership": "join"}
				}
			},
			"third_party_invite": {
				"my_token": {
					"type": "m.room.third_party_invite",
					"sender": "@u1:a",
					"room_id": "!r1:a",
					"state_key": "my_token",
					"event_id": "$e3:a",
					"content": {
						"display_name": "foo...@bar...",
						"public_key": "pubkey",
						"key_validity_url": "https://example.tld/isvalid",
						"public_keys": [
							{
								"public_key": "mrV51jApZKahGjfMhlevp+QtSSTDKCLaLVCzYc4HELY",
								"key_validity_url": "https://example.tld/isvalid"
							}
						]
					}
				}
			}
		},
		"allowed": [{
			"type": "m.room.member",
			"sender": "@u1:a",
			"room_id": "!r1:a",
			"state_key": "@u2:a",
			"event_id": "$e4:a",
			"content": {
				"membership": "invite",
				"third_party_invite": {
					"display_name": "foo...@bar...",
					"signed": {
						"token": "my_token",
						"mxid": "@u2:a",
						"signatures": {
							"example.tld": {
								"ed25519:0": "CibGFS0vX93quJFppsQbYQKJFIwxiYEK87lNmekS/fdetUMXPdR2wwNDd09J1jJ28GCH3GogUTuFDB1ScPFxBg"
							}
						}
					}
				}
			}
		}],
		"not_allowed": [{
			"type": "m.room.member",
			"sender": "@u1:a",
			"room_id": "!r1:a",
			"state_key": "@u2:a",
			"event_id": "$e4:a",
			"content": {
				"membership": "invite",
				"third_party_invite": {
					"display_name": "foo...@bar...",
					"signed": {
						"token": "my_token",
						"mxid": "@u2:a",
						"signatures": {
							"example.tld": {
								"ed25519:0": "some_signature"
							}
						}
					}
				}
			},
			"unsigned": {
				"not_allowed": "Bad signature"
			}
		}, {
			"type": "m.room.member",
			"sender": "@u1:a",
			"room_id": "!r1:a",
			"state_key": "@u2:a",
			"event_id": "$e5:a",
			"content": {
				"membership": "invite",
				"third_party_invite": {
					"display_name": "foo...@bar...",
					"signed": {
						"token": "my_token",
						"mxid": "@u3:a",
						"signatures": {
							"example.tld": {
								"ed25519:0": "CibGFS0vX93quJFppsQbYQKJFIwxiYEK87lNmekS/fdetUMXPdR2wwNDd09J1jJ28GCH3GogUTuFDB1ScPFxBg"
							}
						}
					}
				}
			},
			"unsigned": {
				"not_allowed": "MXID doesn't match state key"
			}
		}, {
			"type": "m.room.member",
			"sender": "@u1:a",
			"room_id": "!r1:a",
			"state_key": "@u2:a",
			"event_id": "$e6:a",
			"content": {
				"membership": "invite",
				"third_party_invite": {
					"display_name": "foo...@bar...",
					"signed": {
						"token": "my_other_token",
						"mxid": "@u2:a",
						"signatures": {
							"example.tld": {
								"ed25519:0": "CibGFS0vX93quJFppsQbYQKJFIwxiYEK87lNmekS/fdetUMXPdR2wwNDd09J1jJ28GCH3GogUTuFDB1ScPFxBg"
							}
						}
					}
				}
			},
			"unsigned": {
				"not_allowed": "Token doesn't refer to a known third-party invite"
			}
		}]
	}`)
}

func TestAllowedNoFederation(t *testing.T) {
	testEventAllowed(t, `{
		"auth_events": {
			"create": {
				"type": "m.room.create",
				"sender": "@u1:a",
				"room_id": "!r1:a",
				"event_id": "$e1:a",
				"content": {
					"creator": "@u1:a",
					"m.federate": false
				}
			},
			"member": {
				"@u1:a": {
					"type": "m.room.member",
					"sender": "@u1:a",
					"room_id": "!r1:a",
					"state_key": "@u1:a",
					"event_id": "$e2:a",
					"content": {"membership": "join"}
				}
			}
		},
		"allowed": [{
			"type": "m.room.message",
			"sender": "@u1:a",
			"room_id": "!r1:a",
			"event_id": "$e3:a",
			"content": {"body": "Test"}
		}],
		"not_allowed": [{
			"type": "m.room.message",
			"sender": "@u1:b",
			"room_id": "!r1:a",
			"event_id": "$e4:a",
			"content": {"body": "Test"},
			"unsigned": {
				"not_allowed": "Sender is from a different server."
			}
		}]
	}`)
}

func TestAllowedWithPowerLevels(t *testing.T) {
	testEventAllowed(t, `{
		"auth_events": {
			"create": {
				"type": "m.room.create",
				"sender": "@u1:a",
				"room_id": "!r1:a",
				"event_id": "$e1:a",
				"content": {"creator": "@u1:a"}
			},
			"member": {
				"@u1:a": {
					"type": "m.room.member",
					"sender": "@u1:a",
					"room_id": "!r1:a",
					"state_key": "@u1:a",
					"event_id": "$e2:a",
					"content": {"membership": "join"}
				},
				"@u2:a": {
					"type": "m.room.member",
					"sender": "@u2:a",
					"room_id": "!r1:a",
					"state_key": "@u2:a",
					"event_id": "$e3:a",
					"content": {"membership": "join"}
				},
				"@u3:b": {
					"type": "m.room.member",
					"sender": "@u3:b",
					"room_id": "!r1:a",
					"state_key": "@u3:b",
					"event_id": "$e4:a",
					"content": {"membership": "join"}
				}
			},
			"power_levels": {
				"type": "m.room.power_levels",
				"sender": "@u1:a",
				"room_id": "!r1:a",
				"event_id": "$e5:a",
				"content": {
					"users": {
						"@u1:a": 100,
						"@u2:a": 50
					},
					"users_default": 0,
					"events": {
						"m.room.join_rules": 100
					},
					"state_default": 50,
					"events_default": 0
				}
			}
		},
		"allowed": [{
			"type": "m.room.message",
			"sender": "@u1:a",
			"room_id": "!r1:a",
			"event_id": "$e6:a",
			"content": {"body": "Test from @u1:a"}
		}, {
			"type": "m.room.message",
			"sender": "@u2:a",
			"room_id": "!r1:a",
			"event_id": "$e7:a",
			"content": {"body": "Test from @u2:a"}
		}, {
			"type": "m.room.message",
			"sender": "@u3:b",
			"room_id": "!r1:a",
			"event_id": "$e8:a",
			"content": {"body": "Test from @u3:b"}
		},{
			"type": "m.room.name",
			"state_key": "",
			"sender": "@u1:a",
			"room_id": "!r1:a",
			"event_id": "$e9:a",
			"content": {"name": "Name set by @u1:a"}
		}, {
			"type": "m.room.name",
			"state_key": "",
			"sender": "@u2:a",
			"room_id": "!r1:a",
			"event_id": "$e10:a",
			"content": {"name": "Name set by @u2:a"}
		}, {
			"type": "m.room.join_rules",
			"state_key": "",
			"sender": "@u1:a",
			"room_id": "!r1:a",
			"event_id": "$e11:a",
			"content": {"join_rule": "public"}
		}, {
			"type": "my.custom.state",
			"state_key": "@u2:a",
			"sender": "@u2:a",
			"room_id": "!r1:a",
			"event_id": "@e12:a",
			"content": {}
		}],
		"not_allowed": [{
			"type": "m.room.name",
			"state_key": "",
			"sender": "@u3:b",
			"room_id": "!r1:a",
			"event_id": "$e13:a",
			"content": {"name": "Name set by @u3:b"},
			"unsigned": {
				"not_allowed": "User @u3:b's level is too low to send a state event"
			}
		}, {
			"type": "m.room.join_rules",
			"sender": "@u2:a",
			"room_id": "!r1:a",
			"event_id": "$e14:a",
			"content": {"name": "Name set by @u3:b"},
			"unsigned": {
				"not_allowed": "User @u2:a's level is too low to send m.room.join_rules"
			}
		}, {
			"type": "m.room.message",
			"sender": "@u4:a",
			"room_id": "!r1:a",
			"event_id": "$e15:a",
			"content": {"Body": "Test from @u4:a"},
			"unsigned": {
				"not_allowed": "User @u4:a is not in the room"
			}
		}, {
			"type": "m.room.message",
			"sender": "@u1:a",
			"room_id": "!r2:a",
			"event_id": "$e16:a",
			"content": {"body": "Test from @u4:a"},
			"unsigned": {
				"not_allowed": "Sent from a different room to the create event"
			}
		}, {
			"type": "my.custom.state",
			"state_key": "@u2:a",
			"sender": "@u1:a",
			"room_id": "!r1:a",
			"event_id": "@e17:a",
			"content": {},
			"unsigned": {
				"not_allowed": "State key starts with '@' and is for a different user"
			}
		}]
	}`)
}

func TestRedactAllowed(t *testing.T) {
	// Test if redacts are allowed correctly in a room with a power level event.
	testEventAllowed(t, `{
		"auth_events": {
			"create": {
				"type": "m.room.create",
				"sender": "@u1:a",
				"room_id": "!r1:a",
				"event_id": "$e1:a",
				"content": {"creator": "@u1:a"}
			},
			"member": {
				"@u1:a": {
					"type": "m.room.member",
					"sender": "@u1:a",
					"room_id": "!r1:a",
					"state_key": "@u1:a",
					"event_id": "$e2:a",
					"content": {"membership": "join"}
				},
				"@u2:a": {
					"type": "m.room.member",
					"sender": "@u2:a",
					"room_id": "!r1:a",
					"state_key": "@u2:a",
					"event_id": "$e3:a",
					"content": {"membership": "join"}
				},
				"@u1:b": {
					"type": "m.room.member",
					"sender": "@u1:b",
					"room_id": "!r1:a",
					"state_key": "@u1:b",
					"event_id": "$e4:a",
					"content": {"membership": "join"}
				}
			},
			"power_levels": {
				"type": "m.room.power_levels",
				"state_key": "",
				"sender": "@u1:a",
				"room_id": "!r1:a",
				"event_id": "$e5:a",
				"content": {
					"users": {
						"@u1:a": 100
					},
					"redact": 100
				}
			}
		},
		"allowed": [{
			"type": "m.room.redaction",
			"sender": "@u1:b",
			"room_id": "!r1:a",
			"redacts": "$event_sent_by_b:b",
			"event_id": "$e6:b",
			"content": {"reason": ""}
		}, {
			"type": "m.room.redaction",
			"sender": "@u2:a",
			"room_id": "!r1:a",
			"redacts": "$event_sent_by_a:a",
			"event_id": "$e7:a",
			"content": {"reason": ""}
		}, {
			"type": "m.room.redaction",
			"sender": "@u1:a",
			"room_id": "!r1:a",
			"redacts": "$event_sent_by_b:b",
			"event_id": "$e8:a",
			"content": {"reason": ""}
		}],
		"not_allowed": [{
			"type": "m.room.redaction",
			"sender": "@u2:a",
			"room_id": "!r1:a",
			"redacts": "$event_sent_by_b:b",
			"event_id": "$e9:a",
			"content": {"reason": ""},
			"unsigned": {
				"not_allowed": "User power level is too low and event is from different server"
			}
		}, {
			"type": "m.room.redaction",
			"sender": "@u1:c",
			"room_id": "!r1:a",
			"redacts": "$event_sent_by_c:c",
			"event_id": "$e10:a",
			"content": {"reason": ""},
			"unsigned": {
				"not_allowed": "User is not in the room"
			}
		}, {
			"type": "m.room.redaction",
			"sender": "@u1:a",
			"room_id": "!r1:a",
			"redacts": "not_a_valid_event_id",
			"event_id": "$e11:a",
			"content": {"reason": ""},
			"unsigned": {
				"not_allowed": "Invalid redacts event ID"
			}
		}, {
			"type": "m.room.redaction",
			"sender": "@u1:a",
			"room_id": "!r1:a",
			"event_id": "$e11:a",
			"content": {"reason": ""},
			"unsigned": {
				"not_allowed": "Missing redacts event ID"
			}
		}]
	}`)
}

func TestAuthEvents(t *testing.T) {
	power, err := NewEventFromTrustedJSON(rawJSON(`{
		"type": "m.room.power_levels",
		"state_key": "",
		"sender": "@u1:a",
		"room_id": "!r1:a",
		"event_id": "$e5:a",
		"content": {
			"users": {
				"@u1:a": 100
			},
			"redact": 100
		}
	}`), false)
	if err != nil {
		t.Fatalf("TestAuthEvents: failed to create power_levels event: %s", err)
	}
	a := NewAuthEvents([]*Event{&power})
	var e *Event
	if e, err = a.PowerLevels(); err != nil || e != &power {
		t.Errorf("TestAuthEvents: failed to get same power_levels event")
	}
	create, err := NewEventFromTrustedJSON(rawJSON(`{
		"type": "m.room.create",
		"state_key": "",
		"sender": "@u1:a",
		"room_id": "!r1:a",
		"event_id": "$e1:a",
		"content": {
			"creator": "@u1:a"
		}
	}`), false)
	if err != nil {
		t.Fatalf("TestAuthEvents: failed to create create event: %s", err)
	}
	if err = a.AddEvent(&create); err != nil {
		t.Errorf("TestAuthEvents: Failed to AddEvent: %s", err)
	}
	if e, err = a.Create(); err != nil || e != &create {
		t.Errorf("TestAuthEvents: failed to get same create event")
	}
}
