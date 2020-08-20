package statistics

import (
	"math"
	"testing"
	"time"

	"go.uber.org/atomic"
)

func TestBackoff(t *testing.T) {
	stats := Statistics{
		FailuresUntilBlacklist: 7,
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

	// Register a failure.
	server.Failure()

	t.Logf("Backoff counter: %d", server.backoffCount.Load())
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
		duration, blacklist := server.BackoffIfRequired(backingOff, interrupt)

		// Register another failure for good measure. This should have no
		// side effects since a backoff is already in progress. If it does
		// then we'll fail.
		until, blacklisted := server.Failure()
		if time.Until(until) > duration {
			t.Fatal("Failure produced unexpected side effect when it shouldn't have")
		}

		// Check if we should be blacklisted by now.
		if i >= stats.FailuresUntilBlacklist {
			if !blacklist {
				t.Fatalf("Backoff %d should have resulted in blacklist but didn't", i)
			} else if blacklist != blacklisted {
				t.Fatalf("BackoffIfRequired and Failure returned different blacklist values")
			} else {
				t.Logf("Backoff %d is blacklisted as expected", i)
				continue
			}
		}

		// Check if the duration is what we expect.
		t.Logf("Backoff %d is for %s", i, duration)
		if wanted := time.Second * time.Duration(math.Exp2(float64(i))); !blacklist && duration != wanted {
			t.Fatalf("Backoff %d should have been %s but was %s", i, wanted, duration)
		}
	}
}
