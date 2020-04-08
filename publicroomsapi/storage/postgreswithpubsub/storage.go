// Copyright 2017-2018 New Vector Ltd
// Copyright 2019-2020 The Matrix.org Foundation C.I.C.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package postgreswithpubsub

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/matrix-org/dendrite/publicroomsapi/storage/postgres"
	"github.com/matrix-org/gomatrixserverlib"

	pubsub "github.com/libp2p/go-libp2p-pubsub"
)

const MaintenanceInterval = time.Second * 10

type discoveredRoom struct {
	time time.Time
	room gomatrixserverlib.PublicRoom
}

// PublicRoomsServerDatabase represents a public rooms server database.
type PublicRoomsServerDatabase struct {
	postgres.PublicRoomsServerDatabase                           //
	pubsub                             *pubsub.PubSub            //
	subscription                       *pubsub.Subscription      //
	foundRooms                         map[string]discoveredRoom // additional rooms we have learned about from the DHT
	foundRoomsMutex                    sync.RWMutex              // protects foundRooms
	maintenanceTimer                   *time.Timer               //
	roomsAdvertised                    atomic.Value              // stores int
}

// NewPublicRoomsServerDatabase creates a new public rooms server database.
func NewPublicRoomsServerDatabase(dataSourceName string, pubsub *pubsub.PubSub) (*PublicRoomsServerDatabase, error) {
	pg, err := postgres.NewPublicRoomsServerDatabase(dataSourceName)
	if err != nil {
		return nil, err
	}
	provider := PublicRoomsServerDatabase{
		pubsub:                    pubsub,
		PublicRoomsServerDatabase: *pg,
		foundRooms:                make(map[string]discoveredRoom),
	}
	if sub, err := pubsub.Subscribe("/matrix/publicRooms"); err == nil {
		provider.subscription = sub
		go provider.MaintenanceTimer()
		go provider.FindRooms()
		provider.roomsAdvertised.Store(0)
		return &provider, nil
	} else {
		return nil, err
	}
}

func (d *PublicRoomsServerDatabase) GetRoomVisibility(ctx context.Context, roomID string) (bool, error) {
	return d.PublicRoomsServerDatabase.GetRoomVisibility(ctx, roomID)
}

func (d *PublicRoomsServerDatabase) SetRoomVisibility(ctx context.Context, visible bool, roomID string) error {
	d.MaintenanceTimer()
	return d.PublicRoomsServerDatabase.SetRoomVisibility(ctx, visible, roomID)
}

func (d *PublicRoomsServerDatabase) CountPublicRooms(ctx context.Context) (int64, error) {
	d.foundRoomsMutex.RLock()
	defer d.foundRoomsMutex.RUnlock()
	return int64(len(d.foundRooms)), nil
}

func (d *PublicRoomsServerDatabase) GetPublicRooms(ctx context.Context, offset int64, limit int16, filter string) ([]gomatrixserverlib.PublicRoom, error) {
	var rooms []gomatrixserverlib.PublicRoom
	if filter == "__local__" {
		if r, err := d.PublicRoomsServerDatabase.GetPublicRooms(ctx, offset, limit, ""); err == nil {
			rooms = append(rooms, r...)
		} else {
			return []gomatrixserverlib.PublicRoom{}, err
		}
	} else {
		d.foundRoomsMutex.RLock()
		defer d.foundRoomsMutex.RUnlock()
		for _, room := range d.foundRooms {
			rooms = append(rooms, room.room)
		}
	}
	return rooms, nil
}

func (d *PublicRoomsServerDatabase) UpdateRoomFromEvents(ctx context.Context, eventsToAdd []gomatrixserverlib.Event, eventsToRemove []gomatrixserverlib.Event) error {
	return d.PublicRoomsServerDatabase.UpdateRoomFromEvents(ctx, eventsToAdd, eventsToRemove)
}

func (d *PublicRoomsServerDatabase) UpdateRoomFromEvent(ctx context.Context, event gomatrixserverlib.Event) error {
	return d.PublicRoomsServerDatabase.UpdateRoomFromEvent(ctx, event)
}

func (d *PublicRoomsServerDatabase) MaintenanceTimer() {
	if d.maintenanceTimer != nil && !d.maintenanceTimer.Stop() {
		<-d.maintenanceTimer.C
	}
	d.Interval()
}

func (d *PublicRoomsServerDatabase) Interval() {
	d.foundRoomsMutex.Lock()
	for k, v := range d.foundRooms {
		if time.Since(v.time) > time.Minute {
			delete(d.foundRooms, k)
		}
	}
	d.foundRoomsMutex.Unlock()
	if err := d.AdvertiseRooms(); err != nil {
		fmt.Println("Failed to advertise room in DHT:", err)
	}
	d.foundRoomsMutex.RLock()
	defer d.foundRoomsMutex.RUnlock()
	fmt.Println("Found", len(d.foundRooms), "room(s), advertised", d.roomsAdvertised.Load(), "room(s)")
	d.maintenanceTimer = time.AfterFunc(MaintenanceInterval, d.Interval)
}

func (d *PublicRoomsServerDatabase) AdvertiseRooms() error {
	dbCtx, dbCancel := context.WithTimeout(context.Background(), 3*time.Second)
	_ = dbCancel
	ourRooms, err := d.GetPublicRooms(dbCtx, 0, 1024, "__local__")
	if err != nil {
		return err
	}
	advertised := 0
	for _, room := range ourRooms {
		if j, err := json.Marshal(room); err == nil {
			if err := d.pubsub.Publish("/matrix/publicRooms", j); err != nil {
				fmt.Println("Failed to publish public room:", err)
			} else {
				advertised++
			}
		}
	}

	d.roomsAdvertised.Store(advertised)
	return nil
}

func (d *PublicRoomsServerDatabase) FindRooms() {
	for {
		msg, err := d.subscription.Next(context.Background())
		if err != nil {
			continue
		}
		received := discoveredRoom{
			time: time.Now(),
		}
		if err := json.Unmarshal(msg.Data, &received.room); err != nil {
			fmt.Println("Unmarshal error:", err)
			continue
		}
		d.foundRoomsMutex.Lock()
		d.foundRooms[received.room.RoomID] = received
		d.foundRoomsMutex.Unlock()
	}
}
