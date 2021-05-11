package cosmosdbutil

import (
	"github.com/matrix-org/dendrite/setup/config"
	"strings"
)

func GetConnectionString(d *config.DataSource) config.DataSource {
	var connString string
	connString = string(*d)
	return config.DataSource(strings.Replace(connString, "cosmosdb:", "", 1))
}

func GetConnectionProperties(connectionString string) map[string]string {
	connectionItemsRaw := strings.Split(connectionString, ";")
	connectionItems := map[string]string{}
	for _, item := range connectionItemsRaw {
		itemSplit := strings.SplitN(item, "=", 2)
		connectionItems[itemSplit[0]] = itemSplit[1]
	}
	return connectionItems
}