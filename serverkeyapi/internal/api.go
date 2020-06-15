package internal

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"fmt"
	"time"

	"github.com/matrix-org/dendrite/internal/config"
	"github.com/matrix-org/dendrite/serverkeyapi/api"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/sirupsen/logrus"
)

type ServerKeyAPI struct {
	api.ServerKeyInternalAPI

	Cfg        *config.Dendrite
	OurKeyRing gomatrixserverlib.KeyRing
	FedClient  *gomatrixserverlib.FederationClient
}

func (s *ServerKeyAPI) QueryLocalKeys(ctx context.Context, request *api.QueryLocalKeysRequest, response *api.QueryLocalKeysResponse) error {
	publicKey := s.Cfg.Matrix.PrivateKey.Public().(ed25519.PublicKey)

	response.ServerKeys.ServerName = s.Cfg.Matrix.ServerName
	response.ServerKeys.VerifyKeys = map[gomatrixserverlib.KeyID]gomatrixserverlib.VerifyKey{
		s.Cfg.Matrix.KeyID: {
			Key: gomatrixserverlib.Base64Bytes(publicKey),
		},
	}
	response.ServerKeys.TLSFingerprints = s.Cfg.Matrix.TLSFingerPrints
	// TODO: Handle old expired keys. We should probably have a configuration section
	// for these, as it's really counter-intuitive for people to have to rake through
	// the database to find their own past keys.
	response.ServerKeys.OldVerifyKeys = map[gomatrixserverlib.KeyID]gomatrixserverlib.OldVerifyKey{}
	response.ServerKeys.ValidUntilTS = gomatrixserverlib.AsTimestamp(time.Now().Add(s.Cfg.Matrix.KeyValidityPeriod))

	toSign, err := json.Marshal(response.ServerKeys.ServerKeyFields)
	if err != nil {
		return err
	}

	response.ServerKeys.Raw, err = gomatrixserverlib.SignJSON(
		string(s.Cfg.Matrix.ServerName), s.Cfg.Matrix.KeyID, s.Cfg.Matrix.PrivateKey, toSign,
	)
	return err
}

func (s *ServerKeyAPI) KeyRing() *gomatrixserverlib.KeyRing {
	// Return a keyring that forces requests to be proxied through the
	// below functions. That way we can enforce things like validity
	// and keeping the cache up-to-date.
	return &gomatrixserverlib.KeyRing{
		KeyDatabase: s,
		KeyFetchers: []gomatrixserverlib.KeyFetcher{},
	}
}

func (s *ServerKeyAPI) StoreKeys(
	_ context.Context,
	results map[gomatrixserverlib.PublicKeyLookupRequest]gomatrixserverlib.PublicKeyLookupResult,
) error {
	// Run in a background context - we don't want to stop this work just
	// because the caller gives up waiting.
	ctx := context.Background()
	// Store any keys that we were given in our database.
	return s.OurKeyRing.KeyDatabase.StoreKeys(ctx, results)
}

// nolint:gocyclo
func (s *ServerKeyAPI) FetchKeys(
	_ context.Context,
	requests map[gomatrixserverlib.PublicKeyLookupRequest]gomatrixserverlib.Timestamp,
) (map[gomatrixserverlib.PublicKeyLookupRequest]gomatrixserverlib.PublicKeyLookupResult, error) {
	// Run in a background context - we don't want to stop this work just
	// because the caller gives up waiting.
	ctx := context.Background()
	results := map[gomatrixserverlib.PublicKeyLookupRequest]gomatrixserverlib.PublicKeyLookupResult{}
	now := gomatrixserverlib.AsTimestamp(time.Now())
	// First, check if any of these key checks are for our own keys. If
	// they are then we will satisfy them directly.
	for req := range requests {
		if req.ServerName == s.Cfg.Matrix.ServerName {
			// We found a key request that is supposed to be for our own
			// keys. Remove it from the request list so we don't hit the
			// database or the fetchers for it.
			delete(requests, req)
			// Look up our own keys.
			request := &api.QueryLocalKeysRequest{}
			response := &api.QueryLocalKeysResponse{}
			if err := s.QueryLocalKeys(ctx, request, response); err != nil {
				return nil, err
			}
			// Depending on whether the key is expired or not, we'll need
			// to write slightly different
			if verifyKeys, ok := response.ServerKeys.VerifyKeys[req.KeyID]; ok {
				// The key is current.
				results[req] = gomatrixserverlib.PublicKeyLookupResult{
					VerifyKey:    verifyKeys,
					ExpiredTS:    gomatrixserverlib.PublicKeyNotExpired,
					ValidUntilTS: response.ServerKeys.ValidUntilTS,
				}
			} else if verifyKeys, ok := response.ServerKeys.OldVerifyKeys[req.KeyID]; ok {
				// The key is expired.
				results[req] = gomatrixserverlib.PublicKeyLookupResult{
					VerifyKey:    verifyKeys.VerifyKey,
					ExpiredTS:    verifyKeys.ExpiredTS,
					ValidUntilTS: gomatrixserverlib.PublicKeyNotValid,
				}
			}
		}
	}
	// Then consult our local database and see if we have the requested
	// keys. These might come from a cache, depending on the database
	// implementation used.
	if dbResults, err := s.OurKeyRing.KeyDatabase.FetchKeys(ctx, requests); err == nil {
		// We successfully got some keys. Add them to the results.
		for req, res := range dbResults {
			results[req] = res
			// If the key is valid right now then we can also remove it
			// from the request list as we don't need to fetch it again
			// in that case.
			if res.WasValidAt(now, true) {
				delete(requests, req)
			}
		}
	}
	// For any key requests that we still have outstanding, next try to
	// fetch them directly. We'll go through each of the key fetchers to
	// ask for the remaining keys.
	for _, fetcher := range s.OurKeyRing.KeyFetchers {
		logrus.WithFields(logrus.Fields{
			"fetcher_name": fetcher.FetcherName(),
		}).Infof("fetching %d key(s)", len(requests))
		if fetcherResults, err := fetcher.FetchKeys(ctx, requests); err == nil {
			// Build a map of the results that we want to commit to the
			// database. We do this in a separate map because otherwise we
			// might end up trying to rewrite database entries.
			storeResults := map[gomatrixserverlib.PublicKeyLookupRequest]gomatrixserverlib.PublicKeyLookupResult{}
			// Now let's look at the results that we got from this fetcher.
			for req, res := range fetcherResults {
				if prev, ok := results[req]; ok {
					// We've already got a previous entry for this request
					// so let's see if the newly retrieved one contains a more
					// up-to-date validity period.
					if res.ValidUntilTS > prev.ValidUntilTS {
						// This key is newer than the one we had so let's store
						// it in the database.
						if req.ServerName != s.Cfg.Matrix.ServerName {
							storeResults[req] = res
						}
					}
				} else {
					// We didn't already have a previous entry for this request
					// so store it in the database anyway for now.
					if req.ServerName != s.Cfg.Matrix.ServerName {
						storeResults[req] = res
					}
				}
				// Update the results map with this new result. If nothing
				// else, we can try verifying against this key.
				results[req] = res
				// If the key is valid right now then we can remove it from the
				// request list as we won't need to re-fetch it.
				if res.WasValidAt(now, true) {
					delete(requests, req)
				}
			}
			// Store the keys from our store map.
			if err = s.OurKeyRing.KeyDatabase.StoreKeys(ctx, storeResults); err != nil {
				return nil, fmt.Errorf("server key API failed to store retrieved keys: %w", err)
			}
		} else {
			logrus.WithError(err).WithFields(logrus.Fields{
				"fetcher_name": fetcher.FetcherName(),
			}).Warnf("failed to retrieve %d key(s)", len(requests))
		}
		// If there are no more keys to look up then stop.
		if len(requests) == 0 {
			logrus.WithFields(logrus.Fields{
				"fetcher_name": fetcher.FetcherName(),
			}).Infof("all keys are up-to-date")
			break
		}
	}
	// Check that we've actually satisfied all of the key requests that we
	// were given. We should report an error if we didn't.
	for req := range requests {
		if _, ok := results[req]; !ok {
			// The results don't contain anything for this specific request, so
			// we've failed to satisfy it from local keys, database keys or from
			// all of the fetchers. Report an error.
			return results, fmt.Errorf(
				"server key API failed to satisfy key request for server %q key ID %q",
				req.ServerName, req.KeyID,
			)
		}
	}
	// Return the keys.
	return results, nil
}

func (s *ServerKeyAPI) FetcherName() string {
	return fmt.Sprintf("ServerKeyAPI (wrapping %q)", s.OurKeyRing.KeyDatabase.FetcherName())
}
