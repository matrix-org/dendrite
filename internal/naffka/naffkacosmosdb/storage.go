package naffkacosmosdb

import (
	"github.com/matrix-org/dendrite/setup/config"

	"github.com/matrix-org/dendrite/internal/cosmosdbapi"
	"github.com/matrix-org/dendrite/internal/cosmosdbutil"
	"github.com/matrix-org/gomatrixserverlib"

	"github.com/matrix-org/naffka/storage/shared"
)

// Database stores information needed by the federation sender
type Database struct {
	shared.Database
	writer       cosmosdbutil.Writer
	connection   cosmosdbapi.CosmosConnection
	databaseName string
	cosmosConfig cosmosdbapi.CosmosConfig
	serverName   gomatrixserverlib.ServerName
}

// NewDatabase opens a new database
func NewDatabase(dsn string) (*Database, error) {
	datasource := config.DatabaseOptions{}
	datasource.ConnectionString = config.DataSource(dsn)
	conn := cosmosdbutil.GetCosmosConnection(&datasource.ConnectionString)
	configConsmos := cosmosdbutil.GetCosmosConfig(&datasource.ConnectionString)
	var d Database
	// var err error
	// if d.db, err = sql.Open("sqlite3", dsn); err != nil {
	// 	return nil, err
	// }
	d.connection = conn
	d.cosmosConfig = configConsmos
	d.writer = cosmosdbutil.NewExclusiveWriterFake()
	topics, err := NewCosmosDBTopicsTable(&d)
	if err != nil {
		return nil, err
	}
	d.Database = shared.Database{
		DB:          nil,
		Writer:      d.writer,
		TopicsTable: topics,
	}
	d.Database.CreateCache()
	d.databaseName = "naffka"
	return &d, nil
}
