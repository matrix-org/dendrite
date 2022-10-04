package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/gomatrixserverlib"
	"golang.org/x/crypto/bcrypt"
	"gopkg.in/yaml.v2"
)

func main() {
	cfg, err := buildConfig(flag.CommandLine, os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	bs, err := yaml.Marshal(cfg)
	if err != nil {
		panic(err)
	}

	fmt.Println(string(bs))
}

func buildConfig(fs *flag.FlagSet, args []string) (*config.Dendrite, error) {
	defaultsForCI := fs.Bool("ci", false, "Populate the configuration with sane defaults for use in CI")
	serverName := fs.String("server", "", "The domain name of the server if not 'localhost'")
	dbURI := fs.String("db", "", "The DB URI to use for all components (PostgreSQL only)")
	dirPath := fs.String("dir", "./", "The folder to use for paths (like SQLite databases, media storage)")
	normalise := fs.String("normalise", "", "Normalise an existing configuration file by adding new/missing options and defaults")
	polylith := fs.Bool("polylith", false, "Generate a config that makes sense for polylith deployments")

	if err := fs.Parse(args); err != nil {
		return nil, err
	}

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
			cfg.ClientAPI.Login.SSO.Enabled = true
			cfg.ClientAPI.Login.SSO.Providers = []config.IdentityProvider{
				{
					Brand: "github",
					OAuth2: config.OAuth2{
						ClientID:     "aclientid",
						ClientSecret: "aclientsecret",
					},
				},
				{
					Brand: "google",
					OIDC: config.OIDC{
						OAuth2: config.OAuth2{
							ClientID:     "aclientid",
							ClientSecret: "aclientsecret",
						},
						DiscoveryURL: "https://accounts.google.com/.well-known/openid-configuration",
					},
				},
			}
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
			if *polylith {
				cfg.Global.JetStream.Addresses = []string{"localhost"}
			}
			cfg.ClientAPI.RegistrationDisabled = false
			cfg.ClientAPI.OpenRegistrationWithoutVerificationEnabled = true
			cfg.ClientAPI.RegistrationSharedSecret = "complement"
			cfg.Global.Presence = config.PresenceOptions{
				EnableInbound:  true,
				EnableOutbound: true,
			}
		}
	} else {
		var err error
		if cfg, err = config.Load(*normalise, !*polylith); err != nil {
			return nil, err
		}
	}

	return cfg, nil
}
