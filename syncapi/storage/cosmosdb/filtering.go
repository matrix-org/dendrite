package cosmosdb

import (
	"fmt"
)

type FilterOrder int

const (
	FilterOrderNone = iota
	FilterOrderAsc
	FilterOrderDesc
)

func getParamName(offset int) string {
	return fmt.Sprintf("@x%d", offset)
}

// prepareWithFilters returns a prepared statement with the
// relevant filters included. It also includes an []interface{}
// list of all the relevant parameters to pass straight to
// QueryContext, QueryRowContext etc.
// We don't take the filter object directly here because the
// fields might come from either a StateFilter or an EventFilter,
// and it's easier just to have the caller extract the relevant
// parts.
func prepareWithFilters(
	collectionName string, query string, params map[string]interface{},
	senders, notsenders, types, nottypes []string, excludeEventIDs []string,
	limit int, order FilterOrder,
) (sql string, paramsResult map[string]interface{}) {
	offset := len(params)
	sql = query
	paramsResult = params
	// "and (@x4 = null OR ARRAY_CONTAINS(@x4, c.mx_syncapi_current_room_state.sender)) " +
	if len(senders) > 0 {
		offset++
		paramName := getParamName(offset)
		sql += fmt.Sprintf("and ARRAY_CONTAINS(%s, c.%s.sender) ", paramName, collectionName)
		paramsResult[paramName] = senders
	}
	// "and (@x5 = null OR NOT ARRAY_CONTAINS(@x5, c.mx_syncapi_current_room_state.sender)) " +
	if len(notsenders) > 0 {
		offset++
		paramName := getParamName(offset)
		sql += fmt.Sprintf("and NOT ARRAY_CONTAINS(%s, c.%s.sender) ", paramName, collectionName)
		paramsResult[getParamName(offset)] = notsenders
	}
	// "and (@x6 = null OR ARRAY_CONTAINS(@x6, c.mx_syncapi_current_room_state.type)) " +
	if len(types) > 0 {
		offset++
		paramName := getParamName(offset)
		sql += fmt.Sprintf("and ARRAY_CONTAINS(%s, c.%s.type) ", paramName, collectionName)
		paramsResult[paramName] = types
	}
	// "and (@x7 = null OR NOT ARRAY_CONTAINS(@x7, c.mx_syncapi_current_room_state.type)) " +
	if len(nottypes) > 0 {
		offset++
		paramName := getParamName(offset)
		sql += fmt.Sprintf("and NOT ARRAY_CONTAINS(%s, c.%s.type) ", paramName, collectionName)
		paramsResult[getParamName(offset)] = nottypes
	}
	// "and (NOT ARRAY_CONTAINS(@x9, c.mx_syncapi_current_room_state.event_id)) "
	if len(excludeEventIDs) > 0 {
		offset++
		paramName := getParamName(offset)
		sql += fmt.Sprintf("and NOT ARRAY_CONTAINS(%s, c.%s.event_id) ", paramName, collectionName)
		paramsResult[getParamName(offset)] = excludeEventIDs
	}
	switch order {
	case FilterOrderAsc:
		sql += fmt.Sprintf("order by c.%s.id asc ", collectionName)
	case FilterOrderDesc:
		sql += fmt.Sprintf("order by c.%s.id desc ", collectionName)
	}
	// query += fmt.Sprintf(" LIMIT $%d", offset+1)
	return
}
