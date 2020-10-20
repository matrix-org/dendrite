package personalities

import (
	"github.com/matrix-org/dendrite/appservice"
	"github.com/matrix-org/dendrite/eduserver"
	"github.com/matrix-org/dendrite/eduserver/cache"
	"github.com/matrix-org/dendrite/federationsender"
	"github.com/matrix-org/dendrite/internal/config"
	"github.com/matrix-org/dendrite/internal/setup"
	"github.com/matrix-org/dendrite/keyserver"
	"github.com/matrix-org/dendrite/roomserver"
	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/signingkeyserver"
	"github.com/matrix-org/dendrite/userapi"
	"github.com/sirupsen/logrus"
)

func Monolith(base *setup.BaseDendrite, cfg *config.Dendrite) {
	httpAddr := config.HTTPAddress("http://" + *httpBindAddr)
	httpsAddr := config.HTTPAddress("https://" + *httpsBindAddr)
	httpAPIAddr := httpAddr

	if *enableHTTPAPIs {
		logrus.Warnf("DANGER! The -api option is enabled, exposing internal APIs on %q!", *apiBindAddr)
		httpAPIAddr = config.HTTPAddress("http://" + *apiBindAddr)
		// If the HTTP APIs are enabled then we need to update the Listen
		// statements in the configuration so that we know where to find
		// the API endpoints. They'll listen on the same port as the monolith
		// itself.
		cfg.AppServiceAPI.InternalAPI.Connect = httpAPIAddr
		cfg.ClientAPI.InternalAPI.Connect = httpAPIAddr
		cfg.EDUServer.InternalAPI.Connect = httpAPIAddr
		cfg.FederationAPI.InternalAPI.Connect = httpAPIAddr
		cfg.FederationSender.InternalAPI.Connect = httpAPIAddr
		cfg.KeyServer.InternalAPI.Connect = httpAPIAddr
		cfg.MediaAPI.InternalAPI.Connect = httpAPIAddr
		cfg.RoomServer.InternalAPI.Connect = httpAPIAddr
		cfg.SigningKeyServer.InternalAPI.Connect = httpAPIAddr
		cfg.SyncAPI.InternalAPI.Connect = httpAPIAddr
	}

	base := setup.NewBaseDendrite(cfg, "Monolith", *enableHTTPAPIs)
	defer base.Close() // nolint: errcheck

	accountDB := base.CreateAccountsDB()
	federation := base.CreateFederationClient()

	skAPI := signingkeyserver.NewInternalAPI(
		&base.Cfg.SigningKeyServer, federation, base.Caches,
	)
	if base.UseHTTPAPIs {
		signingkeyserver.AddInternalRoutes(base.InternalAPIMux, skAPI, base.Caches)
		skAPI = base.SigningKeyServerHTTPClient()
	}
	keyRing := skAPI.KeyRing()

	rsImpl := roomserver.NewInternalAPI(
		base, keyRing,
	)
	// call functions directly on the impl unless running in HTTP mode
	rsAPI := rsImpl
	if base.UseHTTPAPIs {
		roomserver.AddInternalRoutes(base.InternalAPIMux, rsImpl)
		rsAPI = base.RoomserverHTTPClient()
	}
	if traceInternal {
		rsAPI = &api.RoomserverInternalAPITrace{
			Impl: rsAPI,
		}
	}

	fsAPI := federationsender.NewInternalAPI(
		base, federation, rsAPI, keyRing,
	)
	if base.UseHTTPAPIs {
		federationsender.AddInternalRoutes(base.InternalAPIMux, fsAPI)
		fsAPI = base.FederationSenderHTTPClient()
	}
	// The underlying roomserver implementation needs to be able to call the fedsender.
	// This is different to rsAPI which can be the http client which doesn't need this dependency
	rsImpl.SetFederationSenderAPI(fsAPI)

	keyAPI := keyserver.NewInternalAPI(&base.Cfg.KeyServer, fsAPI)
	userAPI := userapi.NewInternalAPI(accountDB, &cfg.UserAPI, cfg.Derived.ApplicationServices, keyAPI)
	keyAPI.SetUserAPI(userAPI)

	eduInputAPI := eduserver.NewInternalAPI(
		base, cache.New(), userAPI,
	)
	if base.UseHTTPAPIs {
		eduserver.AddInternalRoutes(base.InternalAPIMux, eduInputAPI)
		eduInputAPI = base.EDUServerClient()
	}

	asAPI := appservice.NewInternalAPI(base, userAPI, rsAPI)
	if base.UseHTTPAPIs {
		appservice.AddInternalRoutes(base.InternalAPIMux, asAPI)
		asAPI = base.AppserviceHTTPClient()
	}

	monolith := setup.Monolith{
		Config:    base.Cfg,
		AccountDB: accountDB,
		Client:    base.CreateClient(),
		FedClient: federation,
		KeyRing:   keyRing,

		AppserviceAPI:       asAPI,
		EDUInternalAPI:      eduInputAPI,
		FederationSenderAPI: fsAPI,
		RoomserverAPI:       rsAPI,
		ServerKeyAPI:        skAPI,
		UserAPI:             userAPI,
		KeyAPI:              keyAPI,
	}
	monolith.AddAllPublicRoutes(
		base.PublicClientAPIMux,
		base.PublicFederationAPIMux,
		base.PublicKeyAPIMux,
		base.PublicMediaAPIMux,
	)

	// Expose the matrix APIs directly rather than putting them under a /api path.
	go func() {
		base.SetupAndServeHTTP(
			httpAPIAddr, // internal API
			httpAddr,    // external API
			nil, nil,    // TLS settings
		)
	}()
	// Handle HTTPS if certificate and key are provided
	if *certFile != "" && *keyFile != "" {
		go func() {
			base.SetupAndServeHTTP(
				setup.NoListener,  // internal API
				httpsAddr,         // external API
				certFile, keyFile, // TLS settings
			)
		}()
	}

	// We want to block forever to let the HTTP and HTTPS handler serve the APIs
	select {}
}
