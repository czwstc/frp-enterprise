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
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestFetchPublicIP(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/plain":
			_, _ = w.Write([]byte("203.0.113.9\n"))
		case "/json":
			_, _ = w.Write([]byte(`{"ip":"2001:db8::1"}`))
		case "/ipapi":
			_, _ = w.Write([]byte(`{"status":"success","query":"198.51.100.7"}`))
		case "/bad":
			_, _ = w.Write([]byte("<html>nope</html>"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	ctx := context.Background()
	ip, err := fetchPublicIP(ctx, srv.URL+"/plain")
	require.NoError(t, err)
	require.Equal(t, "203.0.113.9", ip)
	ip, err = fetchPublicIP(ctx, srv.URL+"/json")
	require.NoError(t, err)
	require.Equal(t, "2001:db8::1", ip)
	ip, err = fetchPublicIP(ctx, srv.URL+"/ipapi")
	require.NoError(t, err)
	require.Equal(t, "198.51.100.7", ip)
	_, err = fetchPublicIP(ctx, srv.URL+"/bad")
	require.Error(t, err)
	_, err = fetchPublicIP(ctx, srv.URL+"/missing")
	require.Error(t, err)
}

func TestPublicIPSource(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("203.0.113.9"))
	}))
	defer srv.Close()
	store := NewSettingsStore(filepath.Join(t.TempDir(), "c.enc"), func() string { return "m" })
	m, err := NewManager(store, nil)
	require.NoError(t, err)

	require.Equal(t, PublicIPServer, m.Status().PublicIPSource.Mode)
	require.Error(t, m.SetPublicIPSource(PublicIPAPI, "ftp://x"))
	require.Error(t, m.SetPublicIPSource("nope", ""))

	require.NoError(t, m.SetPublicIPSource(PublicIPAPI, srv.URL))
	require.Eventually(t, func() bool { return m.Status().PublicIP == "203.0.113.9" }, 5*time.Second, 20*time.Millisecond)
	require.Equal(t, srv.URL, m.Settings().PublicIP.URL)

	require.NoError(t, m.SetPublicIPSource(PublicIPAPI, ""))
	require.Equal(t, DefaultPublicIPAPI, m.Settings().PublicIP.URL)

	require.NoError(t, m.SetPublicIPSource(PublicIPOff, ""))
	require.Empty(t, m.Status().PublicIP)
}
