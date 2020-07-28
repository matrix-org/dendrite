package config

type UserAPI struct {
	Matrix *Global `yaml:"-"`

	Listen Address `yaml:"listen"`
	Bind   Address `yaml:"bind"`

	// The Account database stores the login details and account information
	// for local users. It is accessed by the UserAPI.
	AccountDatabase        DataSource      `yaml:"account_database"`
	AccountDatabaseOptions DatabaseOptions `yaml:"account_database_options"`
	// The Device database stores session information for the devices of logged
	// in local users. It is accessed by the UserAPI.
	DeviceDatabase        DataSource      `yaml:"device_database"`
	DeviceDatabaseOptions DatabaseOptions `yaml:"device_database_options"`
}

func (c *UserAPI) Defaults() {
	c.Listen = "localhost:7781"
	c.Bind = "localhost:7781"
	c.AccountDatabase = "file:userapi_accounts.db"
	c.DeviceDatabase = "file:userapi_devices.db"
	c.AccountDatabaseOptions.Defaults()
	c.DeviceDatabaseOptions.Defaults()
}

func (c *UserAPI) Verify(configErrs *configErrors) {
	checkNotEmpty(configErrs, "user_api.listen", string(c.Listen))
	checkNotEmpty(configErrs, "user_api.bind", string(c.Bind))
	checkNotEmpty(configErrs, "user_api.account_database", string(c.AccountDatabase))
	checkNotEmpty(configErrs, "user_api.device_database", string(c.DeviceDatabase))
}
