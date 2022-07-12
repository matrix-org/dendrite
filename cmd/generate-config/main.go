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
	defaultsForCI := flag.Bool("ci", false, "Populate the configuration with sane defaults for use in CI")
	serverName := flag.String("server", "", "The domain name of the server if not 'localhost'")
	dbURI := flag.String("db", "", "The DB URI to use for all components if not SQLite files")
	normalise := flag.String("normalise", "", "Normalise a given configuration file")
	polylith := flag.Bool("polylith", false, "Generate a config that makes sense for polylith deployments")
	flag.Parse()

	var cfg *config.Dendrite
	if *normalise == "" {
		cfg = &config.Dendrite{
			Version: config.Version,
		}
		cfg.Defaults(true, !*polylith)
	} else {
		var err error
		if cfg, err = config.Load(*normalise, !*polylith); err != nil {
			panic(err)
		}
	}
	if *serverName != "" {
		cfg.Global.ServerName = gomatrixserverlib.ServerName(*serverName)
	}
	if *dbURI != "" {
		if *polylith {
			cfg.AppServiceAPI.Database.ConnectionString = config.DataSource(*dbURI)
			cfg.FederationAPI.Database.ConnectionString = config.DataSource(*dbURI)
			cfg.KeyServer.Database.ConnectionString = config.DataSource(*dbURI)
			cfg.MSCs.Database.ConnectionString = config.DataSource(*dbURI)
			cfg.MediaAPI.Database.ConnectionString = config.DataSource(*dbURI)
			cfg.RoomServer.Database.ConnectionString = config.DataSource(*dbURI)
			cfg.SyncAPI.Database.ConnectionString = config.DataSource(*dbURI)
			cfg.UserAPI.AccountDatabase.ConnectionString = config.DataSource(*dbURI)
		} else {
			cfg.Global.DatabaseOptions.ConnectionString = config.DataSource(*dbURI)
		}
	}
	if *defaultsForCI {
		cfg.AppServiceAPI.DisableTLSValidation = true
		cfg.ClientAPI.RateLimiting.Enabled = false
		cfg.FederationAPI.DisableTLSValidation = false
		// don't hit matrix.org when running tests!!!
		cfg.FederationAPI.KeyPerspectives = config.KeyPerspectives{}
		cfg.MSCs.MSCs = []string{"msc2836", "msc2946", "msc2444", "msc2753"}
		cfg.Logging[0].Level = "trace"
		cfg.Logging[0].Type = "std"
		cfg.UserAPI.BCryptCost = bcrypt.MinCost
		cfg.Global.JetStream.InMemory = true
		cfg.ClientAPI.RegistrationDisabled = false
		cfg.ClientAPI.OpenRegistrationWithoutVerificationEnabled = true
		cfg.ClientAPI.RegistrationSharedSecret = "complement"
		cfg.Global.Presence = config.PresenceOptions{
			EnableInbound:  true,
			EnableOutbound: true,
		}
	}

	j, err := yaml.Marshal(cfg)
	if err != nil {
		panic(err)
	}

	fmt.Println(string(j))
}
