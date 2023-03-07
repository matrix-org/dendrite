package main

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/json"
	"encoding/pem"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"os"
	"regexp"
	"sort"
	"time"

	"github.com/matrix-org/gomatrixserverlib"
	"github.com/tidwall/gjson"
)

var requestFrom = flag.String("from", "", "the server name that the request should originate from")
var requestKey = flag.String("key", "matrix_key.pem", "the private key to use when signing the request")
var requestTo = flag.String("to", "", "the server name to start backfilling from")
var startEventID = flag.String("eventid", "", "the event ID to start backfilling from")
var roomID = flag.String("room", "", "the room ID to backfill")

// nolint: gocyclo
func main() {
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
		panic(err)
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
	}

	ctx := context.Background()
	eventID := *startEventID
	start := time.Now()
	defer func() {
		log.Printf("Backfilling took: %s", time.Since(start))
	}()
	f, err := os.Create(tokenise(*roomID) + "_backfill.csv")
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close() // nolint: errcheck
	seenEvents := make(map[string]struct{})

	encoder := json.NewEncoder(f)

	for {
		log.Printf("[%d] going to request %s\n", len(seenEvents), eventID)
		evs, err := gomatrixserverlib.RequestBackfill(ctx, serverName, b, &nopJSONVerifier{}, *roomID, "9", []string{eventID}, 100)
		if err != nil && len(evs) == 0 {
			log.Printf("failed to backfill, retrying: %s", err)
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
				log.Fatal(err)
			}
			if x.Type() == gomatrixserverlib.MRoomCreate {
				createSeen = true
			}
		}
		// We've reached the beginng of the room
		if createSeen {
			log.Printf("[%d] Reached beginning of the room, exiting", len(seenEvents))
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
			log.Printf("no new eventID found in backfill response")
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
	FedClient *gomatrixserverlib.FederationClient
	servers   map[gomatrixserverlib.ServerName]struct{}
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
		if v == "matrix.org" { // will be added to the front anyway
			continue
		}
		servers = append(servers, v)
	}
	rand.Shuffle(len(servers), func(i, j int) {
		servers[i], servers[j] = servers[j], servers[i]
	})

	// always prefer matrix.org
	servers = append([]gomatrixserverlib.ServerName{"matrix.org"}, servers...)

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
