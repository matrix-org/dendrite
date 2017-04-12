package config

// Sync contains the config information necessary to spin up a sync-server process.
type Sync struct {
	// The topic for events which are written by the room server output log.
	RoomserverOutputTopic string `yaml:"roomserver_topic"`
	// A list of URIs to consume events from. These kafka logs should be produced by a Room Server.
	KafkaConsumerURIs []string `yaml:"consumer_uris"`
	// The postgres connection config for connecting to the database e.g a postgres:// URI
	DataSource string `yaml:"database"`
}
