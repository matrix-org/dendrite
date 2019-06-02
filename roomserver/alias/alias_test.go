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
)

type MockRoomserverAliasAPIDatabase struct {
	methodModes map[string]string
	attempts    int
}

// Those methods can be essentially noop
func (db MockRoomserverAliasAPIDatabase) SetRoomAlias(ctx context.Context, alias string, roomID string) error {
	return nil
}

func (db MockRoomserverAliasAPIDatabase) GetAliasesForRoomID(ctx context.Context, roomID string) ([]string, error) {
	aliases := make([]string, 0)
	return aliases, nil
}

func (db MockRoomserverAliasAPIDatabase) RemoveRoomAlias(ctx context.Context, alias string) error {
	return nil
}

// This method needs to change depending on test case
func (db *MockRoomserverAliasAPIDatabase) GetRoomIDForAlias(
	ctx context.Context,
	alias string,
) (string, error) {
	switch db.methodModes["GetRoomIDForAlias"] {
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
	methodModes map[string]string
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
	switch q.methodModes["RoomAliasExists"] {
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

	setup := func(modes map[string]string) *RoomserverAliasAPI {
		mockAliasAPIDB := &MockRoomserverAliasAPIDatabase{modes, 0}
		mockAppServiceQueryAPI := MockAppServiceQueryAPI{modes}

		return &RoomserverAliasAPI{
			DB:            mockAliasAPIDB,
			AppserviceAPI: mockAppServiceQueryAPI,
		}
	}

	t.Run("Found local alias", func(t *testing.T) {
		methodModes := make(map[string]string)
		methodModes["GetRoomIDForAlias"] = "found"
		methodModes["RoomAliasExists"] = "error"
		aliasAPI := setup(methodModes)

		err := aliasAPI.GetRoomIDForAlias(args.ctx, args.request, args.response)
		if err != nil {
			t.Errorf("Got %s; wanted no error", err)
		}

		if args.response.RoomID != "123" {
			t.Errorf("Got %s; wanted 123", args.response.RoomID)
		}
	})

	t.Run("found appservice alias", func(t *testing.T) {
		methodModes := make(map[string]string)
		methodModes["GetRoomIDForAlias"] = "emptyFound"
		methodModes["RoomAliasExists"] = "found"
		aliasAPI := setup(methodModes)

		if err := aliasAPI.GetRoomIDForAlias(args.ctx, args.request, args.response); err != nil {
			t.Fatalf("Got %s; wanted no error", err)
		}

		if args.response.RoomID != "123" {
			t.Errorf("Got %s; wanted 123", args.response.RoomID)
		}
	})

	t.Run("error returned from DB", func(t *testing.T) {
		methodModes := make(map[string]string)
		methodModes["GetRoomIDForAlias"] = "error"
		aliasAPI := setup(methodModes)

		err := aliasAPI.GetRoomIDForAlias(args.ctx, args.request, args.response)
		if err == nil {
			t.Fatalf("Got no error; wanted error from DB")
		} else if !strings.Contains(err.Error(), "GetRoomIDForAlias") {
			t.Errorf("Got %s; wanted error from GetRoomIDForAlias", err)
		}
	})

	t.Run("error returned from appserviceAPI", func(t *testing.T) {
		methodModes := make(map[string]string)
		methodModes["GetRoomIDForAlias"] = "empty"
		methodModes["RoomAliasExists"] = "error"
		aliasAPI := setup(methodModes)

		err := aliasAPI.GetRoomIDForAlias(args.ctx, args.request, args.response)
		if err == nil {
			t.Fatalf("Got no error; wanted error from appserviceAPI")
		} else if !strings.Contains(err.Error(), "RoomAliasExists") {
			t.Errorf("Got %s; wanted error from RoomAliasExists", err)
		}
	})

	t.Run("no errors but no alias", func(t *testing.T) {
		methodModes := make(map[string]string)
		methodModes["GetRoomIDForAlias"] = "empty"
		methodModes["RoomAliasExists"] = "empty"
		aliasAPI := setup(methodModes)
		args.response.RoomID = "Should be empty"

		if err := aliasAPI.GetRoomIDForAlias(args.ctx, args.request, args.response); err != nil {
			t.Fatalf("Got %s; wanted no error", err)
		}
		if args.response.RoomID != "" {
			t.Errorf("response.RoomID should have been empty")
		}
	})
}
