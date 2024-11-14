// Copyright 2024 New Vector Ltd.
// Copyright 2022 The Matrix.org Foundation C.I.C.
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

//go:build !wasm
// +build !wasm

package fulltext

import (
	"regexp"
	"strings"

	"github.com/blevesearch/bleve/v2"
	"github.com/element-hq/dendrite/setup/process"
	"github.com/matrix-org/gomatrixserverlib/spec"

	// side effect imports to allow all possible languages
	_ "github.com/blevesearch/bleve/v2/analysis/lang/ar"
	_ "github.com/blevesearch/bleve/v2/analysis/lang/cjk"
	_ "github.com/blevesearch/bleve/v2/analysis/lang/ckb"
	_ "github.com/blevesearch/bleve/v2/analysis/lang/da"
	_ "github.com/blevesearch/bleve/v2/analysis/lang/de"
	_ "github.com/blevesearch/bleve/v2/analysis/lang/en"
	_ "github.com/blevesearch/bleve/v2/analysis/lang/es"
	_ "github.com/blevesearch/bleve/v2/analysis/lang/fa"
	_ "github.com/blevesearch/bleve/v2/analysis/lang/fi"
	_ "github.com/blevesearch/bleve/v2/analysis/lang/fr"
	_ "github.com/blevesearch/bleve/v2/analysis/lang/hi"
	_ "github.com/blevesearch/bleve/v2/analysis/lang/hr"
	_ "github.com/blevesearch/bleve/v2/analysis/lang/hu"
	_ "github.com/blevesearch/bleve/v2/analysis/lang/it"
	_ "github.com/blevesearch/bleve/v2/analysis/lang/nl"
	_ "github.com/blevesearch/bleve/v2/analysis/lang/no"
	_ "github.com/blevesearch/bleve/v2/analysis/lang/pt"
	_ "github.com/blevesearch/bleve/v2/analysis/lang/ro"
	_ "github.com/blevesearch/bleve/v2/analysis/lang/ru"
	_ "github.com/blevesearch/bleve/v2/analysis/lang/sv"
	_ "github.com/blevesearch/bleve/v2/analysis/lang/tr"
	"github.com/blevesearch/bleve/v2/mapping"

	"github.com/element-hq/dendrite/setup/config"
)

// Search contains all existing bleve.Index
type Search struct {
	FulltextIndex bleve.Index
}

type Indexer interface {
	Index(elements ...IndexElement) error
	Delete(eventID string) error
	Search(term string, roomIDs, keys []string, limit, from int, orderByStreamPos bool) (*bleve.SearchResult, error)
	GetHighlights(result *bleve.SearchResult) []string
	Close() error
}

// IndexElement describes the layout of an element to index
type IndexElement struct {
	EventID        string
	RoomID         string
	Content        string
	ContentType    string
	StreamPosition int64
}

// SetContentType sets i.ContentType given an identifier
func (i *IndexElement) SetContentType(v string) {
	switch v {
	case "m.room.message":
		i.ContentType = "content.body"
	case spec.MRoomName:
		i.ContentType = "content.name"
	case spec.MRoomTopic:
		i.ContentType = "content.topic"
	}
}

// New opens a new/existing fulltext index
func New(processCtx *process.ProcessContext, cfg config.Fulltext) (fts *Search, err error) {
	fts = &Search{}
	fts.FulltextIndex, err = openIndex(cfg)
	if err != nil {
		return nil, err
	}
	go func() {
		processCtx.ComponentStarted()
		// Wait for the processContext to be done, indicating that Dendrite is shutting down.
		<-processCtx.WaitForShutdown()
		_ = fts.Close()
		processCtx.ComponentFinished()
	}()
	return fts, nil
}

// Close closes the fulltext index
func (f *Search) Close() error {
	return f.FulltextIndex.Close()
}

// Index indexes the given elements
func (f *Search) Index(elements ...IndexElement) error {
	batch := f.FulltextIndex.NewBatch()

	for _, element := range elements {
		err := batch.Index(element.EventID, element)
		if err != nil {
			return err
		}
	}
	return f.FulltextIndex.Batch(batch)
}

// Delete deletes an indexed element by the eventID
func (f *Search) Delete(eventID string) error {
	return f.FulltextIndex.Delete(eventID)
}

var highlightMatcher = regexp.MustCompile("<mark>(.*?)</mark>")

// GetHighlights extracts the highlights from a SearchResult.
func (f *Search) GetHighlights(result *bleve.SearchResult) []string {
	if result == nil {
		return []string{}
	}

	seenMatches := make(map[string]struct{})

	for _, hit := range result.Hits {
		if hit.Fragments == nil {
			continue
		}
		fragments, ok := hit.Fragments["Content"]
		if !ok {
			continue
		}
		for _, x := range fragments {
			substringMatches := highlightMatcher.FindAllStringSubmatch(x, -1)
			for _, matches := range substringMatches {
				for i := range matches {
					if i == 0 { // skip first match, this is the complete substring match
						continue
					}
					if _, ok := seenMatches[matches[i]]; ok {
						continue
					}
					seenMatches[matches[i]] = struct{}{}
				}
			}
		}
	}

	res := make([]string, 0, len(seenMatches))
	for m := range seenMatches {
		res = append(res, m)
	}
	return res
}

// Search searches the index given a search term, roomIDs and keys.
func (f *Search) Search(term string, roomIDs, keys []string, limit, from int, orderByStreamPos bool) (*bleve.SearchResult, error) {
	qry := bleve.NewConjunctionQuery()
	termQuery := bleve.NewBooleanQuery()

	terms := strings.Split(term, " ")
	for _, term := range terms {
		matchQuery := bleve.NewMatchQuery(term)
		matchQuery.SetField("Content")
		termQuery.AddMust(matchQuery)
	}
	qry.AddQuery(termQuery)

	roomQuery := bleve.NewBooleanQuery()
	for _, roomID := range roomIDs {
		roomSearch := bleve.NewMatchQuery(roomID)
		roomSearch.SetField("RoomID")
		roomQuery.AddShould(roomSearch)
	}
	if len(roomIDs) > 0 {
		qry.AddQuery(roomQuery)
	}
	keyQuery := bleve.NewBooleanQuery()
	for _, key := range keys {
		keySearch := bleve.NewMatchQuery(key)
		keySearch.SetField("ContentType")
		keyQuery.AddShould(keySearch)
	}
	if len(keys) > 0 {
		qry.AddQuery(keyQuery)
	}

	s := bleve.NewSearchRequestOptions(qry, limit, from, false)
	s.Fields = []string{"*"}
	s.SortBy([]string{"_score"})
	if orderByStreamPos {
		s.SortBy([]string{"-StreamPosition"})
	}

	// Highlight some words
	s.Highlight = bleve.NewHighlight()
	s.Highlight.Fields = []string{"Content"}

	return f.FulltextIndex.Search(s)
}

func openIndex(cfg config.Fulltext) (bleve.Index, error) {
	m := getMapping(cfg)
	if cfg.InMemory {
		return bleve.NewMemOnly(m)
	}
	if index, err := bleve.Open(string(cfg.IndexPath)); err == nil {
		return index, nil
	}

	index, err := bleve.New(string(cfg.IndexPath), m)
	if err != nil {
		return nil, err
	}
	return index, nil
}

func getMapping(cfg config.Fulltext) *mapping.IndexMappingImpl {
	enFieldMapping := bleve.NewTextFieldMapping()
	enFieldMapping.Analyzer = cfg.Language

	eventMapping := bleve.NewDocumentMapping()
	eventMapping.AddFieldMappingsAt("Content", enFieldMapping)
	eventMapping.AddFieldMappingsAt("StreamPosition", bleve.NewNumericFieldMapping())

	// Index entries as is
	idFieldMapping := bleve.NewKeywordFieldMapping()
	eventMapping.AddFieldMappingsAt("ContentType", idFieldMapping)
	eventMapping.AddFieldMappingsAt("RoomID", idFieldMapping)
	eventMapping.AddFieldMappingsAt("EventID", idFieldMapping)

	indexMapping := bleve.NewIndexMapping()
	indexMapping.AddDocumentMapping("Event", eventMapping)
	indexMapping.DefaultType = "Event"
	return indexMapping
}
