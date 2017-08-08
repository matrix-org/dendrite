// Copyright 2017 Vector Creations Ltd
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

package api

import (
	"net/http"
)

// SetRoomVisibilityRequest is a request to SetRoomVisibility
type SetRoomVisibilityRequest struct {
	// ID of the room of which the visibility will be updated
	RoomID string `json:"room_id"`
	// The new visibility of the room. Either "public" or "private"
	Visibility string `json:"visibility"`
}

// SetRoomVisibilityResponse is a response to SetRoomVisibility
type SetRoomVisibilityResponse struct{}

// GetRoomVisibilityRequest is a request to GetRoomVisibility
type GetRoomVisibilityRequest struct {
	// ID of the room to get the visibility of
	RoomID string `json:"room_id"`
}

// GetRoomVisibilityResponse is a response to GetRoomVisibility
type GetRoomVisibilityResponse struct {
	// Visibility of the room. Either "public" or "private"
	Visibility string `json:"visibility"`
}

// GetPublicRoomsRequest is a request to GetPublicRooms
type GetPublicRoomsRequest struct {
	Limit int16  `json:"limit"`
	Since string `json:"since"`
}

// GetPublicRoomsResponse is a response to GetPublicRooms
type GetPublicRoomsResponse struct {
	Chunk                  []PublicRoomsChunk `json:"chunk"`
	NextBatch              string             `json:"next_batch"`
	PrevBatch              string             `json:"prev_batch"`
	TotalRoomCountEstimate int64              `json:"total_room_count_estimate"`
}

// PublicRoomsChunk implements the PublicRoomsChunk structure from the Matrix spec
type PublicRoomsChunk struct {
	RoomID         string   `json:"room_id"`
	Aliases        []string `json:"aliases"`
	CanonicalAlias string   `json:"canonical_alias"`
	Name           string   `json:"name"`
	Topic          string   `json:"topic"`
	AvatarURL      string   `json:"avatar_url"`
	JoinedMembers  int64    `json:"num_joined_members"`
	WorldReadable  bool     `json:"world_readable"`
	GuestCanJoin   bool     `json:"guest_can_join"`
}

// RoomserverPublicRoomAPI is used to update or retrieve the visibility setting
// of a room, or to retrieve all of the public rooms
type RoomserverPublicRoomAPI interface {
	// Set the visibility for a room
	SetRoomVisibility(
		req *SetRoomVisibilityRequest,
		response *SetRoomVisibilityResponse,
	) error

	// Get the visibility for a room
	GetRoomVisibility(
		req *GetRoomVisibilityRequest,
		response *GetRoomVisibilityResponse,
	) error

	// Get all rooms publicly visible rooms on the server
	GetPublicRooms(
		req *GetPublicRoomsRequest,
		response *GetPublicRoomsResponse,
	) error
}

// RoomserverSetRoomVisibilityPath is the HTTP path for the SetRoomVisibility API
const RoomserverSetRoomVisibilityPath = "/api/roomserver/setRoomVisibility"

// RoomserverGetRoomVisibilityPath is the HTTP path for the GetRoomVisibility API
const RoomserverGetRoomVisibilityPath = "/api/roomserver/getRoomVisibility"

// RoomserverGetPublicRoomsPath is the HTTP path for the GetPublicRooms API
const RoomserverGetPublicRoomsPath = "/api/roomserver/getPublicRooms"

// NewRoomserverPublicRoomAPIHTTP creates a RoomserverPublicRoomAPI implemented by talking to a HTTP POST API.
// If httpClient is nil then it uses the http.DefaultClient
func NewRoomserverPublicRoomAPIHTTP(roomserverURL string, httpClient *http.Client) RoomserverPublicRoomAPI {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &httpRoomserverPublicRoomAPI{roomserverURL, httpClient}
}

type httpRoomserverPublicRoomAPI struct {
	roomserverURL string
	httpClient    *http.Client
}

// SetRoomVisibility implements RoomserverPublicRoomAPI
func (h *httpRoomserverPublicRoomAPI) SetRoomVisibility(
	request *SetRoomVisibilityRequest,
	response *SetRoomVisibilityResponse,
) error {
	apiURL := h.roomserverURL + RoomserverSetRoomVisibilityPath
	return postJSON(h.httpClient, apiURL, request, response)
}

// GetRoomVisibility implements RoomserverPublicRoomAPI
func (h *httpRoomserverPublicRoomAPI) GetRoomVisibility(
	request *GetRoomVisibilityRequest,
	response *GetRoomVisibilityResponse,
) error {
	apiURL := h.roomserverURL + RoomserverGetRoomVisibilityPath
	return postJSON(h.httpClient, apiURL, request, response)
}

// GetPublicRooms implements RoomserverPublicRoomAPI
func (h *httpRoomserverPublicRoomAPI) GetPublicRooms(
	request *GetPublicRoomsRequest,
	response *GetPublicRoomsResponse,
) error {
	apiURL := h.roomserverURL + RoomserverGetPublicRoomsPath
	return postJSON(h.httpClient, apiURL, request, response)
}
