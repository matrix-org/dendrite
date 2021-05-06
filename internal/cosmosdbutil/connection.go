package cosmosdbutil

import (
	"github.com/matrix-org/dendrite/setup/config"
	"strings"
)

func GetConnectionString(d *config.DataSource) config.DataSource {
	var connString string
	connString = string(*d)
	return config.DataSource(strings.Replace(connString, "cosmosdb:", "file:", 1))
}