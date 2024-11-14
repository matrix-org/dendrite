// Copyright 2024 New Vector Ltd.
// Copyright 2022 The Matrix.org Foundation C.I.C.
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

//go:build !wasm
// +build !wasm

package util

import (
	"runtime"
)

func getMemoryStats(p *phoneHomeStats) error {
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)
	p.stats["memory_rss"] = memStats.Alloc
	return nil
}
