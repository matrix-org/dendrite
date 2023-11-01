package internal

import (
	"context"
	"time"

	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/gomatrixserverlib/fclient"
	"github.com/matrix-org/gomatrixserverlib/spec"
)

const defaultTimeout = time.Second * 30

// Functions here are "proxying" calls to the gomatrixserverlib federation
// client.

func (a *FederationInternalAPI) MakeJoin(
	ctx context.Context, origin, s spec.ServerName, roomID, userID string,
) (res gomatrixserverlib.MakeJoinResponse, err error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()
	ires, err := a.federation.MakeJoin(ctx, origin, s, roomID, userID)
	if err != nil {
		return &fclient.RespMakeJoin{}, err
	}
	return &ires, nil
}

func (a *FederationInternalAPI) SendJoin(
	ctx context.Context, origin, s spec.ServerName, event gomatrixserverlib.PDU,
) (res gomatrixserverlib.SendJoinResponse, err error) {
	ctx, cancel := context.WithTimeout(ctx, time.Minute*5)
	defer cancel()
	ires, err := a.federation.SendJoin(ctx, origin, s, event)
	if err != nil {
		return &fclient.RespSendJoin{}, err
	}
	return &ires, nil
}

func (a *FederationInternalAPI) GetEventAuth(
	ctx context.Context, origin, s spec.ServerName,
	roomVersion gomatrixserverlib.RoomVersion, roomID, eventID string,
) (res fclient.RespEventAuth, err error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()
	ires, err := a.doRequestIfNotBlacklisted(s, func() (interface{}, error) {
		return a.federation.GetEventAuth(ctx, origin, s, roomVersion, roomID, eventID)
	})
	if err != nil {
		return fclient.RespEventAuth{}, err
	}
	return ires.(fclient.RespEventAuth), nil
}

func (a *FederationInternalAPI) GetUserDevices(
	ctx context.Context, origin, s spec.ServerName, userID string,
) (fclient.RespUserDevices, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()
	ires, err := a.doRequestIfNotBlacklisted(s, func() (interface{}, error) {
		return a.federation.GetUserDevices(ctx, origin, s, userID)
	})
	if err != nil {
		return fclient.RespUserDevices{}, err
	}
	return ires.(fclient.RespUserDevices), nil
}

func (a *FederationInternalAPI) ClaimKeys(
	ctx context.Context, origin, s spec.ServerName, oneTimeKeys map[string]map[string]string,
) (fclient.RespClaimKeys, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()
	ires, err := a.doRequestIfNotBlacklisted(s, func() (interface{}, error) {
		return a.federation.ClaimKeys(ctx, origin, s, oneTimeKeys)
	})
	if err != nil {
		return fclient.RespClaimKeys{}, err
	}
	return ires.(fclient.RespClaimKeys), nil
}

func (a *FederationInternalAPI) QueryKeys(
	ctx context.Context, origin, s spec.ServerName, keys map[string][]string,
) (fclient.RespQueryKeys, error) {
	ires, err := a.doRequestIfNotBackingOffOrBlacklisted(s, func() (interface{}, error) {
		return a.federation.QueryKeys(ctx, origin, s, keys)
	})
	if err != nil {
		return fclient.RespQueryKeys{}, err
	}
	return ires.(fclient.RespQueryKeys), nil
}

func (a *FederationInternalAPI) Backfill(
	ctx context.Context, origin, s spec.ServerName, roomID string, limit int, eventIDs []string,
) (res gomatrixserverlib.Transaction, err error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()
	ires, err := a.doRequestIfNotBlacklisted(s, func() (interface{}, error) {
		return a.federation.Backfill(ctx, origin, s, roomID, limit, eventIDs)
	})
	if err != nil {
		return gomatrixserverlib.Transaction{}, err
	}
	return ires.(gomatrixserverlib.Transaction), nil
}

func (a *FederationInternalAPI) LookupState(
	ctx context.Context, origin, s spec.ServerName, roomID, eventID string, roomVersion gomatrixserverlib.RoomVersion,
) (res gomatrixserverlib.StateResponse, err error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()
	ires, err := a.doRequestIfNotBlacklisted(s, func() (interface{}, error) {
		return a.federation.LookupState(ctx, origin, s, roomID, eventID, roomVersion)
	})
	if err != nil {
		return &fclient.RespState{}, err
	}
	r := ires.(fclient.RespState)
	return &r, nil
}

func (a *FederationInternalAPI) LookupStateIDs(
	ctx context.Context, origin, s spec.ServerName, roomID, eventID string,
) (res gomatrixserverlib.StateIDResponse, err error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()
	ires, err := a.doRequestIfNotBlacklisted(s, func() (interface{}, error) {
		return a.federation.LookupStateIDs(ctx, origin, s, roomID, eventID)
	})
	if err != nil {
		return fclient.RespStateIDs{}, err
	}
	return ires.(fclient.RespStateIDs), nil
}

func (a *FederationInternalAPI) LookupMissingEvents(
	ctx context.Context, origin, s spec.ServerName, roomID string,
	missing fclient.MissingEvents, roomVersion gomatrixserverlib.RoomVersion,
) (res fclient.RespMissingEvents, err error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()
	ires, err := a.doRequestIfNotBlacklisted(s, func() (interface{}, error) {
		return a.federation.LookupMissingEvents(ctx, origin, s, roomID, missing, roomVersion)
	})
	if err != nil {
		return fclient.RespMissingEvents{}, err
	}
	return ires.(fclient.RespMissingEvents), nil
}

func (a *FederationInternalAPI) GetEvent(
	ctx context.Context, origin, s spec.ServerName, eventID string,
) (res gomatrixserverlib.Transaction, err error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()
	ires, err := a.doRequestIfNotBlacklisted(s, func() (interface{}, error) {
		return a.federation.GetEvent(ctx, origin, s, eventID)
	})
	if err != nil {
		return gomatrixserverlib.Transaction{}, err
	}
	return ires.(gomatrixserverlib.Transaction), nil
}

func (a *FederationInternalAPI) LookupServerKeys(
	ctx context.Context, s spec.ServerName, keyRequests map[gomatrixserverlib.PublicKeyLookupRequest]spec.Timestamp,
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
	ctx context.Context, origin, s spec.ServerName, r fclient.MSC2836EventRelationshipsRequest,
	roomVersion gomatrixserverlib.RoomVersion,
) (res fclient.MSC2836EventRelationshipsResponse, err error) {
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	ires, err := a.doRequestIfNotBlacklisted(s, func() (interface{}, error) {
		return a.federation.MSC2836EventRelationships(ctx, origin, s, r, roomVersion)
	})
	if err != nil {
		return res, err
	}
	return ires.(fclient.MSC2836EventRelationshipsResponse), nil
}

func (a *FederationInternalAPI) RoomHierarchies(
	ctx context.Context, origin, s spec.ServerName, roomID string, suggestedOnly bool,
) (res fclient.RoomHierarchyResponse, err error) {
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	ires, err := a.doRequestIfNotBlacklisted(s, func() (interface{}, error) {
		return a.federation.RoomHierarchy(ctx, origin, s, roomID, suggestedOnly)
	})
	if err != nil {
		return res, err
	}
	return ires.(fclient.RoomHierarchyResponse), nil
}
