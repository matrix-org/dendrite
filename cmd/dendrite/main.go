package main

import (
	"flag"
	"fmt"

	"github.com/matrix-org/dendrite/cmd/dendrite/personalities"
	"github.com/matrix-org/dendrite/internal/config"
	"github.com/matrix-org/dendrite/internal/setup"
)

type starter func(base *setup.BaseDendrite, cfg *config.Dendrite)

// nolint:gocyclo
func main() {
	cfg := setup.ParseFlags(true)
	component := flag.Arg(0)

	base := setup.NewBaseDendrite(cfg, component, false) // TODO
	defer base.Close()                                   // nolint: errcheck

	components := map[string]starter{
		"appservice":       personalities.Appservice,
		"clientapi":        personalities.ClientAPI,
		"eduserver":        personalities.EDUServer,
		"federationapi":    personalities.FederationAPI,
		"federationsender": personalities.FederationSender,
		"keyserver":        personalities.KeyServer,
		"mediaapi":         personalities.MediaAPI,
		"monolith":         personalities.Monolith,
		"roomserver":       personalities.RoomServer,
		"signingkeyserver": personalities.SigningKeyServer,
		"syncapi":          personalities.SyncAPI,
		"userapi":          personalities.UserAPI,
	}

	if start, ok := components[component]; ok {
		start(base, cfg)
	} else {
		fmt.Printf("dendrite: unknown component %q\n", component)
		fmt.Println("valid components:")
		for c := range components {
			fmt.Printf("- %s\n", c)
		}
	}
}
