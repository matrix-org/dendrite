package sqlite3

import (
	"database/sql"
	"fmt"

	"github.com/matrix-org/dendrite/internal/sqlutil"
)

// prepareWithFilters returns a prepared statement with the
// relevant filters included. It also includes an []interface{}
// list of all the relevant parameters to pass straight to
// QueryContext, QueryRowContext etc.
// We don't take the filter object directly here because the
// fields might come from either a StateFilter or an EventFilter,
// and it's easier just to have the caller extract the relevant
// parts.
func prepareWithFilters(
	db *sql.DB, query string, params []interface{},
	senders, notsenders, types, nottypes []string,
	limit int, order string,
) (*sql.Stmt, []interface{}, error) {
	offset := len(params)
	if count := len(senders); count > 0 {
		query += " AND sender IN " + sqlutil.QueryVariadicOffset(count, offset)
		for _, v := range senders {
			params, offset = append(params, v), offset+1
		}
	}
	if count := len(notsenders); count > 0 {
		query += " AND sender NOT IN " + sqlutil.QueryVariadicOffset(count, offset)
		for _, v := range notsenders {
			params, offset = append(params, v), offset+1
		}
	}
	if count := len(types); count > 0 {
		query += " AND type IN " + sqlutil.QueryVariadicOffset(count, offset)
		for _, v := range types {
			params, offset = append(params, v), offset+1
		}
	}
	if count := len(nottypes); count > 0 {
		query += " AND type NOT IN " + sqlutil.QueryVariadicOffset(count, offset)
		for _, v := range nottypes {
			params, offset = append(params, v), offset+1
		}
	}
	if order != "" {
		query += " ORDER BY id " + order
	}
	query += fmt.Sprintf(" LIMIT $%d", offset+1)
	params = append(params, limit)

	stmt, err := db.Prepare(query)
	if err != nil {
		return nil, nil, fmt.Errorf("s.db.Prepare: %w", err)
	}
	return stmt, params, nil
}
