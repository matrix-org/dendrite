// Copyright 2019 Serra Allgood
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

package alias

import (
	"context"
	"fmt"
	"strings"
	"testing"

	appserviceAPI "github.com/matrix-org/dendrite/appservice/api"
	roomserverAPI "github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/roomserver/types"
	"github.com/matrix-org/gomatrixserverlib"
)

type MockRoomserverAliasAPIDatabase struct {
	mode     string
	attempts int
}

// These methods can be essentially noop
func (db MockRoomserverAliasAPIDatabase) SetRoomAlias(ctx context.Context, alias string, roomID string, creatorUserID string) error {
	return nil
}

func (db MockRoomserverAliasAPIDatabase) GetAliasesForRoomID(ctx context.Context, roomID string) ([]string, error) {
	aliases := make([]string, 0)
	return aliases, nil
}

func (db MockRoomserverAliasAPIDatabase) RemoveRoomAlias(ctx context.Context, alias string) error {
	return nil
}

func (db *MockRoomserverAliasAPIDatabase) GetCreatorIDForAlias(
	ctx context.Context, alias string,
) (string, error) {
	return "", nil
}

func (db *MockRoomserverAliasAPIDatabase) RoomNID(
	ctx context.Context, roomID string,
) (types.RoomNID, error) {
	return 1, nil
}

func (db *MockRoomserverAliasAPIDatabase) GetRoomVersionForRoom(
	ctx context.Context, roomNID types.RoomNID,
) (gomatrixserverlib.RoomVersion, error) {
	return gomatrixserverlib.RoomVersionV1, nil
}

// This method needs to change depending on test case
func (db *MockRoomserverAliasAPIDatabase) GetRoomIDForAlias(
	ctx context.Context,
	alias string,
) (string, error) {
	switch db.mode {
	case "empty":
		return "", nil
	case "error":
		return "", fmt.Errorf("found an error from GetRoomIDForAlias")
	case "found":
		return "123", nil
	case "emptyFound":
		switch db.attempts {
		case 0:
			db.attempts = 1
			return "", nil
		case 1:
			db.attempts = 0
			return "123", nil
		default:
			return "", nil
		}
	default:
		return "", fmt.Errorf("unknown option used")
	}
}

type MockAppServiceQueryAPI struct {
	mode string
}

// This method can be noop
func (q MockAppServiceQueryAPI) UserIDExists(
	ctx context.Context,
	req *appserviceAPI.UserIDExistsRequest,
	resp *appserviceAPI.UserIDExistsResponse,
) error {
	return nil
}

func (q MockAppServiceQueryAPI) RoomAliasExists(
	ctx context.Context,
	req *appserviceAPI.RoomAliasExistsRequest,
	resp *appserviceAPI.RoomAliasExistsResponse,
) error {
	switch q.mode {
	case "error":
		return fmt.Errorf("found an error from RoomAliasExists")
	case "found":
		resp.AliasExists = true
		return nil
	case "empty":
		resp.AliasExists = false
		return nil
	default:
		return fmt.Errorf("Unknown option used")
	}
}

func TestGetRoomIDForAlias(t *testing.T) {
	type arguments struct {
		ctx      context.Context
		request  *roomserverAPI.GetRoomIDForAliasRequest
		response *roomserverAPI.GetRoomIDForAliasResponse
	}
	args := arguments{
		context.Background(),
		&roomserverAPI.GetRoomIDForAliasRequest{},
		&roomserverAPI.GetRoomIDForAliasResponse{},
	}
	type testCase struct {
		name      string
		dbMode    string
		queryMode string
		wantError bool
		errorMsg  string
		want      string
	}
	tt := []testCase{
		{
			"found local alias",
			"found",
			"error",
			false,
			"",
			"123",
		},
		{
			"found appservice alias",
			"emptyFound",
			"found",
			false,
			"",
			"123",
		},
		{
			"error returned from DB",
			"error",
			"",
			true,
			"GetRoomIDForAlias",
			"",
		},
		{
			"error returned from appserviceAPI",
			"empty",
			"error",
			true,
			"RoomAliasExists",
			"",
		},
		{
			"no errors but no alias",
			"empty",
			"empty",
			false,
			"",
			"",
		},
	}

	setup := func(dbMode, queryMode string) *RoomserverAliasAPI {
		mockAliasAPIDB := &MockRoomserverAliasAPIDatabase{dbMode, 0}
		mockAppServiceQueryAPI := MockAppServiceQueryAPI{queryMode}

		return &RoomserverAliasAPI{
			DB:            mockAliasAPIDB,
			AppserviceAPI: mockAppServiceQueryAPI,
		}
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			aliasAPI := setup(tc.dbMode, tc.queryMode)

			err := aliasAPI.GetRoomIDForAlias(args.ctx, args.request, args.response)
			if tc.wantError {
				if err == nil {
					t.Fatalf("Got no error; wanted error from %s", tc.errorMsg)
				} else if !strings.Contains(err.Error(), tc.errorMsg) {
					t.Fatalf("Got %s; wanted error from %s", err, tc.errorMsg)
				}
			} else if err != nil {
				t.Fatalf("Got %s; wanted no error", err)
			} else if args.response.RoomID != tc.want {
				t.Errorf("Got '%s'; wanted '%s'", args.response.RoomID, tc.want)
			}
		})
	}
}
