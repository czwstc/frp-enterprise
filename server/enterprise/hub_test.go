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
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/fatedier/frp/pkg/enterprise"
	"github.com/fatedier/frp/pkg/msg"
)

type fakeSession struct {
	runID  string
	login  *msg.Login
	addr   string
	mu     sync.Mutex
	sent   []msg.Message
	done   chan struct{}
	closed bool
}

func newFakeSession(runID, clientID, code string) *fakeSession {
	return &fakeSession{
		runID: runID,
		login: &msg.Login{ClientID: clientID, Metas: map[string]string{
			msg.EntMetaAuthCode: code, msg.EntMetaCaps: msg.EntCapsV1,
		}},
		addr: "203.0.113.7:51000",
		done: make(chan struct{}),
	}
}

func (f *fakeSession) RunID() string         { return f.runID }
func (f *fakeSession) LoginMsg() *msg.Login  { return f.login }
func (f *fakeSession) RemoteAddr() string    { return f.addr }
func (f *fakeSession) Done() <-chan struct{} { return f.done }
func (f *fakeSession) SendMsg(m msg.Message) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sent = append(f.sent, m)
	return nil
}

func (f *fakeSession) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.closed {
		f.closed = true
		close(f.done)
	}
	return nil
}

func (f *fakeSession) messages() []msg.Message {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]msg.Message(nil), f.sent...)
}

func loginKind(err error) string {
	var le *enterprise.LoginError
	if errors.As(err, &le) {
		return le.Kind
	}
	return ""
}

func newTestHub(t *testing.T, ports PortLister) *Hub {
	t.Helper()
	h, err := NewHub(Options{
		StorePath:        filepath.Join(t.TempDir(), "store.json"),
		ListPorts:        ports,
		SnapshotDebounce: 10 * time.Millisecond,
	})
	require.NoError(t, err)
	t.Cleanup(h.Close)
	return h
}

func login(clientID, code string) *msg.Login {
	return &msg.Login{ClientID: clientID, Hostname: "host-" + clientID, Metas: map[string]string{msg.EntMetaAuthCode: code}}
}

func TestCheckLoginClientGeneratedCode(t *testing.T) {
	h := newTestHub(t, nil)

	require.Equal(t, enterprise.LoginErrRequired, loginKind(h.CheckLogin(login("dev1", ""), "1.2.3.4:5")))
	require.Equal(t, enterprise.LoginErrInvalid, loginKind(h.CheckLogin(login("dev1", "ab"), "1.2.3.4:5")))

	code := "ABCD-EFGH-JKLM-NPQR"
	require.Equal(t, enterprise.LoginErrPending, loginKind(h.CheckLogin(login("dev1", code), "1.2.3.4:5")))
	c, ok := h.Store().Get(code)
	require.True(t, ok)
	require.Equal(t, StatusPending, c.Status)
	require.Equal(t, SourceClient, c.Source)
	require.Equal(t, "1.2.3.4", c.LastIP)
	require.Equal(t, "host-dev1", c.Hostname)

	// Still pending on retry; a second device cannot take over the pending code.
	require.Equal(t, enterprise.LoginErrPending, loginKind(h.CheckLogin(login("dev1", code), "1.2.3.4:5")))
	require.Equal(t, enterprise.LoginErrBound, loginKind(h.CheckLogin(login("dev2", code), "1.2.3.4:5")))

	_, err := h.Approve(code)
	require.NoError(t, err)
	require.NoError(t, h.CheckLogin(login("dev1", code), "1.2.3.4:5"))
	// Lower case input is normalized.
	require.NoError(t, h.CheckLogin(login("dev1", "abcd-efgh-jklm-npqr"), "1.2.3.4:5"))
	require.Equal(t, enterprise.LoginErrBound, loginKind(h.CheckLogin(login("dev2", code), "1.2.3.4:5")))

	_, err = h.Reject(code)
	require.NoError(t, err)
	require.Equal(t, enterprise.LoginErrRejected, loginKind(h.CheckLogin(login("dev1", code), "1.2.3.4:5")))
}

func TestCheckLoginServerGeneratedCode(t *testing.T) {
	h := newTestHub(t, nil)
	c, err := h.GenerateCode("office laptop")
	require.NoError(t, err)
	require.Equal(t, StatusApproved, c.Status)
	require.Equal(t, SourceServer, c.Source)
	require.Empty(t, c.BoundClientID)

	require.NoError(t, h.CheckLogin(login("dev1", c.Code), "1.2.3.4:5"))
	got, _ := h.Store().Get(c.Code)
	require.Equal(t, "dev1", got.BoundClientID)
	require.Equal(t, enterprise.LoginErrBound, loginKind(h.CheckLogin(login("dev2", c.Code), "1.2.3.4:5")))

	_, err = h.Unbind(c.Code)
	require.NoError(t, err)
	require.NoError(t, h.CheckLogin(login("dev2", c.Code), "1.2.3.4:5"))

	_, err = h.Revoke(c.Code)
	require.NoError(t, err)
	require.Equal(t, enterprise.LoginErrRejected, loginKind(h.CheckLogin(login("dev2", c.Code), "1.2.3.4:5")))
}

func TestStorePersists(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "store.json")
	h, err := NewHub(Options{StorePath: path})
	require.NoError(t, err)
	c, err := h.GenerateCode("x")
	require.NoError(t, err)
	_, err = h.SendNotice("info", "hello", "world", "")
	require.NoError(t, err)
	h.Close()

	h2, err := NewHub(Options{StorePath: path})
	require.NoError(t, err)
	defer h2.Close()
	got, ok := h2.Store().Get(c.Code)
	require.True(t, ok)
	require.Equal(t, "x", got.Note)
	require.Len(t, h2.Notices(), 1)
}

func TestRevokeKicksSessionAndSnapshot(t *testing.T) {
	var mu sync.Mutex
	ports := []PortUse{}
	h := newTestHub(t, func() []PortUse {
		mu.Lock()
		defer mu.Unlock()
		return append([]PortUse(nil), ports...)
	})

	c1, _ := h.GenerateCode("")
	c2, _ := h.GenerateCode("")
	s1 := newFakeSession("run1", "dev1", c1.Code)
	s2 := newFakeSession("run2", "dev2", c2.Code)
	require.NoError(t, h.CheckLogin(s1.login, s1.addr))
	require.NoError(t, h.CheckLogin(s2.login, s2.addr))

	mu.Lock()
	ports = append(ports, PortUse{Proto: "tcp", Port: 6001, RunID: "run1", ProxyName: "ssh"})
	mu.Unlock()
	h.OnControlActive(s1)
	h.OnControlActive(s2)

	snap := lastSnapshot(s2)
	require.NotNil(t, snap)
	require.Equal(t, "203.0.113.7", snap.ClientIP)
	require.Len(t, snap.Ports, 1)
	require.False(t, snap.Ports[0].Mine)
	require.Empty(t, snap.Ports[0].ProxyName)

	snap = lastSnapshot(s1)
	require.True(t, snap.Ports[0].Mine)
	require.Equal(t, "ssh", snap.Ports[0].ProxyName)

	views := h.ListCodes("")
	online := 0
	for _, v := range views {
		if v.Online {
			online++
		}
	}
	require.Equal(t, 2, online)

	// A notice targeted at code 2 only reaches session 2.
	_, err := h.SendNotice("warning", "maintenance", "tonight", c2.Code)
	require.NoError(t, err)
	require.Equal(t, 1, countNotices(s2))
	require.Equal(t, 0, countNotices(s1))

	_, err = h.Revoke(c1.Code)
	require.NoError(t, err)
	select {
	case <-s1.done:
	case <-time.After(time.Second):
		t.Fatal("session 1 was not closed")
	}
	require.Eventually(t, func() bool {
		for _, v := range h.ListCodes("") {
			if v.Code == c1.Code && v.Online {
				return false
			}
		}
		return true
	}, time.Second, 10*time.Millisecond)

	// Port changes are broadcast after the debounce.
	before := countSnapshots(s2)
	mu.Lock()
	ports = append(ports, PortUse{Proto: "udp", Port: 6002, RunID: "run2", ProxyName: "dns"})
	mu.Unlock()
	h.MarkDirty()
	require.Eventually(t, func() bool { return countSnapshots(s2) > before }, time.Second, 10*time.Millisecond)
	snap = lastSnapshot(s2)
	require.Len(t, snap.Ports, 2)
	require.Equal(t, "tcp", snap.Ports[0].Proto)
	require.True(t, snap.Ports[1].Mine)
}

func TestSendNoticeValidation(t *testing.T) {
	h := newTestHub(t, nil)
	_, err := h.SendNotice("info", "", "", "")
	require.Error(t, err)
	_, err = h.SendNotice("bogus", "t", "c", "")
	require.Error(t, err)
	n, err := h.SendNotice("", "t", "c", "")
	require.NoError(t, err)
	require.Equal(t, "info", n.Level)
}

func lastSnapshot(s *fakeSession) *msg.EntSnapshot {
	msgs := s.messages()
	for i := len(msgs) - 1; i >= 0; i-- {
		if snap, ok := msgs[i].(*msg.EntSnapshot); ok {
			return snap
		}
	}
	return nil
}

func countSnapshots(s *fakeSession) int {
	n := 0
	for _, m := range s.messages() {
		if _, ok := m.(*msg.EntSnapshot); ok {
			n++
		}
	}
	return n
}

func countNotices(s *fakeSession) int {
	n := 0
	for _, m := range s.messages() {
		if _, ok := m.(*msg.EntNotice); ok {
			n++
		}
	}
	return n
}
