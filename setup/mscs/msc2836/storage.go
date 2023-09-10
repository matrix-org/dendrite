package msc2836

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"

	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/dendrite/roomserver/types"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/gomatrixserverlib/spec"
	"github.com/matrix-org/util"
)

type eventInfo struct {
	EventID        string
	OriginServerTS spec.Timestamp
	RoomID         string
}

type Database interface {
	// StoreRelation stores the parent->child and child->parent relationship for later querying.
	// Also stores the event metadata e.g timestamp
	StoreRelation(ctx context.Context, ev *types.HeaderedEvent) error
	// ChildrenForParent returns the events who have the given `eventID` as an m.relationship with the
	// provided `relType`. The returned slice is sorted by origin_server_ts according to whether
	// `recentFirst` is true or false.
	ChildrenForParent(ctx context.Context, eventID, relType string, recentFirst bool) ([]eventInfo, error)
	// ParentForChild returns the parent event for the given child `eventID`. The eventInfo should be nil if
	// there is no parent for this child event, with no error. The parent eventInfo can be missing the
	// timestamp if the event is not known to the server.
	ParentForChild(ctx context.Context, eventID, relType string) (*eventInfo, error)
	// UpdateChildMetadata persists the children_count and children_hash from this event if and only if
	// the count is greater than what was previously there. If the count is updated, the event will be
	// updated to be unexplored.
	UpdateChildMetadata(ctx context.Context, ev *types.HeaderedEvent) error
	// ChildMetadata returns the children_count and children_hash for the event ID in question.
	// Also returns the `explored` flag, which is set to true when MarkChildrenExplored is called and is set
	// back to `false` when a larger count is inserted via UpdateChildMetadata.
	// Returns nil error if the event ID does not exist.
	ChildMetadata(ctx context.Context, eventID string) (count int, hash []byte, explored bool, err error)
	// MarkChildrenExplored sets the 'explored' flag on this event to `true`.
	MarkChildrenExplored(ctx context.Context, eventID string) error
}

type DB struct {
	db                                     *sql.DB
	writer                                 sqlutil.Writer
	insertEdgeStmt                         *sql.Stmt
	insertNodeStmt                         *sql.Stmt
	selectChildrenForParentOldestFirstStmt *sql.Stmt
	selectChildrenForParentRecentFirstStmt *sql.Stmt
	selectParentForChildStmt               *sql.Stmt
	updateChildMetadataStmt                *sql.Stmt
	selectChildMetadataStmt                *sql.Stmt
	updateChildMetadataExploredStmt        *sql.Stmt
}

// NewDatabase loads the database for msc2836
func NewDatabase(conMan *sqlutil.Connections, dbOpts *config.DatabaseOptions) (Database, error) {
	if dbOpts.ConnectionString.IsPostgres() {
		return newPostgresDatabase(conMan, dbOpts)
	}
	return newSQLiteDatabase(conMan, dbOpts)
}

func newPostgresDatabase(conMan *sqlutil.Connections, dbOpts *config.DatabaseOptions) (Database, error) {
	d := DB{}
	var err error
	if d.db, d.writer, err = conMan.Connection(dbOpts); err != nil {
		return nil, err
	}
	_, err = d.db.Exec(`
	CREATE TABLE IF NOT EXISTS msc2836_edges (
		parent_event_id TEXT NOT NULL,
		child_event_id TEXT NOT NULL,
		rel_type TEXT NOT NULL,
		parent_room_id TEXT NOT NULL,
		parent_servers TEXT NOT NULL,
		CONSTRAINT msc2836_edges_uniq UNIQUE (parent_event_id, child_event_id, rel_type)
	);

	CREATE TABLE IF NOT EXISTS msc2836_nodes (
		event_id TEXT PRIMARY KEY NOT NULL,
		origin_server_ts BIGINT NOT NULL,
		room_id TEXT NOT NULL,
		unsigned_children_count BIGINT NOT NULL,
		unsigned_children_hash TEXT NOT NULL,
		explored SMALLINT NOT NULL
	);
	`)
	if err != nil {
		return nil, err
	}
	if d.insertEdgeStmt, err = d.db.Prepare(`
		INSERT INTO msc2836_edges(parent_event_id, child_event_id, rel_type, parent_room_id, parent_servers)
		VALUES($1, $2, $3, $4, $5)
		ON CONFLICT DO NOTHING
	`); err != nil {
		return nil, err
	}
	if d.insertNodeStmt, err = d.db.Prepare(`
		INSERT INTO msc2836_nodes(event_id, origin_server_ts, room_id, unsigned_children_count, unsigned_children_hash, explored)
		VALUES($1, $2, $3, $4, $5, $6)
		ON CONFLICT DO NOTHING
	`); err != nil {
		return nil, err
	}
	selectChildrenQuery := `
	SELECT child_event_id, origin_server_ts, room_id FROM msc2836_edges
	LEFT JOIN msc2836_nodes ON msc2836_edges.child_event_id = msc2836_nodes.event_id
	WHERE parent_event_id = $1 AND rel_type = $2
	ORDER BY origin_server_ts
	`
	if d.selectChildrenForParentOldestFirstStmt, err = d.db.Prepare(selectChildrenQuery + "ASC"); err != nil {
		return nil, err
	}
	if d.selectChildrenForParentRecentFirstStmt, err = d.db.Prepare(selectChildrenQuery + "DESC"); err != nil {
		return nil, err
	}
	if d.selectParentForChildStmt, err = d.db.Prepare(`
		SELECT parent_event_id, parent_room_id FROM msc2836_edges
		WHERE child_event_id = $1 AND rel_type = $2
	`); err != nil {
		return nil, err
	}
	if d.updateChildMetadataStmt, err = d.db.Prepare(`
		UPDATE msc2836_nodes SET unsigned_children_count=$1, unsigned_children_hash=$2, explored=$3 WHERE event_id=$4
	`); err != nil {
		return nil, err
	}
	if d.selectChildMetadataStmt, err = d.db.Prepare(`
		SELECT unsigned_children_count, unsigned_children_hash, explored FROM msc2836_nodes WHERE event_id=$1
	`); err != nil {
		return nil, err
	}
	if d.updateChildMetadataExploredStmt, err = d.db.Prepare(`
		UPDATE msc2836_nodes SET explored=$1 WHERE event_id=$2
	`); err != nil {
		return nil, err
	}
	return &d, err
}

func newSQLiteDatabase(conMan *sqlutil.Connections, dbOpts *config.DatabaseOptions) (Database, error) {
	d := DB{}
	var err error
	if d.db, d.writer, err = conMan.Connection(dbOpts); err != nil {
		return nil, err
	}
	_, err = d.db.Exec(`
	CREATE TABLE IF NOT EXISTS msc2836_edges (
		parent_event_id TEXT NOT NULL,
		child_event_id TEXT NOT NULL,
		rel_type TEXT NOT NULL,
		parent_room_id TEXT NOT NULL,
		parent_servers TEXT NOT NULL,
		UNIQUE (parent_event_id, child_event_id, rel_type)
	);

	CREATE TABLE IF NOT EXISTS msc2836_nodes (
		event_id TEXT PRIMARY KEY NOT NULL,
		origin_server_ts BIGINT NOT NULL,
		room_id TEXT NOT NULL,
		unsigned_children_count BIGINT NOT NULL,
		unsigned_children_hash TEXT NOT NULL,
		explored SMALLINT NOT NULL
	);
	`)
	if err != nil {
		return nil, err
	}
	if d.insertEdgeStmt, err = d.db.Prepare(`
		INSERT INTO msc2836_edges(parent_event_id, child_event_id, rel_type, parent_room_id, parent_servers)
		VALUES($1, $2, $3, $4, $5)
		ON CONFLICT (parent_event_id, child_event_id, rel_type) DO NOTHING
	`); err != nil {
		return nil, err
	}
	if d.insertNodeStmt, err = d.db.Prepare(`
		INSERT INTO msc2836_nodes(event_id, origin_server_ts, room_id, unsigned_children_count, unsigned_children_hash, explored)
		VALUES($1, $2, $3, $4, $5, $6)
		ON CONFLICT DO NOTHING
	`); err != nil {
		return nil, err
	}
	selectChildrenQuery := `
	SELECT child_event_id, origin_server_ts, room_id FROM msc2836_edges
	LEFT JOIN msc2836_nodes ON msc2836_edges.child_event_id = msc2836_nodes.event_id
	WHERE parent_event_id = $1 AND rel_type = $2
	ORDER BY origin_server_ts
	`
	if d.selectChildrenForParentOldestFirstStmt, err = d.db.Prepare(selectChildrenQuery + "ASC"); err != nil {
		return nil, err
	}
	if d.selectChildrenForParentRecentFirstStmt, err = d.db.Prepare(selectChildrenQuery + "DESC"); err != nil {
		return nil, err
	}
	if d.selectParentForChildStmt, err = d.db.Prepare(`
		SELECT parent_event_id, parent_room_id FROM msc2836_edges
		WHERE child_event_id = $1 AND rel_type = $2
	`); err != nil {
		return nil, err
	}
	if d.updateChildMetadataStmt, err = d.db.Prepare(`
		UPDATE msc2836_nodes SET unsigned_children_count=$1, unsigned_children_hash=$2, explored=$3 WHERE event_id=$4
	`); err != nil {
		return nil, err
	}
	if d.selectChildMetadataStmt, err = d.db.Prepare(`
		SELECT unsigned_children_count, unsigned_children_hash, explored FROM msc2836_nodes WHERE event_id=$1
	`); err != nil {
		return nil, err
	}
	if d.updateChildMetadataExploredStmt, err = d.db.Prepare(`
		UPDATE msc2836_nodes SET explored=$1 WHERE event_id=$2
	`); err != nil {
		return nil, err
	}
	return &d, nil
}

func (p *DB) StoreRelation(ctx context.Context, ev *types.HeaderedEvent) error {
	parent, child, relType := parentChildEventIDs(ev)
	if parent == "" || child == "" {
		return nil
	}
	relationRoomID, relationServers := roomIDAndServers(ev)
	relationServersJSON, err := json.Marshal(relationServers)
	if err != nil {
		return err
	}
	count, hash := extractChildMetadata(ev)
	return p.writer.Do(p.db, nil, func(txn *sql.Tx) error {
		_, err := txn.Stmt(p.insertEdgeStmt).ExecContext(ctx, parent, child, relType, relationRoomID, string(relationServersJSON))
		if err != nil {
			return err
		}
		util.GetLogger(ctx).Infof("StoreRelation child=%s parent=%s rel_type=%s", child, parent, relType)
		_, err = txn.Stmt(p.insertNodeStmt).ExecContext(ctx, ev.EventID(), ev.OriginServerTS(), ev.RoomID(), count, base64.RawStdEncoding.EncodeToString(hash), 0)
		return err
	})
}

func (p *DB) UpdateChildMetadata(ctx context.Context, ev *types.HeaderedEvent) error {
	eventCount, eventHash := extractChildMetadata(ev)
	if eventCount == 0 {
		return nil // nothing to update with
	}

	// extract current children count/hash, if they are less than the current event then update the columns and set to unexplored
	count, hash, _, err := p.ChildMetadata(ctx, ev.EventID())
	if err != nil {
		return err
	}
	if eventCount > count || (eventCount == count && !bytes.Equal(hash, eventHash)) {
		_, err = p.updateChildMetadataStmt.ExecContext(ctx, eventCount, base64.RawStdEncoding.EncodeToString(eventHash), 0, ev.EventID())
		return err
	}
	return nil
}

func (p *DB) ChildMetadata(ctx context.Context, eventID string) (count int, hash []byte, explored bool, err error) {
	var b64hash string
	var exploredInt int
	if err = p.selectChildMetadataStmt.QueryRowContext(ctx, eventID).Scan(&count, &b64hash, &exploredInt); err != nil {
		if err == sql.ErrNoRows {
			err = nil
		}
		return
	}
	hash, err = base64.RawStdEncoding.DecodeString(b64hash)
	explored = exploredInt > 0
	return
}

func (p *DB) MarkChildrenExplored(ctx context.Context, eventID string) error {
	_, err := p.updateChildMetadataExploredStmt.ExecContext(ctx, 1, eventID)
	return err
}

func (p *DB) ChildrenForParent(ctx context.Context, eventID, relType string, recentFirst bool) ([]eventInfo, error) {
	var rows *sql.Rows
	var err error
	if recentFirst {
		rows, err = p.selectChildrenForParentRecentFirstStmt.QueryContext(ctx, eventID, relType)
	} else {
		rows, err = p.selectChildrenForParentOldestFirstStmt.QueryContext(ctx, eventID, relType)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close() // nolint: errcheck
	var children []eventInfo
	for rows.Next() {
		var evInfo eventInfo
		if err := rows.Scan(&evInfo.EventID, &evInfo.OriginServerTS, &evInfo.RoomID); err != nil {
			return nil, err
		}
		children = append(children, evInfo)
	}
	return children, nil
}

func (p *DB) ParentForChild(ctx context.Context, eventID, relType string) (*eventInfo, error) {
	var ei eventInfo
	err := p.selectParentForChildStmt.QueryRowContext(ctx, eventID, relType).Scan(&ei.EventID, &ei.RoomID)
	if err == sql.ErrNoRows {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	return &ei, nil
}

func parentChildEventIDs(ev *types.HeaderedEvent) (parent, child, relType string) {
	if ev == nil {
		return
	}
	body := struct {
		Relationship struct {
			RelType string `json:"rel_type"`
			EventID string `json:"event_id"`
		} `json:"m.relationship"`
	}{}
	if err := json.Unmarshal(ev.Content(), &body); err != nil {
		return
	}
	if body.Relationship.EventID == "" || body.Relationship.RelType == "" {
		return
	}
	return body.Relationship.EventID, ev.EventID(), body.Relationship.RelType
}

func roomIDAndServers(ev *types.HeaderedEvent) (roomID string, servers []string) {
	servers = []string{}
	if ev == nil {
		return
	}
	body := struct {
		RoomID  string   `json:"relationship_room_id"`
		Servers []string `json:"relationship_servers"`
	}{}
	if err := json.Unmarshal(ev.Unsigned(), &body); err != nil {
		return
	}
	return body.RoomID, body.Servers
}

func extractChildMetadata(ev *types.HeaderedEvent) (count int, hash []byte) {
	unsigned := struct {
		Counts map[string]int   `json:"children"`
		Hash   spec.Base64Bytes `json:"children_hash"`
	}{}
	if err := json.Unmarshal(ev.Unsigned(), &unsigned); err != nil {
		// expected if there is no unsigned field at all
		return
	}
	for _, c := range unsigned.Counts {
		count += c
	}
	hash = unsigned.Hash
	return
}
