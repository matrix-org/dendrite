package statistics

import (
	"math"
	"testing"
	"time"

	"go.uber.org/atomic"
)

func TestBackoff(t *testing.T) {
	stats := Statistics{
		FailuresUntilBlacklist: 6,
	}
	server := ServerStatistics{
		statistics: &stats,
		serverName: "test.com",
	}

	// Start by checking that counting successes works.
	server.Success()
	if successes := server.SuccessCount(); successes != 1 {
		t.Fatalf("Expected success count 1, got %d", successes)
	}

	// Now we want to cause a series of failures. We'll do this
	// as many times as we need to blacklist. We'll check that we
	// were blacklisted at the right time based on the threshold.
	failures := stats.FailuresUntilBlacklist
	for i := uint32(1); i <= failures; i++ {
		if server.Failure() == (i < stats.FailuresUntilBlacklist) {
			t.Fatalf("Failure %d resulted in blacklist too soon", i)
		}
	}

	t.Logf("Failure counter: %d", server.failCounter)
	t.Logf("Backoff counter: %d", server.backoffCount)
	backingOff := atomic.Bool{}

	// Now we're going to simulate backing off a few times to see
	// what happens.
	for i := uint32(1); i <= 10; i++ {
		// Interrupt the backoff - it doesn't really matter if it
		// completes but we will find out how long the backoff should
		// have been.
		interrupt := make(chan bool, 1)
		close(interrupt)

		// Get the duration.
		duration := server.BackoffIfRequired(backingOff, interrupt)
		t.Logf("Backoff %d is for %s", i, duration)

		// Check if the duration is what we expect.
		if wanted := time.Second * time.Duration(math.Exp2(float64(i))); duration != wanted {
			t.Fatalf("Backoff should have been %s but was %s", wanted, duration)
		}
	}
}
