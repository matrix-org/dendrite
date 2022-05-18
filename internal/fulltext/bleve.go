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
	"strings"
	"time"

	"github.com/blevesearch/bleve/v2"
	"github.com/blevesearch/bleve/v2/analysis/lang/en"
	"github.com/blevesearch/bleve/v2/search/query"
)

// Search contains all existing bleve.Index
type Search struct {
	MessageIndex bleve.Index
}

// IndexElement describes the layout of an element to index
type IndexElement struct {
	EventID string    `json:"event_id,omitempty"`
	RoomID  string    `json:"room_id,omitempty"`
	Content string    `json:"content,omitempty"`
	Time    time.Time `json:"timestamp,omitempty"`
}

// New opens a new/existing fulltext index
func New(path string) (*Search, error) {
	fts := &Search{}
	var err error
	fts.MessageIndex, err = openIndex(path)
	if err != nil {
		return nil, err
	}
	return fts, nil
}

// Close closes the fulltext index
func (f *Search) Close() error {
	return f.MessageIndex.Close()
}

// Index indexes a given element
func (f *Search) Index(e IndexElement) error {
	return f.MessageIndex.Index(e.EventID, e)
}

// BatchIndex indexes the given elements
func (f *Search) BatchIndex(elements []IndexElement) error {
	batch := f.MessageIndex.NewBatch()

	for _, element := range elements {
		err := batch.Index(element.EventID, element)
		if err != nil {
			return err
		}
	}
	return f.MessageIndex.Batch(batch)
}

// Delete deletes an indexed element by the eventID
func (f *Search) Delete(eventID string) error {
	return f.MessageIndex.Delete(eventID)
}

// Search searches the index given a search term
func (f *Search) Search(term string, roomIDs []string, limit, from int, orderByTime bool) (*bleve.SearchResult, error) {
	terms := strings.Split(term, " ")

	qry := bleve.NewConjunctionQuery()
	for _, t := range terms {
		qry.AddQuery(bleve.NewQueryStringQuery(t))
	}

	for _, roomID := range roomIDs {
		roomSearch := bleve.NewMatchQuery(roomID)
		roomSearch.SetField("room_id")
		roomSearch.SetOperator(query.MatchQueryOperatorAnd)
		qry.AddQuery(roomSearch)
	}

	s := bleve.NewSearchRequest(qry)
	s.Size = limit
	s.From = from

	s.SortBy([]string{"_score"})
	if orderByTime {
		s.SortBy([]string{"-timestamp"})
	}

	return f.MessageIndex.Search(s)
}

func openIndex(path string) (bleve.Index, error) {
	if index, err := bleve.Open(path); err == nil {
		return index, nil
	}

	enFieldMapping := bleve.NewTextFieldMapping()
	enFieldMapping.Analyzer = en.AnalyzerName

	eventMapping := bleve.NewDocumentMapping()

	eventMapping.AddFieldMappingsAt("content", enFieldMapping)
	eventMapping.AddFieldMappingsAt("room_id", bleve.NewTextFieldMapping())

	idMapping := bleve.NewTextFieldMapping()
	idMapping.IncludeInAll = false
	idMapping.Index = false
	idMapping.IncludeTermVectors = false
	idMapping.SkipFreqNorm = true
	eventMapping.AddFieldMappingsAt("event_id", idMapping)

	mapping := bleve.NewIndexMapping()
	mapping.AddDocumentMapping("event", eventMapping)
	mapping.DefaultType = "event"
	mapping.TypeField = "type"
	mapping.DefaultAnalyzer = "en"

	index, err := bleve.New(path, mapping)
	if err != nil {
		return nil, err
	}
	return index, nil
}

type IndexElements []IndexElement

// Len implements sort.Interface
func (ie IndexElements) Len() int {
	return len(ie)
}

// Less implements sort.Interface
func (ie IndexElements) Less(i, j int) bool {
	return ie[i].Time.After(ie[j].Time)
}

// Swap implements sort.Interface
func (ie IndexElements) Swap(i, j int) {
	ie[i], ie[j] = ie[j], ie[i]
}
