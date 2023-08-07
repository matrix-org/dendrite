package config

import (
	"fmt"

	"github.com/matrix-org/gomatrixserverlib"
	log "github.com/sirupsen/logrus"
)

type RoomServer struct {
	Matrix *Global `yaml:"-"`

	DefaultRoomVersion gomatrixserverlib.RoomVersion `yaml:"default_room_version,omitempty"`

	Database DatabaseOptions `yaml:"database,omitempty"`
}

func (c *RoomServer) Defaults(opts DefaultOpts) {
	c.DefaultRoomVersion = DefaultForDefaultRoomVersion()
	if opts.Generate {
		if !opts.SingleDatabase {
			c.Database.ConnectionString = "file:roomserver.db"
		}
	}
}

func (c *RoomServer) Verify(configErrs *ConfigErrors) {
	if c.Matrix.DatabaseOptions.ConnectionString == "" {
		checkNotEmpty(configErrs, "room_server.database.connection_string", string(c.Database.ConnectionString))
	}

	if !gomatrixserverlib.KnownRoomVersion(c.DefaultRoomVersion) {
		configErrs.Add(fmt.Sprintf("invalid value for config key 'room_server.default_room_version': unsupported room version: %q", c.DefaultRoomVersion))
	} else if !gomatrixserverlib.StableRoomVersion(c.DefaultRoomVersion) {
		log.Warnf("WARNING: Provided default room version %q is unstable", c.DefaultRoomVersion)
	}
}

// Returns the value that is the default for the room_server.default_room_version config key
//
// Do not use this if you want the default room version, use roomserverAPI.DefaultRoomVersion instead.
// This function exists for easier test writing.
func DefaultForDefaultRoomVersion() gomatrixserverlib.RoomVersion {
	return gomatrixserverlib.RoomVersionV10
}
