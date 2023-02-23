package auth

import (
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestLdapAuthenticator_Authenticate_DirectBind_AdminUser(t *testing.T) {
	authenticator := NewLdapAuthenticator(config.Ldap{
		Uri:                 "ldap://openldap:389",
		BaseDn:              "dc=example,dc=org",
		AdminBindEnabled:    false,
		UserBindDn:          "cn={username},ou=users,dc=example,dc=org",
		AdminGroupDn:        "cn=admin,ou=groups,dc=example,dc=org",
		AdminGroupFilter:    "(memberUid={username})",
		AdminGroupAttribute: "memberUid",
	})

	isAdmin, err := authenticator.Authenticate("user1", "password")

	assert.Nil(t, err)
	assert.True(t, isAdmin)
}

func TestLdapAuthenticator_Authenticate_DirectBind_RegularUser(t *testing.T) {
	authenticator := NewLdapAuthenticator(config.Ldap{
		Uri:                 "ldap://openldap:389",
		BaseDn:              "dc=example,dc=org",
		AdminBindEnabled:    false,
		UserBindDn:          "cn={username},ou=users,dc=example,dc=org",
		AdminGroupDn:        "cn=admin,ou=groups,dc=example,dc=org",
		AdminGroupFilter:    "(memberUid={username})",
		AdminGroupAttribute: "memberUid",
	})

	isAdmin, err := authenticator.Authenticate("user2", "password")

	assert.Nil(t, err)
	assert.False(t, isAdmin)
}

func TestLdapAuthenticator_Authenticate_AdminBind(t *testing.T) {
	authenticator := NewLdapAuthenticator(config.Ldap{
		Uri:                 "ldap://openldap:389",
		BaseDn:              "dc=example,dc=org",
		AdminBindEnabled:    true,
		AdminBindDn:         "cn=admin,dc=example,dc=org",
		AdminBindPassword:   "password",
		AdminGroupDn:        "cn=admin,ou=groups,dc=example,dc=org",
		AdminGroupFilter:    "(memberUid={username})",
		AdminGroupAttribute: "memberUid",
		SearchBaseDn:        "ou=users,dc=example,dc=org",
		SearchFilter:        "(&(objectclass=inetOrgPerson)(cn={username}))",
		SearchAttribute:     "cn",
	})

	isAdmin, err := authenticator.Authenticate("user1", "password")

	assert.Nil(t, err)
	assert.True(t, isAdmin)
}

func TestLdapAuthenticator_Authenticate_AdminBind_UserNotFound(t *testing.T) {
	authenticator := NewLdapAuthenticator(config.Ldap{
		Uri:                 "ldap://openldap:389",
		BaseDn:              "dc=example,dc=org",
		AdminBindEnabled:    true,
		AdminBindDn:         "cn=admin,dc=example,dc=org",
		AdminBindPassword:   "password",
		AdminGroupDn:        "cn=admin,ou=groups,dc=example,dc=org",
		AdminGroupFilter:    "(memberUid={username})",
		AdminGroupAttribute: "memberUid",
		SearchBaseDn:        "ou=users,dc=example,dc=org",
		SearchFilter:        "(&(objectclass=inetOrgPerson)(cn={username}))",
		SearchAttribute:     "cn",
	})

	_, err := authenticator.Authenticate("user_not_found", "")

	assert.NotNil(t, err)
}

func TestLdapAuthenticator_Authenticate_DirectBind_WrongPassword(t *testing.T) {
	authenticator := NewLdapAuthenticator(config.Ldap{
		Uri:              "ldap://openldap:1389",
		BaseDn:           "dc=example,dc=org",
		UserBindDn:       "cn={username},ou=users,dc=example,dc=org",
		AdminBindEnabled: false,
	})

	_, err := authenticator.Authenticate("user2", "password_wrong")

	assert.NotNil(t, err)
}
