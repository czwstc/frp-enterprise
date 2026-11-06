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

package adminauth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1" //nolint:gosec // RFC 6238 TOTP uses HMAC-SHA1, as authenticator apps do.
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base32"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"math/big"
	"net/url"
	"strings"
	"time"
)

const (
	totpPeriod = 30
	totpDigits = 6
	// totpSkew accepts codes one period before and after now.
	totpSkew = 1
)

var b32 = base32.StdEncoding.WithPadding(base32.NoPadding)

// newTOTPSecret returns a random 160-bit secret in base32.
func newTOTPSecret() (string, error) {
	b := make([]byte, 20)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return b32.EncodeToString(b), nil
}

func totpAt(secret string, step int64) (string, error) {
	key, err := b32.DecodeString(strings.ToUpper(strings.TrimSpace(secret)))
	if err != nil {
		return "", err
	}
	var msg [8]byte
	binary.BigEndian.PutUint64(msg[:], uint64(step))
	mac := hmac.New(sha1.New, key)
	mac.Write(msg[:])
	sum := mac.Sum(nil)
	off := sum[len(sum)-1] & 0x0f
	v := binary.BigEndian.Uint32(sum[off:off+4]) & 0x7fffffff
	return fmt.Sprintf("%06d", v%1000000), nil
}

func totpStep(t time.Time) int64 { return t.Unix() / totpPeriod }

// verifyTOTP checks code against the secret around time t. It returns the
// step that matched, which must be greater than lastStep so that a code is
// used once.
func verifyTOTP(secret, code string, t time.Time, lastStep int64) (int64, bool) {
	code = strings.ReplaceAll(strings.TrimSpace(code), " ", "")
	if len(code) != totpDigits {
		return 0, false
	}
	now := totpStep(t)
	for d := -totpSkew; d <= totpSkew; d++ {
		step := now + int64(d)
		if step <= lastStep {
			continue
		}
		want, err := totpAt(secret, step)
		if err != nil {
			return 0, false
		}
		if subtle.ConstantTimeCompare([]byte(want), []byte(code)) == 1 {
			return step, true
		}
	}
	return 0, false
}

// otpauthURL is what authenticator apps scan.
func otpauthURL(issuer, account, secret string) string {
	label := url.PathEscape(issuer + ":" + account)
	v := url.Values{}
	v.Set("secret", secret)
	v.Set("issuer", issuer)
	v.Set("algorithm", "SHA1")
	v.Set("digits", fmt.Sprint(totpDigits))
	v.Set("period", fmt.Sprint(totpPeriod))
	return "otpauth://totp/" + label + "?" + v.Encode()
}

const recoveryAlphabet = "abcdefghjkmnpqrstuvwxyz23456789"

// newRecoveryCodes returns n codes formatted as xxxxx-xxxxx.
func newRecoveryCodes(n int) ([]string, error) {
	out := make([]string, 0, n)
	limit := big.NewInt(int64(len(recoveryAlphabet)))
	for range n {
		var b strings.Builder
		for i := range 10 {
			if i == 5 {
				b.WriteByte('-')
			}
			v, err := rand.Int(rand.Reader, limit)
			if err != nil {
				return nil, err
			}
			b.WriteByte(recoveryAlphabet[v.Int64()])
		}
		out = append(out, b.String())
	}
	return out, nil
}

func normalizeRecoveryCode(code string) string {
	code = strings.ToLower(strings.TrimSpace(code))
	code = strings.ReplaceAll(code, " ", "")
	if len(code) == 10 && !strings.Contains(code, "-") {
		code = code[:5] + "-" + code[5:]
	}
	return code
}

func hashRecoveryCode(code string) string {
	sum := sha256.Sum256([]byte("frp-recovery:" + normalizeRecoveryCode(code)))
	return hex.EncodeToString(sum[:])
}
