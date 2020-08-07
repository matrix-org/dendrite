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

	server.Success()
	if successes := server.SuccessCount(); successes != 1 {
		t.Fatalf("Expected success count 1, got %d", successes)
	}

	for i := uint32(1); i <= stats.FailuresUntilBlacklist; i++ {
		if server.Failure() == (i < stats.FailuresUntilBlacklist) {
			t.Fatalf("Failure %d resulted in blacklist too soon", i)
		}
	}

	t.Logf("Failure counter: %d", server.failCounter)
	t.Logf("Backoff counter: %d", server.backoffCount)

	backingOff := atomic.Bool{}

	for i := uint32(1); i <= 10; i++ {
		interrupt := make(chan bool, 1)
		close(interrupt)

		duration := server.BackoffIfRequired(backingOff, interrupt)
		t.Logf("Backoff %d is for %s", i, duration)

		if i < stats.FailuresUntilBlacklist {
			if wanted := time.Second * time.Duration(math.Exp2(float64(i))); duration != wanted {
				t.Fatalf("Backoff should have been %s but was %s", wanted, duration)
			}
		} else {
			if duration != 0 {
				t.Fatalf("Backoff should have been zero but was %s", duration)
			}
		}
	}
}
