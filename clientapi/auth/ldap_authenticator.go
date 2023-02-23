package auth

import (
	"github.com/go-ldap/ldap/v3"
	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/util"
	"net/http"
	"strings"
)

type LdapAuthenticator struct {
	config config.Ldap
}

func NewLdapAuthenticator(config config.Ldap) *LdapAuthenticator {
	return &LdapAuthenticator{
		config: config,
	}
}

func (l *LdapAuthenticator) Authenticate(username, password string) (bool, *util.JSONResponse) {
	var conn *ldap.Conn
	conn, err := ldap.DialURL(l.config.Uri)
	if err != nil {
		return false, &util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: jsonerror.Unknown("unable to connect to ldap: " + err.Error()),
		}
	}
	defer conn.Close()

	if l.config.AdminBindEnabled {
		err = conn.Bind(l.config.AdminBindDn, l.config.AdminBindPassword)
		if err != nil {
			return false, &util.JSONResponse{
				Code: http.StatusInternalServerError,
				JSON: jsonerror.Unknown("unable to bind to ldap: " + err.Error()),
			}
		}
		filter := strings.ReplaceAll(l.config.SearchFilter, "{username}", username)
		searchRequest := ldap.NewSearchRequest(
			l.config.SearchBaseDn, ldap.ScopeWholeSubtree, ldap.NeverDerefAliases,
			0, 0, false, filter, []string{l.config.SearchAttribute}, nil,
		)
		result, err := conn.Search(searchRequest)
		if err != nil {
			return false, &util.JSONResponse{
				Code: http.StatusInternalServerError,
				JSON: jsonerror.Unknown("unable to bind to search ldap: " + err.Error()),
			}
		}
		if len(result.Entries) > 1 {
			return false, &util.JSONResponse{
				Code: http.StatusUnauthorized,
				JSON: jsonerror.BadJSON("'user' must be duplicated."),
			}
		}
		if len(result.Entries) < 1 {
			return false, &util.JSONResponse{
				Code: http.StatusUnauthorized,
				JSON: jsonerror.BadJSON("'user' not found."),
			}
		}

		userDN := result.Entries[0].DN
		err = conn.Bind(userDN, password)
		if err != nil {
			return false, &util.JSONResponse{
				Code: http.StatusUnauthorized,
				JSON: jsonerror.InvalidUsername(err.Error()),
			}
		}
	} else {
		bindDn := strings.ReplaceAll(l.config.UserBindDn, "{username}", username)
		err = conn.Bind(bindDn, password)
		if err != nil {
			return false, &util.JSONResponse{
				Code: http.StatusUnauthorized,
				JSON: jsonerror.InvalidUsername(err.Error()),
			}
		}
	}

	isAdmin, err := l.isLdapAdmin(conn, username)
	if err != nil {
		return false, &util.JSONResponse{
			Code: http.StatusUnauthorized,
			JSON: jsonerror.InvalidUsername(err.Error()),
		}
	}
	return isAdmin, nil
}

func (l *LdapAuthenticator) isLdapAdmin(conn *ldap.Conn, username string) (bool, error) {
	searchRequest := ldap.NewSearchRequest(
		l.config.AdminGroupDn,
		ldap.ScopeWholeSubtree, ldap.DerefAlways, 0, 0, false,
		strings.ReplaceAll(l.config.AdminGroupFilter, "{username}", username),
		[]string{l.config.AdminGroupAttribute},
		nil)

	sr, err := conn.Search(searchRequest)
	if err != nil {
		return false, err
	}

	if len(sr.Entries) < 1 {
		return false, nil
	}
	return true, nil
}
