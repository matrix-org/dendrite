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
) (*gomatrixserverlib.RespState, error) {
	// A list of events that we have retried, if they were not included in
	// the auth events supplied in the send_join.
	retries := map[string][]gomatrixserverlib.Event{}

	// Define a function which we can pass to Check to retrieve missing
	// auth events inline. This greatly increases our chances of not having
	// to repeat the entire set of checks just for a missing event or two.
	missingAuth := func(roomVersion gomatrixserverlib.RoomVersion, eventIDs []string) ([]gomatrixserverlib.Event, error) {
		returning := []gomatrixserverlib.Event{}

		// See if we have retry entries for each of the supplied event IDs.
		for _, eventID := range eventIDs {
			// If we've already satisfied a request for this event ID before then
			// just append the results. We won't retry the request.
			if retry, ok := retries[eventID]; ok {
				if retry == nil {
					return nil, fmt.Errorf("missingAuth: not retrying failed event ID %q", eventID)
				}
				returning = append(returning, retry...)
				continue
			}

			// Make a note of the fact that we tried to do something with this
			// event ID, even if we don't succeed.
			retries[event.EventID()] = nil

			// Try to retrieve the event from the server that sent us the send
			// join response.
			tx, txerr := r.federation.GetEvent(ctx, server, eventID)
			if txerr != nil {
				return nil, fmt.Errorf("missingAuth r.federation.GetEvent: %w", txerr)
			}

			// For each event returned, add it to the set of return events. We
			// also will populate the retries, in case someone asks for this
			// event ID again.
			for _, pdu := range tx.PDUs {
				// Try to parse the event.
				ev, everr := gomatrixserverlib.NewEventFromUntrustedJSON(pdu, roomVersion)
				if everr != nil {
					return nil, fmt.Errorf("missingAuth gomatrixserverlib.NewEventFromUntrustedJSON: %w", everr)
				}

				// Check the signatures of the event.
				if res, err := gomatrixserverlib.VerifyEventSignatures(ctx, []gomatrixserverlib.Event{ev}, r.keyRing); err != nil {
					return nil, fmt.Errorf("missingAuth VerifyEventSignatures: %w", err)
				} else {
					for _, err := range res {
						if err != nil {
							return nil, fmt.Errorf("missingAuth VerifyEventSignatures: %w", err)
						}
					}
				}

				// If the event is OK then add it to the results and the retry map.
				returning = append(returning, ev)
				retries[event.EventID()] = append(retries[event.EventID()], ev)
				retries[ev.EventID()] = append(retries[ev.EventID()], ev)
			}
		}

		return returning, nil
	}

	// TODO: Can we expand Check here to return a list of missing auth
	// events rather than failing one at a time?
	rs, err := respSendJoin.Check(ctx, r.keyRing, event, missingAuth)
	if err != nil {
		return nil, fmt.Errorf("respSendJoin: %w", err)
	}
	return rs, nil
}
