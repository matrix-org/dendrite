package config

type UserAPI struct {
	Matrix *Global `yaml:"-"`

	Listen Address `yaml:"listen"`
	Bind   Address `yaml:"bind"`

	// The Account database stores the login details and account information
	// for local users. It is accessed by the UserAPI.
	AccountDatabase DatabaseOptions `yaml:"account_database"`
	// The Device database stores session information for the devices of logged
	// in local users. It is accessed by the UserAPI.
	DeviceDatabase DatabaseOptions `yaml:"device_database"`
}

func (c *UserAPI) Defaults() {
	c.Listen = "localhost:7781"
	c.Bind = "localhost:7781"
	c.AccountDatabase.Defaults()
	c.DeviceDatabase.Defaults()
	c.AccountDatabase.ConnectionString = "file:userapi_accounts.db"
	c.DeviceDatabase.ConnectionString = "file:userapi_devices.db"
}

func (c *UserAPI) Verify(configErrs *ConfigErrors, isMonolith bool) {
	checkNotEmpty(configErrs, "user_api.listen", string(c.Listen))
	checkNotEmpty(configErrs, "user_api.bind", string(c.Bind))
	checkNotEmpty(configErrs, "user_api.account_database.connection_string", string(c.AccountDatabase.ConnectionString))
	checkNotEmpty(configErrs, "user_api.device_database.connection_string", string(c.DeviceDatabase.ConnectionString))
}
