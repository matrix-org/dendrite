package naffka

import (
	"database/sql"
	"sync"
	"time"
)

const postgresqlSchema = `
-- The topic table assigns each topic a unique numeric ID.
CREATE SEQUENCE IF NOT EXISTS naffka_topic_nid_seq;
CREATE TABLE IF NOT EXISTS naffka_topics (
	topic_name TEXT PRIMARY KEY,
	topic_nid  BIGINT NOT NULL DEFAULT nextval('naffka_topic_nid_seq')
);

-- The messages table contains the actual messages.
CREATE TABLE IF NOT EXISTS naffka_messages (
	topic_nid BIGINT NOT NULL,
	message_offset BIGINT NOT NULL,
	message_key BYTEA NOT NULL,
	message_value BYTEA NOT NULL,
	message_timestamp_ns BIGINT NOT NULL,
	UNIQUE (topic_nid, message_offset)
);
`

const insertTopicSQL = "" +
	"INSERT INTO naffka_topics (topic_name) VALUES ($1)" +
	" ON CONFLICT DO NOTHING" +
	" RETURNING (topic_nid)"

const selectTopicSQL = "" +
	"SELECT topic_nid FROM naffka_topics WHERE topic_name = $1"

const selectTopicsSQL = "" +
	"SELECT topic_name, topic_nid FROM naffka_topics"

const insertMessageSQL = "" +
	"INSERT INTO naffka_messages (topic_nid, message_offset, message_key, message_value, message_timestamp_ns)" +
	" VALUES ($1, $2, $3, $4, $5)"

const selectMessagesSQL = "" +
	"SELECT message_offset, message_key, message_value, message_timestamp_ns" +
	" FROM naffka_messages WHERE topic_nid = $1 AND $2 <= message_offset AND message_offset < $3" +
	" ORDER BY message_offset ASC"

const selectMaxOffsetSQL = "" +
	"SELECT message_offset FROM naffka_messages WHERE topic_nid = $1" +
	" ORDER BY message_offset DESC LIMIT 1"

type postgresqlDatabase struct {
	db                  *sql.DB
	topicsMutex         sync.Mutex
	topicNIDs           map[string]int64
	insertTopicStmt     *sql.Stmt
	selectTopicStmt     *sql.Stmt
	selectTopicsStmt    *sql.Stmt
	insertMessageStmt   *sql.Stmt
	selectMessagesStmt  *sql.Stmt
	selectMaxOffsetStmt *sql.Stmt
}

// NewPostgresqlDatabase creates a new naffka database using a postgresql database.
// Returns an error if there was a problem setting up the database.
func NewPostgresqlDatabase(db *sql.DB) (Database, error) {
	var err error

	p := &postgresqlDatabase{
		db:        db,
		topicNIDs: map[string]int64{},
	}

	if _, err = db.Exec(postgresqlSchema); err != nil {
		return nil, err
	}

	for _, s := range []struct {
		sql  string
		stmt **sql.Stmt
	}{
		{insertTopicSQL, &p.insertTopicStmt},
		{selectTopicSQL, &p.selectTopicStmt},
		{selectTopicsSQL, &p.selectTopicsStmt},
		{insertMessageSQL, &p.insertMessageStmt},
		{selectMessagesSQL, &p.selectMessagesStmt},
		{selectMaxOffsetSQL, &p.selectMaxOffsetStmt},
	} {
		*s.stmt, err = db.Prepare(s.sql)
		if err != nil {
			return nil, err
		}
	}
	return p, nil
}

// StoreMessages implements Database.
func (p *postgresqlDatabase) StoreMessages(topic string, messages []Message) error {
	// Store the messages inside a single database transaction.
	return withTransaction(p.db, func(txn *sql.Tx) error {
		s := txn.Stmt(p.insertMessageStmt)
		topicNID, err := p.assignTopicNID(txn, topic)
		if err != nil {
			return err
		}
		for _, m := range messages {
			_, err = s.Exec(topicNID, m.Offset, m.Key, m.Value, m.Timestamp.UnixNano())
			if err != nil {
				return err
			}
		}
		return nil
	})
}

// FetchMessages implements Database.
func (p *postgresqlDatabase) FetchMessages(topic string, startOffset, endOffset int64) (messages []Message, err error) {
	topicNID, err := p.getTopicNID(nil, topic)
	if err != nil {
		return
	}
	rows, err := p.selectMessagesStmt.Query(topicNID, startOffset, endOffset)
	if err != nil {
		return
	}
	defer rows.Close()
	for rows.Next() {
		var (
			offset        int64
			key           []byte
			value         []byte
			timestampNano int64
		)
		if err = rows.Scan(&offset, &key, &value, &timestampNano); err != nil {
			return
		}
		messages = append(messages, Message{
			Offset:    offset,
			Key:       key,
			Value:     value,
			Timestamp: time.Unix(0, timestampNano),
		})
	}
	return
}

// MaxOffsets implements Database.
func (p *postgresqlDatabase) MaxOffsets() (map[string]int64, error) {
	topicNames, err := p.selectTopics()
	if err != nil {
		return nil, err
	}
	result := map[string]int64{}
	for topicName, topicNID := range topicNames {
		// Lookup the maximum offset.
		maxOffset, err := p.selectMaxOffset(topicNID)
		if err != nil {
			return nil, err
		}
		if maxOffset > -1 {
			// Don't include the topic if we haven't sent any messages on it.
			result[topicName] = maxOffset
		}
		// Prefill the numeric ID cache.
		p.addTopicNIDToCache(topicName, topicNID)
	}
	return result, nil
}

// selectTopics fetches the names and numeric IDs for all the topics the
// database is aware of.
func (p *postgresqlDatabase) selectTopics() (map[string]int64, error) {
	rows, err := p.selectTopicsStmt.Query()
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := map[string]int64{}
	for rows.Next() {
		var (
			topicName string
			topicNID  int64
		)
		if err = rows.Scan(&topicName, &topicNID); err != nil {
			return nil, err
		}
		result[topicName] = topicNID
	}
	return result, nil
}

// selectMaxOffset selects the maximum offset for a topic.
// Returns -1 if there aren't any messages for that topic.
// Returns an error if there was a problem talking to the database.
func (p *postgresqlDatabase) selectMaxOffset(topicNID int64) (maxOffset int64, err error) {
	err = p.selectMaxOffsetStmt.QueryRow(topicNID).Scan(&maxOffset)
	if err == sql.ErrNoRows {
		return -1, nil
	}
	return maxOffset, err
}

// getTopicNID finds the numeric ID for a topic.
// The txn argument is optional, this can be used outside a transaction
// by setting the txn argument to nil.
func (p *postgresqlDatabase) getTopicNID(txn *sql.Tx, topicName string) (topicNID int64, err error) {
	// Get from the cache.
	topicNID = p.getTopicNIDFromCache(topicName)
	if topicNID != 0 {
		return topicNID, nil
	}
	// Get from the database
	s := p.selectTopicStmt
	if txn != nil {
		s = txn.Stmt(s)
	}
	err = s.QueryRow(topicName).Scan(&topicNID)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	// Update the shared cache.
	p.addTopicNIDToCache(topicName, topicNID)
	return topicNID, nil
}

// assignTopicNID assigns a new numeric ID to a topic.
// The txn argument is mandatory, this is always called inside a transaction.
func (p *postgresqlDatabase) assignTopicNID(txn *sql.Tx, topicName string) (topicNID int64, err error) {
	// Check if we already have a numeric ID for the topic name.
	topicNID, err = p.getTopicNID(txn, topicName)
	if err != nil {
		return 0, err
	}
	if topicNID != 0 {
		return topicNID, err
	}
	// We don't have a numeric ID for the topic name so we add an entry to the
	// topics table. If the insert stmt succeeds then it will return the ID.
	err = txn.Stmt(p.insertTopicStmt).QueryRow(topicName).Scan(&topicNID)
	if err == sql.ErrNoRows {
		// If the insert stmt succeeded, but didn't return any rows then it
		// means that someone has added a row for the topic name between us
		// selecting it the first time and us inserting our own row.
		// (N.B. postgres only returns modified rows when using "RETURNING")
		// So we can now just select the row that someone else added.
		// TODO: This is probably unnecessary since naffka writes to a topic
		// from a single thread.
		return p.getTopicNID(txn, topicName)
	}
	if err != nil {
		return 0, err
	}
	// Update the cache.
	p.addTopicNIDToCache(topicName, topicNID)
	return topicNID, nil
}

// getTopicNIDFromCache returns the topicNID from the cache or returns 0 if the
// topic is not in the cache.
func (p *postgresqlDatabase) getTopicNIDFromCache(topicName string) (topicNID int64) {
	p.topicsMutex.Lock()
	defer p.topicsMutex.Unlock()
	return p.topicNIDs[topicName]
}

// addTopicNIDToCache adds the numeric ID for the topic to the cache.
func (p *postgresqlDatabase) addTopicNIDToCache(topicName string, topicNID int64) {
	p.topicsMutex.Lock()
	defer p.topicsMutex.Unlock()
	p.topicNIDs[topicName] = topicNID
}

// withTransaction runs a block of code passing in an SQL transaction
// If the code returns an error or panics then the transactions is rolledback
// Otherwise the transaction is committed.
func withTransaction(db *sql.DB, fn func(txn *sql.Tx) error) (err error) {
	txn, err := db.Begin()
	if err != nil {
		return
	}
	defer func() {
		if r := recover(); r != nil {
			txn.Rollback()
			panic(r)
		} else if err != nil {
			txn.Rollback()
		} else {
			err = txn.Commit()
		}
	}()
	err = fn(txn)
	return
}
