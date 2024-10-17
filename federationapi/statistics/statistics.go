package statistics

import (
	"context"
	"math"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/element-hq/dendrite/federationapi/storage"
	"github.com/matrix-org/gomatrixserverlib/spec"
)

// Statistics contains information about all of the remote federated
// hosts that we have interacted with. It is basically a threadsafe
// wrapper.
type Statistics struct {
	DB      storage.Database
	servers map[spec.ServerName]*ServerStatistics
	mutex   sync.RWMutex

	backoffTimers map[spec.ServerName]*time.Timer
	backoffMutex  sync.RWMutex

	// How many times should we tolerate consecutive failures before we
	// just blacklist the host altogether? The backoff is exponential,
	// so the max time here to attempt is 2**failures seconds.
	FailuresUntilBlacklist uint32

	// How many times should we tolerate consecutive failures before we
	// mark the destination as offline. At this point we should attempt
	// to send messages to the user's async relay servers if we know them.
	FailuresUntilAssumedOffline uint32

	enableRelays bool
}

func NewStatistics(
	db storage.Database,
	failuresUntilBlacklist uint32,
	failuresUntilAssumedOffline uint32,
	enableRelays bool,
) Statistics {
	return Statistics{
		DB:                          db,
		FailuresUntilBlacklist:      failuresUntilBlacklist,
		FailuresUntilAssumedOffline: failuresUntilAssumedOffline,
		backoffTimers:               make(map[spec.ServerName]*time.Timer),
		servers:                     make(map[spec.ServerName]*ServerStatistics),
		enableRelays:                enableRelays,
	}
}

// ForServer returns server statistics for the given server name. If it
// does not exist, it will create empty statistics and return those.
func (s *Statistics) ForServer(serverName spec.ServerName) *ServerStatistics {
	// Look up if we have statistics for this server already.
	s.mutex.RLock()
	server, found := s.servers[serverName]
	s.mutex.RUnlock()
	// If we don't, then make one.
	if !found {
		s.mutex.Lock()
		server = &ServerStatistics{
			statistics:        s,
			serverName:        serverName,
			knownRelayServers: []spec.ServerName{},
		}
		s.servers[serverName] = server
		s.mutex.Unlock()
		blacklisted, err := s.DB.IsServerBlacklisted(serverName)
		if err != nil {
			logrus.WithError(err).Errorf("Failed to get blacklist entry %q", serverName)
		} else {
			server.blacklisted.Store(blacklisted)
		}

		// Don't bother hitting the database 2 additional times
		// if we don't want to use relays.
		if !s.enableRelays {
			return server
		}

		assumedOffline, err := s.DB.IsServerAssumedOffline(context.Background(), serverName)
		if err != nil {
			logrus.WithError(err).Errorf("Failed to get assumed offline entry %q", serverName)
		} else {
			server.assumedOffline.Store(assumedOffline)
		}

		knownRelayServers, err := s.DB.P2PGetRelayServersForServer(context.Background(), serverName)
		if err != nil {
			logrus.WithError(err).Errorf("Failed to get relay server list for %q", serverName)
		} else {
			server.relayMutex.Lock()
			server.knownRelayServers = knownRelayServers
			server.relayMutex.Unlock()
		}
	}
	return server
}

type SendMethod uint8

const (
	SendDirect SendMethod = iota
	SendViaRelay
)

// ServerStatistics contains information about our interactions with a
// remote federated host, e.g. how many times we were successful, how
// many times we failed etc. It also manages the backoff time and black-
// listing a remote host if it remains uncooperative.
type ServerStatistics struct {
	statistics        *Statistics     //
	serverName        spec.ServerName //
	blacklisted       atomic.Bool     // is the node blacklisted
	assumedOffline    atomic.Bool     // is the node assumed to be offline
	backoffStarted    atomic.Bool     // is the backoff started
	backoffUntil      atomic.Value    // time.Time until this backoff interval ends
	backoffCount      atomic.Uint32   // number of times BackoffDuration has been called
	successCounter    atomic.Uint32   // how many times have we succeeded?
	backoffNotifier   func()          // notifies destination queue when backoff completes
	notifierMutex     sync.Mutex
	knownRelayServers []spec.ServerName
	relayMutex        sync.Mutex
}

const maxJitterMultiplier = 1.4
const minJitterMultiplier = 0.8

// duration returns how long the next backoff interval should be.
func (s *ServerStatistics) duration(count uint32) time.Duration {
	// Add some jitter to minimise the chance of having multiple backoffs
	// ending at the same time.
	jitter := rand.Float64()*(maxJitterMultiplier-minJitterMultiplier) + minJitterMultiplier
	duration := time.Millisecond * time.Duration(math.Exp2(float64(count))*jitter*1000)
	return duration
}

// cancel will interrupt the currently active backoff.
func (s *ServerStatistics) cancel() {
	s.blacklisted.Store(false)
	s.backoffUntil.Store(time.Time{})

	s.ClearBackoff()
}

// AssignBackoffNotifier configures the channel to send to when
// a backoff completes.
func (s *ServerStatistics) AssignBackoffNotifier(notifier func()) {
	s.notifierMutex.Lock()
	defer s.notifierMutex.Unlock()
	s.backoffNotifier = notifier
}

// Success updates the server statistics with a new successful
// attempt, which increases the sent counter and resets the idle and
// failure counters. If a host was blacklisted at this point then
// we will unblacklist it.
// `relay` specifies whether the success was to the actual destination
// or one of their relay servers.
func (s *ServerStatistics) Success(method SendMethod) {
	s.cancel()
	s.backoffCount.Store(0)
	// NOTE : Sending to the final destination vs. a relay server has
	// slightly different semantics.
	if method == SendDirect {
		s.successCounter.Add(1)
		if s.blacklisted.Load() && s.statistics.DB != nil {
			if err := s.statistics.DB.RemoveServerFromBlacklist(s.serverName); err != nil {
				logrus.WithError(err).Errorf("Failed to remove %q from blacklist", s.serverName)
			}
		}

		s.removeAssumedOffline()
	}
}

// Failure marks a failure and starts backing off if needed.
// It will return the time that the current failure
// will result in backoff waiting until, and a bool signalling
// whether we have blacklisted and therefore to give up.
func (s *ServerStatistics) Failure() (time.Time, bool) {
	// Return immediately if we have blacklisted this node.
	if s.blacklisted.Load() {
		return time.Time{}, true
	}

	// If we aren't already backing off, this call will start
	// a new backoff period, increase the failure counter and
	// start a goroutine which will wait out the backoff and
	// unset the backoffStarted flag when done.
	if s.backoffStarted.CompareAndSwap(false, true) {
		backoffCount := s.backoffCount.Add(1)

		if backoffCount >= s.statistics.FailuresUntilAssumedOffline {
			s.assumedOffline.CompareAndSwap(false, true)
			if s.statistics.DB != nil {
				if err := s.statistics.DB.SetServerAssumedOffline(context.Background(), s.serverName); err != nil {
					logrus.WithError(err).Errorf("Failed to set %q as assumed offline", s.serverName)
				}
			}
		}

		if backoffCount >= s.statistics.FailuresUntilBlacklist {
			s.blacklisted.Store(true)
			if s.statistics.DB != nil {
				if err := s.statistics.DB.AddServerToBlacklist(s.serverName); err != nil {
					logrus.WithError(err).Errorf("Failed to add %q to blacklist", s.serverName)
				}
			}
			s.ClearBackoff()
			return time.Time{}, true
		}

		// We're starting a new back off so work out what the next interval
		// will be.
		count := s.backoffCount.Load()
		until := time.Now().Add(s.duration(count))
		s.backoffUntil.Store(until)

		s.statistics.backoffMutex.Lock()
		s.statistics.backoffTimers[s.serverName] = time.AfterFunc(time.Until(until), s.backoffFinished)
		s.statistics.backoffMutex.Unlock()
	}

	return s.backoffUntil.Load().(time.Time), false
}

// MarkServerAlive removes the assumed offline and blacklisted statuses from this server.
// Returns whether the server was blacklisted before this point.
func (s *ServerStatistics) MarkServerAlive() bool {
	s.removeAssumedOffline()
	wasBlacklisted := s.removeBlacklist()
	return wasBlacklisted
}

// ClearBackoff stops the backoff timer for this destination if it is running
// and removes the timer from the backoffTimers map.
func (s *ServerStatistics) ClearBackoff() {
	// If the timer is still running then stop it so it's memory is cleaned up sooner.
	s.statistics.backoffMutex.Lock()
	defer s.statistics.backoffMutex.Unlock()
	if timer, ok := s.statistics.backoffTimers[s.serverName]; ok {
		timer.Stop()
	}
	delete(s.statistics.backoffTimers, s.serverName)

	s.backoffStarted.Store(false)
}

// backoffFinished will clear the previous backoff and notify the destination queue.
func (s *ServerStatistics) backoffFinished() {
	s.ClearBackoff()

	// Notify the destinationQueue if one is currently running.
	s.notifierMutex.Lock()
	defer s.notifierMutex.Unlock()
	if s.backoffNotifier != nil {
		s.backoffNotifier()
	}
}

// BackoffInfo returns information about the current or previous backoff.
// Returns the last backoffUntil time.
func (s *ServerStatistics) BackoffInfo() *time.Time {
	until, ok := s.backoffUntil.Load().(time.Time)
	if ok {
		return &until
	}
	return nil
}

// Blacklisted returns true if the server is blacklisted and false
// otherwise.
func (s *ServerStatistics) Blacklisted() bool {
	return s.blacklisted.Load()
}

// AssumedOffline returns true if the server is assumed offline and false
// otherwise.
func (s *ServerStatistics) AssumedOffline() bool {
	return s.assumedOffline.Load()
}

// removeBlacklist removes the blacklisted status from the server.
// Returns whether the server was blacklisted.
func (s *ServerStatistics) removeBlacklist() bool {
	var wasBlacklisted bool

	if s.Blacklisted() {
		wasBlacklisted = true
		_ = s.statistics.DB.RemoveServerFromBlacklist(s.serverName)
	}
	s.cancel()
	s.backoffCount.Store(0)

	return wasBlacklisted
}

// removeAssumedOffline removes the assumed offline status from the server.
func (s *ServerStatistics) removeAssumedOffline() {
	if s.AssumedOffline() {
		_ = s.statistics.DB.RemoveServerAssumedOffline(context.Background(), s.serverName)
	}
	s.assumedOffline.Store(false)
}

// SuccessCount returns the number of successful requests. This is
// usually useful in constructing transaction IDs.
func (s *ServerStatistics) SuccessCount() uint32 {
	return s.successCounter.Load()
}

// KnownRelayServers returns the list of relay servers associated with this
// server.
func (s *ServerStatistics) KnownRelayServers() []spec.ServerName {
	s.relayMutex.Lock()
	defer s.relayMutex.Unlock()
	return s.knownRelayServers
}

func (s *ServerStatistics) AddRelayServers(relayServers []spec.ServerName) {
	seenSet := make(map[spec.ServerName]bool)
	uniqueList := []spec.ServerName{}
	for _, srv := range relayServers {
		if seenSet[srv] {
			continue
		}
		seenSet[srv] = true
		uniqueList = append(uniqueList, srv)
	}

	err := s.statistics.DB.P2PAddRelayServersForServer(context.Background(), s.serverName, uniqueList)
	if err != nil {
		logrus.WithError(err).Errorf("Failed to add relay servers for %q. Servers: %v", s.serverName, uniqueList)
		return
	}

	for _, newServer := range uniqueList {
		alreadyKnown := false
		knownRelayServers := s.KnownRelayServers()
		for _, srv := range knownRelayServers {
			if srv == newServer {
				alreadyKnown = true
			}
		}
		if !alreadyKnown {
			{
				s.relayMutex.Lock()
				s.knownRelayServers = append(s.knownRelayServers, newServer)
				s.relayMutex.Unlock()
			}
		}
	}
}
