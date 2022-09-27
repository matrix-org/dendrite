package main

import (
	"flag"
	"fmt"
	"path/filepath"

	"github.com/matrix-org/gomatrixserverlib"
	"golang.org/x/crypto/bcrypt"
	"gopkg.in/yaml.v2"

	"github.com/matrix-org/dendrite/setup/config"
)

func main() {
	defaultsForCI := flag.Bool("ci", false, "Populate the configuration with sane defaults for use in CI")
	serverName := flag.String("server", "", "The domain name of the server if not 'localhost'")
	dbURI := flag.String("db", "", "The DB URI to use for all components (PostgreSQL only)")
	dirPath := flag.String("dir", "./", "The folder to use for paths (like SQLite databases, media storage)")
	normalise := flag.String("normalise", "", "Normalise an existing configuration file by adding new/missing options and defaults")
	polylith := flag.Bool("polylith", false, "Generate a config that makes sense for polylith deployments")
	flag.Parse()

	var cfg *config.Dendrite
	if *normalise == "" {
		cfg = &config.Dendrite{
			Version: config.Version,
		}
		cfg.Defaults(config.DefaultOpts{
			Generate:   true,
			Monolithic: !*polylith,
		})
		if *serverName != "" {
			cfg.Global.ServerName = gomatrixserverlib.ServerName(*serverName)
		}
		uri := config.DataSource(*dbURI)
		if *polylith || uri.IsSQLite() || uri == "" {
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
					path := filepath.Join(*dirPath, fmt.Sprintf("dendrite_%s.db", name))
					db.ConnectionString = config.DataSource(fmt.Sprintf("file:%s", path))
				} else {
					db.ConnectionString = uri
				}
			}
		} else {
			cfg.Global.DatabaseOptions.ConnectionString = uri
		}
		cfg.Logging = []config.LogrusHook{
			{
				Type:  "file",
				Level: "info",
				Params: map[string]interface{}{
					"path": filepath.Join(*dirPath, "log"),
				},
			},
		}
		if *defaultsForCI {
			cfg.AppServiceAPI.DisableTLSValidation = true
			cfg.ClientAPI.RateLimiting.Enabled = false
			cfg.FederationAPI.DisableTLSValidation = false
			// don't hit matrix.org when running tests!!!
			cfg.FederationAPI.KeyPerspectives = config.KeyPerspectives{}
			cfg.MediaAPI.BasePath = config.Path(filepath.Join(*dirPath, "media"))
			cfg.MSCs.MSCs = []string{"msc2836", "msc2946", "msc2444", "msc2753"}
			cfg.Logging[0].Level = "trace"
			cfg.Logging[0].Type = "std"
			cfg.UserAPI.BCryptCost = bcrypt.MinCost
			cfg.Global.JetStream.InMemory = true
			cfg.Global.JetStream.StoragePath = config.Path(*dirPath)
			cfg.ClientAPI.RegistrationDisabled = false
			cfg.ClientAPI.OpenRegistrationWithoutVerificationEnabled = true
			cfg.ClientAPI.RegistrationSharedSecret = "complement"
			cfg.Global.Presence = config.PresenceOptions{
				EnableInbound:  true,
				EnableOutbound: true,
			}
			cfg.SyncAPI.Fulltext = config.Fulltext{
				Enabled:   true,
				IndexPath: config.Path(filepath.Join(*dirPath, "searchindex")),
				InMemory:  true,
				Language:  "en",
			}
		}
	} else {
		var err error
		if cfg, err = config.Load(*normalise, !*polylith); err != nil {
			panic(err)
		}
	}

	j, err := yaml.Marshal(cfg)
	if err != nil {
		panic(err)
	}

	fmt.Println(string(j))
}
