package main

import (
	"flag"
	"fmt"

	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/gomatrixserverlib"
	"golang.org/x/crypto/bcrypt"
	"gopkg.in/yaml.v2"
)

func main() {
	defaultsForCI := flag.Bool("ci", false, "sane defaults for CI testing")
	serverName := flag.String("server", "", "The domain name of the server if not 'localhost'")
	dbURI := flag.String("db", "", "The DB URI to use for all components if not SQLite files")
	flag.Parse()

	cfg := &config.Dendrite{
		Version: config.Version,
	}
	cfg.Defaults(true)
	if *serverName != "" {
		cfg.Global.ServerName = gomatrixserverlib.ServerName(*serverName)
	}
	if *dbURI != "" {
		cfg.AppServiceAPI.Database.ConnectionString = config.DataSource(*dbURI)
		cfg.FederationAPI.Database.ConnectionString = config.DataSource(*dbURI)
		cfg.KeyServer.Database.ConnectionString = config.DataSource(*dbURI)
		cfg.MSCs.Database.ConnectionString = config.DataSource(*dbURI)
		cfg.MediaAPI.Database.ConnectionString = config.DataSource(*dbURI)
		cfg.RoomServer.Database.ConnectionString = config.DataSource(*dbURI)
		cfg.SyncAPI.Database.ConnectionString = config.DataSource(*dbURI)
		cfg.UserAPI.AccountDatabase.ConnectionString = config.DataSource(*dbURI)
		cfg.UserAPI.DeviceDatabase.ConnectionString = config.DataSource(*dbURI)
	}
	cfg.Global.TrustedIDServers = []string{
		"matrix.org",
		"vector.im",
	}
	cfg.Logging = []config.LogrusHook{
		{
			Type:  "file",
			Level: "info",
			Params: map[string]interface{}{
				"path": "/var/log/dendrite",
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

	if *defaultsForCI {
		cfg.AppServiceAPI.DisableTLSValidation = true
		cfg.ClientAPI.RateLimiting.Enabled = false
		cfg.FederationAPI.DisableTLSValidation = true
		// don't hit matrix.org when running tests!!!
		cfg.FederationAPI.KeyPerspectives = config.KeyPerspectives{}
		cfg.MSCs.MSCs = []string{"msc2836", "msc2946", "msc2444", "msc2753"}
		cfg.Logging[0].Level = "trace"
		cfg.UserAPI.BCryptCost = bcrypt.MinCost
		cfg.Global.JetStream.InMemory = true
	}

	j, err := yaml.Marshal(cfg)
	if err != nil {
		panic(err)
	}

	fmt.Println(string(j))
}
