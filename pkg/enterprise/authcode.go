// Copyright 2026 The frp Authors
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

// Package enterprise holds helpers shared by the enterprise features of frps
// and frpc.
package enterprise

import (
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"strings"
)

// authCodeAlphabet leaves out characters that are easy to confuse: 0/O, 1/I.
const authCodeAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"

const (
	authCodeGroups    = 4
	authCodeGroupSize = 4

	minAuthCodeLen = 6
	maxAuthCodeLen = 64
)

// GenerateAuthCode returns a random code formatted as XXXX-XXXX-XXXX-XXXX.
func GenerateAuthCode() (string, error) {
	var b strings.Builder
	limit := big.NewInt(int64(len(authCodeAlphabet)))
	for g := range authCodeGroups {
		if g > 0 {
			b.WriteByte('-')
		}
		for range authCodeGroupSize {
			n, err := rand.Int(rand.Reader, limit)
			if err != nil {
				return "", err
			}
			b.WriteByte(authCodeAlphabet[n.Int64()])
		}
	}
	return b.String(), nil
}

// NormalizeAuthCode trims spaces and upper-cases the code.
func NormalizeAuthCode(code string) string {
	return strings.ToUpper(strings.TrimSpace(code))
}

// ValidateAuthCode checks a normalized code: 6-64 characters of A-Z, 0-9 and '-'.
func ValidateAuthCode(code string) error {
	if code == "" {
		return errors.New("auth code is empty")
	}
	if len(code) < minAuthCodeLen || len(code) > maxAuthCodeLen {
		return fmt.Errorf("auth code must be %d-%d characters", minAuthCodeLen, maxAuthCodeLen)
	}
	for _, r := range code {
		switch {
		case r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-':
		default:
			return fmt.Errorf("auth code contains invalid character %q", r)
		}
	}
	return nil
}

// Login error kinds sent to the client in LoginResp.Error, prefixed with
// "ent:<kind>:" so that clients can recognize them.
const (
	LoginErrPending  = "pending"
	LoginErrRejected = "rejected"
	LoginErrBound    = "bound"
	LoginErrRequired = "required"
	LoginErrInvalid  = "invalid"
)

// LoginErrorPrefix is the prefix of every enterprise login error.
const LoginErrorPrefix = "ent:"

// LoginError is a login refusal that is always sent to the client verbatim.
type LoginError struct {
	Kind string
	Msg  string
}

func (e *LoginError) Error() string {
	return LoginErrorPrefix + e.Kind + ": " + e.Msg
}

// ParseLoginErrorKind returns the kind of an enterprise login error found in
// s, or "" if s does not contain one.
func ParseLoginErrorKind(s string) string {
	_, rest, ok := strings.Cut(s, LoginErrorPrefix)
	if !ok {
		return ""
	}
	kind, _, ok := strings.Cut(rest, ":")
	if !ok {
		return ""
	}
	switch kind {
	case LoginErrPending, LoginErrRejected, LoginErrBound, LoginErrRequired, LoginErrInvalid:
		return kind
	}
	return ""
}
