// Copyright 2022 The Matrix.org Foundation C.I.C.
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

package fulltext

import (
	"github.com/matrix-org/dendrite/setup/config"
	"time"
)

type Search struct{}
type IndexElement struct {
	EventID        string
	RoomID         string
	Content        string
	ContentType    string
	StreamPosition int64
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

func (f *Search) Index(e IndexElement) error {
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
