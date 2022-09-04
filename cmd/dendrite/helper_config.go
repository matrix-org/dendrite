package main

import (
	"fmt"
	"golang.org/x/crypto/bcrypt"
	"path/filepath"

	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/gomatrixserverlib"
)

var (
	serverName    string
	dbURI         string
	defaultsForCI = false

	cfg = &config.Dendrite{}
)

func configDefaults(generate, monolithic bool, dirPath string) {
	cfg.Version = config.Version
	cfg.Defaults(config.DefaultOpts{
		Generate:   generate,
		Monolithic: monolithic,
	})
	if generate {
		if serverName != "" {
			cfg.Global.ServerName = gomatrixserverlib.ServerName(serverName)
		}
		uri := config.DataSource(dbURI)
		if monolithic || uri.IsSQLite() || uri == "" {
			for name, db := range map[string]*config.DatabaseOptions{
				"federationapi": &cfg.FederationAPI.Database,
				"keyserver":     &cfg.KeyServer.Database,
				"mscs":          &cfg.MSCs.Database,
				"mediaapi":      &cfg.MediaAPI.Database,
				"roomserver":    &cfg.RoomServer.Database,
				"syncapi":       &cfg.SyncAPI.Database,
				"userapi":       &cfg.UserAPI.AccountDatabase,
			} {
				if uri == "" {
					path := filepath.Join(dirPath, fmt.Sprintf("dendrite_%s.db", name))
					db.ConnectionString = config.DataSource(fmt.Sprintf("file:%s", path))
				} else {
					db.ConnectionString = uri
				}
			}
		} else {
			cfg.Global.DatabaseOptions.ConnectionString = uri
		}
	}
	/* --
	cfg.Global.TrustedIDServers = []string{
		"matrix.org",
		"vector.im",
	}
	*/
	cfg.Logging = []config.LogrusHook{
		{
			Type:  "file",
			Level: "info",
			Params: map[string]interface{}{
				"path": filepath.Join(dirPath, "log"),
			},
		},
	}

	cfg.FederationAPI.KeyPerspectives = config.KeyPerspectives{
		{
			ServerName: "matrix.org",
			Keys: []config.KeyPerspectiveTrustKey{
				{
					KeyID:     "ed25519:auto",
					PublicKey: "Noi6WqcDj0QmPxCNQqgezwTlBKrfqehY1u2FyWP9uYw",
				},
				{
					KeyID:     "ed25519:a_RXGa",
					PublicKey: "l8Hft5qXKn1vfHrg3p4+W8gELQVo8N13JkluMfmn2sQ",
				},
			},
		},
	}
	cfg.MediaAPI.ThumbnailSizes = []config.ThumbnailSize{
		{
			Width:        32,
			Height:       32,
			ResizeMethod: "crop",
		},
		{
			Width:        96,
			Height:       96,
			ResizeMethod: "crop",
		},
		{
			Width:        640,
			Height:       480,
			ResizeMethod: "scale",
		},
	}
	if defaultsForCI {
		cfg.AppServiceAPI.DisableTLSValidation = true
		cfg.ClientAPI.RateLimiting.Enabled = false
		cfg.FederationAPI.DisableTLSValidation = false
		// don't hit matrix.org when running tests!!!
		cfg.FederationAPI.KeyPerspectives = config.KeyPerspectives{}
		cfg.MediaAPI.BasePath = config.Path(filepath.Join(dirPath, "media"))
		cfg.MSCs.MSCs = []string{"msc2836", "msc2946", "msc2444", "msc2753"}
		cfg.Logging[0].Level = "trace"
		cfg.Logging[0].Type = "std"
		cfg.UserAPI.BCryptCost = bcrypt.MinCost
		cfg.Global.JetStream.InMemory = true
		cfg.Global.JetStream.StoragePath = config.Path(dirPath)
		cfg.ClientAPI.RegistrationDisabled = false
		cfg.ClientAPI.OpenRegistrationWithoutVerificationEnabled = true
		cfg.ClientAPI.RegistrationSharedSecret = "complement"
		cfg.Global.Presence = config.PresenceOptions{
			EnableInbound:  true,
			EnableOutbound: true,
		}
		cfg.SyncAPI.Fulltext = config.Fulltext{
			Enabled:   true,
			IndexPath: config.Path(filepath.Join(dirPath, "searchindex")),
			InMemory:  true,
			Language:  "en",
		}
	}
}
