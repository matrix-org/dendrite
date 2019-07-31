package common

import "github.com/matrix-org/gomatrixserverlib"

func ValidateRedaction(
	redacted, redaction *gomatrixserverlib.Event,
) (badEvents, needPowerLevelCheck bool, err error) {
	// Don't allow redaction of events in different rooms
	if redaction.RoomID() != redacted.RoomID() {
		return true, false, nil
	}

	// Don't allow an event to redact itself
	if redaction.Redacts() == redaction.EventID() {
		return true, false, nil
	}

	// Don't allow two events to redact each other
	if redacted.Redacts() == redaction.EventID() {
		return true, false, nil
	}

	var expectedDomain, redactorDomain gomatrixserverlib.ServerName
	if _, expectedDomain, err = gomatrixserverlib.SplitID(
		'@', redacted.Sender(),
	); err != nil {
		return false, false, err
	}
	if _, redactorDomain, err = gomatrixserverlib.SplitID(
		'@', redaction.Sender(),
	); err != nil {
		return false, false, err
	}

	if expectedDomain != redactorDomain {
		return false, true, err
	}

	return false, false, nil
}
