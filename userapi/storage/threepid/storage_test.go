package threepid

import (
	"context"
	"database/sql"
	"math/rand"
	"os"
	"strconv"
	"testing"

	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/dendrite/userapi/api"
	"github.com/matryer/is"
)

var testSession = api.Session{
	// Sid:          "123",
	ClientSecret: "099azAZ.=_-",
	ThreePid:     "azAZ09!#$%&'*+-/=?^_`{|}~@bar09.com",
	Token:        "fooBAR123",
	NextLink:     "https://example.com?user=foo",
	SendAttempt:  0,
	Validated:    true,
	ValidatedAt:  0,
}

var testCtx = context.Background()

func mustNewDatabaseWithTestSession(is *is.I) *Db {
	randPostfix := strconv.Itoa(rand.Int())
	dbPath := os.TempDir() + "/dendrite-" + randPostfix
	println(dbPath)
	dut, err := newSQLiteDatabase(&config.DatabaseOptions{
		ConnectionString: config.DataSource("file:" + dbPath),
	})
	is.NoErr(err)
	err = dut.InsertSession(testCtx, &testSession)
	is.NoErr(err)
	return dut
}
func TestGetSession(t *testing.T) {
	is := is.New(t)
	dut := mustNewDatabaseWithTestSession(is)
	s, err := dut.GetSession(testCtx, testSession.Sid)
	is.NoErr(err)
	is.Equal(*s, testSession)
}

func TestGetSessionByThreePidAndSecret(t *testing.T) {
	is := is.New(t)
	dut := mustNewDatabaseWithTestSession(is)
	s, err := dut.GetSessionByThreePidAndSecret(testCtx, testSession.ThreePid, testSession.ClientSecret)
	is.NoErr(err)
	is.Equal(*s, testSession)
}

func TestBumpSendAttempt(t *testing.T) {
	is := is.New(t)
	dut := mustNewDatabaseWithTestSession(is)
	nextLink := "https://foo.bar"
	err := dut.UpdateSendAttemptNextLink(testCtx, testSession.Sid, nextLink)
	is.NoErr(err)
	s, err := dut.GetSession(testCtx, testSession.Sid)
	is.NoErr(err)
	is.Equal(s.SendAttempt, 1)
	is.Equal(s.NextLink, nextLink)
}

func TestDeleteSession(t *testing.T) {
	is := is.New(t)
	dut := mustNewDatabaseWithTestSession(is)
	err := dut.DeleteSession(testCtx, testSession.Sid)
	is.NoErr(err)
	_, err = dut.GetSession(testCtx, testSession.Sid)
	is.Equal(err, sql.ErrNoRows)
}

func TestValidateSession(t *testing.T) {
	is := is.New(t)
	dut := mustNewDatabaseWithTestSession(is)
	validatedAt := 1_623_406_296
	err := dut.ValidateSession(testCtx, testSession.Sid, validatedAt)
	is.NoErr(err)
	session, err := dut.GetSession(testCtx, testSession.Sid)
	is.NoErr(err)
	is.Equal(session.Validated, true)
	is.Equal(session.ValidatedAt, validatedAt)
}
