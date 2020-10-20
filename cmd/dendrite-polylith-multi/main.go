package main

import (
	"flag"
	"os"
	"strings"

	"github.com/matrix-org/dendrite/cmd/dendrite-polylith-multi/personalities"
	"github.com/matrix-org/dendrite/internal/config"
	"github.com/matrix-org/dendrite/internal/setup"
	"github.com/sirupsen/logrus"
)

type starter func(base *setup.BaseDendrite, cfg *config.Dendrite)

// nolint:gocyclo
func main() {
	flag.Parse()

	component := flag.Arg(0)
	components := map[string]starter{
		"appservice":       personalities.Appservice,
		"clientapi":        personalities.ClientAPI,
		"eduserver":        personalities.EDUServer,
		"federationapi":    personalities.FederationAPI,
		"federationsender": personalities.FederationSender,
		"keyserver":        personalities.KeyServer,
		"mediaapi":         personalities.MediaAPI,
		"roomserver":       personalities.RoomServer,
		"signingkeyserver": personalities.SigningKeyServer,
		"syncapi":          personalities.SyncAPI,
		"userapi":          personalities.UserAPI,
	}

	start, ok := components[component]
	if !ok {
		logrus.Errorf("Unknown component %q specified", component)

		var list []string
		for c := range components {
			list = append(list, c)
		}

		logrus.Infof("Valid components: %s", strings.Join(list, ", "))
		os.Exit(1)
	}

	logrus.Infof("Starting %q component", component)

	cfg := setup.ParseFlags(true)

	base := setup.NewBaseDendrite(cfg, component, false) // TODO
	defer base.Close()                                   // nolint: errcheck

	start(base, cfg)
}
