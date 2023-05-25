// Copyright 2022 The Matrix.org Foundation C.I.C.
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

package relay

import (
	"context"
	"sync"
	"time"

	federationAPI "github.com/matrix-org/dendrite/federationapi/api"
	relayServerAPI "github.com/matrix-org/dendrite/relayapi/api"
	"github.com/matrix-org/gomatrixserverlib/spec"
	"github.com/sirupsen/logrus"
	"go.uber.org/atomic"
)

const (
	relayServerRetryInterval = time.Second * 30
)

type RelayServerRetriever struct {
	ctx                 context.Context
	serverName          spec.ServerName
	federationAPI       federationAPI.FederationInternalAPI
	relayAPI            relayServerAPI.RelayInternalAPI
	relayServersQueried map[spec.ServerName]bool
	queriedServersMutex sync.Mutex
	running             atomic.Bool
	quit                chan bool
}

func NewRelayServerRetriever(
	ctx context.Context,
	serverName spec.ServerName,
	federationAPI federationAPI.FederationInternalAPI,
	relayAPI relayServerAPI.RelayInternalAPI,
	quit chan bool,
) RelayServerRetriever {
	return RelayServerRetriever{
		ctx:                 ctx,
		serverName:          serverName,
		federationAPI:       federationAPI,
		relayAPI:            relayAPI,
		relayServersQueried: make(map[spec.ServerName]bool),
		running:             *atomic.NewBool(false),
		quit:                quit,
	}
}

func (r *RelayServerRetriever) InitializeRelayServers(eLog *logrus.Entry) {
	request := federationAPI.P2PQueryRelayServersRequest{Server: spec.ServerName(r.serverName)}
	response := federationAPI.P2PQueryRelayServersResponse{}
	err := r.federationAPI.P2PQueryRelayServers(r.ctx, &request, &response)
	if err != nil {
		eLog.Warnf("Failed obtaining list of this node's relay servers: %s", err.Error())
	}

	r.queriedServersMutex.Lock()
	defer r.queriedServersMutex.Unlock()
	for _, server := range response.RelayServers {
		r.relayServersQueried[server] = false
	}

	eLog.Infof("Registered relay servers: %v", response.RelayServers)
}

func (r *RelayServerRetriever) SetRelayServers(servers []spec.ServerName) {
	UpdateNodeRelayServers(r.serverName, servers, r.ctx, r.federationAPI)

	// Replace list of servers to sync with and mark them all as unsynced.
	r.queriedServersMutex.Lock()
	defer r.queriedServersMutex.Unlock()
	r.relayServersQueried = make(map[spec.ServerName]bool)
	for _, server := range servers {
		r.relayServersQueried[server] = false
	}

	r.StartSync()
}

func (r *RelayServerRetriever) GetRelayServers() []spec.ServerName {
	r.queriedServersMutex.Lock()
	defer r.queriedServersMutex.Unlock()
	relayServers := []spec.ServerName{}
	for server := range r.relayServersQueried {
		relayServers = append(relayServers, server)
	}

	return relayServers
}

func (r *RelayServerRetriever) GetQueriedServerStatus() map[spec.ServerName]bool {
	r.queriedServersMutex.Lock()
	defer r.queriedServersMutex.Unlock()

	result := map[spec.ServerName]bool{}
	for server, queried := range r.relayServersQueried {
		result[server] = queried
	}
	return result
}

func (r *RelayServerRetriever) StartSync() {
	if !r.running.Load() {
		logrus.Info("Starting relay server sync")
		go r.SyncRelayServers(r.quit)
	}
}

func (r *RelayServerRetriever) IsRunning() bool {
	return r.running.Load()
}

func (r *RelayServerRetriever) SyncRelayServers(stop <-chan bool) {
	defer r.running.Store(false)

	t := time.NewTimer(relayServerRetryInterval)
	for {
		relayServersToQuery := []spec.ServerName{}
		func() {
			r.queriedServersMutex.Lock()
			defer r.queriedServersMutex.Unlock()
			for server, complete := range r.relayServersQueried {
				if !complete {
					relayServersToQuery = append(relayServersToQuery, server)
				}
			}
		}()
		if len(relayServersToQuery) == 0 {
			// All relay servers have been synced.
			logrus.Info("Finished syncing with all known relays")
			return
		}
		r.queryRelayServers(relayServersToQuery)
		t.Reset(relayServerRetryInterval)

		select {
		case <-stop:
			if !t.Stop() {
				<-t.C
			}
			logrus.Info("Stopped relay server retriever")
			return
		case <-t.C:
		}
	}
}

func (r *RelayServerRetriever) queryRelayServers(relayServers []spec.ServerName) {
	logrus.Info("Querying relay servers for any available transactions")
	for _, server := range relayServers {
		userID, err := spec.NewUserID("@user:"+string(r.serverName), false)
		if err != nil {
			return
		}

		logrus.Infof("Syncing with relay: %s", string(server))
		err = r.relayAPI.PerformRelayServerSync(context.Background(), *userID, server)
		if err == nil {
			func() {
				r.queriedServersMutex.Lock()
				defer r.queriedServersMutex.Unlock()
				r.relayServersQueried[server] = true
			}()
			// TODO : What happens if your relay receives new messages after this point?
			// Should you continue to check with them, or should they try and contact you?
			// They could send a "new_async_events" message your way maybe?
			// Then you could mark them as needing to be queried again.
			// What if you miss this message?
			// Maybe you should try querying them again after a certain period of time as a backup?
		} else {
			logrus.Errorf("Failed querying relay server: %s", err.Error())
		}
	}
}

func UpdateNodeRelayServers(
	node spec.ServerName,
	relays []spec.ServerName,
	ctx context.Context,
	fedAPI federationAPI.FederationInternalAPI,
) {
	// Get the current relay list
	request := federationAPI.P2PQueryRelayServersRequest{Server: node}
	response := federationAPI.P2PQueryRelayServersResponse{}
	err := fedAPI.P2PQueryRelayServers(ctx, &request, &response)
	if err != nil {
		logrus.Warnf("Failed obtaining list of relay servers for %s: %s", node, err.Error())
	}

	// Remove old, non-matching relays
	var serversToRemove []spec.ServerName
	for _, existingServer := range response.RelayServers {
		shouldRemove := true
		for _, newServer := range relays {
			if newServer == existingServer {
				shouldRemove = false
				break
			}
		}

		if shouldRemove {
			serversToRemove = append(serversToRemove, existingServer)
		}
	}
	removeRequest := federationAPI.P2PRemoveRelayServersRequest{
		Server:       node,
		RelayServers: serversToRemove,
	}
	removeResponse := federationAPI.P2PRemoveRelayServersResponse{}
	err = fedAPI.P2PRemoveRelayServers(ctx, &removeRequest, &removeResponse)
	if err != nil {
		logrus.Warnf("Failed removing old relay servers for %s: %s", node, err.Error())
	}

	// Add new relays
	addRequest := federationAPI.P2PAddRelayServersRequest{
		Server:       node,
		RelayServers: relays,
	}
	addResponse := federationAPI.P2PAddRelayServersResponse{}
	err = fedAPI.P2PAddRelayServers(ctx, &addRequest, &addResponse)
	if err != nil {
		logrus.Warnf("Failed adding relay servers for %s: %s", node, err.Error())
	}
}
