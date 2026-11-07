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

package branding

import (
	"encoding/base64"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDefaultsAreValid(t *testing.T) {
	d := Defaults()
	require.NoError(t, d.Normalize())
}

func TestNormalizeRejectsBadInput(t *testing.T) {
	cases := map[string]func(s *Settings){
		"empty name":     func(s *Settings) { s.SiteName = " " },
		"long name":      func(s *Settings) { s.SiteName = strings.Repeat("名", 41) },
		"markup":         func(s *Settings) { s.Subtitle = "<b>x</b>" },
		"accent":         func(s *Settings) { s.Accent = "blue" },
		"theme":          func(s *Settings) { s.Theme = "pink" },
		"link scheme":    func(s *Settings) { s.TopLink.URL = "javascript:alert(1)" },
		"link no label":  func(s *Settings) { s.TopLink.Text = ""; s.TopLink.Image = "" },
		"logo not image": func(s *Settings) { s.Logo = "data:text/html;base64,PGgxPg==" },
		"logo too large": func(s *Settings) {
			s.Logo = "data:image/png;base64," + base64.StdEncoding.EncodeToString(make([]byte, MaxImageBytes+1))
		},
		"bad link mode":   func(s *Settings) { s.TopLink.Mode = "x" },
		"bad logo mode":   func(s *Settings) { s.LogoMode = "x" },
		"logo bad base64": func(s *Settings) { s.Logo = "data:image/png;base64,@@@" },
	}
	for name, mutate := range cases {
		s := Defaults()
		mutate(&s)
		require.Error(t, s.Normalize(), name)
	}
	s := Defaults()
	s.TopLink.Mode = LinkNone
	s.TopLink.URL = ""
	require.NoError(t, s.Normalize())
	s.Accent = "#ABCDEF"
	s.Logo = "data:image/png;base64," + base64.StdEncoding.EncodeToString([]byte("png"))
	require.NoError(t, s.Normalize())
	require.Equal(t, "#abcdef", s.Accent)
}

func TestStorePersistsAndNotifies(t *testing.T) {
	path := filepath.Join(t.TempDir(), "b", "branding.json")
	s, err := Open(path)
	require.NoError(t, err)
	require.Equal(t, "HDIT FRPE", s.Get().SiteName)

	var got Settings
	s.OnChange(func(b Settings) { got = b })
	next := s.Get()
	next.SiteName = "汇鼎内网穿透"
	next.Accent = "#0e8f96"
	saved, err := s.Update(next, "zchen")
	require.NoError(t, err)
	require.Equal(t, int64(1), saved.Version)
	require.Equal(t, "zchen", saved.UpdatedBy)
	require.Equal(t, "汇鼎内网穿透", got.SiteName)

	bad := s.Get()
	bad.Accent = "nope"
	_, err = s.Update(bad, "zchen")
	require.Error(t, err)

	s2, err := Open(path)
	require.NoError(t, err)
	require.Equal(t, "汇鼎内网穿透", s2.Get().SiteName)
	require.Equal(t, int64(1), s2.Get().Version)
}
