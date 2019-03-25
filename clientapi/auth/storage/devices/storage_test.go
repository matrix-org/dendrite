package devices

import (
	"context"
	"fmt"
	"github.com/matrix-org/dendrite/clientapi/auth/authtypes"
	"github.com/stretchr/testify/assert"
	"testing"
)

const dataSourceName = "postgres://dendrite:itsasecret@postgres/dendrite_device?sslmode=disable"

//const dataSourceLocal = "postgres://dendrite:itsasecret@localhost:15432/dendrite_device?sslmode=disable"

type deviceSpec struct {
	localPart   string
	devId       string
	accessToken string
	displayName string
}

func TestDatabase_GetDevicesByLocalpart(t *testing.T) {
	db, err := NewDatabase(dataSourceName, "localhost")
	assert.Nil(t, err)

	devSpec := deviceSpec{
		localPart:   "get-device-test-local-part",
		devId:       "get-device-test-device-id",
		accessToken: "get-device-test-access-token",
		displayName: "get-device-test-display-name",
	}
	dev, err := createTestDevice(&devSpec, 5)
	assert.Nil(t, err)
	for _, d := range dev {
		assert.Contains(t, d.ID, "get-device-test-device-id")
		assert.Contains(t, d.AccessToken, "get-device-test-access-token")
	}

	ctx := context.Background()
	devices, err := db.GetDevicesByLocalpart(ctx, "get-device-test-local-part0")
	assert.Nil(t, err)
	assert.Contains(t, devices[0].UserID, "get-device-test-local-part0")
}

func TestDatabase_CreateDevice(t *testing.T) {
	devSpec := deviceSpec{
		localPart:   "create-test-local-part",
		devId:       "create-test-device-id",
		accessToken: "create-test-access-token",
		displayName: "create-test-display-name",
	}
	dev, err := createTestDevice(&devSpec, 5)
	assert.Nil(t, err)
	for _, d := range dev {
		assert.Contains(t, d.AccessToken, "create-test-access-token")
		assert.Contains(t, d.ID, "create-test-device-id")
	}
}

// create a number of device entries in the database, using ``devSpecScheme`` as the creation pattern.
func createTestDevice(devSpecScheme *deviceSpec, count int) (devices []*authtypes.Device, err error) {
	db, err := NewDatabase(dataSourceName, "localhost")
	if err != nil {
		fmt.Println(err)
	}

	for i := 0; i < count; i++ {
		devId := fmt.Sprintf("%s%d", devSpecScheme.devId, i)
		displayName := fmt.Sprintf("%s%d", devSpecScheme.displayName, i)
		if device, err := db.CreateDevice(
			context.Background(),
			fmt.Sprintf("%s%d", devSpecScheme.localPart, i),
			&devId,
			fmt.Sprintf("%s%d", devSpecScheme.accessToken, i),
			&displayName); err != nil {
			fmt.Println(err)
			return nil, err
		} else {
			devices = append(devices, device)
		}
	}
	return devices, nil
}
