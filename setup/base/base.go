// Copyright 2024 New Vector Ltd.
// Copyright 2020 The Matrix.org Foundation C.I.C.
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

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
	"sync/atomic"
	"syscall"
	"time"

	sentryhttp "github.com/getsentry/sentry-go/http"
	"github.com/matrix-org/gomatrixserverlib/fclient"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/gorilla/mux"
	"github.com/kardianos/minwinsvc"
	"github.com/element-hq/dendrite/internal"
	"github.com/element-hq/dendrite/internal/httputil"

	"github.com/sirupsen/logrus"

	"github.com/element-hq/dendrite/setup/config"
	"github.com/element-hq/dendrite/setup/process"
)

//go:embed static/*.gotmpl
var staticContent embed.FS

//go:embed static/client/login
var loginFallback embed.FS
var StaticContent = staticContent

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
func CreateFederationClient(cfg *config.Dendrite, dnsCache *fclient.DNSCache) fclient.FederationClient {
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
		fclient.WithUserAgent(fmt.Sprintf("Dendrite/%s", internal.VersionString())),
	}
	if cfg.Global.DNSCache.Enabled {
		opts = append(opts, fclient.WithDNSCache(dnsCache))
	}
	client := fclient.NewFederationClient(
		identities, opts...,
	)
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

	// We only need the files beneath the static/client/login folder.
	sub, err := fs.Sub(loginFallback, "static/client/login")
	if err != nil {
		logrus.Panicf("unable to read embedded files, this should never happen: %s", err)
	}
	// Serve a static page for login fallback
	routers.Static.PathPrefix("/client/login/").Handler(http.StripPrefix("/_matrix/static/client/login/", http.FileServer(http.FS(sub))))

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
