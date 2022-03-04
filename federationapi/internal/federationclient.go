package internal

import (
	"context"
	"time"

	"github.com/matrix-org/gomatrixserverlib"
)

// Functions here are "proxying" calls to the gomatrixserverlib federation
// client.

func (a *FederationInternalAPI) GetEventAuth(
	ctx context.Context, s gomatrixserverlib.ServerName,
	roomVersion gomatrixserverlib.RoomVersion, roomID, eventID string,
) (res gomatrixserverlib.RespEventAuth, err error) {
	ctx, cancel := context.WithTimeout(ctx, time.Second*30)
	defer cancel()
	ires, err := a.doRequestIfNotBlacklisted(s, func() (interface{}, error) {
		return a.federation.GetEventAuth(ctx, s, roomVersion, roomID, eventID)
	})
	if err != nil {
		return gomatrixserverlib.RespEventAuth{}, err
	}
	return ires.(gomatrixserverlib.RespEventAuth), nil
}

func (a *FederationInternalAPI) GetUserDevices(
	ctx context.Context, s gomatrixserverlib.ServerName, userID string,
) (gomatrixserverlib.RespUserDevices, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Second*30)
	defer cancel()
	ires, err := a.doRequestIfNotBlacklisted(s, func() (interface{}, error) {
		return a.federation.GetUserDevices(ctx, s, userID)
	})
	if err != nil {
		return gomatrixserverlib.RespUserDevices{}, err
	}
	return ires.(gomatrixserverlib.RespUserDevices), nil
}

func (a *FederationInternalAPI) ClaimKeys(
	ctx context.Context, s gomatrixserverlib.ServerName, oneTimeKeys map[string]map[string]string,
) (gomatrixserverlib.RespClaimKeys, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Second*30)
	defer cancel()
	ires, err := a.doRequestIfNotBackingOffOrBlacklisted(s, func() (interface{}, error) {
		return a.federation.ClaimKeys(ctx, s, oneTimeKeys)
	})
	if err != nil {
		return gomatrixserverlib.RespClaimKeys{}, err
	}
	return ires.(gomatrixserverlib.RespClaimKeys), nil
}

func (a *FederationInternalAPI) QueryKeys(
	ctx context.Context, s gomatrixserverlib.ServerName, keys map[string][]string,
) (gomatrixserverlib.RespQueryKeys, error) {
	ires, err := a.doRequestIfNotBackingOffOrBlacklisted(s, func() (interface{}, error) {
		return a.federation.QueryKeys(ctx, s, keys)
	})
	if err != nil {
		return gomatrixserverlib.RespQueryKeys{}, err
	}
	return ires.(gomatrixserverlib.RespQueryKeys), nil
}

func (a *FederationInternalAPI) Backfill(
	ctx context.Context, s gomatrixserverlib.ServerName, roomID string, limit int, eventIDs []string,
) (res gomatrixserverlib.Transaction, err error) {
	ctx, cancel := context.WithTimeout(ctx, time.Second*30)
	defer cancel()
	ires, err := a.doRequestIfNotBlacklisted(s, func() (interface{}, error) {
		return a.federation.Backfill(ctx, s, roomID, limit, eventIDs)
	})
	if err != nil {
		return gomatrixserverlib.Transaction{}, err
	}
	return ires.(gomatrixserverlib.Transaction), nil
}

func (a *FederationInternalAPI) LookupState(
	ctx context.Context, s gomatrixserverlib.ServerName, roomID, eventID string, roomVersion gomatrixserverlib.RoomVersion,
) (res gomatrixserverlib.RespState, err error) {
	ctx, cancel := context.WithTimeout(ctx, time.Second*30)
	defer cancel()
	ires, err := a.doRequestIfNotBlacklisted(s, func() (interface{}, error) {
		return a.federation.LookupState(ctx, s, roomID, eventID, roomVersion)
	})
	if err != nil {
		return gomatrixserverlib.RespState{}, err
	}
	return ires.(gomatrixserverlib.RespState), nil
}

func (a *FederationInternalAPI) LookupStateIDs(
	ctx context.Context, s gomatrixserverlib.ServerName, roomID, eventID string,
) (res gomatrixserverlib.RespStateIDs, err error) {
	ctx, cancel := context.WithTimeout(ctx, time.Second*30)
	defer cancel()
	ires, err := a.doRequestIfNotBlacklisted(s, func() (interface{}, error) {
		return a.federation.LookupStateIDs(ctx, s, roomID, eventID)
	})
	if err != nil {
		return gomatrixserverlib.RespStateIDs{}, err
	}
	return ires.(gomatrixserverlib.RespStateIDs), nil
}

func (a *FederationInternalAPI) LookupMissingEvents(
	ctx context.Context, s gomatrixserverlib.ServerName, roomID string,
	missing gomatrixserverlib.MissingEvents, roomVersion gomatrixserverlib.RoomVersion,
) (res gomatrixserverlib.RespMissingEvents, err error) {
	ctx, cancel := context.WithTimeout(ctx, time.Second*30)
	defer cancel()
	ires, err := a.doRequestIfNotBlacklisted(s, func() (interface{}, error) {
		return a.federation.LookupMissingEvents(ctx, s, roomID, missing, roomVersion)
	})
	if err != nil {
		return gomatrixserverlib.RespMissingEvents{}, err
	}
	return ires.(gomatrixserverlib.RespMissingEvents), nil
}

func (a *FederationInternalAPI) GetEvent(
	ctx context.Context, s gomatrixserverlib.ServerName, eventID string,
) (res gomatrixserverlib.Transaction, err error) {
	ctx, cancel := context.WithTimeout(ctx, time.Second*30)
	defer cancel()
	ires, err := a.doRequestIfNotBlacklisted(s, func() (interface{}, error) {
		return a.federation.GetEvent(ctx, s, eventID)
	})
	if err != nil {
		return gomatrixserverlib.Transaction{}, err
	}
	return ires.(gomatrixserverlib.Transaction), nil
}

func (a *FederationInternalAPI) LookupServerKeys(
	ctx context.Context, s gomatrixserverlib.ServerName, keyRequests map[gomatrixserverlib.PublicKeyLookupRequest]gomatrixserverlib.Timestamp,
) ([]gomatrixserverlib.ServerKeys, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	ires, err := a.doRequestIfNotBlacklisted(s, func() (interface{}, error) {
		return a.federation.LookupServerKeys(ctx, s, keyRequests)
	})
	if err != nil {
		return []gomatrixserverlib.ServerKeys{}, err
	}
	return ires.([]gomatrixserverlib.ServerKeys), nil
}

func (a *FederationInternalAPI) MSC2836EventRelationships(
	ctx context.Context, s gomatrixserverlib.ServerName, r gomatrixserverlib.MSC2836EventRelationshipsRequest,
	roomVersion gomatrixserverlib.RoomVersion,
) (res gomatrixserverlib.MSC2836EventRelationshipsResponse, err error) {
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	ires, err := a.doRequestIfNotBlacklisted(s, func() (interface{}, error) {
		return a.federation.MSC2836EventRelationships(ctx, s, r, roomVersion)
	})
	if err != nil {
		return res, err
	}
	return ires.(gomatrixserverlib.MSC2836EventRelationshipsResponse), nil
}

func (a *FederationInternalAPI) MSC2946Spaces(
	ctx context.Context, s gomatrixserverlib.ServerName, roomID string, suggestedOnly bool,
) (res gomatrixserverlib.MSC2946SpacesResponse, err error) {
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	ires, err := a.doRequestIfNotBlacklisted(s, func() (interface{}, error) {
		return a.federation.MSC2946Spaces(ctx, s, roomID, suggestedOnly)
	})
	if err != nil {
		return res, err
	}
	return ires.(gomatrixserverlib.MSC2946SpacesResponse), nil
}
