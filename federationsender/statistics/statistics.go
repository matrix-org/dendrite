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
	backoffUntil   atomic.Value                 // time.Time until this backoff interval ends
	backoffCount   atomic.Uint32                // number of times BackoffDuration has been called
	successCounter atomic.Uint32                // how many times have we succeeded?
}

// duration returns how long the next backoff interval should be.
func (s *ServerStatistics) duration(count uint32) time.Duration {
	return time.Second * time.Duration(math.Exp2(float64(count)))
}

// Success updates the server statistics with a new successful
// attempt, which increases the sent counter and resets the idle and
// failure counters. If a host was blacklisted at this point then
// we will unblacklist it.
func (s *ServerStatistics) Success() {
	s.successCounter.Add(1)
	s.backoffStarted.Store(false)
	s.backoffCount.Store(0)
	s.blacklisted.Store(false)
	if s.statistics.DB != nil {
		if err := s.statistics.DB.RemoveServerFromBlacklist(s.serverName); err != nil {
			logrus.WithError(err).Errorf("Failed to remove %q from blacklist", s.serverName)
		}
	}
}

// Failure marks a failure and starts backing off if needed.
// The next call to BackoffIfRequired will do the right thing
// after this. It will return the time that the current failure
// will result in backoff waiting until, and a bool signalling
// whether we have blacklisted and therefore to give up.
func (s *ServerStatistics) Failure() (time.Time, bool) {
	// If we aren't already backing off, this call will start
	// a new backoff period. Reset the counter to 0 so that
	// we backoff only for short periods of time to start with.
	if s.backoffStarted.CAS(false, true) {
		s.backoffCount.Store(0)
	}

	// Check if we have blacklisted this node.
	if s.blacklisted.Load() {
		return time.Now(), true
	}

	// If we're already backing off and we haven't yet surpassed
	// the deadline then return that. Repeated calls to Failure
	// within a single backoff interval will have no side effects.
	if until, ok := s.backoffUntil.Load().(time.Time); ok && !time.Now().After(until) {
		return until, false
	}

	// We're either backing off and have passed the deadline, or
	// we aren't backing off, so work out what the next interval
	// will be.
	count := s.backoffCount.Load()
	until := time.Now().Add(s.duration(count))
	s.backoffUntil.Store(until)
	return until, false
}

// BackoffInfo returns information about the current or previous backoff.
// Returns the last backoffUntil time and whether the server is currently blacklisted or not.
func (s *ServerStatistics) BackoffInfo() (*time.Time, bool) {
	until, ok := s.backoffUntil.Load().(time.Time)
	if ok {
		return &until, s.blacklisted.Load()
	}
	return nil, s.blacklisted.Load()
}

// BackoffIfRequired will block for as long as the current
// backoff requires, if needed. Otherwise it will do nothing.
// Returns the amount of time to backoff for and whether to give up or not.
func (s *ServerStatistics) BackoffIfRequired(backingOff atomic.Bool, interrupt <-chan bool) (time.Duration, bool) {
	if started := s.backoffStarted.Load(); !started {
		return 0, false
	}

	// Work out if we should be blacklisting at this point.
	count := s.backoffCount.Inc()
	if count >= s.statistics.FailuresUntilBlacklist {
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
		return 0, true
	}

	// Work out when we should wait until.
	duration := s.duration(count)
	until := time.Now().Add(duration)
	s.backoffUntil.Store(until)

	// Notify the destination queue that we're backing off now.
	backingOff.Store(true)
	defer backingOff.Store(false)

	// Work out how long we should be backing off for.
	logrus.Warnf("Backing off %q for %s", s.serverName, duration)

	// Wait for either an interruption or for the backoff to
	// complete.
	select {
	case <-interrupt:
		logrus.Debugf("Interrupting backoff for %q", s.serverName)
	case <-time.After(duration):
	}

	return duration, false
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
