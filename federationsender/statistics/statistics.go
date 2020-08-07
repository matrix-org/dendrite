package statistics

import (
	"math"
	"sync"
	"time"

	"github.com/matrix-org/dendrite/federationsender/storage"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/sirupsen/logrus"
	"go.uber.org/atomic"
)

// Statistics contains information about all of the remote federated
// hosts that we have interacted with. It is basically a threadsafe
// wrapper.
type Statistics struct {
	DB      storage.Database
	servers map[gomatrixserverlib.ServerName]*ServerStatistics
	mutex   sync.RWMutex

	// How many times should we tolerate consecutive failures before we
	// just blacklist the host altogether? The backoff is exponential,
	// so the max time here to attempt is 2**failures seconds.
	FailuresUntilBlacklist uint32
}

// ForServer returns server statistics for the given server name. If it
// does not exist, it will create empty statistics and return those.
func (s *Statistics) ForServer(serverName gomatrixserverlib.ServerName) *ServerStatistics {
	// If the map hasn't been initialised yet then do that.
	if s.servers == nil {
		s.mutex.Lock()
		s.servers = make(map[gomatrixserverlib.ServerName]*ServerStatistics)
		s.mutex.Unlock()
	}
	// Look up if we have statistics for this server already.
	s.mutex.RLock()
	server, found := s.servers[serverName]
	s.mutex.RUnlock()
	// If we don't, then make one.
	if !found {
		s.mutex.Lock()
		server = &ServerStatistics{
			statistics: s,
			serverName: serverName,
		}
		s.servers[serverName] = server
		s.mutex.Unlock()
		blacklisted, err := s.DB.IsServerBlacklisted(serverName)
		if err != nil {
			logrus.WithError(err).Errorf("Failed to get blacklist entry %q", serverName)
		} else {
			server.blacklisted.Store(blacklisted)
		}
	}
	return server
}

// ServerStatistics contains information about our interactions with a
// remote federated host, e.g. how many times we were successful, how
// many times we failed etc. It also manages the backoff time and black-
// listing a remote host if it remains uncooperative.
type ServerStatistics struct {
	statistics     *Statistics                  //
	serverName     gomatrixserverlib.ServerName //
	blacklisted    atomic.Bool                  // is the node blacklisted
	backoffStarted atomic.Bool                  // is the backoff started
	backoffCount   atomic.Uint32                // number of times BackoffDuration has been called
	failCounter    atomic.Uint32                // how many times have we failed?
	successCounter atomic.Uint32                // how many times have we succeeded?
}

// Success updates the server statistics with a new successful
// attempt, which increases the sent counter and resets the idle and
// failure counters. If a host was blacklisted at this point then
// we will unblacklist it.
func (s *ServerStatistics) Success() {
	s.successCounter.Add(1)
	s.failCounter.Store(0)
	s.backoffStarted.Store(false)
	s.backoffCount.Store(0)
	s.blacklisted.Store(false)
	if s.statistics.DB != nil {
		if err := s.statistics.DB.RemoveServerFromBlacklist(s.serverName); err != nil {
			logrus.WithError(err).Errorf("Failed to remove %q from blacklist", s.serverName)
		}
	}
}

// Failure marks a failure and works out when to backoff until. It
// returns true if the worker should give up altogether because of
// too many consecutive failures. At this point the host is marked
// as blacklisted.
func (s *ServerStatistics) Failure() bool {
	// Increase the fail counter.
	failCounter := s.failCounter.Add(1)

	// Check that we haven't failed more times than is acceptable.
	if failCounter >= s.statistics.FailuresUntilBlacklist {
		// We've exceeded the maximum amount of times we're willing
		// to back off, which is probably in the region of hours by
		// now. Mark the host as blacklisted and tell the caller to
		// give up.
		s.blacklisted.Store(true)
		if s.statistics.DB != nil {
			if err := s.statistics.DB.AddServerToBlacklist(s.serverName); err != nil {
				logrus.WithError(err).Errorf("Failed to add %q to blacklist", s.serverName)
			}
		}
		return true
	}

	if s.backoffStarted.CAS(false, true) {
		s.backoffCount.Store(0)
	}

	return false
}

// BackoffIfRequired will block for as long as the current
// backoff requires, if needed. Otherwise it will do nothing.
func (s *ServerStatistics) BackoffIfRequired(backingOff atomic.Bool, interrupt <-chan bool) time.Duration {
	if started := s.backoffStarted.Load(); !started {
		return 0
	}

	// Work out how many times we've backed off so far. If we
	// have passed the failure counter then we can stop backing
	// off - we've done our time.
	count := s.backoffCount.Inc()
	if count >= s.failCounter.Load() {
		s.backoffStarted.Store(false)
		return 0
	}

	// Notify the destination queue that we're backing off now.
	backingOff.Store(true)
	defer backingOff.Store(false)

	// Work out how long we should be backing off for.
	duration := time.Second * time.Duration(math.Exp2(float64(count)))
	logrus.Debugf("Backing off %q for %d", s.serverName, duration)

	// Wait for either an interruption or for the backoff to
	// complete.
	select {
	case <-interrupt:
		logrus.Infof("Interrupting backoff for %q", s.serverName)
	case <-time.After(duration):
	}

	// Drain the interrupt queue - helps with tests.
	for range interrupt {
	}

	return duration
}

// Blacklisted returns true if the server is blacklisted and false
// otherwise.
func (s *ServerStatistics) Blacklisted() bool {
	return s.blacklisted.Load()
}

// SuccessCount returns the number of successful requests. This is
// usually useful in constructing transaction IDs.
func (s *ServerStatistics) SuccessCount() uint32 {
	return s.successCounter.Load()
}
