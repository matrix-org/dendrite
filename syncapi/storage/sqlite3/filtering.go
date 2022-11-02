package sqlite3

import (
	"database/sql"
	"fmt"

	"github.com/matrix-org/dendrite/internal/sqlutil"
)

type FilterOrder int

const (
	FilterOrderNone = iota
	FilterOrderAsc
	FilterOrderDesc
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
	db *sql.DB, txn *sql.Tx, query string, params []interface{},
	senders, notsenders, types, nottypes *[]string, excludeEventIDs []string,
	containsURL *bool, limit int, order FilterOrder,
) (*sql.Stmt, []interface{}, error) {
	offset := len(params)
	if senders != nil {
		if count := len(*senders); count > 0 {
			query += " AND sender IN " + sqlutil.QueryVariadicOffset(count, offset)
			for _, v := range *senders {
				params, offset = append(params, v), offset+1
			}
		} else {
			query += ` AND sender = ""`
		}
	}
	if notsenders != nil {
		if count := len(*notsenders); count > 0 {
			query += " AND sender NOT IN " + sqlutil.QueryVariadicOffset(count, offset)
			for _, v := range *notsenders {
				params, offset = append(params, v), offset+1
			}
		} else {
			query += ` AND sender NOT = ""`
		}
	}
	if types != nil {
		if count := len(*types); count > 0 {
			query += " AND type IN " + sqlutil.QueryVariadicOffset(count, offset)
			for _, v := range *types {
				params, offset = append(params, v), offset+1
			}
		} else {
			query += ` AND type = ""`
		}
	}
	if nottypes != nil {
		if count := len(*nottypes); count > 0 {
			query += " AND type NOT IN " + sqlutil.QueryVariadicOffset(count, offset)
			for _, v := range *nottypes {
				params, offset = append(params, v), offset+1
			}
		} else {
			query += ` AND type NOT = ""`
		}
	}
	if containsURL != nil {
		query += fmt.Sprintf(" AND contains_url = %v", *containsURL)
	}
	if count := len(excludeEventIDs); count > 0 {
		query += " AND event_id NOT IN " + sqlutil.QueryVariadicOffset(count, offset)
		for _, v := range excludeEventIDs {
			params, offset = append(params, v), offset+1
		}
	}
	switch order {
	case FilterOrderAsc:
		query += " ORDER BY id ASC"
	case FilterOrderDesc:
		query += " ORDER BY id DESC"
	}
	if limit > 0 {
		query += fmt.Sprintf(" LIMIT $%d", offset+1)
		params = append(params, limit)
	}

	var stmt *sql.Stmt
	var err error
	if txn != nil {
		stmt, err = txn.Prepare(query)
	} else {
		stmt, err = db.Prepare(query)
	}
	if err != nil {
		return nil, nil, fmt.Errorf("s.db.Prepare: %w", err)
	}
	return stmt, params, nil
}
