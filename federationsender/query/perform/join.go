package perform

import (
	"context"
	"fmt"

	"github.com/matrix-org/gomatrixserverlib"
)

// This file contains helpers for the PerformJoin function.

type joinContext struct {
	federation *gomatrixserverlib.FederationClient
	keyRing    *gomatrixserverlib.KeyRing
}

// Returns a new join context.
func JoinContext(f *gomatrixserverlib.FederationClient, k *gomatrixserverlib.KeyRing) *joinContext {
	return &joinContext{
		federation: f,
		keyRing:    k,
	}
}

// checkSendJoinResponse checks that all of the signatures are correct
// and that the join is allowed by the supplied state.
func (r joinContext) CheckSendJoinResponse(
	ctx context.Context,
	event gomatrixserverlib.Event,
	server gomatrixserverlib.ServerName,
	respMakeJoin gomatrixserverlib.RespMakeJoin,
	respSendJoin gomatrixserverlib.RespSendJoin,
) error {
	// A list of events that we have retried, if they were not included in
	// the auth events supplied in the send_join.
	retries := map[string]bool{}

retryCheck:
	// TODO: Can we expand Check here to return a list of missing auth
	// events rather than failing one at a time?
	if err := respSendJoin.Check(ctx, r.keyRing, event); err != nil {
		switch e := err.(type) {
		case gomatrixserverlib.MissingAuthEventError:
			// Check that we haven't already retried for this event, prevents
			// us from ending up in endless loops
			if !retries[e.AuthEventID] {
				// Ask the server that we're talking to right now for the event
				tx, txerr := r.federation.GetEvent(ctx, server, e.AuthEventID)
				if txerr != nil {
					return fmt.Errorf("r.federation.GetEvent: %w", txerr)
				}
				// For each event returned, add it to the auth events.
				for _, pdu := range tx.PDUs {
					ev, everr := gomatrixserverlib.NewEventFromUntrustedJSON(pdu, respMakeJoin.RoomVersion)
					if everr != nil {
						return fmt.Errorf("gomatrixserverlib.NewEventFromUntrustedJSON: %w", everr)
					}
					respSendJoin.AuthEvents = append(respSendJoin.AuthEvents, ev)
				}
				// Mark the event as retried and then give the check another go.
				retries[e.AuthEventID] = true
				goto retryCheck
			}
			return fmt.Errorf("respSendJoin (after retries): %w", e)
		default:
			return fmt.Errorf("respSendJoin: %w", err)
		}
	}
	return nil
}
