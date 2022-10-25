package internal

import (
	"context"
	"crypto/ed25519"
	"fmt"
	"time"

	"github.com/matrix-org/gomatrixserverlib"
	"github.com/sirupsen/logrus"
)

func (s *FederationInternalAPI) KeyRing() *gomatrixserverlib.KeyRing {
	// Return a keyring that forces requests to be proxied through the
	// below functions. That way we can enforce things like validity
	// and keeping the cache up-to-date.
	return s.keyRing
}

func (s *FederationInternalAPI) StoreKeys(
	_ context.Context,
	results map[gomatrixserverlib.PublicKeyLookupRequest]gomatrixserverlib.PublicKeyLookupResult,
) error {
	// Run in a background context - we don't want to stop this work just
	// because the caller gives up waiting.
	ctx := context.Background()

	// Store any keys that we were given in our database.
	return s.keyRing.KeyDatabase.StoreKeys(ctx, results)
}

func (s *FederationInternalAPI) FetchKeys(
	_ context.Context,
	requests map[gomatrixserverlib.PublicKeyLookupRequest]gomatrixserverlib.Timestamp,
) (map[gomatrixserverlib.PublicKeyLookupRequest]gomatrixserverlib.PublicKeyLookupResult, error) {
	// Run in a background context - we don't want to stop this work just
	// because the caller gives up waiting.
	ctx := context.Background()
	now := gomatrixserverlib.AsTimestamp(time.Now())
	results := map[gomatrixserverlib.PublicKeyLookupRequest]gomatrixserverlib.PublicKeyLookupResult{}
	origRequests := map[gomatrixserverlib.PublicKeyLookupRequest]gomatrixserverlib.Timestamp{}
	for k, v := range requests {
		origRequests[k] = v
	}

	// First, check if any of these key checks are for our own keys. If
	// they are then we will satisfy them directly.
	s.handleLocalKeys(ctx, requests, results)

	// Then consult our local database and see if we have the requested
	// keys. These might come from a cache, depending on the database
	// implementation used.
	if err := s.handleDatabaseKeys(ctx, now, requests, results); err != nil {
		return nil, err
	}

	// For any key requests that we still have outstanding, next try to
	// fetch them directly. We'll go through each of the key fetchers to
	// ask for the remaining keys
	for _, fetcher := range s.keyRing.KeyFetchers {
		// If there are no more keys to look up then stop.
		if len(requests) == 0 {
			break
		}

		// Ask the fetcher to look up our keys.
		if err := s.handleFetcherKeys(ctx, now, fetcher, requests, results); err != nil {
			logrus.WithError(err).WithFields(logrus.Fields{
				"fetcher_name": fetcher.FetcherName(),
			}).Errorf("Failed to retrieve %d key(s)", len(requests))
			continue
		}
	}

	// Check that we've actually satisfied all of the key requests that we
	// were given. We should report an error if we didn't.
	for req := range origRequests {
		if _, ok := results[req]; !ok {
			// The results don't contain anything for this specific request, so
			// we've failed to satisfy it from local keys, database keys or from
			// all of the fetchers. Report an error.
			logrus.Warnf("Failed to retrieve key %q for server %q", req.KeyID, req.ServerName)
		}
	}

	// Return the keys.
	return results, nil
}

func (s *FederationInternalAPI) FetcherName() string {
	return fmt.Sprintf("FederationInternalAPI (wrapping %q)", s.keyRing.KeyDatabase.FetcherName())
}

// handleLocalKeys handles cases where the key request contains
// a request for our own server keys, either current or old.
func (s *FederationInternalAPI) handleLocalKeys(
	_ context.Context,
	requests map[gomatrixserverlib.PublicKeyLookupRequest]gomatrixserverlib.Timestamp,
	results map[gomatrixserverlib.PublicKeyLookupRequest]gomatrixserverlib.PublicKeyLookupResult,
) {
	for req := range requests {
		if !s.cfg.Matrix.IsLocalServerName(req.ServerName) {
			continue
		}
		if req.KeyID == s.cfg.Matrix.KeyID {
			// We found a key request that is supposed to be for our own
			// keys. Remove it from the request list so we don't hit the
			// database or the fetchers for it.
			delete(requests, req)

			// Insert our own key into the response.
			results[req] = gomatrixserverlib.PublicKeyLookupResult{
				VerifyKey: gomatrixserverlib.VerifyKey{
					Key: gomatrixserverlib.Base64Bytes(s.cfg.Matrix.PrivateKey.Public().(ed25519.PublicKey)),
				},
				ExpiredTS:    gomatrixserverlib.PublicKeyNotExpired,
				ValidUntilTS: gomatrixserverlib.AsTimestamp(time.Now().Add(s.cfg.Matrix.KeyValidityPeriod)),
			}
		} else {
			// The key request doesn't match our current key. Let's see
			// if it matches any of our old verify keys.
			for _, oldVerifyKey := range s.cfg.Matrix.OldVerifyKeys {
				if req.KeyID == oldVerifyKey.KeyID {
					// We found a key request that is supposed to be an expired
					// key.
					delete(requests, req)

					// Insert our own key into the response.
					results[req] = gomatrixserverlib.PublicKeyLookupResult{
						VerifyKey: gomatrixserverlib.VerifyKey{
							Key: gomatrixserverlib.Base64Bytes(oldVerifyKey.PrivateKey.Public().(ed25519.PublicKey)),
						},
						ExpiredTS:    oldVerifyKey.ExpiredAt,
						ValidUntilTS: gomatrixserverlib.PublicKeyNotValid,
					}

					// No need to look at the other keys.
					break
				}
			}
		}
	}
}

// handleDatabaseKeys handles cases where the key requests can be
// satisfied from our local database/cache.
func (s *FederationInternalAPI) handleDatabaseKeys(
	ctx context.Context,
	now gomatrixserverlib.Timestamp,
	requests map[gomatrixserverlib.PublicKeyLookupRequest]gomatrixserverlib.Timestamp,
	results map[gomatrixserverlib.PublicKeyLookupRequest]gomatrixserverlib.PublicKeyLookupResult,
) error {
	// Ask the database/cache for the keys.
	dbResults, err := s.keyRing.KeyDatabase.FetchKeys(ctx, requests)
	if err != nil {
		return err
	}

	// We successfully got some keys. Add them to the results.
	for req, res := range dbResults {
		// The key we've retrieved from the database/cache might
		// have passed its validity period, but right now, it's
		// the best thing we've got, and it might be sufficient to
		// verify a past event.
		results[req] = res

		// If the key is valid right now then we can also remove it
		// from the request list as we don't need to fetch it again
		// in that case. If the key isn't valid right now, then by
		// leaving it in the 'requests' map, we'll try to update the
		// key using the fetchers in handleFetcherKeys.
		if res.WasValidAt(now, true) {
			delete(requests, req)
		}
	}
	return nil
}

// handleFetcherKeys handles cases where a fetcher can satisfy
// the remaining requests.
func (s *FederationInternalAPI) handleFetcherKeys(
	ctx context.Context,
	_ gomatrixserverlib.Timestamp,
	fetcher gomatrixserverlib.KeyFetcher,
	requests map[gomatrixserverlib.PublicKeyLookupRequest]gomatrixserverlib.Timestamp,
	results map[gomatrixserverlib.PublicKeyLookupRequest]gomatrixserverlib.PublicKeyLookupResult,
) error {
	logrus.WithFields(logrus.Fields{
		"fetcher_name": fetcher.FetcherName(),
	}).Infof("Fetching %d key(s)", len(requests))

	// Create a context that limits our requests to 30 seconds.
	fetcherCtx, fetcherCancel := context.WithTimeout(ctx, time.Second*30)
	defer fetcherCancel()

	// Try to fetch the keys.
	fetcherResults, err := fetcher.FetchKeys(fetcherCtx, requests)
	if err != nil {
		return fmt.Errorf("fetcher.FetchKeys: %w", err)
	}

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
				storeResults[req] = res
			}
		} else {
			// We didn't already have a previous entry for this request
			// so store it in the database anyway for now.
			storeResults[req] = res
		}

		// Update the results map with this new result. If nothing
		// else, we can try verifying against this key.
		results[req] = res

		// Remove it from the request list so we won't re-fetch it.
		delete(requests, req)
	}

	// Store the keys from our store map.
	if err = s.keyRing.KeyDatabase.StoreKeys(context.Background(), storeResults); err != nil {
		logrus.WithError(err).WithFields(logrus.Fields{
			"fetcher_name":  fetcher.FetcherName(),
			"database_name": s.keyRing.KeyDatabase.FetcherName(),
		}).Errorf("Failed to store keys in the database")
		return fmt.Errorf("server key API failed to store retrieved keys: %w", err)
	}

	if len(storeResults) > 0 {
		logrus.WithFields(logrus.Fields{
			"fetcher_name": fetcher.FetcherName(),
		}).Infof("Updated %d of %d key(s) in database (%d keys remaining)", len(storeResults), len(results), len(requests))
	}

	return nil
}
