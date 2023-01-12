package statistics

import (
	"math"
	"testing"
	"time"

	"github.com/matrix-org/dendrite/federationapi/storage"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/stretchr/testify/assert"
)

func TestBackoff(t *testing.T) {
	stats := NewStatistics(nil, 7, 2)
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
	stats := NewStatistics(storage.NewFakeFederationDatabase(), 8, 3)
	server := ServerStatistics{statistics: &stats}
	server.AddRelayServers([]gomatrixserverlib.ServerName{"relay1", "relay1", "relay2"})
	relayServers := server.KnownRelayServers()
	assert.Equal(t, []gomatrixserverlib.ServerName{"relay1", "relay2"}, relayServers)
	server.AddRelayServers([]gomatrixserverlib.ServerName{"relay1", "relay1", "relay2"})
	relayServers = server.KnownRelayServers()
	assert.Equal(t, []gomatrixserverlib.ServerName{"relay1", "relay2"}, relayServers)
}
