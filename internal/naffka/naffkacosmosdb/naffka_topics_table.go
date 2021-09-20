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

type TopicCosmos struct {
	TopicName string `json:"topic_name"`
	TopicNID  int64  `json:"topic_nid"`
}

type TopicCosmosNumber struct {
	Number int64 `json:"number"`
}

type TopicCosmosData struct {
	cosmosdbapi.CosmosDocument
	Topic TopicCosmos `json:"mx_naffka_topic"`
}

type MessageCosmos struct {
	TopicNID           int64  `json:"topic_nid"`
	MessageOffset      int64  `json:"message_offset"`
	MessageKey         []byte `json:"message_key"`
	MessageValue       []byte `json:"message_value"`
	MessageTimestampNS int64  `json:"message_timestamp_ns"`
}

type MessageCosmosData struct {
	cosmosdbapi.CosmosDocument
	Message MessageCosmos `json:"mx_naffka_message"`
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

func queryTopic(s *topicsStatements, ctx context.Context, qry string, params map[string]interface{}) ([]TopicCosmosData, error) {
	var dbCollectionName = cosmosdbapi.GetCollectionName(s.DB.databaseName, s.tableNameTopics)
	var pk = cosmosdbapi.GetPartitionKey(s.DB.cosmosConfig.ContainerName, dbCollectionName)
	var response []TopicCosmosData

	var optionsQry = cosmosdbapi.GetQueryDocumentsOptions(pk)
	var query = cosmosdbapi.GetQuery(qry, params)
	_, err := cosmosdbapi.GetClient(s.DB.connection).QueryDocuments(
		ctx,
		s.DB.cosmosConfig.DatabaseName,
		s.DB.cosmosConfig.ContainerName,
		query,
		&response,
		optionsQry)

	if err != nil {
		return nil, err
	}
	return response, nil
}

func queryTopicNumber(s *topicsStatements, ctx context.Context, qry string, params map[string]interface{}) ([]TopicCosmosNumber, error) {
	var dbCollectionName = cosmosdbapi.GetCollectionName(s.DB.databaseName, s.tableNameTopics)
	var pk = cosmosdbapi.GetPartitionKey(s.DB.cosmosConfig.ContainerName, dbCollectionName)
	var response []TopicCosmosNumber

	var optionsQry = cosmosdbapi.GetQueryDocumentsOptions(pk)
	var query = cosmosdbapi.GetQuery(qry, params)
	_, err := cosmosdbapi.GetClient(s.DB.connection).QueryDocuments(
		ctx,
		s.DB.cosmosConfig.DatabaseName,
		s.DB.cosmosConfig.ContainerName,
		query,
		&response,
		optionsQry)

	if err != nil {
		return nil, err
	}
	return response, nil
}

func queryMessage(s *topicsStatements, ctx context.Context, qry string, params map[string]interface{}) ([]MessageCosmosData, error) {
	var dbCollectionName = cosmosdbapi.GetCollectionName(s.DB.databaseName, s.tableNameMessages)
	var pk = cosmosdbapi.GetPartitionKey(s.DB.cosmosConfig.ContainerName, dbCollectionName)
	var response []MessageCosmosData

	var optionsQry = cosmosdbapi.GetQueryDocumentsOptions(pk)
	var query = cosmosdbapi.GetQuery(qry, params)
	_, err := cosmosdbapi.GetClient(s.DB.connection).QueryDocuments(
		ctx,
		s.DB.cosmosConfig.DatabaseName,
		s.DB.cosmosConfig.ContainerName,
		query,
		&response,
		optionsQry)

	if err != nil {
		return nil, err
	}
	return response, nil
}

func getTopic(s *topicsStatements, ctx context.Context, pk string, docId string) (*TopicCosmosData, error) {
	response := TopicCosmosData{}
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

	var dbCollectionName = cosmosdbapi.GetCollectionName(t.DB.databaseName, t.tableNameTopics)
	// 	topic_name TEXT UNIQUE,
	docId := fmt.Sprintf("%s", topicName)
	cosmosDocId := cosmosdbapi.GetDocumentId(t.DB.cosmosConfig.ContainerName, dbCollectionName, docId)
	pk := cosmosdbapi.GetPartitionKey(t.DB.cosmosConfig.ContainerName, dbCollectionName)

	dbData, _ := getTopic(t, ctx, pk, cosmosDocId)
	if dbData != nil {
		dbData.SetUpdateTime()
		dbData.Topic.TopicName = topicName
	} else {
		data := TopicCosmos{
			TopicNID:  topicNID,
			TopicName: topicName,
		}

		dbData = &TopicCosmosData{
			CosmosDocument: cosmosdbapi.GenerateDocument(dbCollectionName, t.DB.cosmosConfig.TenantName, pk, cosmosDocId),
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

	var dbCollectionName = cosmosdbapi.GetCollectionName(t.DB.databaseName, t.tableNameTopics)
	params := map[string]interface{}{
		"@x1": dbCollectionName,
	}

	// stmt := sqlutil.TxStmt(txn, t.selectNextTopicNIDStmt)
	// err = stmt.QueryRowContext(ctx).Scan(&topicNID)
	rows, err := queryTopicNumber(t, ctx, t.selectNextTopicNIDStmt, params)

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

	var dbCollectionName = cosmosdbapi.GetCollectionName(t.DB.databaseName, t.tableNameTopics)
	// 	topic_name TEXT UNIQUE,
	docId := fmt.Sprintf("%s", topicName)
	cosmosDocId := cosmosdbapi.GetDocumentId(t.DB.cosmosConfig.ContainerName, dbCollectionName, docId)
	pk := cosmosdbapi.GetPartitionKey(t.DB.cosmosConfig.ContainerName, dbCollectionName)

	// err = stmt.QueryRowContext(ctx, topicName).Scan(&topicNID)
	res, err := getTopic(t, ctx, pk, cosmosDocId)

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

	var dbCollectionName = cosmosdbapi.GetCollectionName(t.DB.databaseName, t.tableNameTopics)
	params := map[string]interface{}{
		"@x1": dbCollectionName,
	}

	// stmt := sqlutil.TxStmt(txn, t.selectTopicsStmt)
	// rows, err := stmt.QueryContext(ctx)
	rows, err := queryTopic(t, ctx, t.selectTopicsStmt, params)

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

	var dbCollectionName = cosmosdbapi.GetCollectionName(t.DB.databaseName, t.tableNameMessages)
	// 	UNIQUE (topic_nid, message_offset)
	docId := fmt.Sprintf("%d_%d", topicNID, messageOffset)
	cosmosDocId := cosmosdbapi.GetDocumentId(t.DB.cosmosConfig.ContainerName, dbCollectionName, docId)
	pk := cosmosdbapi.GetPartitionKey(t.DB.cosmosConfig.ContainerName, dbCollectionName)

	data := MessageCosmos{
		TopicNID:           topicNID,
		MessageOffset:      messageOffset,
		MessageKey:         topicKey,
		MessageValue:       topicValue,
		MessageTimestampNS: messageTimestampNs,
	}

	dbData := &MessageCosmosData{
		CosmosDocument: cosmosdbapi.GenerateDocument(dbCollectionName, t.DB.cosmosConfig.TenantName, pk, cosmosDocId),
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

	var dbCollectionName = cosmosdbapi.GetCollectionName(t.DB.databaseName, t.tableNameMessages)
	params := map[string]interface{}{
		"@x1": dbCollectionName,
		"@x2": topicNID,
		"@x3": startOffset,
		"@x4": endOffset,
	}

	// stmt := sqlutil.TxStmt(txn, t.selectMessagesStmt)
	// rows, err := stmt.QueryContext(ctx, topicNID, startOffset, endOffset)
	rows, err := queryMessage(t, ctx, t.selectMessagesStmt, params)

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

	var dbCollectionName = cosmosdbapi.GetCollectionName(t.DB.databaseName, t.tableNameMessages)
	params := map[string]interface{}{
		"@x1": dbCollectionName,
		"@x2": topicNID,
	}

	// stmt := sqlutil.TxStmt(txn, t.selectMaxOffsetStmt)
	// err = stmt.QueryRowContext(ctx, topicNID).Scan(&offset)
	rows, err := queryMessage(t, ctx, t.selectMaxOffsetStmt, params)

	if err != nil {
		return 0, err
	}

	if len(rows) == 0 {
		return 0, nil
	}

	offset = rows[0].Message.MessageOffset
	return
}
