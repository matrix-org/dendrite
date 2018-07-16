// Copyright 2018 Vector Creations Ltd
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

package workers

import (
	"context"
	"crypto/tls"
	"fmt"
	"io/ioutil"
	"math"
	"net/http"
	"time"

	"github.com/matrix-org/dendrite/appservice/storage"
	"github.com/matrix-org/dendrite/common/config"
	log "github.com/sirupsen/logrus"
)

const (
	// Timeout for requests to an application service to complete
	requestTimeout = time.Second * 300
)

const protocolPath = "/_matrix/app/unstable/thirdparty/protocol/"

// ThirdPartyWorker interfaces with a given application service on third party
// network related information.
// At the moment it simply asks for information on protocols that an application
// service supports, then exits.
func ThirdPartyWorker(
	db *storage.Database,
	appservice config.ApplicationService,
) {
	ctx := context.Background()

	// Grab the HTTP client for sending requests to app services
	client := &http.Client{
		Timeout: requestTimeout,
		// TODO: Verify certificates
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true, // nolint: gas
			},
		},
	}

	backoffCount := 0

	// Retrieve protocol information from the application service
	for i := 0; i < len(appservice.Protocols); i++ {
		protocolID := appservice.Protocols[i]
		protocolDefinition, err := retreiveProtocolInformation(
			ctx, client, appservice, protocolID,
		)
		if err != nil {
			log.WithFields(log.Fields{
				"appservice":       appservice.ID,
				"backoff_exponent": backoffCount,
			}).WithError(err).Warn("error contacting appservice thirdparty endpoints")

			// Backoff before contacting again
			backoff(&backoffCount)

			// Try this protocol again
			i--
			continue
		}

		// Cache protocol definition for clients to request later
		storeProtocolDefinition(ctx, db, appservice, protocolID, protocolDefinition)
	}
}

// backoff for the request amount of 2^number seconds
// We want to support a few different use cases. Application services that don't
// implement these endpoints and thus will always return an error. Application
// services that are not currently up when Dendrite starts. Application services
// that are broken for a while but will come back online later.
// We can support all of these without being too resource intensive with
// exponential backoff.
func backoff(exponent *int) {
	// Calculate how long to backoff for
	backoffDuration := time.Duration(math.Pow(2, float64(*exponent)))
	backoffSeconds := time.Second * backoffDuration

	if *exponent < 6 {
		*exponent++
	}

	// Backoff
	time.Sleep(backoffSeconds)
}

// retreiveProtocolInformation contacts an application service and asks for
// information about a given protocol.
func retreiveProtocolInformation(
	ctx context.Context,
	httpClient *http.Client,
	appservice config.ApplicationService,
	protocol string,
) (string, error) {
	// Create a request to the application service
	requestURL := appservice.URL + protocolPath + protocol
	req, err := http.NewRequest(http.MethodGet, requestURL, nil)
	if err != nil {
		return "", err
	}

	// Perform the request
	resp, err := httpClient.Do(req.WithContext(ctx))
	if err != nil {
		return "", err
	}

	// Check that the request was successful
	if resp.StatusCode != http.StatusOK {
		// TODO: Handle non-200 error codes from application services
		return "", fmt.Errorf("non-OK status code %d returned from AS", resp.StatusCode)
	}

	// Read the response body
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	return string(body), nil
}

// storeProtocolDefinition stores a protocol definition along with the protocol
// ID in the database
func storeProtocolDefinition(
	ctx context.Context,
	db *storage.Database,
	appservice config.ApplicationService,
	protocolID, protocolDefinition string,
) error {
	return db.StoreProtocolDefinition(ctx, protocolID, protocolDefinition)
}
