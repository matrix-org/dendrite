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

package alias

import (
	"context"
	"fmt"
	"testing"

	appserviceAPI "github.com/matrix-org/dendrite/appservice/api"
	roomserverAPI "github.com/matrix-org/dendrite/roomserver/api"
)

type MockRoomserverAliasAPIDatabase struct{}

func (db MockRoomserverAliasAPIDatabase) SetRoomAlias(ctx context.Context, alias string, roomID string) error {
	return nil
}

func (db MockRoomserverAliasAPIDatabase) GetRoomIDForAlias(ctx context.Context, alias string) (string, error) {
	return "123", nil
}

func (db MockRoomserverAliasAPIDatabase) GetAliasesForRoomID(ctx context.Context, roomID string) ([]string, error) {
	aliases := make([]string, 0)
	return aliases, nil
}

func (db MockRoomserverAliasAPIDatabase) RemoveRoomAlias(ctx context.Context, alias string) error {
	return nil
}

type MockAppServiceQueryAPI struct{}

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
	return fmt.Errorf("Should not have called this")
}

func TestGetRoomIDForAlias(t *testing.T) {
	type args struct {
		ctx      context.Context
		request  *roomserverAPI.GetRoomIDForAliasRequest
		response *roomserverAPI.GetRoomIDForAliasResponse
	}

	t.Run("Found local alias", func(t *testing.T) {
		aliasAPI := &RoomserverAliasAPI{
			DB:            MockRoomserverAliasAPIDatabase{},
			AppserviceAPI: MockAppServiceQueryAPI{},
		}
		args := args{
			context.Background(),
			&roomserverAPI.GetRoomIDForAliasRequest{},
			&roomserverAPI.GetRoomIDForAliasResponse{},
		}

		err := aliasAPI.GetRoomIDForAlias(args.ctx, args.request, args.response)
		if err != nil {
			t.Errorf("Got %s; wanted no error", err)
		}

		if args.response.RoomID != "123" {
			t.Errorf("Got %s; wanted 123", args.response.RoomID)
		}
	})
}
