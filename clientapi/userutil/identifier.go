// Copyright 2022 The Matrix.org Foundation C.I.C.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package userutil

import (
	"bytes"
	"encoding/json"
	"errors"
)

// An Identifier identifies a user. There are many kinds, and this is
// the common interface for them.
//
// If you need to handle an identifier as JSON, use the AnyIdentifier wrapper.
// Passing around identifiers in code, the raw Identifier is enough.
//
// See https://matrix.org/docs/spec/client_server/r0.6.1#identifier-types
type Identifier interface {
	// IdentifierType returns the identifier type, like "m.id.user".
	IdentifierType() IdentifierType

	// String returns a debug-output string representation. The format
	// is not specified.
	String() string
}

// A UserIdentifier contains an MXID. It may be only the local part.
type UserIdentifier struct {
	UserID string `json:"user"`
}

func (i *UserIdentifier) IdentifierType() IdentifierType { return IdentifierUser }
func (i *UserIdentifier) String() string                 { return i.UserID }

// A ThirdPartyIdentifier references an identifier in another system.
type ThirdPartyIdentifier struct {
	// Medium is normally MediumEmail.
	Medium Medium `json:"medium"`

	// Address is the medium-specific identifier.
	Address string `json:"address"`
}

func (i *ThirdPartyIdentifier) IdentifierType() IdentifierType { return IdentifierThirdParty }
func (i *ThirdPartyIdentifier) String() string                 { return string(i.Medium) + ":" + i.Address }

// A PhoneIdentifier references a phone number.
type PhoneIdentifier struct {
	// Country is a ISO-3166-1 alpha-2 country code.
	Country string `json:"country"`

	// PhoneNumber is a country-specific phone number, as it would be dialled from.
	PhoneNumber string `json:"phone"`
}

func (i *PhoneIdentifier) IdentifierType() IdentifierType { return IdentifierPhone }
func (i *PhoneIdentifier) String() string                 { return i.Country + ":" + i.PhoneNumber }

// UnknownIdentifier is the catch-all for identifiers this code doesn't know about.
// It simply stores raw JSON.
type UnknownIdentifier struct {
	json.RawMessage
	Type IdentifierType
}

func (i *UnknownIdentifier) IdentifierType() IdentifierType { return i.Type }
func (i *UnknownIdentifier) String() string                 { return "unknown/" + string(i.Type) }

// AnyIdentifier is a wrapper that allows marshalling and unmarshalling the various
// types of identifiers to/from JSON. Always use this in data types that will be
// used in JSON manipulation.
type AnyIdentifier struct {
	Identifier
}

func (i AnyIdentifier) MarshalJSON() ([]byte, error) {
	v := struct {
		*UserIdentifier
		*ThirdPartyIdentifier
		*PhoneIdentifier
		Type IdentifierType `json:"type"`
	}{
		Type: i.Identifier.IdentifierType(),
	}
	switch iid := i.Identifier.(type) {
	case *UserIdentifier:
		v.UserIdentifier = iid
	case *ThirdPartyIdentifier:
		v.ThirdPartyIdentifier = iid
	case *PhoneIdentifier:
		v.PhoneIdentifier = iid
	case *UnknownIdentifier:
		return iid.RawMessage, nil
	}
	return json.Marshal(v)
}

func (i *AnyIdentifier) UnmarshalJSON(bs []byte) error {
	var hdr struct {
		Type IdentifierType `json:"type"`
	}
	if err := json.Unmarshal(bs, &hdr); err != nil {
		return err
	}
	switch hdr.Type {
	case IdentifierUser:
		var ui UserIdentifier
		i.Identifier = &ui
		return json.Unmarshal(bs, &ui)
	case IdentifierThirdParty:
		var tpi ThirdPartyIdentifier
		i.Identifier = &tpi
		return json.Unmarshal(bs, &tpi)
	case IdentifierPhone:
		var pi PhoneIdentifier
		i.Identifier = &pi
		return json.Unmarshal(bs, &pi)
	case "":
		return errors.New("missing identifier type")
	default:
		i.Identifier = &UnknownIdentifier{RawMessage: json.RawMessage(bytes.TrimSpace(bs)), Type: hdr.Type}
		return nil
	}
}

// IdentifierType describes the type of identifier.
type IdentifierType string

const (
	IdentifierUser       IdentifierType = "m.id.user"
	IdentifierThirdParty IdentifierType = "m.id.thirdparty"
	IdentifierPhone      IdentifierType = "m.id.phone"
)

// Medium describes the interpretation of a third-party identifier.
type Medium string

const (
	// MediumEmail signifies that the address is an email address.
	MediumEmail Medium = "email"
)
