package main

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/json"
	"encoding/pem"
	"flag"
	"fmt"
	"math/rand"
	"os"
	"regexp"
	"sort"
	"time"

	"github.com/matrix-org/gomatrixserverlib"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/tidwall/gjson"
)

var requestFrom = flag.String("from", "", "the server name that the request should originate from")
var requestKey = flag.String("key", "matrix_key.pem", "the private key to use when signing the request")
var requestTo = flag.String("to", "", "the server name to start backfilling from")
var startEventID = flag.String("eventid", "", "the event ID to start backfilling from")
var roomID = flag.String("room", "", "the room ID to backfill")

// nolint: gocyclo
func main() {
	zerolog.TimeFieldFormat = time.RFC3339Nano
	log.Logger = log.Output(zerolog.ConsoleWriter{
		Out:        os.Stderr,
		TimeFormat: "15:04:05.000",
	})
	flag.Parse()

	if requestFrom == nil || *requestFrom == "" {
		fmt.Println("expecting: furl -from origin.com [-key matrix_key.pem] https://path/to/url")
		fmt.Println("supported flags:")
		flag.PrintDefaults()
		os.Exit(1)
	}

	if requestTo == nil || *requestTo == "" {
		fmt.Println("expecting a non empty -to value")
		flag.PrintDefaults()
		os.Exit(1)
	}
	if roomID == nil || *roomID == "" {
		fmt.Println("expecting a non empty -room value")
		flag.PrintDefaults()
		os.Exit(1)
	}
	if startEventID == nil || *startEventID == "" {
		fmt.Println("expecting a non empty -eventid value")
		flag.PrintDefaults()
		os.Exit(1)
	}

	data, err := os.ReadFile(*requestKey)
	if err != nil {
		log.Fatal().
			Err(err).
			Msg("failed to read file")
	}

	var privateKey ed25519.PrivateKey
	keyBlock, _ := pem.Decode(data)
	if keyBlock == nil {
		panic("keyBlock is nil")
	}
	if keyBlock.Type == "MATRIX PRIVATE KEY" {
		_, privateKey, err = ed25519.GenerateKey(bytes.NewReader(keyBlock.Bytes))
		if err != nil {
			panic(err)
		}
	} else {
		panic("unexpected key block")
	}

	serverName := gomatrixserverlib.ServerName(*requestFrom)
	client := gomatrixserverlib.NewFederationClient(
		[]*gomatrixserverlib.SigningIdentity{
			{
				ServerName: serverName,
				KeyID:      gomatrixserverlib.KeyID(keyBlock.Headers["Key-ID"]),
				PrivateKey: privateKey,
			},
		},
		gomatrixserverlib.WithKeepAlives(true),
	)

	b := &backfiller{
		FedClient: client,
		servers: map[gomatrixserverlib.ServerName]struct{}{
			gomatrixserverlib.ServerName(*requestTo): {},
		},
		preferServer: gomatrixserverlib.ServerName(*requestTo),
	}

	ctx := context.Background()
	eventID := *startEventID
	start := time.Now()
	seenEvents := make(map[string]struct{})

	defer func() {
		log.Debug().
			TimeDiff("duration", time.Now(), start).
			Int("events", len(seenEvents)).
			Msg("Finished backfilling")
	}()
	f, err := os.Create(tokenise(*roomID) + "_backfill.json")
	if err != nil {
		log.Fatal().
			Err(err).
			Msg("failed to create JSON file")
	}
	defer f.Close() // nolint: errcheck

	encoder := json.NewEncoder(f)

	for {
		log.Debug().
			Int("events", len(seenEvents)).
			Str("event_id", eventID).
			Msg("requesting event")
		evs, err := gomatrixserverlib.RequestBackfill(ctx, serverName, b, &nopJSONVerifier{}, *roomID, "9", []string{eventID}, 100)
		if err != nil && len(evs) == 0 {
			log.Err(err).Msg("failed to backfill, retrying")
			continue
		}
		var createSeen bool
		sort.Sort(headeredEvents(evs))
		for _, x := range evs {
			if _, ok := seenEvents[x.EventID()]; ok {
				continue
			}

			sender := gomatrixserverlib.ServerName(gjson.GetBytes(x.JSON(), "origin").Str)
			if sender != "" && sender != serverName {
				b.servers[sender] = struct{}{}
			}

			if x.Type() == "m.room.message" {
				x.Redact()
			}

			// The following ensures we preserve the "_event_id" field
			err = encoder.Encode(x)
			if err != nil {
				log.Fatal().
					Err(err).
					Msg("failed to write to file")
			}
			if x.Type() == gomatrixserverlib.MRoomCreate {
				createSeen = true
			}
		}
		// We've reached the beginning of the room
		if createSeen {
			log.Debug().
				Int("events", len(seenEvents)).
				Msg("Reached beginning of the room, existing")
			return
		}

		// Remember the event ID before trying to find a new one
		beforeEvID := eventID
		for _, x := range evs {
			if _, ok := seenEvents[x.EventID()]; ok {
				continue
			}
			if x.EventID() == beforeEvID {
				continue
			}
			eventID = x.EventID()
			break
		}
		if beforeEvID == eventID {
			log.Debug().
				Msg("no new eventID found in backfill response")
			return
		}
		// Finally store which events we've already seen
		for _, x := range evs {
			seenEvents[x.EventID()] = struct{}{}
		}
		time.Sleep(time.Second) // don't hit remotes to hard
	}
}

type headeredEvents []*gomatrixserverlib.HeaderedEvent

func (h headeredEvents) Len() int {
	return len(h)
}

func (h headeredEvents) Less(i, j int) bool {
	return h[i].Depth() < h[j].Depth()
}

func (h headeredEvents) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
}

type backfiller struct {
	FedClient    *gomatrixserverlib.FederationClient
	servers      map[gomatrixserverlib.ServerName]struct{}
	preferServer gomatrixserverlib.ServerName
}

func (b backfiller) StateIDsBeforeEvent(ctx context.Context, event *gomatrixserverlib.HeaderedEvent) ([]string, error) {
	return []string{}, nil
}

func (b backfiller) StateBeforeEvent(ctx context.Context, roomVer gomatrixserverlib.RoomVersion, event *gomatrixserverlib.HeaderedEvent, eventIDs []string) (map[string]*gomatrixserverlib.Event, error) {
	return nil, nil
}

func (b backfiller) Backfill(ctx context.Context, origin, server gomatrixserverlib.ServerName, roomID string, limit int, fromEventIDs []string) (gomatrixserverlib.Transaction, error) {
	return b.FedClient.Backfill(ctx, origin, server, roomID, limit, fromEventIDs)
}

func (b backfiller) ServersAtEvent(ctx context.Context, roomID, eventID string) []gomatrixserverlib.ServerName {
	servers := make([]gomatrixserverlib.ServerName, 0, len(b.servers)+1)
	for v := range b.servers {
		if v == b.preferServer { // will be added to the front anyway
			continue
		}
		servers = append(servers, v)
	}
	rand.Shuffle(len(servers), func(i, j int) {
		servers[i], servers[j] = servers[j], servers[i]
	})

	// always prefer specified server
	servers = append([]gomatrixserverlib.ServerName{b.preferServer}, servers...)

	if len(servers) > 5 {
		servers = servers[:5]
	}
	return servers
}

func (b backfiller) ProvideEvents(roomVer gomatrixserverlib.RoomVersion, eventIDs []string) ([]*gomatrixserverlib.Event, error) {
	return []*gomatrixserverlib.Event{}, nil
}

var safeCharacters = regexp.MustCompile("[^A-Za-z0-9$]+")

func tokenise(str string) string {
	return safeCharacters.ReplaceAllString(str, "_")
}

// NopJSONVerifier is a JSONVerifier that verifies nothing and returns no errors.
type nopJSONVerifier struct {
	// this verifier verifies nothing
}

func (t *nopJSONVerifier) VerifyJSONs(ctx context.Context, requests []gomatrixserverlib.VerifyJSONRequest) ([]gomatrixserverlib.VerifyJSONResult, error) {
	result := make([]gomatrixserverlib.VerifyJSONResult, len(requests))
	return result, nil
}
