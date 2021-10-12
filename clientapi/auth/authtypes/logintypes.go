package authtypes

// LoginType are specified by http://matrix.org/docs/spec/client_server/r0.2.0.html#login-types
type LoginType string

// The relevant login types implemented in Dendrite
const (
	LoginTypePassword           = "m.login.password"
	LoginTypeDummy              = "m.login.dummy"
	LoginTypeChallengeResponse  = "m.login.challenge_response"
	LoginTypeSharedSecret       = "org.matrix.login.shared_secret"
	LoginTypeRecaptcha          = "m.login.recaptcha"
	LoginTypeApplicationService = "m.login.application_service"
)
