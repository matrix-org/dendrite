package devices

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/matrix-org/dendrite/clientapi/auth/authtypes"
	"github.com/stretchr/testify/assert"
)

var dataSource string
var insideCi = false
var insideDocker = false

const dbName = "dendrite_device"

func init() {
	for _, val := range os.Environ() {
		tokens := strings.Split(val, "=")
		if tokens[0] == "CI" && tokens[1] == "true" {
			insideCi = true
		}
	}
	if !insideCi {
		if _, err := os.Open("/.dockerenv"); err == nil {
			insideDocker = true
		}
	}

	if insideCi {
		dataSource = fmt.Sprintf("postgres://postgres@localhost/%s?sslmode=disable", dbName)
	} else if insideDocker {
		dataSource = fmt.Sprintf("postgres://dendrite:itsasecret@postgres/%s?sslmode=disable", dbName)
	} else {
		dataSource = fmt.Sprintf("postgres://dendrite:itsasecret@localhost:15432/%s?sslmode=disable", dbName)
	}

	if insideCi {
		database := "dendrite_device"
		cmd := exec.Command("psql", "postgres")
		cmd.Stdin = strings.NewReader(
			fmt.Sprintf("DROP DATABASE IF EXISTS %s; CREATE DATABASE %s;", database, database),
		)
		// Send stdout and stderr to our stderr so that we see error messages from
		// the psql process
		cmd.Stdout = os.Stderr
		cmd.Stderr = os.Stderr
		_ = cmd.Run()
	}
}

type deviceSpec struct {
	localPart   string
	devID       string
	accessToken string
	displayName string
}

func TestDatabase_GetDevicesByLocalpart(t *testing.T) {
	dropTable(dataSource)
	db, err := NewDatabase(dataSource, "localhost")
	assert.Nil(t, err)

	devSpec := deviceSpec{
		localPart:   "get-device-test-local-part",
		devID:       "get-device-test-device-id",
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
		devID:       "create-test-device-id",
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
	db, err := NewDatabase(dataSource, "localhost")
	if err != nil {
		fmt.Println(err)
		return nil, err
	}

	for i := 0; i < count; i++ {
		devID := fmt.Sprintf("%s%d", devSpecScheme.devID, i)
		displayName := fmt.Sprintf("%s%d", devSpecScheme.displayName, i)
		if device, err := db.CreateDevice(
			context.Background(),
			fmt.Sprintf("%s%d", devSpecScheme.localPart, i),
			&devID,
			fmt.Sprintf("%s%d", devSpecScheme.accessToken, i),
			&displayName); err != nil {
			fmt.Println(err)
		} else {
			devices = append(devices, device)
		}
	}
	return devices, nil
}

func dropTable(dataSource string) {
	if db, err := sql.Open("postgres", dataSource); err == nil && db != nil {
		_, _ = db.Exec("DROP TABLE device_devices;")
	} else {
		panic("Error! Unable to refresh the database!")
	}
}
