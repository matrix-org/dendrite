package v2

import (
	"time"

	"github.com/matrix-org/dendrite/roomserver/types"
	"github.com/prometheus/client_golang/prometheus"
)

var calculateStateDurations = prometheus.NewSummaryVec(
	prometheus.SummaryOpts{
		Namespace: "dendrite",
		Subsystem: "roomserver",
		Name:      "calculate_state_duration_microseconds",
		Help:      "How long it takes to calculate the state after a list of events",
	},
	// Takes two labels:
	//   algorithm:
	//      The algorithm used to calculate the state or the step it failed on if it failed.
	//      Labels starting with "_" are used to indicate when the algorithm fails halfway.
	//  outcome:
	//      Whether the state was successfully calculated.
	//
	// The possible values for algorithm are:
	//    empty_state -> The list of events was empty so the state is empty.
	//    no_change -> The state hasn't changed.
	//    single_delta -> There was a single event added to the state in a way that can be encoded as a single delta
	//    full_state_no_conflicts -> We created a new copy of the full room state, but didn't enounter any conflicts
	//                               while doing so.
	//    full_state_with_conflicts -> We created a new copy of the full room state and had to resolve conflicts to do so.
	//    _load_state_block_nids -> Failed loading the state block nids for a single previous state.
	//    _load_combined_state -> Failed to load the combined state.
	//    _resolve_conflicts -> Failed to resolve conflicts.
	[]string{"algorithm", "outcome"},
)

var calculateStatePrevEventLength = prometheus.NewSummaryVec(
	prometheus.SummaryOpts{
		Namespace: "dendrite",
		Subsystem: "roomserver",
		Name:      "calculate_state_prev_event_length",
		Help:      "The length of the list of events to calculate the state after",
	},
	[]string{"algorithm", "outcome"},
)

var calculateStateFullStateLength = prometheus.NewSummaryVec(
	prometheus.SummaryOpts{
		Namespace: "dendrite",
		Subsystem: "roomserver",
		Name:      "calculate_state_full_state_length",
		Help:      "The length of the full room state.",
	},
	[]string{"algorithm", "outcome"},
)

var calculateStateConflictLength = prometheus.NewSummaryVec(
	prometheus.SummaryOpts{
		Namespace: "dendrite",
		Subsystem: "roomserver",
		Name:      "calculate_state_conflict_state_length",
		Help:      "The length of the conflicted room state.",
	},
	[]string{"algorithm", "outcome"},
)

type calculateStateMetrics struct {
	algorithm       string
	startTime       time.Time
	prevEventLength int
	fullStateLength int
	conflictLength  int
}

func (c *calculateStateMetrics) stop(stateNID types.StateSnapshotNID, err error) (types.StateSnapshotNID, error) {
	var outcome string
	if err == nil {
		outcome = "success"
	} else {
		outcome = "failure"
	}
	endTime := time.Now()
	calculateStateDurations.WithLabelValues(c.algorithm, outcome).Observe(
		float64(endTime.Sub(c.startTime).Nanoseconds()) / 1000.,
	)
	calculateStatePrevEventLength.WithLabelValues(c.algorithm, outcome).Observe(
		float64(c.prevEventLength),
	)
	calculateStateFullStateLength.WithLabelValues(c.algorithm, outcome).Observe(
		float64(c.fullStateLength),
	)
	calculateStateConflictLength.WithLabelValues(c.algorithm, outcome).Observe(
		float64(c.conflictLength),
	)
	return stateNID, err
}

func init() {
	prometheus.MustRegister(
		calculateStateDurations, calculateStatePrevEventLength,
		calculateStateFullStateLength, calculateStateConflictLength,
	)
}
