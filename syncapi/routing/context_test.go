package routing

import (
	"net/http"
	"reflect"
	"testing"

	"github.com/matrix-org/gomatrixserverlib"
)

func Test_parseContextParams(t *testing.T) {

	noParamsReq, _ := http.NewRequest("GET", "https://localhost:8800/_matrix/client/r0/rooms/!hyi4UaxS9mUXpSG9:localhost:8800/context/%24um_T82QqAXN8PayGiBW7j9WExpqTIQ7-JRq-Q6xpIf8?access_token=5dMB0z4tiulyBvCaIKgyjuWG71ybDiYIwNJVJ2UmxRI", nil)
	limit2Req, _ := http.NewRequest("GET", "https://localhost:8800/_matrix/client/r0/rooms/!hyi4UaxS9mUXpSG9:localhost:8800/context/%24um_T82QqAXN8PayGiBW7j9WExpqTIQ7-JRq-Q6xpIf8?access_token=5dMB0z4tiulyBvCaIKgyjuWG71ybDiYIwNJVJ2UmxRI&limit=2", nil)
	limit10000Req, _ := http.NewRequest("GET", "https://localhost:8800/_matrix/client/r0/rooms/!hyi4UaxS9mUXpSG9:localhost:8800/context/%24um_T82QqAXN8PayGiBW7j9WExpqTIQ7-JRq-Q6xpIf8?access_token=5dMB0z4tiulyBvCaIKgyjuWG71ybDiYIwNJVJ2UmxRI&limit=10000", nil)
	invalidLimitReq, _ := http.NewRequest("GET", "https://localhost:8800/_matrix/client/r0/rooms/!hyi4UaxS9mUXpSG9:localhost:8800/context/%24um_T82QqAXN8PayGiBW7j9WExpqTIQ7-JRq-Q6xpIf8?access_token=5dMB0z4tiulyBvCaIKgyjuWG71ybDiYIwNJVJ2UmxRI&limit=100as", nil)
	lazyLoadReq, _ := http.NewRequest("GET", "https://localhost:8800//_matrix/client/r0/rooms/!kvEtX3rFamfwKHO3:localhost:8800/context/%24GjmkRbajRHy8_cxcSbUU4qF_njV8yHeLphI2azTrPaI?limit=2&filter=%7B+%22lazy_load_members%22+%3A+true+%7D&access_token=t1Njzm74w3G40CJ5xrlf1V2haXom0z0Iq1qyyVWhbVo", nil)
	invalidFilterReq, _ := http.NewRequest("GET", "https://localhost:8800//_matrix/client/r0/rooms/!kvEtX3rFamfwKHO3:localhost:8800/context/%24GjmkRbajRHy8_cxcSbUU4qF_njV8yHeLphI2azTrPaI?limit=2&filter=%7B+%22lazy_load_members%22+%3A+true&access_token=t1Njzm74w3G40CJ5xrlf1V2haXom0z0Iq1qyyVWhbVo", nil)
	tests := []struct {
		name       string
		req        *http.Request
		wantFilter *gomatrixserverlib.RoomEventFilter
		wantErr    bool
	}{
		{
			name:       "no params set",
			req:        noParamsReq,
			wantFilter: &gomatrixserverlib.RoomEventFilter{Limit: 10},
		},
		{
			name:       "limit 2 param set",
			req:        limit2Req,
			wantFilter: &gomatrixserverlib.RoomEventFilter{Limit: 2},
		},
		{
			name:       "limit 10000 param set",
			req:        limit10000Req,
			wantFilter: &gomatrixserverlib.RoomEventFilter{Limit: 100},
		},
		{
			name:       "filter lazy_load_members param set",
			req:        lazyLoadReq,
			wantFilter: &gomatrixserverlib.RoomEventFilter{Limit: 2, LazyLoadMembers: true},
		},
		{
			name:    "invalid limit req",
			req:     invalidLimitReq,
			wantErr: true,
		},
		{
			name:    "invalid filter req",
			req:     invalidFilterReq,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotFilter, err := parseRoomEventFilter(tt.req)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseRoomEventFilter() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(gotFilter, tt.wantFilter) {
				t.Errorf("parseRoomEventFilter() gotFilter = %v, want %v", gotFilter, tt.wantFilter)
			}
		})
	}
}
