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

package base

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/getsentry/sentry-go"
	sentryhttp "github.com/getsentry/sentry-go/http"
	"github.com/matrix-org/dendrite/internal/caching"
	"github.com/matrix-org/dendrite/internal/httputil"
	"github.com/matrix-org/dendrite/internal/pushgateway"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/atomic"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"

	"github.com/matrix-org/dendrite/internal"
	"github.com/matrix-org/dendrite/setup/process"
	userdb "github.com/matrix-org/dendrite/userapi/storage"

	"github.com/gorilla/mux"

	appserviceAPI "github.com/matrix-org/dendrite/appservice/api"
	asinthttp "github.com/matrix-org/dendrite/appservice/inthttp"
	eduServerAPI "github.com/matrix-org/dendrite/eduserver/api"
	eduinthttp "github.com/matrix-org/dendrite/eduserver/inthttp"
	federationAPI "github.com/matrix-org/dendrite/federationapi/api"
	federationIntHTTP "github.com/matrix-org/dendrite/federationapi/inthttp"
	keyserverAPI "github.com/matrix-org/dendrite/keyserver/api"
	keyinthttp "github.com/matrix-org/dendrite/keyserver/inthttp"
	roomserverAPI "github.com/matrix-org/dendrite/roomserver/api"
	rsinthttp "github.com/matrix-org/dendrite/roomserver/inthttp"
	"github.com/matrix-org/dendrite/setup/config"
	userapi "github.com/matrix-org/dendrite/userapi/api"
	userapiinthttp "github.com/matrix-org/dendrite/userapi/inthttp"
	"github.com/sirupsen/logrus"

	_ "net/http/pprof"
)

// BaseDendrite is a base for creating new instances of dendrite. It parses
// command line flags and config, and exposes methods for creating various
// resources. All errors are handled by logging then exiting, so all methods
// should only be used during start up.
// Must be closed when shutting down.
type BaseDendrite struct {
	*process.ProcessContext
	componentName          string
	tracerCloser           io.Closer
	PublicClientAPIMux     *mux.Router
	PublicFederationAPIMux *mux.Router
	PublicKeyAPIMux        *mux.Router
	PublicMediaAPIMux      *mux.Router
	PublicWellKnownAPIMux  *mux.Router
	InternalAPIMux         *mux.Router
	SynapseAdminMux        *mux.Router
	UseHTTPAPIs            bool
	apiHttpClient          *http.Client
	Cfg                    *config.Dendrite
	Caches                 *caching.Caches
	DNSCache               *gomatrixserverlib.DNSCache
}

const NoListener = ""

const HTTPServerTimeout = time.Minute * 5
const HTTPClientTimeout = time.Second * 30

type BaseDendriteOptions int

const (
	NoCacheMetrics BaseDendriteOptions = iota
	UseHTTPAPIs
)

// NewBaseDendrite creates a new instance to be used by a component.
// The componentName is used for logging purposes, and should be a friendly name
// of the compontent running, e.g. "SyncAPI"
func NewBaseDendrite(cfg *config.Dendrite, componentName string, options ...BaseDendriteOptions) *BaseDendrite {
	useHTTPAPIs := false
	cacheMetrics := true
	for _, opt := range options {
		switch opt {
		case NoCacheMetrics:
			cacheMetrics = false
		case UseHTTPAPIs:
			useHTTPAPIs = true
		}
	}

	configErrors := &config.ConfigErrors{}
	cfg.Verify(configErrors, componentName == "Monolith") // TODO: better way?
	if len(*configErrors) > 0 {
		for _, err := range *configErrors {
			logrus.Errorf("Configuration error: %s", err)
		}
		logrus.Fatalf("Failed to start due to configuration errors")
	}

	internal.SetupStdLogging()
	internal.SetupHookLogging(cfg.Logging, componentName)
	internal.SetupPprof()

	logrus.Infof("Dendrite version %s", internal.VersionString())

	closer, err := cfg.SetupTracing("Dendrite" + componentName)
	if err != nil {
		logrus.WithError(err).Panicf("failed to start opentracing")
	}

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

	cache, err := caching.NewInMemoryLRUCache(cacheMetrics)
	if err != nil {
		logrus.WithError(err).Warnf("Failed to create cache")
	}

	var dnsCache *gomatrixserverlib.DNSCache
	if cfg.Global.DNSCache.Enabled {
		dnsCache = gomatrixserverlib.NewDNSCache(
			cfg.Global.DNSCache.CacheSize,
			cfg.Global.DNSCache.CacheLifetime,
		)
		logrus.Infof(
			"DNS cache enabled (size %d, lifetime %s)",
			cfg.Global.DNSCache.CacheSize,
			cfg.Global.DNSCache.CacheLifetime,
		)
	}

	apiClient := http.Client{
		Timeout: time.Minute * 10,
		Transport: &http2.Transport{
			AllowHTTP: true,
			DialTLS: func(network, addr string, _ *tls.Config) (net.Conn, error) {
				// Ordinarily HTTP/2 would expect TLS, but the remote listener is
				// H2C-enabled (HTTP/2 without encryption). Overriding the DialTLS
				// function with a plain Dial allows us to trick the HTTP client
				// into establishing a HTTP/2 connection without TLS.
				// TODO: Eventually we will want to look at authenticating and
				// encrypting these internal HTTP APIs, at which point we will have
				// to reconsider H2C and change all this anyway.
				return net.Dial(network, addr)
			},
		},
	}

	// Ideally we would only use SkipClean on routes which we know can allow '/' but due to
	// https://github.com/gorilla/mux/issues/460 we have to attach this at the top router.
	// When used in conjunction with UseEncodedPath() we get the behaviour we want when parsing
	// path parameters:
	// /foo/bar%2Fbaz    == [foo, bar%2Fbaz]  (from UseEncodedPath)
	// /foo/bar%2F%2Fbaz == [foo, bar%2F%2Fbaz] (from SkipClean)
	// In particular, rooms v3 event IDs are not urlsafe and can include '/' and because they
	// are randomly generated it results in flakey tests.
	// We need to be careful with media APIs if they read from a filesystem to make sure they
	// are not inadvertently reading paths without cleaning, else this could introduce a
	// directory traversal attack e.g /../../../etc/passwd

	return &BaseDendrite{
		ProcessContext:         process.NewProcessContext(),
		componentName:          componentName,
		UseHTTPAPIs:            useHTTPAPIs,
		tracerCloser:           closer,
		Cfg:                    cfg,
		Caches:                 cache,
		DNSCache:               dnsCache,
		PublicClientAPIMux:     mux.NewRouter().SkipClean(true).PathPrefix(httputil.PublicClientPathPrefix).Subrouter().UseEncodedPath(),
		PublicFederationAPIMux: mux.NewRouter().SkipClean(true).PathPrefix(httputil.PublicFederationPathPrefix).Subrouter().UseEncodedPath(),
		PublicKeyAPIMux:        mux.NewRouter().SkipClean(true).PathPrefix(httputil.PublicKeyPathPrefix).Subrouter().UseEncodedPath(),
		PublicMediaAPIMux:      mux.NewRouter().SkipClean(true).PathPrefix(httputil.PublicMediaPathPrefix).Subrouter().UseEncodedPath(),
		PublicWellKnownAPIMux:  mux.NewRouter().SkipClean(true).PathPrefix(httputil.PublicWellKnownPrefix).Subrouter().UseEncodedPath(),
		InternalAPIMux:         mux.NewRouter().SkipClean(true).PathPrefix(httputil.InternalPathPrefix).Subrouter().UseEncodedPath(),
		SynapseAdminMux:        mux.NewRouter().SkipClean(true).PathPrefix("/_synapse/").Subrouter().UseEncodedPath(),
		apiHttpClient:          &apiClient,
	}
}

// Close implements io.Closer
func (b *BaseDendrite) Close() error {
	return b.tracerCloser.Close()
}

// AppserviceHTTPClient returns the AppServiceQueryAPI for hitting the appservice component over HTTP.
func (b *BaseDendrite) AppserviceHTTPClient() appserviceAPI.AppServiceQueryAPI {
	a, err := asinthttp.NewAppserviceClient(b.Cfg.AppServiceURL(), b.apiHttpClient)
	if err != nil {
		logrus.WithError(err).Panic("CreateHTTPAppServiceAPIs failed")
	}
	return a
}

// RoomserverHTTPClient returns RoomserverInternalAPI for hitting the roomserver over HTTP.
func (b *BaseDendrite) RoomserverHTTPClient() roomserverAPI.RoomserverInternalAPI {
	rsAPI, err := rsinthttp.NewRoomserverClient(b.Cfg.RoomServerURL(), b.apiHttpClient, b.Caches)
	if err != nil {
		logrus.WithError(err).Panic("RoomserverHTTPClient failed", b.apiHttpClient)
	}
	return rsAPI
}

// UserAPIClient returns UserInternalAPI for hitting the userapi over HTTP.
func (b *BaseDendrite) UserAPIClient() userapi.UserInternalAPI {
	userAPI, err := userapiinthttp.NewUserAPIClient(b.Cfg.UserAPIURL(), b.apiHttpClient)
	if err != nil {
		logrus.WithError(err).Panic("UserAPIClient failed", b.apiHttpClient)
	}
	return userAPI
}

// EDUServerClient returns EDUServerInputAPI for hitting the EDU server over HTTP
func (b *BaseDendrite) EDUServerClient() eduServerAPI.EDUServerInputAPI {
	e, err := eduinthttp.NewEDUServerClient(b.Cfg.EDUServerURL(), b.apiHttpClient)
	if err != nil {
		logrus.WithError(err).Panic("EDUServerClient failed", b.apiHttpClient)
	}
	return e
}

// FederationAPIHTTPClient returns FederationInternalAPI for hitting
// the federation API server over HTTP
func (b *BaseDendrite) FederationAPIHTTPClient() federationAPI.FederationInternalAPI {
	f, err := federationIntHTTP.NewFederationAPIClient(b.Cfg.FederationAPIURL(), b.apiHttpClient, b.Caches)
	if err != nil {
		logrus.WithError(err).Panic("FederationAPIHTTPClient failed", b.apiHttpClient)
	}
	return f
}

// KeyServerHTTPClient returns KeyInternalAPI for hitting the key server over HTTP
func (b *BaseDendrite) KeyServerHTTPClient() keyserverAPI.KeyInternalAPI {
	f, err := keyinthttp.NewKeyServerClient(b.Cfg.KeyServerURL(), b.apiHttpClient)
	if err != nil {
		logrus.WithError(err).Panic("KeyServerHTTPClient failed", b.apiHttpClient)
	}
	return f
}

// PushGatewayHTTPClient returns a new client for interacting with (external) Push Gateways.
func (b *BaseDendrite) PushGatewayHTTPClient() pushgateway.Client {
	return pushgateway.NewHTTPClient(b.Cfg.UserAPI.PushGatewayDisableTLSValidation)
}

// CreateAccountsDB creates a new instance of the accounts database. Should only
// be called once per component.
func (b *BaseDendrite) CreateAccountsDB() userdb.Database {
	db, err := userdb.NewDatabase(
		&b.Cfg.UserAPI.AccountDatabase,
		b.Cfg.Global.ServerName,
		b.Cfg.UserAPI.BCryptCost,
		b.Cfg.UserAPI.OpenIDTokenLifetimeMS,
		userapi.DefaultLoginTokenLifetime,
	)
	if err != nil {
		logrus.WithError(err).Panicf("failed to connect to accounts db")
	}

	return db
}

// CreateClient creates a new client (normally used for media fetch requests).
// Should only be called once per component.
func (b *BaseDendrite) CreateClient() *gomatrixserverlib.Client {
	if b.Cfg.Global.DisableFederation {
		return gomatrixserverlib.NewClient(
			gomatrixserverlib.WithTransport(noOpHTTPTransport),
		)
	}
	opts := []gomatrixserverlib.ClientOption{
		gomatrixserverlib.WithSkipVerify(b.Cfg.FederationAPI.DisableTLSValidation),
	}
	if b.Cfg.Global.DNSCache.Enabled {
		opts = append(opts, gomatrixserverlib.WithDNSCache(b.DNSCache))
	}
	client := gomatrixserverlib.NewClient(opts...)
	client.SetUserAgent(fmt.Sprintf("Dendrite/%s", internal.VersionString()))
	return client
}

// CreateFederationClient creates a new federation client. Should only be called
// once per component.
func (b *BaseDendrite) CreateFederationClient() *gomatrixserverlib.FederationClient {
	if b.Cfg.Global.DisableFederation {
		return gomatrixserverlib.NewFederationClient(
			b.Cfg.Global.ServerName, b.Cfg.Global.KeyID, b.Cfg.Global.PrivateKey,
			gomatrixserverlib.WithTransport(noOpHTTPTransport),
		)
	}
	opts := []gomatrixserverlib.ClientOption{
		gomatrixserverlib.WithTimeout(time.Minute * 5),
		gomatrixserverlib.WithSkipVerify(b.Cfg.FederationAPI.DisableTLSValidation),
	}
	if b.Cfg.Global.DNSCache.Enabled {
		opts = append(opts, gomatrixserverlib.WithDNSCache(b.DNSCache))
	}
	client := gomatrixserverlib.NewFederationClient(
		b.Cfg.Global.ServerName, b.Cfg.Global.KeyID,
		b.Cfg.Global.PrivateKey, opts...,
	)
	client.SetUserAgent(fmt.Sprintf("Dendrite/%s", internal.VersionString()))
	return client
}

// SetupAndServeHTTP sets up the HTTP server to serve endpoints registered on
// ApiMux under /api/ and adds a prometheus handler under /metrics.
func (b *BaseDendrite) SetupAndServeHTTP(
	internalHTTPAddr, externalHTTPAddr config.HTTPAddress,
	certFile, keyFile *string,
) {
	internalAddr, _ := internalHTTPAddr.Address()
	externalAddr, _ := externalHTTPAddr.Address()

	externalRouter := mux.NewRouter().SkipClean(true).UseEncodedPath()
	internalRouter := externalRouter

	externalServ := &http.Server{
		Addr:         string(externalAddr),
		WriteTimeout: HTTPServerTimeout,
		Handler:      externalRouter,
	}
	internalServ := externalServ

	if internalAddr != NoListener && externalAddr != internalAddr {
		// H2C allows us to accept HTTP/2 connections without TLS
		// encryption. Since we don't currently require any form of
		// authentication or encryption on these internal HTTP APIs,
		// H2C gives us all of the advantages of HTTP/2 (such as
		// stream multiplexing and avoiding head-of-line blocking)
		// without enabling TLS.
		internalH2S := &http2.Server{}
		internalRouter = mux.NewRouter().SkipClean(true).UseEncodedPath()
		internalServ = &http.Server{
			Addr:    string(internalAddr),
			Handler: h2c.NewHandler(internalRouter, internalH2S),
		}
	}

	internalRouter.PathPrefix(httputil.InternalPathPrefix).Handler(b.InternalAPIMux)
	if b.Cfg.Global.Metrics.Enabled {
		internalRouter.Handle("/metrics", httputil.WrapHandlerInBasicAuth(promhttp.Handler(), b.Cfg.Global.Metrics.BasicAuth))
	}

	var clientHandler http.Handler
	clientHandler = b.PublicClientAPIMux
	if b.Cfg.Global.Sentry.Enabled {
		sentryHandler := sentryhttp.New(sentryhttp.Options{
			Repanic: true,
		})
		clientHandler = sentryHandler.Handle(b.PublicClientAPIMux)
	}
	var federationHandler http.Handler
	federationHandler = b.PublicFederationAPIMux
	if b.Cfg.Global.Sentry.Enabled {
		sentryHandler := sentryhttp.New(sentryhttp.Options{
			Repanic: true,
		})
		federationHandler = sentryHandler.Handle(b.PublicFederationAPIMux)
	}
	externalRouter.PathPrefix(httputil.PublicClientPathPrefix).Handler(clientHandler)
	if !b.Cfg.Global.DisableFederation {
		externalRouter.PathPrefix(httputil.PublicKeyPathPrefix).Handler(b.PublicKeyAPIMux)
		externalRouter.PathPrefix(httputil.PublicFederationPathPrefix).Handler(federationHandler)
	}
	externalRouter.PathPrefix("/_synapse/").Handler(b.SynapseAdminMux)
	externalRouter.PathPrefix(httputil.PublicMediaPathPrefix).Handler(b.PublicMediaAPIMux)
	externalRouter.PathPrefix(httputil.PublicWellKnownPrefix).Handler(b.PublicWellKnownAPIMux)

	if internalAddr != NoListener && internalAddr != externalAddr {
		go func() {
			var internalShutdown atomic.Bool // RegisterOnShutdown can be called more than once
			logrus.Infof("Starting internal %s listener on %s", b.componentName, internalServ.Addr)
			b.ProcessContext.ComponentStarted()
			internalServ.RegisterOnShutdown(func() {
				if internalShutdown.CAS(false, true) {
					b.ProcessContext.ComponentFinished()
					logrus.Infof("Stopped internal HTTP listener")
				}
			})
			if certFile != nil && keyFile != nil {
				if err := internalServ.ListenAndServeTLS(*certFile, *keyFile); err != nil {
					if err != http.ErrServerClosed {
						logrus.WithError(err).Fatal("failed to serve HTTPS")
					}
				}
			} else {
				if err := internalServ.ListenAndServe(); err != nil {
					if err != http.ErrServerClosed {
						logrus.WithError(err).Fatal("failed to serve HTTP")
					}
				}
			}
			logrus.Infof("Stopped internal %s listener on %s", b.componentName, internalServ.Addr)
		}()
	}

	if externalAddr != NoListener {
		go func() {
			var externalShutdown atomic.Bool // RegisterOnShutdown can be called more than once
			logrus.Infof("Starting external %s listener on %s", b.componentName, externalServ.Addr)
			b.ProcessContext.ComponentStarted()
			externalServ.RegisterOnShutdown(func() {
				if externalShutdown.CAS(false, true) {
					b.ProcessContext.ComponentFinished()
					logrus.Infof("Stopped external HTTP listener")
				}
			})
			if certFile != nil && keyFile != nil {
				if err := externalServ.ListenAndServeTLS(*certFile, *keyFile); err != nil {
					if err != http.ErrServerClosed {
						logrus.WithError(err).Fatal("failed to serve HTTPS")
					}
				}
			} else {
				if err := externalServ.ListenAndServe(); err != nil {
					if err != http.ErrServerClosed {
						logrus.WithError(err).Fatal("failed to serve HTTP")
					}
				}
			}
			logrus.Infof("Stopped external %s listener on %s", b.componentName, externalServ.Addr)
		}()
	}

	<-b.ProcessContext.WaitForShutdown()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_ = internalServ.Shutdown(ctx)
	_ = externalServ.Shutdown(ctx)
	logrus.Infof("Stopped HTTP listeners")
}

func (b *BaseDendrite) WaitForShutdown() {
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	<-sigs
	signal.Reset(syscall.SIGINT, syscall.SIGTERM)

	logrus.Warnf("Shutdown signal received")

	b.ProcessContext.ShutdownDendrite()
	b.ProcessContext.WaitForComponentsToFinish()
	if b.Cfg.Global.Sentry.Enabled {
		if !sentry.Flush(time.Second * 5) {
			logrus.Warnf("failed to flush all Sentry events!")
		}
	}

	logrus.Warnf("Dendrite is exiting now")
}
