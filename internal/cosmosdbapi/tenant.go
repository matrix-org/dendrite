package cosmosdbapi

type Tenant struct {
	DatabaseName string
	TenantName   string
}

//TODO: Move into Config or the JWT
func DefaultConfig() Tenant {
	return Tenant{
		DatabaseName: "safezone_local",
		TenantName:   "criticalarc.com",
	}
}
