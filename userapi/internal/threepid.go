package internal

import (
	"context"
	"database/sql"
	"net/url"
	"strconv"
	"time"

	"github.com/matrix-org/dendrite/internal"
	"github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/dendrite/userapi/mail"
	"github.com/matrix-org/dendrite/userapi/storage/threepid"
)

const (
	tokenByteLength = 48
)

func (a *UserInternalAPI) CreateSession(ctx context.Context, req *api.CreateSessionRequest, res *api.CreateSessionResponse) error {
	s, err := a.ThreePidDB.GetSessionByThreePidAndSecret(ctx, req.ThreePid, req.ClientSecret)
	if err != nil {
		if err == sql.ErrNoRows {
			var token string
			token, err = internal.GenerateBlob(tokenByteLength)
			if err != nil {
				return err
			}
			s = &api.Session{
				ClientSecret: req.ClientSecret,
				ThreePid:     req.ThreePid,
				SendAttempt:  req.SendAttempt,
				Token:        token,
				NextLink:     req.NextLink,
			}
			s.Sid, err = a.ThreePidDB.InsertSession(ctx, req.ClientSecret, req.ThreePid, token, req.NextLink, 0, false, req.SendAttempt)
			if err != nil {
				return err
			}
		} else {
			return err
		}
	} else {
		if req.SendAttempt > s.SendAttempt {
			err = a.ThreePidDB.UpdateSendAttemptNextLink(ctx, s.Sid, req.NextLink)
			if err != nil {
				return err
			}
		} else {
			res.Sid = s.Sid
			return nil
		}
	}
	res.Sid = s.Sid
	query := url.Values{
		"sid":           []string{strconv.Itoa(int(s.Sid))},
		"client_secret": []string{s.ClientSecret},
		"token":         []string{s.Token},
	}
	link := url.URL{
		Scheme:   "https",
		Host:     string(a.ServerName),
		Path:     req.SessionType.SubmitPath(),
		RawQuery: query.Encode(),
	}
	// TODO - if we fail sending email, send_attempt for next requests must be bumped,
	// otherwise we will just return nil from this function and not sent email
	return a.Mail.Send(&mail.Mail{
		To:    s.ThreePid,
		Link:  link.String(),
		Token: s.Token,
		Extra: req.Extra,
	}, req.SessionType)
}

func (a *UserInternalAPI) ValidateSession(ctx context.Context, req *api.ValidateSessionRequest, res *api.ValidateSessionResponse) error {
	s, err := getSessionByOwnership(ctx, &req.SessionOwnership, a.ThreePidDB)
	if err != nil {
		return err
	}
	if s.Token != req.Token {
		return api.ErrBadSession
	}
	err = a.ThreePidDB.ValidateSession(ctx, s.Sid, time.Now().Unix())
	if err != nil {
		return err
	}
	res.NextLink = s.NextLink
	return nil
}

func (a *UserInternalAPI) GetThreePidForSession(ctx context.Context, req *api.SessionOwnership, res *api.GetThreePidForSessionResponse) error {
	s, err := getSessionByOwnership(ctx, req, a.ThreePidDB)
	if err != nil {
		return err
	}
	res.ThreePid = s.ThreePid
	return nil
}

func (a *UserInternalAPI) DeleteSession(ctx context.Context, req *api.SessionOwnership, res struct{}) error {
	s, err := getSessionByOwnership(ctx, req, a.ThreePidDB)
	if err != nil {
		return err
	}
	return a.ThreePidDB.DeleteSession(ctx, s.Sid)
}

func (a *UserInternalAPI) IsSessionValidated(ctx context.Context, req *api.SessionOwnership, res *api.IsSessionValidatedResponse) error {
	s, err := getSessionByOwnership(ctx, req, a.ThreePidDB)
	if err != nil {
		return err
	}
	res.Validated = s.Validated
	res.ValidatedAt = int(s.ValidatedAt)
	res.ThreePid = s.ThreePid
	return nil
}

func getSessionByOwnership(ctx context.Context, so *api.SessionOwnership, d threepid.Database) (*api.Session, error) {
	s, err := d.GetSession(ctx, so.Sid)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, api.ErrBadSession
		}
		return nil, err
	}
	if s.ClientSecret != so.ClientSecret {
		return nil, api.ErrBadSession
	}
	return s, err
}
