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
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"net"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/getsentry/sentry-go"
	sentryhttp "github.com/getsentry/sentry-go/http"
	"github.com/matrix-org/dendrite/setup/jetstream"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/atomic"

	"github.com/matrix-org/dendrite/internal"
	"github.com/matrix-org/dendrite/internal/httputil"
	"github.com/matrix-org/dendrite/internal/sqlutil"

	"github.com/gorilla/mux"
	"github.com/kardianos/minwinsvc"

	"github.com/sirupsen/logrus"

	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/dendrite/setup/process"
)

//go:embed static/*.gotmpl
var staticContent embed.FS

// BaseDendrite is a base for creating new instances of dendrite. It parses
// command line flags and config, and exposes methods for creating various
// resources. All errors are handled by logging then exiting, so all methods
// should only be used during start up.
// Must be closed when shutting down.
type BaseDendrite struct {
	*process.ProcessContext
	tracerCloser      io.Closer
	Routers           httputil.Routers
	NATS              *jetstream.NATSInstance
	Cfg               *config.Dendrite
	DNSCache          *gomatrixserverlib.DNSCache
	ConnectionManager sqlutil.Connections
	EnableMetrics     bool
	startupLock       sync.Mutex
}

const HTTPServerTimeout = time.Minute * 5

type BaseDendriteOptions int

const (
	DisableMetrics BaseDendriteOptions = iota
)

// NewBaseDendrite creates a new instance to be used by a component.
func NewBaseDendrite(cfg *config.Dendrite, options ...BaseDendriteOptions) *BaseDendrite {
	platformSanityChecks()
	enableMetrics := true
	for _, opt := range options {
		switch opt {
		case DisableMetrics:
			enableMetrics = false
		}
	}

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

	// If we're in monolith mode, we'll set up a global pool of database
	// connections. A component is welcome to use this pool if they don't
	// have a separate database config of their own.
	cm := sqlutil.NewConnectionManager()
	if cfg.Global.DatabaseOptions.ConnectionString != "" {
		if cfg.Global.DatabaseOptions.ConnectionString.IsSQLite() {
			logrus.Panic("Using a global database connection pool is not supported with SQLite databases")
		}
		_, _, err := cm.Connection(&cfg.Global.DatabaseOptions)
		if err != nil {
			logrus.WithError(err).Panic("Failed to set up global database connections")
		}
		logrus.Debug("Using global database connection pool")
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
		ProcessContext:    process.NewProcessContext(),
		tracerCloser:      closer,
		Cfg:               cfg,
		DNSCache:          dnsCache,
		Routers:           httputil.NewRouters(),
		NATS:              &jetstream.NATSInstance{},
		ConnectionManager: cm,
		EnableMetrics:     enableMetrics,
	}
}

// Close implements io.Closer
func (b *BaseDendrite) Close() error {
	b.ProcessContext.ShutdownDendrite()
	b.ProcessContext.WaitForShutdown()
	return b.tracerCloser.Close()
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
		gomatrixserverlib.WithWellKnownSRVLookups(true),
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
	identities := b.Cfg.Global.SigningIdentities()
	if b.Cfg.Global.DisableFederation {
		return gomatrixserverlib.NewFederationClient(
			identities, gomatrixserverlib.WithTransport(noOpHTTPTransport),
		)
	}
	opts := []gomatrixserverlib.ClientOption{
		gomatrixserverlib.WithTimeout(time.Minute * 5),
		gomatrixserverlib.WithSkipVerify(b.Cfg.FederationAPI.DisableTLSValidation),
		gomatrixserverlib.WithKeepAlives(!b.Cfg.FederationAPI.DisableHTTPKeepalives),
	}
	if b.Cfg.Global.DNSCache.Enabled {
		opts = append(opts, gomatrixserverlib.WithDNSCache(b.DNSCache))
	}
	client := gomatrixserverlib.NewFederationClient(
		identities, opts...,
	)
	client.SetUserAgent(fmt.Sprintf("Dendrite/%s", internal.VersionString()))
	return client
}

func (b *BaseDendrite) ConfigureAdminEndpoints() {
	b.Routers.DendriteAdmin.HandleFunc("/monitor/up", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	})
	b.Routers.DendriteAdmin.HandleFunc("/monitor/health", func(w http.ResponseWriter, r *http.Request) {
		if isDegraded, reasons := b.ProcessContext.IsDegraded(); isDegraded {
			w.WriteHeader(503)
			_ = json.NewEncoder(w).Encode(struct {
				Warnings []string `json:"warnings"`
			}{
				Warnings: reasons,
			})
			return
		}
		w.WriteHeader(200)
	})
}

// SetupAndServeHTTP sets up the HTTP server to serve client & federation APIs
// and adds a prometheus handler under /_dendrite/metrics.
func (b *BaseDendrite) SetupAndServeHTTP(
	externalHTTPAddr config.ServerAddress,
	certFile, keyFile *string,
) {
	// Manually unlocked right before actually serving requests,
	// as we don't return from this method (defer doesn't work).
	b.startupLock.Lock()

	externalRouter := mux.NewRouter().SkipClean(true).UseEncodedPath()

	externalServ := &http.Server{
		Addr:         externalHTTPAddr.Address,
		WriteTimeout: HTTPServerTimeout,
		Handler:      externalRouter,
		BaseContext: func(_ net.Listener) context.Context {
			return b.ProcessContext.Context()
		},
	}

	//Redirect for Landing Page
	externalRouter.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, httputil.PublicStaticPath, http.StatusFound)
	})

	if b.Cfg.Global.Metrics.Enabled {
		externalRouter.Handle("/metrics", httputil.WrapHandlerInBasicAuth(promhttp.Handler(), b.Cfg.Global.Metrics.BasicAuth))
	}

	b.ConfigureAdminEndpoints()

	// Parse and execute the landing page template
	tmpl := template.Must(template.ParseFS(staticContent, "static/*.gotmpl"))
	landingPage := &bytes.Buffer{}
	if err := tmpl.ExecuteTemplate(landingPage, "index.gotmpl", map[string]string{
		"Version": internal.VersionString(),
	}); err != nil {
		logrus.WithError(err).Fatal("failed to execute landing page template")
	}

	b.Routers.Static.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(landingPage.Bytes())
	})

	var clientHandler http.Handler
	clientHandler = b.Routers.Client
	if b.Cfg.Global.Sentry.Enabled {
		sentryHandler := sentryhttp.New(sentryhttp.Options{
			Repanic: true,
		})
		clientHandler = sentryHandler.Handle(b.Routers.Client)
	}
	var federationHandler http.Handler
	federationHandler = b.Routers.Federation
	if b.Cfg.Global.Sentry.Enabled {
		sentryHandler := sentryhttp.New(sentryhttp.Options{
			Repanic: true,
		})
		federationHandler = sentryHandler.Handle(b.Routers.Federation)
	}
	externalRouter.PathPrefix(httputil.DendriteAdminPathPrefix).Handler(b.Routers.DendriteAdmin)
	externalRouter.PathPrefix(httputil.PublicClientPathPrefix).Handler(clientHandler)
	if !b.Cfg.Global.DisableFederation {
		externalRouter.PathPrefix(httputil.PublicKeyPathPrefix).Handler(b.Routers.Keys)
		externalRouter.PathPrefix(httputil.PublicFederationPathPrefix).Handler(federationHandler)
	}
	externalRouter.PathPrefix(httputil.SynapseAdminPathPrefix).Handler(b.Routers.SynapseAdmin)
	externalRouter.PathPrefix(httputil.PublicMediaPathPrefix).Handler(b.Routers.Media)
	externalRouter.PathPrefix(httputil.PublicWellKnownPrefix).Handler(b.Routers.WellKnown)
	externalRouter.PathPrefix(httputil.PublicStaticPath).Handler(b.Routers.Static)

	b.startupLock.Unlock()

	externalRouter.NotFoundHandler = httputil.NotFoundCORSHandler
	externalRouter.MethodNotAllowedHandler = httputil.NotAllowedHandler

	if externalHTTPAddr.Enabled() {
		go func() {
			var externalShutdown atomic.Bool // RegisterOnShutdown can be called more than once
			logrus.Infof("Starting external listener on %s", externalServ.Addr)
			b.ProcessContext.ComponentStarted()
			externalServ.RegisterOnShutdown(func() {
				if externalShutdown.CompareAndSwap(false, true) {
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
				if externalHTTPAddr.IsUnixSocket() {
					err := os.Remove(externalHTTPAddr.Address)
					if err != nil && !errors.Is(err, fs.ErrNotExist) {
						logrus.WithError(err).Fatal("failed to remove existing unix socket")
					}
					listener, err := net.Listen(externalHTTPAddr.Network(), externalHTTPAddr.Address)
					if err != nil {
						logrus.WithError(err).Fatal("failed to serve unix socket")
					}
					err = os.Chmod(externalHTTPAddr.Address, externalHTTPAddr.UnixSocketPermission)
					if err != nil {
						logrus.WithError(err).Fatal("failed to set unix socket permissions")
					}
					if err := externalServ.Serve(listener); err != nil {
						if err != http.ErrServerClosed {
							logrus.WithError(err).Fatal("failed to serve unix socket")
						}
					}

				} else {
					if err := externalServ.ListenAndServe(); err != nil {
						if err != http.ErrServerClosed {
							logrus.WithError(err).Fatal("failed to serve HTTP")
						}
					}
				}
			}
			logrus.Infof("Stopped external listener on %s", externalServ.Addr)
		}()
	}

	minwinsvc.SetOnExit(b.ProcessContext.ShutdownDendrite)
	<-b.ProcessContext.WaitForShutdown()

	logrus.Infof("Stopping HTTP listeners")
	_ = externalServ.Shutdown(context.Background())
	logrus.Infof("Stopped HTTP listeners")
}

func (b *BaseDendrite) WaitForShutdown() {
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	select {
	case <-sigs:
	case <-b.ProcessContext.WaitForShutdown():
	}
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
