// Copyright 2017 Vector Creations Ltd
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
	"flag"
	"strings"
	"time"

	"github.com/eyedeekay/goSam"
	"github.com/eyedeekay/onramp"
	"github.com/getsentry/sentry-go"
	"github.com/matrix-org/dendrite/internal"
	"github.com/matrix-org/dendrite/internal/caching"
	"github.com/matrix-org/dendrite/internal/httputil"
	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/dendrite/setup/jetstream"
	"github.com/matrix-org/dendrite/setup/process"
	"github.com/matrix-org/gomatrixserverlib/fclient"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"

	"bytes"
	"context"
	"errors"
	"html/template"
	"io/fs"
	"net"
	"net/http"
	"net/url"

	"github.com/matrix-org/dendrite/appservice"
	"github.com/matrix-org/dendrite/federationapi"
	"github.com/matrix-org/dendrite/roomserver"
	"github.com/matrix-org/dendrite/setup"
	basepkg "github.com/matrix-org/dendrite/setup/base"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/dendrite/setup/mscs"
	"github.com/matrix-org/dendrite/userapi"

	_ "net/http/pprof"
	"os"

	sentryhttp "github.com/getsentry/sentry-go/http"
	"github.com/gorilla/mux"
	"github.com/kardianos/minwinsvc"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/atomic"
)

var (
	unixSocket = flag.String("unix-socket", "",
		"EXPERIMENTAL(unstable): The HTTP listening unix socket for the server (disables http[s]-bind-address feature)",
	)
	unixSocketPermission = flag.String("unix-socket-permission", "755",
		"EXPERIMENTAL(unstable): The HTTP listening unix socket permission for the server (in chmod format like 755)",
	)
	httpBindAddr  = flag.String("http-bind-address", ":8008", "The HTTP listening port for the server")
	httpsBindAddr = flag.String("https-bind-address", ":8448", "The HTTPS listening port for the server")
	certFile      = flag.String("tls-cert", "", "The PEM formatted X509 certificate to use for TLS")
	keyFile       = flag.String("tls-key", "", "The PEM private key to use for TLS")
)

func main() {
	cfg := setup.ParseFlags(true)
	httpAddr := config.ServerAddress{}
	httpsAddr := config.ServerAddress{}
	if *unixSocket == "" {
		http, err := config.HTTPAddress("http://" + *httpBindAddr)
		if err != nil {
			logrus.WithError(err).Fatalf("Failed to parse http address")
		}
		httpAddr = http
		https, err := config.HTTPAddress("https://" + *httpsBindAddr)
		if err != nil {
			logrus.WithError(err).Fatalf("Failed to parse https address")
		}
		httpsAddr = https
	} else {
		socket, err := config.UnixSocketAddress(*unixSocket, *unixSocketPermission)
		if err != nil {
			logrus.WithError(err).Fatalf("Failed to parse unix socket")
		}
		httpAddr = socket
	}

	configErrors := &config.ConfigErrors{}
	cfg.Verify(configErrors)
	if len(*configErrors) > 0 {
		for _, err := range *configErrors {
			logrus.Errorf("Configuration error: %s", err)
		}
		logrus.Fatalf("Failed to start due to configuration errors")
	}
	processCtx := process.NewProcessContext()

	internal.SetupStdLogging()
	internal.SetupHookLogging(cfg.Logging)
	internal.SetupPprof()

	basepkg.PlatformSanityChecks()

	logrus.Infof("Dendrite version %s", internal.VersionString())
	if !cfg.ClientAPI.RegistrationDisabled && cfg.ClientAPI.OpenRegistrationWithoutVerificationEnabled {
		logrus.Warn("Open registration is enabled")
	}

	// create DNS cache
	var dnsCache *fclient.DNSCache
	if cfg.Global.DNSCache.Enabled {
		dnsCache = fclient.NewDNSCache(
			cfg.Global.DNSCache.CacheSize,
			cfg.Global.DNSCache.CacheLifetime,
		)
		logrus.Infof(
			"DNS cache enabled (size %d, lifetime %s)",
			cfg.Global.DNSCache.CacheSize,
			cfg.Global.DNSCache.CacheLifetime,
		)
	}

	// setup tracing
	closer, err := cfg.SetupTracing()
	if err != nil {
		logrus.WithError(err).Panicf("failed to start opentracing")
	}
	defer closer.Close() // nolint: errcheck

	// setup sentry
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
		go func() {
			processCtx.ComponentStarted()
			<-processCtx.WaitForShutdown()
			if !sentry.Flush(time.Second * 5) {
				logrus.Warnf("failed to flush all Sentry events!")
			}
			processCtx.ComponentFinished()
		}()
	}

	federationClient := basepkg.CreateFederationClient(cfg, dnsCache)
	httpClient := basepkg.CreateClient(cfg, dnsCache)

	// prepare required dependencies
	cm := sqlutil.NewConnectionManager(processCtx, cfg.Global.DatabaseOptions)
	routers := httputil.NewRouters()

	caches := caching.NewRistrettoCache(cfg.Global.Cache.EstimatedMaxSize, cfg.Global.Cache.MaxAge, caching.EnableMetrics)
	natsInstance := jetstream.NATSInstance{}
	rsAPI := roomserver.NewInternalAPI(processCtx, cfg, cm, &natsInstance, caches, caching.EnableMetrics)
	fsAPI := federationapi.NewInternalAPI(
		processCtx, cfg, cm, &natsInstance, federationClient, rsAPI, caches, nil, false,
	)

	keyRing := fsAPI.KeyRing()

	userAPI := userapi.NewInternalAPI(processCtx, cfg, cm, &natsInstance, rsAPI, federationClient)
	asAPI := appservice.NewInternalAPI(processCtx, cfg, &natsInstance, userAPI, rsAPI)

	// The underlying roomserver implementation needs to be able to call the fedsender.
	// This is different to rsAPI which can be the http client which doesn't need this
	// dependency. Other components also need updating after their dependencies are up.
	rsAPI.SetFederationAPI(fsAPI, keyRing)
	rsAPI.SetAppserviceAPI(asAPI)
	rsAPI.SetUserAPI(userAPI)

	monolith := setup.Monolith{
		Config:    cfg,
		Client:    httpClient,
		FedClient: federationClient,
		KeyRing:   keyRing,

		AppserviceAPI: asAPI,
		// always use the concrete impl here even in -http mode because adding public routes
		// must be done on the concrete impl not an HTTP client else fedapi will call itself
		FederationAPI: fsAPI,
		RoomserverAPI: rsAPI,
		UserAPI:       userAPI,
	}
	monolith.AddAllPublicRoutes(processCtx, cfg, routers, cm, &natsInstance, caches, caching.EnableMetrics)

	if len(cfg.MSCs.MSCs) > 0 {
		if err := mscs.Enable(cfg, cm, routers, &monolith, caches); err != nil {
			logrus.WithError(err).Fatalf("Failed to enable MSCs")
		}
	}

	upCounter := prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "dendrite",
		Name:      "up",
		ConstLabels: map[string]string{
			"version": internal.VersionString(),
		},
	})
	upCounter.Add(1)
	prometheus.MustRegister(upCounter)

	// Expose the matrix APIs directly rather than putting them under a /api path.
	go func() {
		SetupAndServeHTTP(processCtx, cfg, routers, httpAddr, nil, nil)
	}()
	// Handle HTTPS if certificate and key are provided
	if *unixSocket == "" && *certFile != "" && *keyFile != "" {
		go func() {
			SetupAndServeHTTP(processCtx, cfg, routers, httpsAddr, certFile, keyFile)
		}()
	}

	// We want to block forever to let the HTTP and HTTPS handler serve the APIs
	basepkg.WaitForShutdown(processCtx)
}

var sam, err = goSam.NewDefaultClient()

func Dial(network, addr string) (net.Conn, error) {
	if network == "unix" {
		return net.Dial(network, addr)
	}

	// convert the addr to a full URL
	url, err := url.Parse(addr)
	if err != nil {
		return nil, err
	}
	if strings.HasSuffix(url.Host, ".i2p") {
		return sam.Dial(network, addr)
	}
	ip := net.ParseIP(url.Host)
	if ip != nil {
		if ip.IsLoopback() {
			return net.Dial(network, addr)
		}
	}
	return net.Dial(network, addr)
}

// SetupAndServeHTTP sets up the HTTP server to serve client & federation APIs
// and adds a prometheus handler under /_dendrite/metrics.
func SetupAndServeHTTP(
	processContext *process.ProcessContext,
	cfg *config.Dendrite,
	routers httputil.Routers,
	externalHTTPAddr config.ServerAddress,
	certFile, keyFile *string,
) {
	// create a transport that uses SAM to dial TCP Connections
	httpClient := &http.Client{
		Transport: &http.Transport{
			Dial: Dial,
		},
	}

	http.DefaultClient = httpClient

	externalRouter := mux.NewRouter().SkipClean(true).UseEncodedPath()

	externalServ := &http.Server{
		Addr:         externalHTTPAddr.Address,
		WriteTimeout: basepkg.HTTPServerTimeout,
		Handler:      externalRouter,
		BaseContext: func(_ net.Listener) context.Context {
			return processContext.Context()
		},
	}

	//Redirect for Landing Page
	externalRouter.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, httputil.PublicStaticPath, http.StatusFound)
	})

	if cfg.Global.Metrics.Enabled {
		externalRouter.Handle("/metrics", httputil.WrapHandlerInBasicAuth(promhttp.Handler(), cfg.Global.Metrics.BasicAuth))
	}

	basepkg.ConfigureAdminEndpoints(processContext, routers)

	// Parse and execute the landing page template
	tmpl := template.Must(template.ParseFS(basepkg.StaticContent, "static/*.gotmpl"))
	landingPage := &bytes.Buffer{}
	if err := tmpl.ExecuteTemplate(landingPage, "index.gotmpl", map[string]string{
		"Version": internal.VersionString(),
	}); err != nil {
		logrus.WithError(err).Fatal("failed to execute landing page template")
	}

	routers.Static.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(landingPage.Bytes())
	})

	var clientHandler http.Handler
	clientHandler = routers.Client
	if cfg.Global.Sentry.Enabled {
		sentryHandler := sentryhttp.New(sentryhttp.Options{
			Repanic: true,
		})
		clientHandler = sentryHandler.Handle(routers.Client)
	}
	var federationHandler http.Handler
	federationHandler = routers.Federation
	if cfg.Global.Sentry.Enabled {
		sentryHandler := sentryhttp.New(sentryhttp.Options{
			Repanic: true,
		})
		federationHandler = sentryHandler.Handle(routers.Federation)
	}
	externalRouter.PathPrefix(httputil.DendriteAdminPathPrefix).Handler(routers.DendriteAdmin)
	externalRouter.PathPrefix(httputil.PublicClientPathPrefix).Handler(clientHandler)
	if !cfg.Global.DisableFederation {
		externalRouter.PathPrefix(httputil.PublicKeyPathPrefix).Handler(routers.Keys)
		externalRouter.PathPrefix(httputil.PublicFederationPathPrefix).Handler(federationHandler)
	}
	externalRouter.PathPrefix(httputil.SynapseAdminPathPrefix).Handler(routers.SynapseAdmin)
	externalRouter.PathPrefix(httputil.PublicMediaPathPrefix).Handler(routers.Media)
	externalRouter.PathPrefix(httputil.PublicWellKnownPrefix).Handler(routers.WellKnown)
	externalRouter.PathPrefix(httputil.PublicStaticPath).Handler(routers.Static)

	externalRouter.NotFoundHandler = httputil.NotFoundCORSHandler
	externalRouter.MethodNotAllowedHandler = httputil.NotAllowedHandler

	if externalHTTPAddr.Enabled() {
		go func() {
			var externalShutdown atomic.Bool // RegisterOnShutdown can be called more than once
			logrus.Infof("Starting external listener on %s", externalServ.Addr)
			processContext.ComponentStarted()
			externalServ.RegisterOnShutdown(func() {
				if externalShutdown.CompareAndSwap(false, true) {
					processContext.ComponentFinished()
					logrus.Infof("Stopped external HTTP listener")
				}
			})
			if certFile != nil && keyFile != nil {
				garlictls, err := onramp.NewGarlic("dendrite-tls", "127.0.0.1:7656", onramp.OPT_DEFAULTS)
				if err != nil {
					logrus.WithError(err).Fatal("failed to create garlic")
				}
				listenertls, err := garlictls.Listen()
				if err != nil {
					logrus.WithError(err).Fatal("failed to serve HTTP")
				}
				addr := listenertls.Addr()
				externalServ.Addr = addr.String()
				if err := externalServ.ServeTLS(listenertls, *certFile, *keyFile); err != nil {
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
					garlic, err := onramp.NewGarlic("dendrite", "127.0.0.1:7656", onramp.OPT_DEFAULTS)
					if err != nil {
						logrus.WithError(err).Fatal("failed to create garlic")
					}
					listener, err := garlic.Listen()
					if err != nil {
						logrus.WithError(err).Fatal("failed to serve HTTP")
					}
					addr := listener.Addr()
					externalServ.Addr = addr.String()
					if err := externalServ.Serve(listener); err != nil {
						if err != http.ErrServerClosed {
							logrus.WithError(err).Fatal("failed to serve HTTP")
						}
					}
				}
			}
			logrus.Infof("Stopped external listener on %s", externalServ.Addr)
		}()
	}

	minwinsvc.SetOnExit(processContext.ShutdownDendrite)
	<-processContext.WaitForShutdown()

	logrus.Infof("Stopping HTTP listeners")
	_ = externalServ.Shutdown(context.Background())
	logrus.Infof("Stopped HTTP listeners")
}
