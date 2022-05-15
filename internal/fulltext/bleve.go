package fulltext

import (
	"github.com/blevesearch/bleve/v2"
)

type Search struct {
	Index bleve.Index
}

type IndexElement struct {
	EventID string
	Type    string
	RoomID  string
	Content string
}

func New(path string) (*Search, error) {
	fts := &Search{}
	var err error
	fts.Index, err = openIndex(path)
	if err != nil {
		return nil, err
	}
	return fts, nil
}

func (f *Search) Close() error {
	return f.Index.Close()
}

func (f *Search) IndexElement(e IndexElement) error {
	return f.Index.Index(e.EventID, e)
}

func (f *Search) DeleteElement(eventID string) error {
	return f.Index.Delete(eventID)
}

func (f *Search) Search(term string) (*bleve.SearchResult, error) {
	qry := bleve.NewQueryStringQuery(term)
	search := bleve.NewSearchRequest(qry)
	return f.Index.Search(search)
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
