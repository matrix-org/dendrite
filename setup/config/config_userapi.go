package config

import "golang.org/x/crypto/bcrypt"

type UserAPI struct {
	Matrix *Global `yaml:"-"`

	InternalAPI InternalAPIOptions `yaml:"internal_api"`

	// The cost when hashing passwords.
	BCryptCost int `yaml:"bcrypt_cost"`

	// The length of time an OpenID token is condidered valid in milliseconds
	OpenIDTokenLifetimeMS int64 `yaml:"openid_token_lifetime_ms"`

	// The Account database stores the login details and account information
	// for local users. It is accessed by the UserAPI.
	AccountDatabase DatabaseOptions `yaml:"account_database"`
	// The Device database stores session information for the devices of logged
	// in local users. It is accessed by the UserAPI.
	DeviceDatabase DatabaseOptions `yaml:"device_database"`
}

const DefaultOpenIDTokenLifetimeMS = 3600000 // 60 minutes

func (c *UserAPI) Defaults() {
	c.InternalAPI.Listen = "http://localhost:7781"
	c.InternalAPI.Connect = "http://localhost:7781"
	c.AccountDatabase.Defaults(10)
	c.DeviceDatabase.Defaults(10)
	c.AccountDatabase.ConnectionString = "file:userapi_accounts.db"
	c.DeviceDatabase.ConnectionString = "file:userapi_devices.db"
	c.BCryptCost = bcrypt.DefaultCost
	c.OpenIDTokenLifetimeMS = DefaultOpenIDTokenLifetimeMS
}

func (c *UserAPI) Verify(configErrs *ConfigErrors, isMonolith bool) {
	checkURL(configErrs, "user_api.internal_api.listen", string(c.InternalAPI.Listen))
	checkURL(configErrs, "user_api.internal_api.connect", string(c.InternalAPI.Connect))
	checkNotEmpty(configErrs, "user_api.account_database.connection_string", string(c.AccountDatabase.ConnectionString))
	checkNotEmpty(configErrs, "user_api.device_database.connection_string", string(c.DeviceDatabase.ConnectionString))
	checkPositive(configErrs, "user_api.openid_token_lifetime_ms", c.OpenIDTokenLifetimeMS)
}
