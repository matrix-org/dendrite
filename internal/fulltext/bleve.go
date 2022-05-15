package fulltext

import (
	"github.com/blevesearch/bleve/v2"
)

// Search contains all existing bleve.Index
type Search struct {
	MessageIndex bleve.Index
}

// IndexElement describes the layout of an element to index
type IndexElement struct {
	EventID string
	Type    string
	RoomID  string
	Content string
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

// Delete deletes an indexed element by the eventID
func (f *Search) Delete(eventID string) error {
	return f.MessageIndex.Delete(eventID)
}

// Search searches the index given a search term
func (f *Search) Search(term string) (*bleve.SearchResult, error) {
	qry := bleve.NewQueryStringQuery(term)
	search := bleve.NewSearchRequest(qry)
	return f.MessageIndex.Search(search)
}

func openIndex(path string) (bleve.Index, error) {
	if index, err := bleve.Open(path); err == nil {
		return index, nil
	}

	index, err := bleve.New(path, bleve.NewIndexMapping())
	if err != nil {
		return nil, err
	}
	return index, nil
}
