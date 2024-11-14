// Copyright 2024 New Vector Ltd.
// Copyright 2020 The Matrix.org Foundation C.I.C.
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

// Package hooks exposes places in Dendrite where custom code can be executed, useful for MSCs.
// Hooks can only be run in monolith mode.
package hooks

import (
	"sync"
)

const (
	// KindNewEventPersisted is a hook which is called with *types.HeaderedEvent
	// It is run when a new event is persisted in the roomserver.
	// Usage:
	//   hooks.Attach(hooks.KindNewEventPersisted, func(headeredEvent interface{}) { ... })
	KindNewEventPersisted = "new_event_persisted"
	// KindNewEventReceived is a hook which is called with *types.HeaderedEvent
	// It is run before a new event is processed by the roomserver. This hook can be used
	// to modify the event before it is persisted by adding data to `unsigned`.
	// Usage:
	//   hooks.Attach(hooks.KindNewEventReceived, func(headeredEvent interface{}) {
	//     ev := headeredEvent.(*types.HeaderedEvent)
	//     _ = ev.SetUnsignedField("key", "val")
	//   })
	KindNewEventReceived = "new_event_received"
)

var (
	hookMap = make(map[string][]func(interface{}))
	hookMu  = sync.Mutex{}
	enabled = false
)

// Enable all hooks. This may slow down the server slightly. Required for MSCs to work.
func Enable() {
	enabled = true
}

// Run any hooks
func Run(kind string, data interface{}) {
	if !enabled {
		return
	}
	cbs := callbacks(kind)
	for _, cb := range cbs {
		cb(data)
	}
}

// Attach a hook
func Attach(kind string, callback func(interface{})) {
	if !enabled {
		return
	}
	hookMu.Lock()
	defer hookMu.Unlock()
	hookMap[kind] = append(hookMap[kind], callback)
}

func callbacks(kind string) []func(interface{}) {
	hookMu.Lock()
	defer hookMu.Unlock()
	return hookMap[kind]
}
