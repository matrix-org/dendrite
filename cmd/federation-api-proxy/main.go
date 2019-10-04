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
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/matrix-org/dendrite/common/basecomponent"

	log "github.com/sirupsen/logrus"
)

const usage = `Usage: %s

Create a single endpoint URL which remote matrix servers can be pointed at.

The server-server API in Dendrite is split across multiple processes
which listen on multiple ports. You cannot point a Matrix server at
any of those ports, as there will be unimplemented functionality.
In addition, all server-server API processes start with the additional
path prefix '/api', which Matrix servers will be unaware of.

This tool will proxy requests for all server-server URLs and forward
them to their respective process. It will also add the '/api' path
prefix to incoming requests.

THIS TOOL IS FOR TESTING AND NOT INTENDED FOR PRODUCTION USE.

Arguments:

`

var (
	federationAPIURL = flag.String("federation-api-url", basecomponent.EnvParse("DENDRITE_FEDERATION_API_URL", ""), "The base URL of the listening 'dendrite-federation-api-server' process. E.g. 'http://localhost:4200'")
	mediaAPIURL      = flag.String("media-api-server-url", basecomponent.EnvParse("DENDRITE_MEDIA_API_URL", ""), "The base URL of the listening 'dendrite-media-api-server' process. E.g. 'http://localhost:7779'")
	bindAddress      = flag.String("bind-address", basecomponent.EnvParse("DENDRITE_FEDERATION_PROXY_ADDRESS", ":8448"), "The listening port for the proxy.")
	certFile         = flag.String("tls-cert", basecomponent.EnvParse("DENDRITE_TLS_CERT_FILE", "server.crt"), "The PEM formatted X509 certificate to use for TLS")
	keyFile          = flag.String("tls-key", basecomponent.EnvParse("DENDRITE_TLS_KEY_FILE", "server.key"), "The PEM private key to use for TLS")
)

func makeProxy(targetURL string) (*httputil.ReverseProxy, error) {
	if !strings.HasSuffix(targetURL, "/") {
		targetURL += "/"
	}
	// Check that we can parse the URL.
	_, err := url.Parse(targetURL)
	if err != nil {
		return nil, err
	}
	return &httputil.ReverseProxy{
		Director: func(req *http.Request) {
			// URL.Path() removes the % escaping from the path.
			// The % encoding will be added back when the url is encoded
			// when the request is forwarded.
			// This means that we will lose any unessecary escaping from the URL.
			// Pratically this means that any distinction between '%2F' and '/'
			// in the URL will be lost by the time it reaches the target.
			path := req.URL.Path
			path = "api" + path
			log.WithFields(log.Fields{
				"path":   path,
				"url":    targetURL,
				"method": req.Method,
			}).Print("proxying request")
			newURL, err := url.Parse(targetURL + path)
			if err != nil {
				// We already checked that we can parse the URL
				// So this shouldn't ever get hit.
				panic(err)
			}
			// Copy the query parameters from the request.
			newURL.RawQuery = req.URL.RawQuery
			req.URL = newURL
		},
	}, nil
}

func main() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, usage, os.Args[0])
		flag.PrintDefaults()
	}

	flag.Parse()

	if *federationAPIURL == "" {
		flag.Usage()
		fmt.Fprintln(os.Stderr, "no --federation-api-url specified.")
		os.Exit(1)
	}

	if *mediaAPIURL == "" {
		flag.Usage()
		fmt.Fprintln(os.Stderr, "no --media-api-server-url specified.")
		os.Exit(1)
	}

	federationProxy, err := makeProxy(*federationAPIURL)
	if err != nil {
		panic(err)
	}

	mediaProxy, err := makeProxy(*mediaAPIURL)
	if err != nil {
		panic(err)
	}

	http.Handle("/_matrix/media/v1/", mediaProxy)
	http.Handle("/", federationProxy)

	srv := &http.Server{
		Addr:         *bindAddress,
		ReadTimeout:  1 * time.Minute, // how long we wait for the client to send the entire request (after connection accept)
		WriteTimeout: 5 * time.Minute, // how long the proxy has to write the full response
	}

	fmt.Println("Proxying requests to:")
	fmt.Println("  /_matrix/media/v1        => ", *mediaAPIURL+"/api/_matrix/media/v1")
	fmt.Println("  /*                       => ", *federationAPIURL+"/api/*")
	fmt.Println("Listening on ", *bindAddress)
	panic(srv.ListenAndServeTLS(*certFile, *keyFile))
}
