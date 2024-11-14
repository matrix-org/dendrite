// Copyright 2024 New Vector Ltd.
// Copyright 2017 Vector Creations Ltd
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"embed"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"text/template"

	"github.com/cretz/bine/tor"
	"github.com/element-hq/dendrite/internal"
	"github.com/element-hq/dendrite/internal/httputil"
	"github.com/element-hq/dendrite/setup/process"
	"github.com/eyedeekay/goSam"
	"github.com/eyedeekay/onramp"
	sentryhttp "github.com/getsentry/sentry-go/http"
	"github.com/gorilla/mux"
	"github.com/kardianos/minwinsvc"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/sirupsen/logrus"

	basepkg "github.com/element-hq/dendrite/setup/base"
	"github.com/element-hq/dendrite/setup/config"
)

func client() (*goSam.Client, error) {
	if skip {
		return nil, nil
	}
	return goSam.NewClient(*samAddr)
}

var sam, samError = client()

func start() (*tor.Tor, error) {
	if skip {
		return nil, nil
	}
	return tor.Start(context.Background(), nil)
}

func dialer() (*tor.Dialer, error) {
	if skip {
		return nil, nil
	}
	return t.Dialer(context.TODO(), nil)
}

var (
	t, terr        = start()
	tdialer, tderr = dialer()
)

// Dial a network connection to an I2P server or a unix socket. Use Tor, or Fail for clearnet addresses.
func DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	if samError != nil {
		return nil, samError
	}
	if network == "unix" {
		return net.Dial(network, addr)
	}

	// convert the addr to a full URL
	url, err := url.Parse(addr)
	if err != nil {
		return nil, err
	}
	if strings.HasSuffix(url.Host, ".i2p") {
		return sam.DialContext(ctx, network, addr)
	}
	if terr != nil {
		return nil, terr
	}
	if (tderr != nil) || (tdialer == nil) {
		return nil, tderr
	}
	return tdialer.DialContext(ctx, network, addr)
}

//go:embed static/*.gotmpl
var staticContent embed.FS

// SetupAndServeHTTPS sets up the HTTPS server to serve client & federation APIs
// and adds a prometheus handler under /_dendrite/metrics.
func SetupAndServeHTTPS(
	processContext *process.ProcessContext,
	cfg *config.Dendrite,
	routers httputil.Routers,
) {
	// create a transport that uses SAM to dial TCP Connections
	httpClient := &http.Client{
		Transport: &http.Transport{
			DialContext: DialContext,
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		},
	}

	http.DefaultClient = httpClient

	garlic, err := onramp.NewGarlic("dendrite", *samAddr, onramp.OPT_HUGE)
	if err != nil {
		logrus.WithError(err).Fatal("failed to create garlic")
	}
	defer garlic.Close() // nolint: errcheck
	listener, err := garlic.ListenTLS()
	if err != nil {
		logrus.WithError(err).Fatal("failed to serve HTTPS")
	}
	defer listener.Close() // nolint: errcheck

	externalHTTPSAddr := config.ServerAddress{}
	https, err := config.HTTPAddress("https://" + listener.Addr().String())
	if err != nil {
		logrus.WithError(err).Fatalf("Failed to parse http address")
	}
	externalHTTPSAddr = https

	externalRouter := mux.NewRouter().SkipClean(true).UseEncodedPath()

	externalServ := &http.Server{
		Addr:         externalHTTPSAddr.Address,
		WriteTimeout: basepkg.HTTPServerTimeout,
		Handler:      externalRouter,
		BaseContext: func(_ net.Listener) context.Context {
			return processContext.Context()
		},
	}

	// Redirect for Landing Page
	externalRouter.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, httputil.PublicStaticPath, http.StatusFound)
	})

	if cfg.Global.Metrics.Enabled {
		externalRouter.Handle("/metrics", httputil.WrapHandlerInBasicAuth(promhttp.Handler(), cfg.Global.Metrics.BasicAuth))
	}

	basepkg.ConfigureAdminEndpoints(processContext, routers)

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

	if externalHTTPSAddr.Enabled() {
		go func() {
			var externalShutdown atomic.Bool // RegisterOnShutdown can be called more than once
			logrus.Infof("Starting external listener on https://%s", externalServ.Addr)
			processContext.ComponentStarted()
			externalServ.RegisterOnShutdown(func() {
				if externalShutdown.CompareAndSwap(false, true) {
					processContext.ComponentFinished()
					logrus.Infof("Stopped external HTTPS listener")
				}
			})
			addr := listener.Addr()
			externalServ.Addr = addr.String()
			if err := externalServ.Serve(listener); err != nil {
				if err != http.ErrServerClosed {
					logrus.WithError(err).Fatal("failed to serve HTTPS")
				}
			}

			logrus.Infof("Stopped external listener on %s", externalServ.Addr)
		}()
	}

	minwinsvc.SetOnExit(processContext.ShutdownDendrite)
	<-processContext.WaitForShutdown()

	logrus.Infof("Stopping HTTPS listeners")
	_ = externalServ.Shutdown(context.Background())
	logrus.Infof("Stopped HTTPS listeners")
}
