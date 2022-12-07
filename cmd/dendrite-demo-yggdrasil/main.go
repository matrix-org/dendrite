// Copyright 2020 The Matrix.org Foundation C.I.C.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"context"
	"crypto/ed25519"
	"crypto/tls"
	"encoding/hex"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/matrix-org/gomatrixserverlib"

	"github.com/gorilla/mux"
	"github.com/matrix-org/dendrite/appservice"
	"github.com/matrix-org/dendrite/cmd/dendrite-demo-yggdrasil/embed"
	"github.com/matrix-org/dendrite/cmd/dendrite-demo-yggdrasil/signing"
	"github.com/matrix-org/dendrite/cmd/dendrite-demo-yggdrasil/yggconn"
	"github.com/matrix-org/dendrite/cmd/dendrite-demo-yggdrasil/yggrooms"
	"github.com/matrix-org/dendrite/federationapi"
	"github.com/matrix-org/dendrite/federationapi/api"
	"github.com/matrix-org/dendrite/internal"
	"github.com/matrix-org/dendrite/internal/httputil"
	"github.com/matrix-org/dendrite/keyserver"
	"github.com/matrix-org/dendrite/roomserver"
	"github.com/matrix-org/dendrite/setup"
	"github.com/matrix-org/dendrite/setup/base"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/dendrite/setup/mscs"
	"github.com/matrix-org/dendrite/test"
	"github.com/matrix-org/dendrite/userapi"
	"github.com/sirupsen/logrus"
)

var (
	instanceName   = flag.String("name", "dendrite-p2p-ygg", "the name of this P2P demo instance")
	instancePort   = flag.Int("port", 8008, "the port that the client API will listen on")
	instancePeer   = flag.String("peer", "", "the static Yggdrasil peers to connect to, comma separated-list")
	instanceListen = flag.String("listen", "tcp://:0", "the port Yggdrasil peers can connect to")
	instanceDir    = flag.String("dir", ".", "the directory to store the databases in (if --config not specified)")
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

	cfg := &config.Dendrite{}

	keyfile := filepath.Join(*instanceDir, *instanceName) + ".pem"
	if _, err := os.Stat(keyfile); os.IsNotExist(err) {
		oldkeyfile := *instanceName + ".key"
		if _, err = os.Stat(oldkeyfile); os.IsNotExist(err) {
			if err = test.NewMatrixKey(keyfile); err != nil {
				panic("failed to generate a new PEM key: " + err.Error())
			}
			if _, sk, err = config.LoadMatrixKey(keyfile, os.ReadFile); err != nil {
				panic("failed to load PEM key: " + err.Error())
			}
			if len(sk) != ed25519.PrivateKeySize {
				panic("the private key is not long enough")
			}
		} else {
			if sk, err = os.ReadFile(oldkeyfile); err != nil {
				panic("failed to read the old private key: " + err.Error())
			}
			if len(sk) != ed25519.PrivateKeySize {
				panic("the private key is not long enough")
			}
			if err := test.SaveMatrixKey(keyfile, sk); err != nil {
				panic("failed to convert the private key to PEM format: " + err.Error())
			}
		}
	} else {
		var err error
		if _, sk, err = config.LoadMatrixKey(keyfile, os.ReadFile); err != nil {
			panic("failed to load PEM key: " + err.Error())
		}
		if len(sk) != ed25519.PrivateKeySize {
			panic("the private key is not long enough")
		}
	}

	pk = sk.Public().(ed25519.PublicKey)

	// use custom config if config flag is set
	if configFlagSet {
		cfg = setup.ParseFlags(true)
	} else {
		cfg.Defaults(config.DefaultOpts{
			Generate:   true,
			Monolithic: true,
		})
		cfg.Global.PrivateKey = sk
		cfg.Global.JetStream.StoragePath = config.Path(filepath.Join(*instanceDir, *instanceName))
		cfg.UserAPI.AccountDatabase.ConnectionString = config.DataSource(fmt.Sprintf("file:%s-account.db", filepath.Join(*instanceDir, *instanceName)))
		cfg.MediaAPI.Database.ConnectionString = config.DataSource(fmt.Sprintf("file:%s-mediaapi.db", filepath.Join(*instanceDir, *instanceName)))
		cfg.SyncAPI.Database.ConnectionString = config.DataSource(fmt.Sprintf("file:%s-syncapi.db", filepath.Join(*instanceDir, *instanceName)))
		cfg.RoomServer.Database.ConnectionString = config.DataSource(fmt.Sprintf("file:%s-roomserver.db", filepath.Join(*instanceDir, *instanceName)))
		cfg.KeyServer.Database.ConnectionString = config.DataSource(fmt.Sprintf("file:%s-keyserver.db", filepath.Join(*instanceDir, *instanceName)))
		cfg.FederationAPI.Database.ConnectionString = config.DataSource(fmt.Sprintf("file:%s-federationapi.db", filepath.Join(*instanceDir, *instanceName)))
		cfg.MSCs.MSCs = []string{"msc2836", "msc2946"}
		cfg.MSCs.Database.ConnectionString = config.DataSource(fmt.Sprintf("file:%s-mscs.db", filepath.Join(*instanceDir, *instanceName)))
		cfg.ClientAPI.RegistrationDisabled = false
		cfg.ClientAPI.OpenRegistrationWithoutVerificationEnabled = true
		cfg.MediaAPI.BasePath = config.Path(*instanceDir)
		cfg.SyncAPI.Fulltext.Enabled = true
		cfg.SyncAPI.Fulltext.IndexPath = config.Path(*instanceDir)
		if err := cfg.Derive(); err != nil {
			panic(err)
		}
	}

	cfg.Global.ServerName = gomatrixserverlib.ServerName(hex.EncodeToString(pk))
	cfg.Global.KeyID = gomatrixserverlib.KeyID(signing.KeyID)

	base := base.NewBaseDendrite(cfg, "Monolith")
	base.ConfigureAdminEndpoints()
	defer base.Close() // nolint: errcheck

	ygg, err := yggconn.Setup(sk, *instanceName, ".", *instancePeer, *instanceListen)
	if err != nil {
		panic(err)
	}

	federation := ygg.CreateFederationClient(base)

	serverKeyAPI := &signing.YggdrasilKeys{}
	keyRing := serverKeyAPI.KeyRing()

	keyAPI := keyserver.NewInternalAPI(base, &base.Cfg.KeyServer, federation)

	rsComponent := roomserver.NewInternalAPI(
		base,
	)
	rsAPI := rsComponent

	userAPI := userapi.NewInternalAPI(base, &cfg.UserAPI, nil, keyAPI, rsAPI, base.PushGatewayHTTPClient())
	keyAPI.SetUserAPI(userAPI)

	asAPI := appservice.NewInternalAPI(base, userAPI, rsAPI)
	rsAPI.SetAppserviceAPI(asAPI)
	fsAPI := federationapi.NewInternalAPI(
		base, federation, rsAPI, base.Caches, keyRing, true,
	)

	rsComponent.SetFederationAPI(fsAPI, keyRing)

	monolith := setup.Monolith{
		Config:    base.Cfg,
		Client:    ygg.CreateClient(base),
		FedClient: federation,
		KeyRing:   keyRing,

		AppserviceAPI: asAPI,
		FederationAPI: fsAPI,
		RoomserverAPI: rsAPI,
		UserAPI:       userAPI,
		KeyAPI:        keyAPI,
		ExtPublicRoomsProvider: yggrooms.NewYggdrasilRoomProvider(
			ygg, fsAPI, federation,
		),
	}
	monolith.AddAllPublicRoutes(base)
	if err := mscs.Enable(base, &monolith); err != nil {
		logrus.WithError(err).Fatalf("Failed to enable MSCs")
	}

	httpRouter := mux.NewRouter().SkipClean(true).UseEncodedPath()
	httpRouter.PathPrefix(httputil.InternalPathPrefix).Handler(base.InternalAPIMux)
	httpRouter.PathPrefix(httputil.PublicClientPathPrefix).Handler(base.PublicClientAPIMux)
	httpRouter.PathPrefix(httputil.PublicMediaPathPrefix).Handler(base.PublicMediaAPIMux)
	httpRouter.PathPrefix(httputil.DendriteAdminPathPrefix).Handler(base.DendriteAdminMux)
	httpRouter.PathPrefix(httputil.SynapseAdminPathPrefix).Handler(base.SynapseAdminMux)
	embed.Embed(httpRouter, *instancePort, "Yggdrasil Demo")

	yggRouter := mux.NewRouter().SkipClean(true).UseEncodedPath()
	yggRouter.PathPrefix(httputil.PublicFederationPathPrefix).Handler(base.PublicFederationAPIMux)
	yggRouter.PathPrefix(httputil.PublicMediaPathPrefix).Handler(base.PublicMediaAPIMux)

	// Build both ends of a HTTP multiplex.
	httpServer := &http.Server{
		Addr:         ":0",
		TLSNextProto: map[string]func(*http.Server, *tls.Conn, http.Handler){},
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  30 * time.Second,
		BaseContext: func(_ net.Listener) context.Context {
			return context.Background()
		},
		Handler: yggRouter,
	}

	go func() {
		logrus.Info("Listening on ", ygg.DerivedServerName())
		logrus.Fatal(httpServer.Serve(ygg))
	}()
	go func() {
		httpBindAddr := fmt.Sprintf(":%d", *instancePort)
		logrus.Info("Listening on ", httpBindAddr)
		logrus.Fatal(http.ListenAndServe(httpBindAddr, httpRouter))
	}()
	go func() {
		logrus.Info("Sending wake-up message to known nodes")
		req := &api.PerformBroadcastEDURequest{}
		res := &api.PerformBroadcastEDUResponse{}
		if err := fsAPI.PerformBroadcastEDU(context.TODO(), req, res); err != nil {
			logrus.WithError(err).Error("Failed to send wake-up message to known nodes")
		}
	}()

	// We want to block forever to let the HTTP and HTTPS handler serve the APIs
	base.WaitForShutdown()
}
