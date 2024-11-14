// Copyright 2024 New Vector Ltd.
// Copyright 2022 The Matrix.org Foundation C.I.C.
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

package fulltext

import (
	"time"

	"github.com/element-hq/dendrite/setup/config"
)

type Search struct{}
type IndexElement struct {
	EventID        string
	RoomID         string
	Content        string
	ContentType    string
	StreamPosition int64
}

type Indexer interface {
	Index(elements ...IndexElement) error
	Delete(eventID string) error
	Search(term string, roomIDs, keys []string, limit, from int, orderByStreamPos bool) (SearchResult, error)
	GetHighlights(result SearchResult) []string
	Close() error
}

type SearchResult struct {
	Status   interface{}   `json:"status"`
	Request  *interface{}  `json:"request"`
	Hits     []interface{} `json:"hits"`
	Total    uint64        `json:"total_hits"`
	MaxScore float64       `json:"max_score"`
	Took     time.Duration `json:"took"`
	Facets   interface{}   `json:"facets"`
}

func (i *IndexElement) SetContentType(v string) {}

func New(cfg config.Fulltext) (fts *Search, err error) {
	return &Search{}, nil
}

func (f *Search) Close() error {
	return nil
}

func (f *Search) Index(e ...IndexElement) error {
	return nil
}

func (f *Search) BatchIndex(elements []IndexElement) error {
	return nil
}

func (f *Search) Delete(eventID string) error {
	return nil
}

func (f *Search) Search(term string, roomIDs, keys []string, limit, from int, orderByStreamPos bool) (SearchResult, error) {
	return SearchResult{}, nil
}

func (f *Search) GetHighlights(result SearchResult) []string {
	return []string{}
}
