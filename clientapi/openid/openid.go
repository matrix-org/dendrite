package openid

// TokenRequest represents the request defined at https://matrix.org/docs/spec/client_server/r0.6.1#id603
type TokenRequest struct {
	UserID       string `json:"userId"`
	RelyingParty string `json:"relyingParty"`
}
