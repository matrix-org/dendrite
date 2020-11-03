package msc2836

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/matrix-org/dendrite/internal/config"
	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/gomatrixserverlib"
)

type Database interface {
	// StoreRelation stores the parent->child and child->parent relationship for later querying.
	StoreRelation(ctx context.Context, ev *gomatrixserverlib.HeaderedEvent) error
	ChildrenForParent(ctx context.Context, eventID string) ([]string, error)
}

type Postgres struct {
	db                          *sql.DB
	insertRelationStmt          *sql.Stmt
	selectChildrenForParentStmt *sql.Stmt
}

func NewPostgresDatabase(dbOpts *config.DatabaseOptions) (Database, error) {
	var p Postgres
	var err error
	if p.db, err = sqlutil.Open(dbOpts); err != nil {
		return nil, err
	}
	_, err = p.db.Exec(`
	CREATE TABLE IF NOT EXISTS msc2836_relationships (
		parent_event_id TEXT NOT NULL,
		child_event_id TEXT NOT NULL,
		parent_room_id TEXT NOT NULL,
		parent_origin_server_ts BIGINT NOT NULL,
		CONSTRAINT msc2836_relationships_unique UNIQUE (parent_event_id, child_event_id)
	);
	`)
	if err != nil {
		return nil, err
	}
	if p.insertRelationStmt, err = p.db.Prepare(`
		INSERT INTO msc2836_relationships(parent_event_id, child_event_id, parent_room_id, parent_origin_server_ts) VALUES($1, $2, $3, $4) ON CONFLICT DO NOTHING
	`); err != nil {
		return nil, err
	}
	if p.selectChildrenForParentStmt, err = p.db.Prepare(`
		SELECT child_event_id FROM msc2836_relationships WHERE parent_event_id = $1
	`); err != nil {
		return nil, err
	}
	return &p, err
}

func (p *Postgres) StoreRelation(ctx context.Context, ev *gomatrixserverlib.HeaderedEvent) error {
	parent, child := parentChildEventIDs(ev)
	if parent == "" || child == "" {
		return nil
	}
	_, err := p.insertRelationStmt.ExecContext(ctx, parent, child, ev.RoomID(), ev.OriginServerTS())
	return err
}

func (p *Postgres) ChildrenForParent(ctx context.Context, eventID string) ([]string, error) {
	return childrenForParent(ctx, eventID, p.selectChildrenForParentStmt)
}

type SQLite struct {
	db                          *sql.DB
	insertRelationStmt          *sql.Stmt
	selectChildrenForParentStmt *sql.Stmt
	writer                      sqlutil.Writer
}

func NewSQLiteDatabase(dbOpts *config.DatabaseOptions) (Database, error) {
	var s SQLite
	var err error
	if s.db, err = sqlutil.Open(dbOpts); err != nil {
		return nil, err
	}
	s.writer = sqlutil.NewExclusiveWriter()
	_, err = s.db.Exec(`
	CREATE TABLE IF NOT EXISTS msc2836_relationships (
		parent_event_id TEXT NOT NULL,
		child_event_id TEXT NOT NULL,
		parent_room_id TEXT NOT NULL,
		parent_origin_server_ts BIGINT NOT NULL,
		UNIQUE (parent_event_id, child_event_id)
	);
	`)
	if err != nil {
		return nil, err
	}
	if s.insertRelationStmt, err = s.db.Prepare(`
		INSERT INTO msc2836_relationships(parent_event_id, child_event_id, parent_room_id, parent_origin_server_ts) VALUES($1, $2, $3, $4) ON CONFLICT (parent_event_id, child_event_id) DO NOTHING
	`); err != nil {
		return nil, err
	}
	if s.selectChildrenForParentStmt, err = s.db.Prepare(`
		SELECT child_event_id FROM msc2836_relationships WHERE parent_event_id = $1
	`); err != nil {
		return nil, err
	}
	return &s, nil
}

func (s *SQLite) StoreRelation(ctx context.Context, ev *gomatrixserverlib.HeaderedEvent) error {
	parent, child := parentChildEventIDs(ev)
	if parent == "" || child == "" {
		return nil
	}
	_, err := s.insertRelationStmt.ExecContext(ctx, parent, child, ev.RoomID(), ev.OriginServerTS())
	return err
}

func (s *SQLite) ChildrenForParent(ctx context.Context, eventID string) ([]string, error) {
	return childrenForParent(ctx, eventID, s.selectChildrenForParentStmt)
}

// NewDatabase loads the database for msc2836
func NewDatabase(dbOpts *config.DatabaseOptions) (Database, error) {
	if dbOpts.ConnectionString.IsPostgres() {
		return NewPostgresDatabase(dbOpts)
	}
	return NewSQLiteDatabase(dbOpts)
}

func parentChildEventIDs(ev *gomatrixserverlib.HeaderedEvent) (parent string, child string) {
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
	if body.Relationship.RelType == "m.reference" && body.Relationship.EventID != "" {
		return body.Relationship.EventID, ev.EventID()
	}
	return
}

func childrenForParent(ctx context.Context, eventID string, stmt *sql.Stmt) ([]string, error) {
	rows, err := stmt.QueryContext(ctx, eventID)
	if err != nil {
		return nil, err
	}
	defer rows.Close() // nolint: errcheck
	var children []string
	for rows.Next() {
		var childID string
		if err := rows.Scan(&childID); err != nil {
			return nil, err
		}
		children = append(children, childID)
	}
	return children, nil
}
