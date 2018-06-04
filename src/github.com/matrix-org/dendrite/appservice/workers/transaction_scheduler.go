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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"time"

	"github.com/matrix-org/dendrite/appservice/storage"
	"github.com/matrix-org/dendrite/common/config"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/sirupsen/logrus"
)

var (
	// TODO: Expose these in the config?
	// Maximum size of events sent in each transaction.
	transactionBatchSize = 50
	// Time to wait between checking for new events to send.
	transactionBreakTime = time.Millisecond * 50
	// Timeout for sending a single transaction to an application service.
	transactionTimeout = time.Second * 15
	// The current transaction ID. Increments after every successful transaction.
	currentTransactionID = 0
)

// SetupTransactionWorkers spawns a separate goroutine for each application
// service. Each of these "workers" handle taking all events intended for their
// app service, batch them up into a single transaction (up to a max transaction
// size), then send that off to the AS's /transactions/{txnID} endpoint. It also
// handles exponentially backing off in case the AS isn't currently available.
func SetupTransactionWorkers(
	cfg *config.Dendrite,
	appserviceDB *storage.Database,
	// Each worker has access to an event counter, which keeps track of the amount
	// of events they still have to send off. The roomserver consumer
	// (consumers/roomserver.go) increments this counter every time a new event for
	// a specific application service is inserted into the database, whereas the
	// counter is decremented by a certain amount when a worker sends some amount
	// of events successfully to an application service. To ensure recovery in the
	// event of a crash, this counter is initialized to the amount of events meant
	// to be sent by a specific worker in the database, so that state is not lost.
	eventCounterMap map[string]int,
) error {
	// Create a worker that handles transmitting events to a single homeserver
	for _, appservice := range cfg.Derived.ApplicationServices {
		// Don't create a worker if this AS doesn't want to receive events
		if appservice.URL != "" {
			go worker(appserviceDB, appservice, eventCounterMap)
		}
	}
	return nil
}

// worker is a goroutine that sends any queued events to the application service
// it is given.
func worker(db *storage.Database, as config.ApplicationService, ecm map[string]int) {
	// Initialize transaction ID counter
	var err error
	currentTransactionID, err = db.GetTxnIDWithAppServiceID(context.TODO(), as.ID)
	if err != nil {
		logrus.WithError(err).Warnf("appservice worker for %s unable to get latest transaction ID from DB",
			as.ID)
	}

	// Create an HTTP client for sending requests to app services
	client := &http.Client{
		Timeout: transactionTimeout,
	}

	// Initialize counter to amount of events currently in the database
	eventCount, err := db.CountEventsWithAppServiceID(context.TODO(), as.ID)
	if err != nil {
		logrus.WithError(err).Warn("appservice worker unable to count queued events from DB")
	}
	ecm[as.ID] = eventCount

	// Initialize backoff exponent (2^x secs). Max 9, aka 512s.
	backoff := 0

	// Loop forever and keep waiting for more events to send
	for {
		// Check if there are any events to send
		if ecm[as.ID] > 0 {
			ctx := context.TODO()

			eventIDs, events, err := db.GetEventsWithAppServiceID(ctx, as.ID, transactionBatchSize)
			if err != nil {
				logrus.WithError(err).Error("appservice worker unable to read queued events from DB")

				// Wait a little bit for DB to possibly recover
				time.Sleep(transactionBreakTime)
				continue
			}

			// Batch events up into a transaction
			transactionJSON, err := createTransaction(events)
			if err != nil {
				logrus.WithError(err).Error("appservice worker unable to marshal events")

				// Wait a little bit before trying again
				time.Sleep(transactionBreakTime)
				continue
			}

			// Send the events off to the application service
			eventsSent, err := send(client, as, transactionJSON, len(events))
			if err != nil {
				// Calculate how long to backoff for
				backoffDuration := time.Duration(math.Pow(2, float64(backoff)))
				backoffSeconds := time.Second * backoffDuration
				logrus.WithError(err).Warnf("unable to send transactions to %s, backing off for %ds",
					as.ID, backoffDuration)

				// Increment backoff count
				backoff++
				if backoff > 9 {
					backoff = 9
				}

				// Backoff
				time.Sleep(backoffSeconds)

				continue
			}

			// We sent successfully, hooray!
			backoff = 0
			ecm[as.ID] -= eventsSent

			// Remove sent events from the DB
			err = db.RemoveEventsByID(ctx, eventIDs)
			if err != nil {
				logrus.WithError(err).Errorf("unable to remove appservice events from the database for %s",
					as.ID)
			}

			// Update transactionID
			currentTransactionID++
			if err = db.UpsertTxnIDWithAppServiceID(context.TODO(), as.ID, currentTransactionID); err != nil {
				logrus.WithError(err).Errorf("unable to update transaction ID for %s",
					as.ID)
			}
		} else {
			// If not, wait a bit and try again
			time.Sleep(transactionBreakTime)
		}
	}
}

// createTransaction takes in a slice of AS events, stores them in an AS
// transaction, and JSON-encodes the results.
func createTransaction(
	events []gomatrixserverlib.ApplicationServiceEvent,
) ([]byte, error) {
	// Create a transactions and store the events inside
	transaction := gomatrixserverlib.ApplicationServiceTransaction{
		Events: events,
	}

	transactionJSON, err := json.Marshal(transaction)
	if err != nil {
		return nil, err
	}

	return transactionJSON, nil
}

// send sends events to an application service. Returns an error if an OK was not
// received back from the application service or the request timed out.
func send(
	client *http.Client,
	appservice config.ApplicationService,
	transaction []byte,
	count int,
) (int, error) {
	// POST a transaction to our AS.
	address := fmt.Sprintf("%s/transactions/%d", appservice.URL, currentTransactionID)
	resp, err := client.Post(address, "application/json", bytes.NewBuffer(transaction))
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close() // nolint: errcheck

	// Check the AS received the events correctly
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf(
			"Non-OK status code %d returned from AS", resp.StatusCode,
		)
	}

	// Return amount of sent events
	return count, nil
}
