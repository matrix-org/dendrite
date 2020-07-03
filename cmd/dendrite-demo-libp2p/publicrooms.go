// Copyright 2020 The Matrix.org Foundation C.I.C.
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

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	currentstateAPI "github.com/matrix-org/dendrite/currentstateserver/api"
	roomserverAPI "github.com/matrix-org/dendrite/roomserver/api"

	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
)

const MaintenanceInterval = time.Second * 10

type discoveredRoom struct {
	time time.Time
	room gomatrixserverlib.PublicRoom
}

type publicRoomsProvider struct {
	pubsub           *pubsub.PubSub
	topic            *pubsub.Topic
	subscription     *pubsub.Subscription
	foundRooms       map[string]discoveredRoom // additional rooms we have learned about from the DHT
	foundRoomsMutex  sync.RWMutex              // protects foundRooms
	maintenanceTimer *time.Timer               //
	roomsAdvertised  atomic.Value              // stores int
	rsAPI            roomserverAPI.RoomserverInternalAPI
	stateAPI         currentstateAPI.CurrentStateInternalAPI
}

func newPublicRoomsProvider(ps *pubsub.PubSub, rsAPI roomserverAPI.RoomserverInternalAPI, stateAPI currentstateAPI.CurrentStateInternalAPI) *publicRoomsProvider {
	return &publicRoomsProvider{
		foundRooms: make(map[string]discoveredRoom),
		pubsub:     ps,
		rsAPI:      rsAPI,
		stateAPI:   stateAPI,
	}
}

func (p *publicRoomsProvider) Start() error {
	if topic, err := p.pubsub.Join("/matrix/publicRooms"); err != nil {
		return err
	} else if sub, err := topic.Subscribe(); err == nil {
		p.topic = topic
		p.subscription = sub
		go p.MaintenanceTimer()
		go p.FindRooms()
		p.roomsAdvertised.Store(0)
	} else {
		return err
	}
	return nil
}

func (p *publicRoomsProvider) MaintenanceTimer() {
	if p.maintenanceTimer != nil && !p.maintenanceTimer.Stop() {
		<-p.maintenanceTimer.C
	}
	p.Interval()
}

func (p *publicRoomsProvider) Interval() {
	p.foundRoomsMutex.Lock()
	for k, v := range p.foundRooms {
		if time.Since(v.time) > time.Minute {
			delete(p.foundRooms, k)
		}
	}
	p.foundRoomsMutex.Unlock()
	if err := p.AdvertiseRooms(); err != nil {
		fmt.Println("Failed to advertise room in DHT:", err)
	}
	p.foundRoomsMutex.RLock()
	defer p.foundRoomsMutex.RUnlock()
	fmt.Println("Found", len(p.foundRooms), "room(s), advertised", p.roomsAdvertised.Load(), "room(s)")
	p.maintenanceTimer = time.AfterFunc(MaintenanceInterval, p.Interval)
}

func (p *publicRoomsProvider) AdvertiseRooms() error {
	ctx := context.Background()
	var queryRes roomserverAPI.QueryPublishedRoomsResponse
	// Query published rooms on our server. This will not invoke clientapi.ExtraPublicRoomsProvider
	err := p.rsAPI.QueryPublishedRooms(ctx, &roomserverAPI.QueryPublishedRoomsRequest{}, &queryRes)
	if err != nil {
		util.GetLogger(ctx).WithError(err).Error("QueryPublishedRooms failed")
		return err
	}
	ourRooms, err := currentstateAPI.PopulatePublicRooms(ctx, queryRes.RoomIDs, p.stateAPI)
	if err != nil {
		util.GetLogger(ctx).WithError(err).Error("PopulatePublicRooms failed")
		return err
	}
	advertised := 0
	for _, room := range ourRooms {
		if j, err := json.Marshal(room); err == nil {
			if err := p.topic.Publish(context.TODO(), j); err != nil {
				fmt.Println("Failed to publish public room:", err)
			} else {
				advertised++
			}
		}
	}

	p.roomsAdvertised.Store(advertised)
	return nil
}

func (p *publicRoomsProvider) FindRooms() {
	for {
		msg, err := p.subscription.Next(context.Background())
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
		fmt.Printf("received %+v \n", received)
		p.foundRoomsMutex.Lock()
		p.foundRooms[received.room.RoomID] = received
		p.foundRoomsMutex.Unlock()
	}
}

func (p *publicRoomsProvider) Rooms() (rooms []gomatrixserverlib.PublicRoom) {
	p.foundRoomsMutex.RLock()
	defer p.foundRoomsMutex.RUnlock()
	for _, dr := range p.foundRooms {
		rooms = append(rooms, dr.room)
	}
	return
}
