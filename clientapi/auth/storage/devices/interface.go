package devices

import (
	"context"

	"github.com/matrix-org/dendrite/clientapi/auth/authtypes"
)

type Database interface {
	GetDeviceByAccessToken(ctx context.Context, token string) (*authtypes.Device, error)
	GetDeviceByID(ctx context.Context, localpart, deviceID string) (*authtypes.Device, error)
	GetDevicesByLocalpart(ctx context.Context, localpart string) ([]authtypes.Device, error)
	CreateDevice(ctx context.Context, localpart string, deviceID *string, accessToken string, displayName *string) (dev *authtypes.Device, returnErr error)
	UpdateDevice(ctx context.Context, localpart, deviceID string, displayName *string) error
	RemoveDevice(ctx context.Context, deviceID, localpart string) error
	RemoveDevices(ctx context.Context, localpart string, devices []string) error
	RemoveAllDevices(ctx context.Context, localpart string) error
}
