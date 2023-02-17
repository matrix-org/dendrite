// Copyright 2020 The Matrix.org Foundation C.I.C.
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

package userapi_test

import (
	"context"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/matrix-org/dendrite/userapi/producers"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
	"github.com/nats-io/nats.go"
	"golang.org/x/crypto/bcrypt"

	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/dendrite/test"
	"github.com/matrix-org/dendrite/test/testrig"
	"github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/dendrite/userapi/internal"
	"github.com/matrix-org/dendrite/userapi/storage"
)

const (
	serverName = gomatrixserverlib.ServerName("example.com")
)

type apiTestOpts struct {
	loginTokenLifetime time.Duration
	serverName         string
}

type dummyProducer struct{}

func (d *dummyProducer) PublishMsg(*nats.Msg, ...nats.PubOpt) (*nats.PubAck, error) {
	return &nats.PubAck{}, nil
}

func MustMakeInternalAPI(t *testing.T, opts apiTestOpts, dbType test.DBType) (api.UserInternalAPI, storage.Database, func()) {
	if opts.loginTokenLifetime == 0 {
		opts.loginTokenLifetime = api.DefaultLoginTokenLifetime * time.Millisecond
	}
	base, baseclose := testrig.CreateBaseDendrite(t, dbType)
	connStr, close := test.PrepareDBConnectionString(t, dbType)
	sName := serverName
	if opts.serverName != "" {
		sName = gomatrixserverlib.ServerName(opts.serverName)
	}
	accountDB, err := storage.NewUserDatabase(base, &config.DatabaseOptions{
		ConnectionString: config.DataSource(connStr),
	}, sName, bcrypt.MinCost, config.DefaultOpenIDTokenLifetimeMS, opts.loginTokenLifetime, "")
	if err != nil {
		t.Fatalf("failed to create account DB: %s", err)
	}

	cfg := &config.UserAPI{
		Matrix: &config.Global{
			SigningIdentity: gomatrixserverlib.SigningIdentity{
				ServerName: sName,
			},
		},
	}

	syncProducer := producers.NewSyncAPI(accountDB, &dummyProducer{}, "", "")
	keyChangeProducer := &producers.KeyChange{DB: accountDB, JetStream: &dummyProducer{}}
	return &internal.UserInternalAPI{
			DB:                accountDB,
			Config:            cfg,
			SyncProducer:      syncProducer,
			KeyChangeProducer: keyChangeProducer,
		}, accountDB, func() {
			close()
			baseclose()
		}
}

func TestQueryProfile(t *testing.T) {
	aliceAvatarURL := "mxc://example.com/alice"
	aliceDisplayName := "Alice"

	testCases := []struct {
		req     api.QueryProfileRequest
		wantRes api.QueryProfileResponse
		wantErr error
	}{
		{
			req: api.QueryProfileRequest{
				UserID: fmt.Sprintf("@alice:%s", serverName),
			},
			wantRes: api.QueryProfileResponse{
				UserExists:  true,
				AvatarURL:   aliceAvatarURL,
				DisplayName: aliceDisplayName,
			},
		},
		{
			req: api.QueryProfileRequest{
				UserID: fmt.Sprintf("@bob:%s", serverName),
			},
			wantRes: api.QueryProfileResponse{
				UserExists: false,
			},
		},
		{
			req: api.QueryProfileRequest{
				UserID: "@alice:wrongdomain.com",
			},
			wantErr: fmt.Errorf("wrong domain"),
		},
	}

	runCases := func(testAPI api.UserInternalAPI, http bool) {
		mode := "monolith"
		if http {
			mode = "HTTP"
		}
		for _, tc := range testCases {
			var gotRes api.QueryProfileResponse
			gotErr := testAPI.QueryProfile(context.TODO(), &tc.req, &gotRes)
			if tc.wantErr == nil && gotErr != nil || tc.wantErr != nil && gotErr == nil {
				t.Errorf("QueryProfile %s error, got %s want %s", mode, gotErr, tc.wantErr)
				continue
			}
			if !reflect.DeepEqual(tc.wantRes, gotRes) {
				t.Errorf("QueryProfile %s response got %+v want %+v", mode, gotRes, tc.wantRes)
			}
		}
	}

	test.WithAllDatabases(t, func(t *testing.T, dbType test.DBType) {
		userAPI, accountDB, close := MustMakeInternalAPI(t, apiTestOpts{}, dbType)
		defer close()
		_, err := accountDB.CreateAccount(context.TODO(), "alice", serverName, "foobar", "", api.AccountTypeUser)
		if err != nil {
			t.Fatalf("failed to make account: %s", err)
		}
		if _, _, err := accountDB.SetAvatarURL(context.TODO(), "alice", serverName, aliceAvatarURL); err != nil {
			t.Fatalf("failed to set avatar url: %s", err)
		}
		if _, _, err := accountDB.SetDisplayName(context.TODO(), "alice", serverName, aliceDisplayName); err != nil {
			t.Fatalf("failed to set display name: %s", err)
		}

		runCases(userAPI, false)
	})
}

// TestPasswordlessLoginFails ensures that a passwordless account cannot
// be logged into using an arbitrary password (effectively a regression test
// for https://github.com/matrix-org/dendrite/issues/2780).
func TestPasswordlessLoginFails(t *testing.T) {
	ctx := context.Background()
	test.WithAllDatabases(t, func(t *testing.T, dbType test.DBType) {
		userAPI, accountDB, close := MustMakeInternalAPI(t, apiTestOpts{}, dbType)
		defer close()
		_, err := accountDB.CreateAccount(ctx, "auser", serverName, "", "", api.AccountTypeAppService)
		if err != nil {
			t.Fatalf("failed to make account: %s", err)
		}

		userReq := &api.QueryAccountByPasswordRequest{
			Localpart:         "auser",
			PlaintextPassword: "apassword",
		}
		userRes := &api.QueryAccountByPasswordResponse{}
		if err := userAPI.QueryAccountByPassword(ctx, userReq, userRes); err != nil {
			t.Fatal(err)
		}
		if userRes.Exists || userRes.Account != nil {
			t.Fatal("QueryAccountByPassword should not return correctly for a passwordless account")
		}
	})
}

func TestLoginToken(t *testing.T) {
	ctx := context.Background()

	t.Run("tokenLoginFlow", func(t *testing.T) {
		test.WithAllDatabases(t, func(t *testing.T, dbType test.DBType) {
			userAPI, accountDB, close := MustMakeInternalAPI(t, apiTestOpts{}, dbType)
			defer close()
			_, err := accountDB.CreateAccount(ctx, "auser", serverName, "apassword", "", api.AccountTypeUser)
			if err != nil {
				t.Fatalf("failed to make account: %s", err)
			}

			t.Log("Creating a login token like the SSO callback would...")

			creq := api.PerformLoginTokenCreationRequest{
				Data: api.LoginTokenData{UserID: "@auser:example.com"},
			}
			var cresp api.PerformLoginTokenCreationResponse
			if err := userAPI.PerformLoginTokenCreation(ctx, &creq, &cresp); err != nil {
				t.Fatalf("PerformLoginTokenCreation failed: %v", err)
			}

			if cresp.Metadata.Token == "" {
				t.Errorf("PerformLoginTokenCreation Token: got %q, want non-empty", cresp.Metadata.Token)
			}
			if cresp.Metadata.Expiration.Before(time.Now()) {
				t.Errorf("PerformLoginTokenCreation Expiration: got %v, want non-expired", cresp.Metadata.Expiration)
			}

			t.Log("Querying the login token like /login with m.login.token would...")

			qreq := api.QueryLoginTokenRequest{Token: cresp.Metadata.Token}
			var qresp api.QueryLoginTokenResponse
			if err := userAPI.QueryLoginToken(ctx, &qreq, &qresp); err != nil {
				t.Fatalf("QueryLoginToken failed: %v", err)
			}

			if qresp.Data == nil {
				t.Errorf("QueryLoginToken Data: got %v, want non-nil", qresp.Data)
			} else if want := "@auser:example.com"; qresp.Data.UserID != want {
				t.Errorf("QueryLoginToken UserID: got %q, want %q", qresp.Data.UserID, want)
			}

			t.Log("Deleting the login token like /login with m.login.token would...")

			dreq := api.PerformLoginTokenDeletionRequest{Token: cresp.Metadata.Token}
			var dresp api.PerformLoginTokenDeletionResponse
			if err := userAPI.PerformLoginTokenDeletion(ctx, &dreq, &dresp); err != nil {
				t.Fatalf("PerformLoginTokenDeletion failed: %v", err)
			}
		})
	})

	t.Run("expiredTokenIsNotReturned", func(t *testing.T) {
		test.WithAllDatabases(t, func(t *testing.T, dbType test.DBType) {
			userAPI, _, close := MustMakeInternalAPI(t, apiTestOpts{loginTokenLifetime: -1 * time.Second}, dbType)
			defer close()

			creq := api.PerformLoginTokenCreationRequest{
				Data: api.LoginTokenData{UserID: "@auser:example.com"},
			}
			var cresp api.PerformLoginTokenCreationResponse
			if err := userAPI.PerformLoginTokenCreation(ctx, &creq, &cresp); err != nil {
				t.Fatalf("PerformLoginTokenCreation failed: %v", err)
			}

			qreq := api.QueryLoginTokenRequest{Token: cresp.Metadata.Token}
			var qresp api.QueryLoginTokenResponse
			if err := userAPI.QueryLoginToken(ctx, &qreq, &qresp); err != nil {
				t.Fatalf("QueryLoginToken failed: %v", err)
			}

			if qresp.Data != nil {
				t.Errorf("QueryLoginToken Data: got %v, want nil", qresp.Data)
			}
		})
	})

	t.Run("deleteWorks", func(t *testing.T) {
		test.WithAllDatabases(t, func(t *testing.T, dbType test.DBType) {
			userAPI, _, close := MustMakeInternalAPI(t, apiTestOpts{}, dbType)
			defer close()

			creq := api.PerformLoginTokenCreationRequest{
				Data: api.LoginTokenData{UserID: "@auser:example.com"},
			}
			var cresp api.PerformLoginTokenCreationResponse
			if err := userAPI.PerformLoginTokenCreation(ctx, &creq, &cresp); err != nil {
				t.Fatalf("PerformLoginTokenCreation failed: %v", err)
			}

			dreq := api.PerformLoginTokenDeletionRequest{Token: cresp.Metadata.Token}
			var dresp api.PerformLoginTokenDeletionResponse
			if err := userAPI.PerformLoginTokenDeletion(ctx, &dreq, &dresp); err != nil {
				t.Fatalf("PerformLoginTokenDeletion failed: %v", err)
			}

			qreq := api.QueryLoginTokenRequest{Token: cresp.Metadata.Token}
			var qresp api.QueryLoginTokenResponse
			if err := userAPI.QueryLoginToken(ctx, &qreq, &qresp); err != nil {
				t.Fatalf("QueryLoginToken failed: %v", err)
			}

			if qresp.Data != nil {
				t.Errorf("QueryLoginToken Data: got %v, want nil", qresp.Data)
			}
		})
	})

	t.Run("deleteUnknownIsNoOp", func(t *testing.T) {
		test.WithAllDatabases(t, func(t *testing.T, dbType test.DBType) {
			userAPI, _, close := MustMakeInternalAPI(t, apiTestOpts{}, dbType)
			defer close()
			dreq := api.PerformLoginTokenDeletionRequest{Token: "non-existent token"}
			var dresp api.PerformLoginTokenDeletionResponse
			if err := userAPI.PerformLoginTokenDeletion(ctx, &dreq, &dresp); err != nil {
				t.Fatalf("PerformLoginTokenDeletion failed: %v", err)
			}
		})
	})
}

func TestQueryAccountByLocalpart(t *testing.T) {
	alice := test.NewUser(t)

	localpart, userServername, _ := gomatrixserverlib.SplitID('@', alice.ID)

	ctx := context.Background()
	test.WithAllDatabases(t, func(t *testing.T, dbType test.DBType) {
		intAPI, db, close := MustMakeInternalAPI(t, apiTestOpts{}, dbType)
		defer close()

		createdAcc, err := db.CreateAccount(ctx, localpart, userServername, "", "", alice.AccountType)
		if err != nil {
			t.Error(err)
		}

		testCases := func(t *testing.T, internalAPI api.UserInternalAPI) {
			// Query existing account
			queryAccResp := &api.QueryAccountByLocalpartResponse{}
			if err = internalAPI.QueryAccountByLocalpart(ctx, &api.QueryAccountByLocalpartRequest{
				Localpart:  localpart,
				ServerName: userServername,
			}, queryAccResp); err != nil {
				t.Error(err)
			}
			if !reflect.DeepEqual(createdAcc, queryAccResp.Account) {
				t.Fatalf("created and queried accounts don't match:\n%+v vs.\n%+v", createdAcc, queryAccResp.Account)
			}

			// Query non-existent account, this should result in an error
			err = internalAPI.QueryAccountByLocalpart(ctx, &api.QueryAccountByLocalpartRequest{
				Localpart:  "doesnotexist",
				ServerName: userServername,
			}, queryAccResp)

			if err == nil {
				t.Fatalf("expected an error, but got none: %+v", queryAccResp)
			}
		}

		testCases(t, intAPI)
	})
}

func TestAccountData(t *testing.T) {
	ctx := context.Background()
	alice := test.NewUser(t)

	testCases := []struct {
		name      string
		inputData *api.InputAccountDataRequest
		wantErr   bool
	}{
		{
			name:      "not a local user",
			inputData: &api.InputAccountDataRequest{UserID: "@notlocal:example.com"},
			wantErr:   true,
		},
		{
			name:      "local user missing datatype",
			inputData: &api.InputAccountDataRequest{UserID: alice.ID},
			wantErr:   true,
		},
		{
			name:      "missing json",
			inputData: &api.InputAccountDataRequest{UserID: alice.ID, DataType: "m.push_rules", AccountData: nil},
			wantErr:   true,
		},
		{
			name:      "with json",
			inputData: &api.InputAccountDataRequest{UserID: alice.ID, DataType: "m.push_rules", AccountData: []byte("{}")},
		},
		{
			name:      "room data",
			inputData: &api.InputAccountDataRequest{UserID: alice.ID, DataType: "m.push_rules", AccountData: []byte("{}"), RoomID: "!dummy:test"},
		},
		{
			name:      "ignored users",
			inputData: &api.InputAccountDataRequest{UserID: alice.ID, DataType: "m.ignored_user_list", AccountData: []byte("{}")},
		},
		{
			name:      "m.fully_read",
			inputData: &api.InputAccountDataRequest{UserID: alice.ID, DataType: "m.fully_read", AccountData: []byte("{}")},
		},
	}

	test.WithAllDatabases(t, func(t *testing.T, dbType test.DBType) {
		intAPI, _, close := MustMakeInternalAPI(t, apiTestOpts{serverName: "test"}, dbType)
		defer close()

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				res := api.InputAccountDataResponse{}
				err := intAPI.InputAccountData(ctx, tc.inputData, &res)
				if tc.wantErr && err == nil {
					t.Fatalf("expected an error, but got none")
				}
				if !tc.wantErr && err != nil {
					t.Fatalf("expected no error, but got: %s", err)
				}

				// query the data again and compare
				queryRes := api.QueryAccountDataResponse{}
				queryReq := api.QueryAccountDataRequest{
					UserID:   tc.inputData.UserID,
					DataType: tc.inputData.DataType,
					RoomID:   tc.inputData.RoomID,
				}
				err = intAPI.QueryAccountData(ctx, &queryReq, &queryRes)
				if err != nil && !tc.wantErr {
					t.Fatal(err)
				}
				// verify global data
				if tc.inputData.RoomID == "" {
					if !reflect.DeepEqual(tc.inputData.AccountData, queryRes.GlobalAccountData[tc.inputData.DataType]) {
						t.Fatalf("expected accountdata to be %s, got %s", string(tc.inputData.AccountData), string(queryRes.GlobalAccountData[tc.inputData.DataType]))
					}
				} else {
					// verify room data
					if !reflect.DeepEqual(tc.inputData.AccountData, queryRes.RoomAccountData[tc.inputData.RoomID][tc.inputData.DataType]) {
						t.Fatalf("expected accountdata to be %s, got %s", string(tc.inputData.AccountData), string(queryRes.RoomAccountData[tc.inputData.RoomID][tc.inputData.DataType]))
					}
				}
			})
		}
	})
}

func TestDevices(t *testing.T) {
	ctx := context.Background()
	alice := test.NewUser(t)
	_ = alice

	dupeAccessToken := util.RandomString(8)

	creationTests := []struct {
		name         string
		inputData    *api.PerformDeviceCreationRequest
		wantErr      bool
		wantNewDevID bool
	}{
		{
			name:      "not a local user",
			inputData: &api.PerformDeviceCreationRequest{Localpart: "test1", ServerName: "notlocal"},
			wantErr:   true,
		},
		{
			name:      "implicit local user",
			inputData: &api.PerformDeviceCreationRequest{Localpart: "test1", AccessToken: util.RandomString(8), NoDeviceListUpdate: true},
		},
		{
			name:      "explicit local user",
			inputData: &api.PerformDeviceCreationRequest{Localpart: "test2", ServerName: "test", AccessToken: util.RandomString(8), NoDeviceListUpdate: true},
		},
		{
			name:      "dupe token - ok",
			inputData: &api.PerformDeviceCreationRequest{Localpart: "test3", ServerName: "test", AccessToken: dupeAccessToken, NoDeviceListUpdate: true},
		},
		{
			name:      "dupe token - not ok",
			inputData: &api.PerformDeviceCreationRequest{Localpart: "test3", ServerName: "test", AccessToken: dupeAccessToken, NoDeviceListUpdate: true},
			wantErr:   true,
		},
		{
			name:      "test3 second device", // used to test deletion later
			inputData: &api.PerformDeviceCreationRequest{Localpart: "test3", ServerName: "test", AccessToken: util.RandomString(8), NoDeviceListUpdate: true},
		},
		{
			name:         "test3 third device", // used to test deletion later
			wantNewDevID: true,
			inputData:    &api.PerformDeviceCreationRequest{Localpart: "test3", ServerName: "test", AccessToken: util.RandomString(8), NoDeviceListUpdate: true},
		},
	}

	deletionTests := []struct {
		name        string
		inputData   *api.PerformDeviceDeletionRequest
		wantErr     bool
		wantDevices int
	}{
		{
			name:      "deletion - not a local user",
			inputData: &api.PerformDeviceDeletionRequest{UserID: "@test:notlocalhost"},
			wantErr:   true,
		},
		{
			name:        "deleting not existing devices should not error",
			inputData:   &api.PerformDeviceDeletionRequest{UserID: "@test1:test", DeviceIDs: []string{"iDontExist"}},
			wantDevices: 1,
		},
		{
			name:        "delete all devices",
			inputData:   &api.PerformDeviceDeletionRequest{UserID: "@test1:test"},
			wantDevices: 0,
		},
		{
			name:        "delete all devices",
			inputData:   &api.PerformDeviceDeletionRequest{UserID: "@test3:test"},
			wantDevices: 0,
		},
	}

	test.WithAllDatabases(t, func(t *testing.T, dbType test.DBType) {
		intAPI, _, close := MustMakeInternalAPI(t, apiTestOpts{serverName: "test"}, dbType)
		defer close()

		for _, tc := range creationTests {
			t.Run(tc.name, func(t *testing.T) {
				res := api.PerformDeviceCreationResponse{}
				deviceID := util.RandomString(8)
				tc.inputData.DeviceID = &deviceID
				if tc.wantNewDevID {
					tc.inputData.DeviceID = nil
				}
				err := intAPI.PerformDeviceCreation(ctx, tc.inputData, &res)
				if tc.wantErr && err == nil {
					t.Fatalf("expected an error, but got none")
				}
				if !tc.wantErr && err != nil {
					t.Fatalf("expected no error, but got: %s", err)
				}
				if !res.DeviceCreated {
					return
				}

				queryDevicesRes := api.QueryDevicesResponse{}
				queryDevicesReq := api.QueryDevicesRequest{UserID: res.Device.UserID}
				if err = intAPI.QueryDevices(ctx, &queryDevicesReq, &queryDevicesRes); err != nil {
					t.Fatal(err)
				}
				// We only want to verify one device
				if len(queryDevicesRes.Devices) > 1 {
					return
				}
				res.Device.AccessToken = ""

				// At this point, there should only be one device
				if !reflect.DeepEqual(*res.Device, queryDevicesRes.Devices[0]) {
					t.Fatalf("expected device to be\n%#v, got \n%#v", *res.Device, queryDevicesRes.Devices[0])
				}
			})
		}

		for _, tc := range deletionTests {
			t.Run(tc.name, func(t *testing.T) {
				delRes := api.PerformDeviceDeletionResponse{}
				err := intAPI.PerformDeviceDeletion(ctx, tc.inputData, &delRes)
				if tc.wantErr && err == nil {
					t.Fatalf("expected an error, but got none")
				}
				if !tc.wantErr && err != nil {
					t.Fatalf("expected no error, but got: %s", err)
				}
				if tc.wantErr {
					return
				}

				queryDevicesRes := api.QueryDevicesResponse{}
				queryDevicesReq := api.QueryDevicesRequest{UserID: tc.inputData.UserID}
				if err = intAPI.QueryDevices(ctx, &queryDevicesReq, &queryDevicesRes); err != nil {
					t.Fatal(err)
				}

				if len(queryDevicesRes.Devices) != tc.wantDevices {
					t.Fatalf("expected %d devices, got %d", tc.wantDevices, len(queryDevicesRes.Devices))
				}

			})
		}
	})
}

// Tests that the session ID of a device is not reused when reusing the same device ID.
func TestDeviceIDReuse(t *testing.T) {
	ctx := context.Background()
	test.WithAllDatabases(t, func(t *testing.T, dbType test.DBType) {
		intAPI, _, close := MustMakeInternalAPI(t, apiTestOpts{serverName: "test"}, dbType)
		defer close()

		res := api.PerformDeviceCreationResponse{}
		// create a first device
		deviceID := util.RandomString(8)
		req := api.PerformDeviceCreationRequest{Localpart: "alice", ServerName: "test", DeviceID: &deviceID, NoDeviceListUpdate: true}
		err := intAPI.PerformDeviceCreation(ctx, &req, &res)
		if err != nil {
			t.Fatal(err)
		}

		// Do the same request again, we expect a different sessionID
		res2 := api.PerformDeviceCreationResponse{}
		err = intAPI.PerformDeviceCreation(ctx, &req, &res2)
		if err != nil {
			t.Fatalf("expected no error, but got: %v", err)
		}

		if res2.Device.SessionID == res.Device.SessionID {
			t.Fatalf("expected a different session ID, but they are the same")
		}
	})
}
