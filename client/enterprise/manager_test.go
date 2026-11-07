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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	v1 "github.com/fatedier/frp/pkg/config/v1"
	"github.com/fatedier/frp/server"
)

func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

type testServer struct {
	bindPort int
	webPort  int
}

func startServer(t *testing.T) *testServer {
	t.Helper()
	ts := &testServer{bindPort: freePort(t), webPort: freePort(t)}
	cfg := &v1.ServerConfig{
		BindAddr:  "127.0.0.1",
		BindPort:  ts.bindPort,
		Auth:      v1.AuthServerConfig{Method: v1.AuthMethodToken, Token: "s3cret"},
		WebServer: v1.WebServerConfig{Addr: "127.0.0.1", Port: ts.webPort, User: "admin", Password: "admin"},
		Enterprise: v1.ServerEnterpriseConfig{
			Enabled:      true,
			StorePath:    filepath.Join(t.TempDir(), "ent.json"),
			BrandingPath: filepath.Join(t.TempDir(), "branding.json"),
		},
	}
	require.NoError(t, cfg.Complete())
	svr, err := server.NewService(cfg)
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	go svr.Run(ctx)
	t.Cleanup(func() {
		cancel()
		_ = svr.Close()
	})
	require.Eventually(t, func() bool {
		c, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", ts.bindPort))
		if err == nil {
			c.Close()
		}
		return err == nil
	}, 5*time.Second, 50*time.Millisecond)
	return ts
}

func (ts *testServer) api(t *testing.T, method, path string, body any) json.RawMessage {
	t.Helper()
	var r io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		r = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, fmt.Sprintf("http://127.0.0.1:%d%s", ts.webPort, path), r)
	require.NoError(t, err)
	req.SetBasicAuth("admin", "admin")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	var env struct {
		Code int             `json:"code"`
		Msg  string          `json:"msg"`
		Data json.RawMessage `json:"data"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&env))
	require.Equal(t, 200, env.Code, env.Msg)
	return env.Data
}

func (ts *testServer) generateCode(t *testing.T) string {
	var c struct {
		Code string `json:"code"`
	}
	require.NoError(t, json.Unmarshal(ts.api(t, "POST", "/api/enterprise/codes", map[string]string{"note": "test"}), &c))
	return c.Code
}

func newTestManager(t *testing.T, machine string) *Manager {
	t.Helper()
	store := NewSettingsStore(filepath.Join(t.TempDir(), "config.enc"), func() string { return machine })
	m, err := NewManager(store, nil)
	require.NoError(t, err)
	t.Cleanup(m.Stop)
	return m
}

func waitState(t *testing.T, m *Manager, want State, timeout time.Duration) {
	t.Helper()
	require.Eventually(t, func() bool { return m.Status().State == want }, timeout, 50*time.Millisecond,
		"want state %s, got %s (%s)", want, m.Status().State, m.Status().Message)
}

func startEchoServer(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { l.Close() })
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			go func() {
				defer c.Close()
				_, _ = io.Copy(c, c)
			}()
		}
	}()
	return l.Addr().(*net.TCPAddr).Port
}

func TestManagerEndToEnd(t *testing.T) {
	ts := startServer(t)
	addr := "127.0.0.1"

	// Device A generates its own code and waits for approval.
	a := newTestManager(t, "machine-a")
	codeA, err := GenerateAuthCode()
	require.NoError(t, err)
	require.Error(t, a.Login(addr, ts.bindPort, "s3cret", "bad"))
	require.NoError(t, a.Login(addr, ts.bindPort, "s3cret", codeA))
	waitState(t, a, StatePending, 10*time.Second)

	ts.api(t, "POST", "/api/enterprise/codes/"+codeA+"/approve", nil)
	waitState(t, a, StateRunning, 20*time.Second)
	require.Eventually(t, func() bool { return a.Status().PublicIP == "127.0.0.1" }, 5*time.Second, 50*time.Millisecond)

	// A maps a port to a local echo server.
	echoPort := startEchoServer(t)
	remotePort := freePort(t)
	require.NoError(t, a.SaveProxy("", ProxySetting{Name: "echo", Type: "tcp", LocalPort: echoPort, RemotePort: remotePort}))
	require.Eventually(t, func() bool {
		st := a.Status()
		return len(st.Proxies) == 1 && st.Proxies[0].Phase == "running"
	}, 10*time.Second, 50*time.Millisecond)
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", remotePort), 2*time.Second)
	require.NoError(t, err)
	_, err = conn.Write([]byte("hello"))
	require.NoError(t, err)
	buf := make([]byte, 5)
	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	_, err = io.ReadFull(conn, buf)
	require.NoError(t, err)
	require.Equal(t, "hello", string(buf))
	conn.Close()

	// Device B logs in with a code generated by the server.
	b := newTestManager(t, "machine-b")
	codeB := ts.generateCode(t)
	require.NoError(t, b.Login(addr, ts.bindPort, "s3cret", strings.ToLower(codeB)))
	waitState(t, b, StateRunning, 10*time.Second)

	// B sees A's port as taken and cannot use it.
	require.Eventually(t, func() bool {
		for _, p := range b.Status().Ports {
			if p.Port == remotePort && !p.Mine && p.ProxyName == "" {
				return true
			}
		}
		return false
	}, 5*time.Second, 50*time.Millisecond)
	err = b.CheckRemotePort("tcp", remotePort, "")
	require.ErrorContains(t, err, "已被其他客户端占用")
	require.Error(t, b.SaveProxy("", ProxySetting{Name: "echo", Type: "tcp", LocalPort: echoPort, RemotePort: remotePort}))
	// A sees the port as its own.
	require.NoError(t, a.CheckRemotePort("tcp", remotePort, "echo"))

	// Notices reach every client.
	ts.api(t, "POST", "/api/enterprise/notices", map[string]string{"title": "维护通知", "content": "今晚 22 点维护"})
	require.Eventually(t, func() bool {
		return len(a.Status().Notices) == 1 && len(b.Status().Notices) == 1
	}, 5*time.Second, 50*time.Millisecond)
	require.Equal(t, "维护通知", a.Status().Notices[0].Title)

	// The server's branding reaches the clients, and so do changes to it.
	require.Eventually(t, func() bool { return b.Status().Branding.SiteName == "HDIT FRPE" }, 5*time.Second, 50*time.Millisecond)
	var brand map[string]any
	require.NoError(t, json.Unmarshal(ts.api(t, "GET", "/api/branding", nil), &brand))
	brand["siteName"] = "汇鼎内网穿透"
	brand["accent"] = "#0e8f96"
	ts.api(t, "PUT", "/api/branding", brand)
	require.Eventually(t, func() bool {
		br := b.Status().Branding
		return br.SiteName == "汇鼎内网穿透" && br.Accent == "#0e8f96"
	}, 5*time.Second, 50*time.Millisecond)
	// It is kept in the encrypted settings for the next start.
	require.Equal(t, "汇鼎内网穿透", b.Settings().Branding.SiteName)

	// A third device cannot reuse A's code.
	c := newTestManager(t, "machine-c")
	require.NoError(t, c.Login(addr, ts.bindPort, "s3cret", codeA))
	waitState(t, c, StateRejected, 10*time.Second)
	require.Contains(t, c.Status().Message, "绑定其他设备")

	// Revoking A disconnects it and stops retrying.
	ts.api(t, "POST", "/api/enterprise/codes/"+codeA+"/revoke", nil)
	waitState(t, a, StateRejected, 20*time.Second)
	require.Eventually(t, func() bool {
		c, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", remotePort), time.Second)
		if err == nil {
			c.Close()
		}
		return err != nil
	}, 10*time.Second, 100*time.Millisecond)

	// The dashboard lists the codes with their state.
	var codes []struct {
		Code   string `json:"code"`
		Status string `json:"status"`
		Online bool   `json:"online"`
	}
	require.NoError(t, json.Unmarshal(ts.api(t, "GET", "/api/enterprise/codes", nil), &codes))
	states := map[string]string{}
	for _, c := range codes {
		states[c.Code] = c.Status
	}
	require.Equal(t, "revoked", states[codeA])
	require.Equal(t, "approved", states[codeB])
}

func TestManagerWrongTokenKeepsRetrying(t *testing.T) {
	ts := startServer(t)
	m := newTestManager(t, "machine-x")
	code, _ := GenerateAuthCode()
	require.NoError(t, m.Login("127.0.0.1", ts.bindPort, "wrong", code))
	require.Eventually(t, func() bool {
		st := m.Status()
		return st.State == StateConnecting && strings.Contains(st.Message, "重试")
	}, 10*time.Second, 50*time.Millisecond)
	m.Stop()
	require.Equal(t, StateIdle, m.Status().State)
}

func TestSettingsStore(t *testing.T) {
	path := filepath.Join(t.TempDir(), "x", "config.enc")
	s := NewSettingsStore(path, func() string { return "m1" })
	st, err := s.Load()
	require.NoError(t, err)
	require.NotEmpty(t, st.DeviceID)
	require.Equal(t, 7000, st.ServerPort)

	st.Token = "super-secret-token"
	st.Proxies = []ProxySetting{{Name: "ssh", Type: "tcp", LocalIP: "127.0.0.1", LocalPort: 22, RemotePort: 6000}}
	require.NoError(t, s.Save(st))

	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	require.NotContains(t, string(raw), "super-secret-token")

	got, err := s.Load()
	require.NoError(t, err)
	require.Equal(t, st, got)

	other := NewSettingsStore(path, func() string { return "m2" })
	_, err = other.Load()
	require.ErrorIs(t, err, ErrSettingsUnreadable)
}

func TestLockPassword(t *testing.T) {
	m := newTestManager(t, "m")
	require.False(t, m.LockEnabled())
	require.True(t, m.VerifyPassword("anything"))
	require.Error(t, m.SetLockPassword("", "12"))
	require.NoError(t, m.SetLockPassword("", "pa55word"))
	require.True(t, m.LockEnabled())
	require.False(t, m.VerifyPassword("wrong"))
	require.True(t, m.VerifyPassword("pa55word"))
	require.Error(t, m.SetLockPassword("wrong", "newpass"))
	require.Error(t, m.DisableLock("wrong"))
	require.NoError(t, m.DisableLock("pa55word"))
	require.False(t, m.LockEnabled())
}

func TestProxySettingNormalize(t *testing.T) {
	p := ProxySetting{Name: " web ", LocalPort: 80}
	require.NoError(t, p.Normalize())
	require.Equal(t, "web", p.Name)
	require.Equal(t, "tcp", p.Type)
	require.Equal(t, "127.0.0.1", p.LocalIP)
	require.Error(t, (&ProxySetting{Name: "a b", LocalPort: 1}).Normalize())
	require.Error(t, (&ProxySetting{Name: "a", Type: "http", LocalPort: 1}).Normalize())
	require.Error(t, (&ProxySetting{Name: "a", LocalPort: 0}).Normalize())
	require.Error(t, (&ProxySetting{Name: "a", LocalPort: 1, RemotePort: 70000}).Normalize())
}

func TestParseServerAddr(t *testing.T) {
	h, p, err := ParseServerAddr("frp.example.com:7001")
	require.NoError(t, err)
	require.Equal(t, "frp.example.com", h)
	require.Equal(t, 7001, p)
	h, p, err = ParseServerAddr("10.0.0.1")
	require.NoError(t, err)
	require.Equal(t, "10.0.0.1", h)
	require.Equal(t, 7000, p)
	_, _, err = ParseServerAddr("host:abc")
	require.Error(t, err)
	_, _, err = ParseServerAddr("")
	require.Error(t, err)
}

func TestLogBuffer(t *testing.T) {
	b := NewLogBuffer(3, nil)
	_, _ = b.Write([]byte("2026-11-06 10:00:00.000 [I] [client/service.go:1] one\ntwo\n"))
	_, _ = b.Write([]byte("three\n"))
	_, _ = b.Write([]byte("four\n"))
	lines := b.Lines()
	require.Len(t, lines, 3)
	require.Equal(t, "two", lines[0].Text)
	require.Equal(t, "four", lines[2].Text)
	require.Equal(t, uint64(4), b.LastSeq())
	b2 := NewLogBuffer(10, nil)
	_, _ = b2.Write([]byte("2026-11-06 10:00:00.000 [W] [x.go:1] hello\n"))
	require.Equal(t, "[x.go:1] hello", b2.Lines()[0].Text)
}
