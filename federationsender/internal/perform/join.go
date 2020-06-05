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
	retries := map[string]*gomatrixserverlib.Event{}

	// Define a function which we can pass to Check to retrieve missing
	// auth events inline. This greatly increases our chances of not having
	// to repeat the entire set of checks just for a missing event or two.
	missingAuth := func(eventID string, roomVersion gomatrixserverlib.RoomVersion) (*gomatrixserverlib.Event, error) {
		if retry, ok := retries[eventID]; ok {
			if retry == nil {
				return nil, fmt.Errorf("missingAuth: not retrying failed event ID %q", eventID)
			}
			return retries[eventID], nil
		}
		// Make a note of the fact that we tried to do something with this
		// event ID, even if we don't succeed.
		retries[eventID] = nil
		// Try to retrieve the event from the server that sent us the send
		// join response.
		tx, txerr := r.federation.GetEvent(ctx, server, eventID)
		if txerr != nil {
			return nil, fmt.Errorf("missingAuth r.federation.GetEvent: %w", txerr)
		}
		// For each event returned, add it to the auth events.
		for _, pdu := range tx.PDUs {
			ev, everr := gomatrixserverlib.NewEventFromUntrustedJSON(pdu, roomVersion)
			if everr != nil {
				return nil, fmt.Errorf("missingAuth gomatrixserverlib.NewEventFromUntrustedJSON: %w", everr)
			}
			respSendJoin.AuthEvents = append(respSendJoin.AuthEvents, ev)
			retries[ev.EventID()] = &ev
		}
		return retries[eventID], nil
	}

	// TODO: Can we expand Check here to return a list of missing auth
	// events rather than failing one at a time?
	if err := respSendJoin.Check(ctx, r.keyRing, event, missingAuth); err != nil {
		return fmt.Errorf("respSendJoin: %w", err)
	}
	return nil
}
