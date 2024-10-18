// Copyright 2024 New Vector Ltd.
// Copyright 2022 The Matrix.org Foundation C.I.C.
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

package main

import (
	"crypto/ed25519"
	"encoding/hex"
	"flag"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"

	"github.com/element-hq/dendrite/cmd/dendrite-demo-pinecone/monolith"
	"github.com/element-hq/dendrite/cmd/dendrite-demo-yggdrasil/signing"
	"github.com/element-hq/dendrite/internal"
	"github.com/element-hq/dendrite/internal/httputil"
	"github.com/element-hq/dendrite/internal/sqlutil"
	"github.com/element-hq/dendrite/setup"
	"github.com/element-hq/dendrite/setup/config"
	"github.com/element-hq/dendrite/setup/process"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/gomatrixserverlib/spec"
	"github.com/sirupsen/logrus"

	pineconeRouter "github.com/matrix-org/pinecone/router"
)

var (
	instanceName            = flag.String("name", "dendrite-p2p-pinecone", "the name of this P2P demo instance")
	instancePort            = flag.Int("port", 8008, "the port that the client API will listen on")
	instancePeer            = flag.String("peer", "", "the static Pinecone peers to connect to, comma separated-list")
	instanceListen          = flag.String("listen", ":0", "the port Pinecone peers can connect to")
	instanceDir             = flag.String("dir", ".", "the directory to store the databases in (if --config not specified)")
	instanceRelayingEnabled = flag.Bool("relay", false, "whether to enable store & forward relaying for other nodes")
)

func main() {
	flag.Parse()
	internal.SetupPprof()

	var pk ed25519.PublicKey
	var sk ed25519.PrivateKey

	// iterate through the cli args and check if the config flag was set
	configFlagSet := false
	for _, arg := range os.Args {
		if arg == "--config" || arg == "-config" {
			configFlagSet = true
			break
		}
	}

	var cfg *config.Dendrite

	// use custom config if config flag is set
	if configFlagSet {
		cfg = setup.ParseFlags(true)
		sk = cfg.Global.PrivateKey
		pk = sk.Public().(ed25519.PublicKey)
	} else {
		keyfile := filepath.Join(*instanceDir, *instanceName) + ".pem"
		oldKeyfile := *instanceName + ".key"
		sk, pk = monolith.GetOrCreateKey(keyfile, oldKeyfile)
		cfg = monolith.GenerateDefaultConfig(sk, *instanceDir, *instanceDir, *instanceName)
	}

	cfg.Global.ServerName = spec.ServerName(hex.EncodeToString(pk))
	cfg.Global.KeyID = gomatrixserverlib.KeyID(signing.KeyID)

	p2pMonolith := monolith.P2PMonolith{}
	p2pMonolith.SetupPinecone(sk)
	p2pMonolith.Multicast.Start()

	if instancePeer != nil && *instancePeer != "" {
		for _, peer := range strings.Split(*instancePeer, ",") {
			p2pMonolith.ConnManager.AddPeer(strings.Trim(peer, " \t\r\n"))
		}
	}

	processCtx := process.NewProcessContext()
	cm := sqlutil.NewConnectionManager(processCtx, cfg.Global.DatabaseOptions)
	routers := httputil.NewRouters()

	enableMetrics := true
	enableWebsockets := true
	p2pMonolith.SetupDendrite(processCtx, cfg, cm, routers, *instancePort, *instanceRelayingEnabled, enableMetrics, enableWebsockets)
	p2pMonolith.StartMonolith()
	p2pMonolith.WaitForShutdown()

	go func() {
		listener, err := net.Listen("tcp", *instanceListen)
		if err != nil {
			panic(err)
		}

		fmt.Println("Listening on", listener.Addr())

		for {
			conn, err := listener.Accept()
			if err != nil {
				logrus.WithError(err).Error("listener.Accept failed")
				continue
			}

			port, err := p2pMonolith.Router.Connect(
				conn,
				pineconeRouter.ConnectionPeerType(pineconeRouter.PeerTypeRemote),
			)
			if err != nil {
				logrus.WithError(err).Error("pSwitch.Connect failed")
				continue
			}

			fmt.Println("Inbound connection", conn.RemoteAddr(), "is connected to port", port)
		}
	}()
}
