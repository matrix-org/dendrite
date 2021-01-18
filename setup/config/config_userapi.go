package config

type UserAPI struct {
	Matrix *Global `yaml:"-"`

	InternalAPI InternalAPIOptions `yaml:"internal_api"`

	// The Account database stores the login details and account information
	// for local users. It is accessed by the UserAPI.
	AccountDatabase DatabaseOptions `yaml:"account_database"`
	// The Device database stores session information for the devices of logged
	// in local users. It is accessed by the UserAPI.
	DeviceDatabase DatabaseOptions `yaml:"device_database"`
}

func (c *UserAPI) Defaults() {
	c.InternalAPI.Listen = "http://localhost:7781"
	c.InternalAPI.Connect = "http://localhost:7781"
	c.AccountDatabase.Defaults()
	c.DeviceDatabase.Defaults()
	c.AccountDatabase.ConnectionString = "file:userapi_accounts.db"
	c.DeviceDatabase.ConnectionString = "file:userapi_devices.db"
}

func (c *UserAPI) Verify(configErrs *ConfigErrors, isMonolith bool) {
	checkURL(configErrs, "user_api.internal_api.listen", string(c.InternalAPI.Listen))
	checkURL(configErrs, "user_api.internal_api.connect", string(c.InternalAPI.Connect))
	checkNotEmpty(configErrs, "user_api.account_database.connection_string", string(c.AccountDatabase.ConnectionString))
	checkNotEmpty(configErrs, "user_api.device_database.connection_string", string(c.DeviceDatabase.ConnectionString))
}
