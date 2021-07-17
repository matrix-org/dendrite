package threepid

import "context"

type API interface {
	CreateSession(context.Context, *CreateSessionRequest, *Session) error
	ValidateSession(context.Context, *ValidateSessionRequest, struct{}) error
	GetThreePidForSession(context.Context, *SessionOwnership, *GetThreePidForSessionResponse) error
	DeleteSession(context.Context, *SessionOwnership, struct{}) error
	IsSessionValidated(context.Context, *SessionOwnership, *IsSessionValidatedResponse) error
}

type CreateSessionRequest struct {
	ClientSecret, NextLink, ThreePid string
}

type ValidateSessionRequest struct {
	SessionOwnership
	Token string
}

type GetThreePidForSessionResponse struct {
	ThreePid string
}

type SessionOwnership struct {
	Sid, ClientSecret string
}

type Session struct {
	Sid, ClientSecret, ThreePid, Token, NextLink string
	SendAttempt                                  int
}

type IsSessionValidatedResponse struct {
	Validated   bool
	ValidatedAt int
}
