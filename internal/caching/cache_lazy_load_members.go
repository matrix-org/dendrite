package caching

import (
	userapi "github.com/matrix-org/dendrite/userapi/api"
)

type lazyLoadingCacheKey struct {
	UserID       string // the user we're querying on behalf of
	DeviceID     string // the user we're querying on behalf of
	RoomID       string // the room in question
	TargetUserID string // the user whose membership we're asking about
}

type LazyLoadCache interface {
	StoreLazyLoadedUser(device *userapi.Device, roomID, userID, eventID string)
	IsLazyLoadedUserCached(device *userapi.Device, roomID, userID string) (string, bool)
	InvalidateLazyLoadedUser(device *userapi.Device, roomID, userID string)
}

func (c Caches) StoreLazyLoadedUser(device *userapi.Device, roomID, userID, eventID string) {
	c.LazyLoading.Set(lazyLoadingCacheKey{
		UserID:       device.UserID,
		DeviceID:     device.ID,
		RoomID:       roomID,
		TargetUserID: userID,
	}, eventID)
}

func (c Caches) IsLazyLoadedUserCached(device *userapi.Device, roomID, userID string) (string, bool) {
	return c.LazyLoading.Get(lazyLoadingCacheKey{
		UserID:       device.UserID,
		DeviceID:     device.ID,
		RoomID:       roomID,
		TargetUserID: userID,
	})
}

func (c Caches) InvalidateLazyLoadedUser(device *userapi.Device, roomID, userID string) {
	c.LazyLoading.Unset(lazyLoadingCacheKey{
		UserID:       device.UserID,
		DeviceID:     device.ID,
		RoomID:       roomID,
		TargetUserID: userID,
	})
}
