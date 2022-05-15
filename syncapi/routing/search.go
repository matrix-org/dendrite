package routing

import (
	"net/http"

	"github.com/matrix-org/dendrite/clientapi/httputil"
	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/dendrite/internal/fulltext"
	"github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
)

func Search(req *http.Request, device *api.Device, fts *fulltext.Search) util.JSONResponse {
	var searchReq SearchRequest
	resErr := httputil.UnmarshalJSONRequest(req, &searchReq)
	if resErr != nil {
		return *resErr
	}

	result, err := fts.Search(searchReq.SearchCategories.RoomEvents.SearchTerm)
	if err != nil {
		return jsonerror.InternalServerError()
	}

	results := []Results{}
	for _, x := range result.Hits {
		results = append(results, Results{
			Rank: x.Score,
			Result: Result{
				Content: Content{
					Body: x.String(),
				},
				EventID: x.ID,
			},
		})
	}

	res := SearchResponse{
		SearchCategories: SearchCategories{
			RoomEvents: RoomEvents{
				Count:   len(results),
				Groups:  Groups{},
				Results: results,
			},
		},
	}

	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: res,
	}
}

type SearchRequest struct {
	SearchCategories struct {
		RoomEvents struct {
			Filter    gomatrixserverlib.StateFilter `json:"filter"`
			Groupings struct {
				GroupBy []struct {
					Key string `json:"key"`
				} `json:"group_by"`
			} `json:"groupings"`
			Keys       []string `json:"keys"`
			OrderBy    string   `json:"order_by"`
			SearchTerm string   `json:"search_term"`
		} `json:"room_events"`
	} `json:"search_categories"`
}

type SearchResponse struct {
	SearchCategories SearchCategories `json:"search_categories"`
}
type RoomResult struct {
	NextBatch string   `json:"next_batch"`
	Order     int      `json:"order"`
	Results   []string `json:"results"`
}
type RoomID struct {
	Room RoomResult
}
type Groups struct {
	RoomID RoomID `json:"room_id"`
}
type Content struct {
	Body          string `json:"body"`
	Format        string `json:"format"`
	FormattedBody string `json:"formatted_body"`
	Msgtype       string `json:"msgtype"`
}
type Unsigned struct {
	Age int `json:"age"`
}
type Result struct {
	Content        Content  `json:"content"`
	EventID        string   `json:"event_id"`
	OriginServerTs int64    `json:"origin_server_ts"`
	RoomID         string   `json:"room_id"`
	Sender         string   `json:"sender"`
	Type           string   `json:"type"`
	Unsigned       Unsigned `json:"unsigned"`
}
type Results struct {
	Rank   float64 `json:"rank"`
	Result Result  `json:"result"`
}
type RoomEvents struct {
	Count      int       `json:"count"`
	Groups     Groups    `json:"groups"`
	Highlights []string  `json:"highlights"`
	NextBatch  string    `json:"next_batch"`
	Results    []Results `json:"results"`
}
type SearchCategories struct {
	RoomEvents RoomEvents `json:"room_events"`
}
