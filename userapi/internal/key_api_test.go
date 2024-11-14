package internal_test

import (
	"context"
	"reflect"
	"testing"

	"github.com/element-hq/dendrite/internal/sqlutil"
	"github.com/element-hq/dendrite/setup/config"
	"github.com/element-hq/dendrite/test"
	"github.com/element-hq/dendrite/userapi/api"
	"github.com/element-hq/dendrite/userapi/internal"
	"github.com/element-hq/dendrite/userapi/storage"
)

func mustCreateDatabase(t *testing.T, dbType test.DBType) (storage.KeyDatabase, func()) {
	t.Helper()
	connStr, close := test.PrepareDBConnectionString(t, dbType)
	cm := sqlutil.NewConnectionManager(nil, config.DatabaseOptions{})
	db, err := storage.NewKeyDatabase(cm, &config.DatabaseOptions{
		ConnectionString: config.DataSource(connStr),
	})
	if err != nil {
		t.Fatalf("failed to create new user db: %v", err)
	}
	return db, func() {
		close()
	}
}

func Test_QueryDeviceMessages(t *testing.T) {
	alice := test.NewUser(t)
	type args struct {
		req *api.QueryDeviceMessagesRequest
		res *api.QueryDeviceMessagesResponse
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
		want    *api.QueryDeviceMessagesResponse
	}{
		{
			name: "no existing keys",
			args: args{
				req: &api.QueryDeviceMessagesRequest{
					UserID: "@doesNotExist:localhost",
				},
				res: &api.QueryDeviceMessagesResponse{},
			},
			want: &api.QueryDeviceMessagesResponse{},
		},
		{
			name: "existing user returns devices",
			args: args{
				req: &api.QueryDeviceMessagesRequest{
					UserID: alice.ID,
				},
				res: &api.QueryDeviceMessagesResponse{},
			},
			want: &api.QueryDeviceMessagesResponse{
				StreamID: 6,
				Devices: []api.DeviceMessage{
					{
						Type: api.TypeDeviceKeyUpdate, StreamID: 5, DeviceKeys: &api.DeviceKeys{
							DeviceID:    "myDevice",
							DisplayName: "first device",
							UserID:      alice.ID,
							KeyJSON:     []byte("ghi"),
						},
					},
					{
						Type: api.TypeDeviceKeyUpdate, StreamID: 6, DeviceKeys: &api.DeviceKeys{
							DeviceID:    "mySecondDevice",
							DisplayName: "second device",
							UserID:      alice.ID,
							KeyJSON:     []byte("jkl"),
						}, // streamID 6
					},
				},
			},
		},
	}

	deviceMessages := []api.DeviceMessage{
		{ // not the user we're looking for
			Type: api.TypeDeviceKeyUpdate, DeviceKeys: &api.DeviceKeys{
				UserID: "@doesNotExist:localhost",
			},
			// streamID 1 for this user
		},
		{ // empty keyJSON will be ignored
			Type: api.TypeDeviceKeyUpdate, DeviceKeys: &api.DeviceKeys{
				DeviceID: "myDevice",
				UserID:   alice.ID,
			}, // streamID 1
		},
		{
			Type: api.TypeDeviceKeyUpdate, DeviceKeys: &api.DeviceKeys{
				DeviceID: "myDevice",
				UserID:   alice.ID,
				KeyJSON:  []byte("abc"),
			}, // streamID 2
		},
		{
			Type: api.TypeDeviceKeyUpdate, DeviceKeys: &api.DeviceKeys{
				DeviceID: "myDevice",
				UserID:   alice.ID,
				KeyJSON:  []byte("def"),
			}, // streamID 3
		},
		{
			Type: api.TypeDeviceKeyUpdate, DeviceKeys: &api.DeviceKeys{
				DeviceID: "myDevice",
				UserID:   alice.ID,
				KeyJSON:  []byte(""),
			}, // streamID 4
		},
		{
			Type: api.TypeDeviceKeyUpdate, DeviceKeys: &api.DeviceKeys{
				DeviceID:    "myDevice",
				DisplayName: "first device",
				UserID:      alice.ID,
				KeyJSON:     []byte("ghi"),
			}, // streamID 5
		},
		{
			Type: api.TypeDeviceKeyUpdate, DeviceKeys: &api.DeviceKeys{
				DeviceID:    "mySecondDevice",
				UserID:      alice.ID,
				KeyJSON:     []byte("jkl"),
				DisplayName: "second device",
			}, // streamID 6
		},
	}
	ctx := context.Background()

	test.WithAllDatabases(t, func(t *testing.T, dbType test.DBType) {
		db, closeDB := mustCreateDatabase(t, dbType)
		defer closeDB()
		if err := db.StoreLocalDeviceKeys(ctx, deviceMessages); err != nil {
			t.Fatalf("failed to store local devicesKeys")
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				a := &internal.UserInternalAPI{
					KeyDatabase: db,
				}
				if err := a.QueryDeviceMessages(ctx, tt.args.req, tt.args.res); (err != nil) != tt.wantErr {
					t.Errorf("QueryDeviceMessages() error = %v, wantErr %v", err, tt.wantErr)
				}
				got := tt.args.res
				if !reflect.DeepEqual(got, tt.want) {
					t.Errorf("QueryDeviceMessages(): got:\n%+v, want:\n%+v", got, tt.want)
				}
			})
		}
	})
}
