// Copyright 2024 New Vector Ltd.
// Copyright 2020 The Matrix.org Foundation C.I.C.
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

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

	"github.com/element-hq/dendrite/internal/caching"
	"github.com/element-hq/dendrite/internal/sqlutil"
	"github.com/element-hq/dendrite/setup/jetstream"
	"github.com/element-hq/dendrite/setup/process"
	"github.com/getsentry/sentry-go"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/gomatrixserverlib/spec"

	"github.com/element-hq/dendrite/appservice"
	"github.com/element-hq/dendrite/cmd/dendrite-demo-yggdrasil/embed"
	"github.com/element-hq/dendrite/cmd/dendrite-demo-yggdrasil/signing"
	"github.com/element-hq/dendrite/cmd/dendrite-demo-yggdrasil/yggconn"
	"github.com/element-hq/dendrite/cmd/dendrite-demo-yggdrasil/yggrooms"
	"github.com/element-hq/dendrite/federationapi"
	"github.com/element-hq/dendrite/federationapi/api"
	"github.com/element-hq/dendrite/internal"
	"github.com/element-hq/dendrite/internal/httputil"
	"github.com/element-hq/dendrite/roomserver"
	"github.com/element-hq/dendrite/setup"
	basepkg "github.com/element-hq/dendrite/setup/base"
	"github.com/element-hq/dendrite/setup/config"
	"github.com/element-hq/dendrite/setup/mscs"
	"github.com/element-hq/dendrite/test"
	"github.com/element-hq/dendrite/userapi"
	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"
)

var (
	instanceName   = flag.String("name", "dendrite-p2p-ygg", "the name of this P2P demo instance")
	instancePort   = flag.Int("port", 8008, "the port that the client API will listen on")
	instancePeer   = flag.String("peer", "", "the static Yggdrasil peers to connect to, comma separated-list")
	instanceListen = flag.String("listen", "tls://:0", "the port Yggdrasil peers can connect to")
	instanceDir    = flag.String("dir", ".", "the directory to store the databases in (if --config not specified)")
)

// nolint: gocyclo
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
			Generate:       true,
			SingleDatabase: true,
		})
		cfg.Global.PrivateKey = sk
		cfg.Global.JetStream.StoragePath = config.Path(filepath.Join(*instanceDir, *instanceName))
		cfg.UserAPI.AccountDatabase.ConnectionString = config.DataSource(fmt.Sprintf("file:%s-account.db", filepath.Join(*instanceDir, *instanceName)))
		cfg.MediaAPI.Database.ConnectionString = config.DataSource(fmt.Sprintf("file:%s-mediaapi.db", filepath.Join(*instanceDir, *instanceName)))
		cfg.SyncAPI.Database.ConnectionString = config.DataSource(fmt.Sprintf("file:%s-syncapi.db", filepath.Join(*instanceDir, *instanceName)))
		cfg.RoomServer.Database.ConnectionString = config.DataSource(fmt.Sprintf("file:%s-roomserver.db", filepath.Join(*instanceDir, *instanceName)))
		cfg.KeyServer.Database.ConnectionString = config.DataSource(fmt.Sprintf("file:%s-keyserver.db", filepath.Join(*instanceDir, *instanceName)))
		cfg.FederationAPI.Database.ConnectionString = config.DataSource(fmt.Sprintf("file:%s-federationapi.db", filepath.Join(*instanceDir, *instanceName)))
		cfg.RelayAPI.Database.ConnectionString = config.DataSource(fmt.Sprintf("file:%s-relayapi.db", filepath.Join(*instanceDir, *instanceName)))
		cfg.MSCs.MSCs = []string{"msc2836"}
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

	cfg.Global.ServerName = spec.ServerName(hex.EncodeToString(pk))
	cfg.Global.KeyID = gomatrixserverlib.KeyID(signing.KeyID)

	configErrors := &config.ConfigErrors{}
	cfg.Verify(configErrors)
	if len(*configErrors) > 0 {
		for _, err := range *configErrors {
			logrus.Errorf("Configuration error: %s", err)
		}
		logrus.Fatalf("Failed to start due to configuration errors")
	}

	internal.SetupStdLogging()
	internal.SetupHookLogging(cfg.Logging)
	internal.SetupPprof()

	logrus.Infof("Dendrite version %s", internal.VersionString())

	if !cfg.ClientAPI.RegistrationDisabled && cfg.ClientAPI.OpenRegistrationWithoutVerificationEnabled {
		logrus.Warn("Open registration is enabled")
	}

	closer, err := cfg.SetupTracing()
	if err != nil {
		logrus.WithError(err).Panicf("failed to start opentracing")
	}
	defer closer.Close() // nolint: errcheck

	if cfg.Global.Sentry.Enabled {
		logrus.Info("Setting up Sentry for debugging...")
		err = sentry.Init(sentry.ClientOptions{
			Dsn:              cfg.Global.Sentry.DSN,
			Environment:      cfg.Global.Sentry.Environment,
			Debug:            true,
			ServerName:       string(cfg.Global.ServerName),
			Release:          "dendrite@" + internal.VersionString(),
			AttachStacktrace: true,
		})
		if err != nil {
			logrus.WithError(err).Panic("failed to start Sentry")
		}
	}

	processCtx := process.NewProcessContext()
	cm := sqlutil.NewConnectionManager(processCtx, cfg.Global.DatabaseOptions)
	routers := httputil.NewRouters()

	basepkg.ConfigureAdminEndpoints(processCtx, routers)
	defer func() {
		processCtx.ShutdownDendrite()
		processCtx.WaitForShutdown()
	}() // nolint: errcheck

	ygg, err := yggconn.Setup(sk, *instanceName, ".", *instancePeer, *instanceListen)
	if err != nil {
		panic(err)
	}

	federation := ygg.CreateFederationClient(cfg)

	serverKeyAPI := &signing.YggdrasilKeys{}
	keyRing := serverKeyAPI.KeyRing()

	caches := caching.NewRistrettoCache(cfg.Global.Cache.EstimatedMaxSize, cfg.Global.Cache.MaxAge, caching.EnableMetrics)
	natsInstance := jetstream.NATSInstance{}
	rsAPI := roomserver.NewInternalAPI(processCtx, cfg, cm, &natsInstance, caches, caching.EnableMetrics)

	fsAPI := federationapi.NewInternalAPI(
		processCtx, cfg, cm, &natsInstance, federation, rsAPI, caches, keyRing, true,
	)
	rsAPI.SetFederationAPI(fsAPI, keyRing)

	userAPI := userapi.NewInternalAPI(processCtx, cfg, cm, &natsInstance, rsAPI, federation, caching.EnableMetrics, fsAPI.IsBlacklistedOrBackingOff)

	asAPI := appservice.NewInternalAPI(processCtx, cfg, &natsInstance, userAPI, rsAPI)
	rsAPI.SetAppserviceAPI(asAPI)

	monolith := setup.Monolith{
		Config:    cfg,
		Client:    ygg.CreateClient(),
		FedClient: federation,
		KeyRing:   keyRing,

		AppserviceAPI: asAPI,
		FederationAPI: fsAPI,
		RoomserverAPI: rsAPI,
		UserAPI:       userAPI,
		ExtPublicRoomsProvider: yggrooms.NewYggdrasilRoomProvider(
			ygg, fsAPI, federation,
		),
	}
	monolith.AddAllPublicRoutes(processCtx, cfg, routers, cm, &natsInstance, caches, caching.EnableMetrics)
	if err := mscs.Enable(cfg, cm, routers, &monolith, caches); err != nil {
		logrus.WithError(err).Fatalf("Failed to enable MSCs")
	}

	httpRouter := mux.NewRouter().SkipClean(true).UseEncodedPath()
	httpRouter.PathPrefix(httputil.PublicClientPathPrefix).Handler(routers.Client)
	httpRouter.PathPrefix(httputil.PublicMediaPathPrefix).Handler(routers.Media)
	httpRouter.PathPrefix(httputil.DendriteAdminPathPrefix).Handler(routers.DendriteAdmin)
	httpRouter.PathPrefix(httputil.SynapseAdminPathPrefix).Handler(routers.SynapseAdmin)
	embed.Embed(httpRouter, *instancePort, "Yggdrasil Demo")

	yggRouter := mux.NewRouter().SkipClean(true).UseEncodedPath()
	yggRouter.PathPrefix(httputil.PublicFederationPathPrefix).Handler(routers.Federation)
	yggRouter.PathPrefix(httputil.PublicMediaPathPrefix).Handler(routers.Media)

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
	basepkg.WaitForShutdown(processCtx)
}
