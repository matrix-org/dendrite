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
	"io/fs"
	"net"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"syscall"
	"time"

	sentryhttp "github.com/getsentry/sentry-go/http"
	"github.com/matrix-org/gomatrixserverlib/fclient"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/atomic"

	"github.com/gorilla/mux"
	"github.com/kardianos/minwinsvc"
	"github.com/matrix-org/dendrite/internal"
	"github.com/matrix-org/dendrite/internal/httputil"

	"github.com/sirupsen/logrus"

	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/dendrite/setup/process"
)

//go:embed static/*.gotmpl
var staticContent embed.FS

const HTTPServerTimeout = time.Minute * 5

// CreateClient creates a new client (normally used for media fetch requests).
// Should only be called once per component.
func CreateClient(cfg *config.Dendrite, dnsCache *fclient.DNSCache) *fclient.Client {
	if cfg.Global.DisableFederation {
		return fclient.NewClient(
			fclient.WithTransport(noOpHTTPTransport),
		)
	}
	opts := []fclient.ClientOption{
		fclient.WithSkipVerify(cfg.FederationAPI.DisableTLSValidation),
		fclient.WithWellKnownSRVLookups(true),
	}
	if cfg.Global.DNSCache.Enabled && dnsCache != nil {
		opts = append(opts, fclient.WithDNSCache(dnsCache))
	}
	client := fclient.NewClient(opts...)
	client.SetUserAgent(fmt.Sprintf("Dendrite/%s", internal.VersionString()))
	return client
}

// CreateFederationClient creates a new federation client. Should only be called
// once per component.
func CreateFederationClient(cfg *config.Dendrite, dnsCache *fclient.DNSCache) *fclient.FederationClient {
	identities := cfg.Global.SigningIdentities()
	if cfg.Global.DisableFederation {
		return fclient.NewFederationClient(
			identities, fclient.WithTransport(noOpHTTPTransport),
		)
	}
	opts := []fclient.ClientOption{
		fclient.WithTimeout(time.Minute * 5),
		fclient.WithSkipVerify(cfg.FederationAPI.DisableTLSValidation),
		fclient.WithKeepAlives(!cfg.FederationAPI.DisableHTTPKeepalives),
	}
	if cfg.Global.DNSCache.Enabled {
		opts = append(opts, fclient.WithDNSCache(dnsCache))
	}
	client := fclient.NewFederationClient(
		identities, opts...,
	)
	client.SetUserAgent(fmt.Sprintf("Dendrite/%s", internal.VersionString()))
	return client
}

func ConfigureAdminEndpoints(processContext *process.ProcessContext, routers httputil.Routers) {
	routers.DendriteAdmin.HandleFunc("/monitor/up", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	})
	routers.DendriteAdmin.HandleFunc("/monitor/health", func(w http.ResponseWriter, r *http.Request) {
		if isDegraded, reasons := processContext.IsDegraded(); isDegraded {
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
func SetupAndServeHTTP(
	processContext *process.ProcessContext,
	cfg *config.Dendrite,
	routers httputil.Routers,
	externalHTTPAddr config.ServerAddress,
	certFile, keyFile *string,
) {
	externalRouter := mux.NewRouter().SkipClean(true).UseEncodedPath()

	externalServ := &http.Server{
		Addr:         externalHTTPAddr.Address,
		WriteTimeout: HTTPServerTimeout,
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

	ConfigureAdminEndpoints(processContext, routers)

	// Parse and execute the landing page template
	tmpl := template.Must(template.ParseFS(staticContent, "static/*.gotmpl"))
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

	minwinsvc.SetOnExit(processContext.ShutdownDendrite)
	<-processContext.WaitForShutdown()

	logrus.Infof("Stopping HTTP listeners")
	_ = externalServ.Shutdown(context.Background())
	logrus.Infof("Stopped HTTP listeners")
}

func WaitForShutdown(processCtx *process.ProcessContext) {
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	select {
	case <-sigs:
	case <-processCtx.WaitForShutdown():
	}
	signal.Reset(syscall.SIGINT, syscall.SIGTERM)

	logrus.Warnf("Shutdown signal received")

	processCtx.ShutdownDendrite()
	processCtx.WaitForComponentsToFinish()

	logrus.Warnf("Dendrite is exiting now")
}
