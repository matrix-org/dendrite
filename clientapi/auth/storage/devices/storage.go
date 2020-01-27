package devices

import (
	"context"
	"net/url"

	"github.com/matrix-org/dendrite/clientapi/auth/authtypes"
	"github.com/matrix-org/dendrite/mediaapi/storage/postgres"
)

type Database interface {
	GetDeviceByAccessToken(ctx context.Context, token string) (*authtypes.Device, error)
	GetDeviceByID(ctx context.Context, localpart, deviceID string) (*authtypes.Device, error)
	GetDevicesByLocalpart(ctx context.Context, localpart string) ([]authtypes.Device, error)
	CreateDevice(ctx context.Context, localpart string, deviceID *string, accessToken string, displayName *string)
	UpdateDevice(ctx context.Context, localpart, deviceID string, displayName *string) error
	RemoveDevice(ctx context.Context, deviceID, localpart string) error
	RemoveAllDevices(ctx context.Context, localpart string) error
}

func Open(dataSourceName string) (Database, error) {
	uri, err := url.Parse(dataSourceName)
	if err != nil {
		return postgres.Open(dataSourceName)
	}
	switch uri.Scheme {
	case "postgres":
		return postgres.Open(dataSourceName)
	case "file":
		//return sqlite3.Open(dataSourceName)
	default:
		return postgres.Open(dataSourceName)
	}
}
