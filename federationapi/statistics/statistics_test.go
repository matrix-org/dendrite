package statistics

import (
	"math"
	"testing"
	"time"

	"github.com/element-hq/dendrite/test"
	"github.com/matrix-org/gomatrixserverlib/spec"
	"github.com/stretchr/testify/assert"
)

const (
	FailuresUntilAssumedOffline = 3
	FailuresUntilBlacklist      = 8
)

func TestBackoff(t *testing.T) {
	stats := NewStatistics(nil, FailuresUntilBlacklist, FailuresUntilAssumedOffline, false)
	server := ServerStatistics{
		statistics: &stats,
		serverName: "test.com",
	}

	// Start by checking that counting successes works.
	server.Success(SendDirect)
	if successes := server.SuccessCount(); successes != 1 {
		t.Fatalf("Expected success count 1, got %d", successes)
	}

	// Register a failure.
	server.Failure()

	t.Logf("Backoff counter: %d", server.backoffCount.Load())

	// Now we're going to simulate backing off a few times to see
	// what happens.
	for i := uint32(1); i <= 10; i++ {
		// Register another failure for good measure. This should have no
		// side effects since a backoff is already in progress. If it does
		// then we'll fail.
		until, blacklisted := server.Failure()
		blacklist := server.Blacklisted()
		assumedOffline := server.AssumedOffline()
		duration := time.Until(until)

		// Unset the backoff, or otherwise our next call will think that
		// there's a backoff in progress and return the same result.
		server.cancel()
		server.backoffStarted.Store(false)

		if i >= stats.FailuresUntilAssumedOffline {
			if !assumedOffline {
				t.Fatalf("Backoff %d should have resulted in assuming the destination was offline but didn't", i)
			}
		}

		// Check if we should be assumed offline by now.
		if i >= stats.FailuresUntilAssumedOffline {
			if !assumedOffline {
				t.Fatalf("Backoff %d should have resulted in assumed offline but didn't", i)
			} else {
				t.Logf("Backoff %d is assumed offline as expected", i)
			}
		} else {
			if assumedOffline {
				t.Fatalf("Backoff %d should not have resulted in assumed offline but did", i)
			} else {
				t.Logf("Backoff %d is not assumed offline as expected", i)
			}
		}

		// Check if we should be blacklisted by now.
		if i >= stats.FailuresUntilBlacklist {
			if !blacklist {
				t.Fatalf("Backoff %d should have resulted in blacklist but didn't", i)
			} else if blacklist != blacklisted {
				t.Fatalf("Blacklisted and Failure returned different blacklist values")
			} else {
				t.Logf("Backoff %d is blacklisted as expected", i)
				continue
			}
		} else {
			if blacklist {
				t.Fatalf("Backoff %d should not have resulted in blacklist but did", i)
			} else {
				t.Logf("Backoff %d is not blacklisted as expected", i)
			}
		}

		// Check if the duration is what we expect.
		t.Logf("Backoff %d is for %s", i, duration)
		roundingAllowance := 0.01
		minDuration := time.Millisecond * time.Duration(math.Exp2(float64(i))*minJitterMultiplier*1000-roundingAllowance)
		maxDuration := time.Millisecond * time.Duration(math.Exp2(float64(i))*maxJitterMultiplier*1000+roundingAllowance)
		var inJitterRange bool
		if duration >= minDuration && duration <= maxDuration {
			inJitterRange = true
		} else {
			inJitterRange = false
		}
		if !blacklist && !inJitterRange {
			t.Fatalf("Backoff %d should have been between %s and %s but was %s", i, minDuration, maxDuration, duration)
		}
	}
}

func TestRelayServersListing(t *testing.T) {
	stats := NewStatistics(test.NewInMemoryFederationDatabase(), FailuresUntilBlacklist, FailuresUntilAssumedOffline, false)
	server := ServerStatistics{statistics: &stats}
	server.AddRelayServers([]spec.ServerName{"relay1", "relay1", "relay2"})
	relayServers := server.KnownRelayServers()
	assert.Equal(t, []spec.ServerName{"relay1", "relay2"}, relayServers)
	server.AddRelayServers([]spec.ServerName{"relay1", "relay1", "relay2"})
	relayServers = server.KnownRelayServers()
	assert.Equal(t, []spec.ServerName{"relay1", "relay2"}, relayServers)
}
