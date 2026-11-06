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

package enterprise

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGenerateAuthCode(t *testing.T) {
	seen := map[string]bool{}
	for range 100 {
		code, err := GenerateAuthCode()
		require.NoError(t, err)
		require.Len(t, code, 19)
		require.NoError(t, ValidateAuthCode(code))
		require.NotContains(t, code, "0")
		require.NotContains(t, code, "O")
		require.False(t, seen[code])
		seen[code] = true
	}
}

func TestValidateAuthCode(t *testing.T) {
	require.Error(t, ValidateAuthCode(""))
	require.Error(t, ValidateAuthCode("ABC"))
	require.Error(t, ValidateAuthCode("abcdef"))
	require.Error(t, ValidateAuthCode("ABC DEF"))
	require.NoError(t, ValidateAuthCode(NormalizeAuthCode("  abcd-ef12 ")))
}

func TestParseLoginErrorKind(t *testing.T) {
	err := fmt.Errorf("register control error: %w", &LoginError{Kind: LoginErrPending, Msg: "waiting"})
	require.Equal(t, LoginErrPending, ParseLoginErrorKind(err.Error()))
	var le *LoginError
	require.True(t, errors.As(err, &le))
	require.Equal(t, "", ParseLoginErrorKind("some other error"))
	require.Equal(t, "", ParseLoginErrorKind("ent:unknown: x"))
}
