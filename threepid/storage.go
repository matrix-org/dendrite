package threepid

type storage interface {
	InsertSession(*Session) error
	GetSession(sid string) (*Session, error)
	GetSessionByThreePidAndSecret(threePid, ClientSecret string) (*Session, error)
	RemoveSession(sid string) error
}
