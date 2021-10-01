package naffkacosmosdb

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/matrix-org/dendrite/internal/cosmosdbapi"
	"github.com/matrix-org/naffka/types"
)

// const sqliteSchema = `
// -- The topic table assigns each topic a unique numeric ID.
// CREATE TABLE IF NOT EXISTS naffka_topics (
// 	topic_name TEXT UNIQUE,
// 	topic_nid  INTEGER PRIMARY KEY AUTOINCREMENT
// );

// -- The messages table contains the actual messages.
// CREATE TABLE IF NOT EXISTS naffka_messages (
// 	topic_nid INTEGER NOT NULL,
// 	message_offset BLOB NOT NULL,
// 	message_key BLOB,
// 	message_value BLOB NOT NULL,
// 	message_timestamp_ns INTEGER NOT NULL,
// 	UNIQUE (topic_nid, message_offset)
// );
// `

type topicCosmos struct {
	TopicName string `json:"topic_name"`
	TopicNID  int64  `json:"topic_nid"`
}

type topicCosmosNumber struct {
	Number int64 `json:"number"`
}

type topicCosmosData struct {
	cosmosdbapi.CosmosDocument
	Topic topicCosmos `json:"mx_naffka_topic"`
}

type messageCosmos struct {
	TopicNID           int64  `json:"topic_nid"`
	MessageOffset      int64  `json:"message_offset"`
	MessageKey         []byte `json:"message_key"`
	MessageValue       []byte `json:"message_value"`
	MessageTimestampNS int64  `json:"message_timestamp_ns"`
}

type messageCosmosData struct {
	cosmosdbapi.CosmosDocument
	Message messageCosmos `json:"mx_naffka_message"`
}

// const insertTopicSQL = "" +
// 	"INSERT INTO naffka_topics (topic_name, topic_nid) VALUES ($1, $2)" +
// 	" ON CONFLICT DO NOTHING"

// "SELECT COUNT(topic_nid)+1 AS topic_nid FROM naffka_topics"
const selectNextTopicNIDSQL = "" +
	"select count(c._ts)+1 as number from c where c._cn = @x1 "

// const selectTopicSQL = "" +
// 	"SELECT topic_nid FROM naffka_topics WHERE topic_name = $1"

// "SELECT topic_name, topic_nid FROM naffka_topics"
const selectTopicsSQL = "" +
	"select * from c where c._cn = @x1 "

// const insertTopicsSQL = "" +
// 	"INSERT INTO naffka_messages (topic_nid, message_offset, message_key, message_value, message_timestamp_ns)" +
// 	" VALUES ($1, $2, $3, $4, $5)"

// "SELECT message_offset, message_key, message_value, message_timestamp_ns" +
// " FROM naffka_messages WHERE topic_nid = $1 AND $2 <= message_offset AND message_offset < $3" +
// " ORDER BY message_offset ASC"
const selectMessagesSQL = "" +
	"select * from c where c._cn = @x1 " +
	"and c.mx_naffka_message.topic_nid = @x2 " +
	"and c.mx_naffka_message.message_offset <= @x3 " +
	"and c.mx_naffka_message.message_offset < @x4 " +
	"order by c.mx_naffka_message.message_offset asc "

// "SELECT message_offset FROM naffka_messages WHERE topic_nid = $1" +
// " ORDER BY message_offset DESC LIMIT 1"
const selectMaxOffsetSQL = "" +
	"select top 1 * from c where c._cn = @x1 " +
	"and c.mx_naffka_message.topic_nid = @x2 " +
	"order by c.mx_naffka_message.message_offset desc "

type topicsStatements struct {
	DB *Database
	// insertTopicStmt        *sql.Stmt
	// insertTopicsStmt       *sql.Stmt
	selectNextTopicNIDStmt string
	// selectTopicStmt        *sql.Stmt
	selectTopicsStmt    string
	selectMessagesStmt  string
	selectMaxOffsetStmt string
	tableNameTopics     string
	tableNameMessages   string
}

func (s *topicsStatements) getCollectionNameTopics() string {
	return cosmosdbapi.GetCollectionName(s.DB.databaseName, s.tableNameTopics)
}

func (s *topicsStatements) getPartitionKeyTopics() string {
	return cosmosdbapi.GetPartitionKeyByCollection(s.DB.cosmosConfig.TenantName, s.getCollectionNameTopics())
}

func (s *topicsStatements) getCollectionNameMessages() string {
	return cosmosdbapi.GetCollectionName(s.DB.databaseName, s.tableNameMessages)
}

func (s *topicsStatements) getPartitionKeyMessages(topicNid int64) string {
	uniqueId := fmt.Sprintf("%d", topicNid)
	return cosmosdbapi.GetPartitionKeyByUniqueId(s.DB.cosmosConfig.TenantName, s.getCollectionNameMessages(), uniqueId)
}

func getTopic(s *topicsStatements, ctx context.Context, pk string, docId string) (*topicCosmosData, error) {
	response := topicCosmosData{}
	err := cosmosdbapi.GetDocumentOrNil(
		s.DB.connection,
		s.DB.cosmosConfig,
		ctx,
		pk,
		docId,
		&response)

	if response.Id == "" {
		return nil, nil
	}

	return &response, err
}

func NewCosmosDBTopicsTable(db *Database) (s *topicsStatements, err error) {
	s = &topicsStatements{
		DB: db,
	}
	s.selectNextTopicNIDStmt = selectNextTopicNIDSQL
	s.selectTopicsStmt = selectTopicsSQL
	s.selectMessagesStmt = selectMessagesSQL
	s.selectMaxOffsetStmt = selectMaxOffsetSQL
	s.tableNameTopics = "topics"
	s.tableNameMessages = "messages"
	return
}

func (t *topicsStatements) InsertTopic(
	ctx context.Context, txn *sql.Tx, topicName string, topicNID int64,
) error {

	// 	"INSERT INTO naffka_topics (topic_name, topic_nid) VALUES ($1, $2)" +
	// 	" ON CONFLICT DO NOTHING"

	// stmt := sqlutil.TxStmt(txn, t.insertTopicStmt)

	// 	topic_nid  INTEGER PRIMARY KEY AUTOINCREMENT
	// idSeq, errSeq := GetNextTopicNID(t, ctx)
	// if(errSeq != nil) {
	// 	return errSeq
	// }

	// 	topic_name TEXT UNIQUE,
	docId := fmt.Sprintf("%s", topicName)
	cosmosDocId := cosmosdbapi.GetDocumentId(t.DB.cosmosConfig.ContainerName, t.getCollectionNameTopics(), docId)

	dbData, _ := getTopic(t, ctx, t.getPartitionKeyTopics(), cosmosDocId)
	if dbData != nil {
		dbData.SetUpdateTime()
		dbData.Topic.TopicName = topicName
	} else {
		data := topicCosmos{
			TopicNID:  topicNID,
			TopicName: topicName,
		}

		dbData = &topicCosmosData{
			CosmosDocument: cosmosdbapi.GenerateDocument(t.getCollectionNameTopics(), t.DB.cosmosConfig.TenantName, t.getPartitionKeyTopics(), cosmosDocId),
			Topic:          data,
		}
	}

	// _, err := stmt.ExecContext(ctx, topicName, topicNID)

	return cosmosdbapi.UpsertDocument(ctx,
		t.DB.connection,
		t.DB.cosmosConfig.DatabaseName,
		t.DB.cosmosConfig.ContainerName,
		dbData.Pk,
		dbData)
}

func (t *topicsStatements) SelectNextTopicNID(
	ctx context.Context, txn *sql.Tx,
) (topicNID int64, err error) {

	// "SELECT COUNT(topic_nid)+1 AS topic_nid FROM naffka_topics"

	params := map[string]interface{}{
		"@x1": t.getCollectionNameTopics(),
	}

	// stmt := sqlutil.TxStmt(txn, t.selectNextTopicNIDStmt)
	// err = stmt.QueryRowContext(ctx).Scan(&topicNID)
	var rows []topicCosmosNumber
	err = cosmosdbapi.PerformQuery(ctx,
		t.DB.connection,
		t.DB.cosmosConfig.DatabaseName,
		t.DB.cosmosConfig.ContainerName,
		t.getPartitionKeyTopics(), t.selectNextTopicNIDStmt, params, &rows)

	if err != nil {
		return 0, err
	}

	if len(rows) == 0 {
		return 0, nil
	}

	topicNID = rows[0].Number
	return
}

func (t *topicsStatements) SelectTopic(
	ctx context.Context, txn *sql.Tx, topicName string,
) (topicNID int64, err error) {

	// "SELECT topic_nid FROM naffka_topics WHERE topic_name = $1"

	// stmt := sqlutil.TxStmt(txn, t.selectTopicStmt)

	// 	topic_name TEXT UNIQUE,
	docId := fmt.Sprintf("%s", topicName)
	cosmosDocId := cosmosdbapi.GetDocumentId(t.DB.cosmosConfig.ContainerName, t.getCollectionNameTopics(), docId)

	// err = stmt.QueryRowContext(ctx, topicName).Scan(&topicNID)
	res, err := getTopic(t, ctx, t.getPartitionKeyTopics(), cosmosDocId)

	if err != nil {
		return 0, err
	}

	if res == nil {
		return 0, nil
	}
	return
}

func (t *topicsStatements) SelectTopics(
	ctx context.Context, txn *sql.Tx,
) (map[string]int64, error) {

	// "SELECT topic_name, topic_nid FROM naffka_topics"

	params := map[string]interface{}{
		"@x1": t.getCollectionNameTopics(),
	}

	// stmt := sqlutil.TxStmt(txn, t.selectTopicsStmt)
	// rows, err := stmt.QueryContext(ctx)
	var rows []topicCosmosData
	err := cosmosdbapi.PerformQuery(ctx,
		t.DB.connection,
		t.DB.cosmosConfig.DatabaseName,
		t.DB.cosmosConfig.ContainerName,
		t.getPartitionKeyTopics(), t.selectTopicsStmt, params, &rows)

	if err != nil {
		return nil, err
	}

	result := map[string]int64{}
	for _, item := range rows {
		var (
			topicName string
			topicNID  int64
		)
		topicName = item.Topic.TopicName
		topicNID = item.Topic.TopicNID
		result[topicName] = topicNID
	}
	return result, nil
}

func (t *topicsStatements) InsertTopics(
	ctx context.Context, txn *sql.Tx, topicNID int64, messageOffset int64,
	topicKey, topicValue []byte, messageTimestampNs int64,
) error {

	// 	"INSERT INTO naffka_messages (topic_nid, message_offset, message_key, message_value, message_timestamp_ns)" +
	// 	" VALUES ($1, $2, $3, $4, $5)"

	// stmt := sqlutil.TxStmt(txn, t.insertTopicsStmt)

	// 	UNIQUE (topic_nid, message_offset)
	docId := fmt.Sprintf("%d,%d", topicNID, messageOffset)
	cosmosDocId := cosmosdbapi.GetDocumentId(t.DB.cosmosConfig.ContainerName, t.getCollectionNameMessages(), docId)

	data := messageCosmos{
		TopicNID:           topicNID,
		MessageOffset:      messageOffset,
		MessageKey:         topicKey,
		MessageValue:       topicValue,
		MessageTimestampNS: messageTimestampNs,
	}

	dbData := &messageCosmosData{
		CosmosDocument: cosmosdbapi.GenerateDocument(t.getCollectionNameMessages(), t.DB.cosmosConfig.TenantName, t.getPartitionKeyMessages(topicNID), cosmosDocId),
		Message:        data,
	}

	// _, err := stmt.ExecContext(ctx, topicNID, messageOffset, topicKey, topicValue, messageTimestampNs)

	var options = cosmosdbapi.GetCreateDocumentOptions(dbData.Pk)
	_, _, err := cosmosdbapi.GetClient(t.DB.connection).CreateDocument(
		ctx,
		t.DB.cosmosConfig.DatabaseName,
		t.DB.cosmosConfig.ContainerName,
		&dbData,
		options)
	return err
}

func (t *topicsStatements) SelectMessages(
	ctx context.Context, txn *sql.Tx, topicNID int64, startOffset, endOffset int64,
) ([]types.Message, error) {

	// "SELECT message_offset, message_key, message_value, message_timestamp_ns" +
	// " FROM naffka_messages WHERE topic_nid = $1 AND $2 <= message_offset AND message_offset < $3" +
	// " ORDER BY message_offset ASC"

	params := map[string]interface{}{
		"@x1": t.getCollectionNameMessages(),
		"@x2": topicNID,
		"@x3": startOffset,
		"@x4": endOffset,
	}

	// stmt := sqlutil.TxStmt(txn, t.selectMessagesStmt)
	// rows, err := stmt.QueryContext(ctx, topicNID, startOffset, endOffset)
	var rows []messageCosmosData
	err := cosmosdbapi.PerformQuery(ctx,
		t.DB.connection,
		t.DB.cosmosConfig.DatabaseName,
		t.DB.cosmosConfig.ContainerName,
		t.getPartitionKeyMessages(topicNID), t.selectMessagesStmt, params, &rows)

	if err != nil {
		return nil, err
	}

	result := []types.Message{}
	for _, item := range rows {
		var msg types.Message
		var ts int64
		msg.Offset = item.Message.MessageOffset
		msg.Key = item.Message.MessageKey
		msg.Value = item.Message.MessageValue
		ts = item.Message.MessageTimestampNS
		msg.Timestamp = time.Unix(0, ts)
		result = append(result, msg)
	}
	return result, nil
}

func (t *topicsStatements) SelectMaxOffset(
	ctx context.Context, txn *sql.Tx, topicNID int64,
) (offset int64, err error) {

	// "SELECT message_offset FROM naffka_messages WHERE topic_nid = $1" +
	// " ORDER BY message_offset DESC LIMIT 1"

	params := map[string]interface{}{
		"@x1": t.getCollectionNameMessages(),
		"@x2": topicNID,
	}

	// stmt := sqlutil.TxStmt(txn, t.selectMaxOffsetStmt)
	// err = stmt.QueryRowContext(ctx, topicNID).Scan(&offset)
	var rows []messageCosmosData
	err = cosmosdbapi.PerformQuery(ctx,
		t.DB.connection,
		t.DB.cosmosConfig.DatabaseName,
		t.DB.cosmosConfig.ContainerName,
		t.getPartitionKeyMessages(topicNID), t.selectMaxOffsetStmt, params, &rows)

	if err != nil {
		return 0, err
	}

	if len(rows) == 0 {
		return 0, nil
	}

	offset = rows[0].Message.MessageOffset
	return
}
