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
	"cmp"
	"errors"
	"fmt"
	"net"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/fatedier/frp/pkg/enterprise"
	"github.com/fatedier/frp/pkg/msg"
	"github.com/fatedier/frp/pkg/util/log"
	"github.com/fatedier/frp/pkg/util/util"
)

// Session is a logged-in client control, as the hub sees it.
type Session interface {
	RunID() string
	LoginMsg() *msg.Login
	RemoteAddr() string
	SendMsg(m msg.Message) error
	Close() error
	Done() <-chan struct{}
}

// PortUse is one tcp/udp port held by a proxy on the server.
type PortUse struct {
	Proto     string
	Port      int
	RunID     string
	ProxyName string
}

// PortLister lists the tcp/udp ports in use on the server.
type PortLister func() []PortUse

// Options configures a Hub.
type Options struct {
	StorePath string
	// AllowPorts describes the allowed port ranges, shown to clients.
	AllowPorts string
	ListPorts  PortLister
	// MaxPending limits the number of pending codes that clients may create.
	MaxPending int
	// SnapshotInterval is the period of the full snapshot broadcast.
	SnapshotInterval time.Duration
	// SnapshotDebounce delays broadcasts after a change, merging bursts.
	SnapshotDebounce time.Duration
	// Audit records client events. It may be nil.
	Audit func(action, target, detail, ip string, ok bool)
	// Branding returns the look sent to clients of EntCapsV2. It may be nil.
	Branding func() *msg.EntBranding
}

type sessionEntry struct {
	s    Session
	code string
	caps bool
	// branding is set for clients that understand EntBranding.
	branding bool
}

// Hub implements auth code login and pushes snapshots and notices to clients.
type Hub struct {
	store *Store
	opts  Options

	mu       sync.Mutex
	sessions map[Session]*sessionEntry

	dirtyCh   chan struct{}
	closeCh   chan struct{}
	closeOnce sync.Once

	auditMu   sync.Mutex
	auditSeen map[string]time.Time
}

// auditRepeatWindow keeps a client that retries a refused login from
// filling the audit log.
const auditRepeatWindow = 10 * time.Minute

func (h *Hub) audit(action, target, detail, ip string, ok bool) {
	if h.opts.Audit == nil {
		return
	}
	key := action + "|" + target + "|" + detail
	h.auditMu.Lock()
	if last, seen := h.auditSeen[key]; seen && time.Since(last) < auditRepeatWindow {
		h.auditMu.Unlock()
		return
	}
	h.auditSeen[key] = time.Now()
	h.auditMu.Unlock()
	h.opts.Audit(action, target, detail, ip, ok)
}

// NewHub opens the store and starts the broadcast loop.
func NewHub(opts Options) (*Hub, error) {
	store, err := OpenStore(opts.StorePath)
	if err != nil {
		return nil, err
	}
	opts.MaxPending = cmp.Or(opts.MaxPending, 1000)
	opts.SnapshotInterval = cmp.Or(opts.SnapshotInterval, 30*time.Second)
	opts.SnapshotDebounce = cmp.Or(opts.SnapshotDebounce, 300*time.Millisecond)
	if opts.ListPorts == nil {
		opts.ListPorts = func() []PortUse { return nil }
	}
	h := &Hub{
		store:     store,
		opts:      opts,
		sessions:  map[Session]*sessionEntry{},
		dirtyCh:   make(chan struct{}, 1),
		closeCh:   make(chan struct{}),
		auditSeen: map[string]time.Time{},
	}
	go h.loop()
	return h, nil
}

// Close stops the broadcast loop.
func (h *Hub) Close() {
	h.closeOnce.Do(func() { close(h.closeCh) })
}

// Store returns the underlying store.
func (h *Hub) Store() *Store { return h.store }

func hostOf(addr string) string {
	if host, _, err := net.SplitHostPort(addr); err == nil {
		return host
	}
	return addr
}

// CheckLogin decides whether a login may proceed. It returns a
// *enterprise.LoginError when the client is refused.
func (h *Hub) CheckLogin(login *msg.Login, remoteAddr string) error {
	err := h.checkLogin(login, remoteAddr)
	var le *enterprise.LoginError
	if errors.As(err, &le) {
		code := enterprise.NormalizeAuthCode(login.Metas[msg.EntMetaAuthCode])
		detail := fmt.Sprintf("%s: device %s, host %s", le.Kind, login.ClientID, login.Hostname)
		h.audit("client.login", code, detail, hostOf(remoteAddr), false)
	}
	return err
}

func (h *Hub) checkLogin(login *msg.Login, remoteAddr string) error {
	code := enterprise.NormalizeAuthCode(login.Metas[msg.EntMetaAuthCode])
	if code == "" {
		return &enterprise.LoginError{Kind: enterprise.LoginErrRequired, Msg: "an auth code is required"}
	}
	if err := enterprise.ValidateAuthCode(code); err != nil {
		return &enterprise.LoginError{Kind: enterprise.LoginErrInvalid, Msg: err.Error()}
	}
	if login.ClientID == "" {
		return &enterprise.LoginError{Kind: enterprise.LoginErrInvalid, Msg: "a client ID is required to bind the auth code"}
	}
	now := time.Now()
	ip := hostOf(remoteAddr)
	touch := func(c *AuthCode) {
		c.Hostname = login.Hostname
		c.OS = login.Os
		c.Arch = login.Arch
		c.Version = login.Version
		c.LastIP = ip
		c.LastSeenAt = now
	}

	if _, ok := h.store.Get(code); !ok {
		if h.store.CountByStatus(StatusPending) >= h.opts.MaxPending {
			return &enterprise.LoginError{Kind: enterprise.LoginErrRejected, Msg: "too many pending auth codes, try again later"}
		}
		c := AuthCode{
			Code:          code,
			Status:        StatusPending,
			Source:        SourceClient,
			BoundClientID: login.ClientID,
			CreatedAt:     now,
			UpdatedAt:     now,
		}
		touch(&c)
		if err := h.store.Insert(c); err != nil && !errors.Is(err, ErrExists) {
			log.Warnf("enterprise: save pending auth code: %v", err)
		}
		log.Infof("enterprise: new auth code [%s] from client [%s] host [%s] is waiting for approval", code, login.ClientID, login.Hostname)
		return pendingError()
	}

	var result error
	_, err := h.store.Update(code, func(c *AuthCode) error {
		result = nil
		switch c.Status {
		case StatusPending:
			if c.BoundClientID != "" && c.BoundClientID != login.ClientID {
				result = boundError()
				return nil
			}
			touch(c)
			result = pendingError()
		case StatusRejected, StatusRevoked:
			result = &enterprise.LoginError{Kind: enterprise.LoginErrRejected, Msg: "the auth code was " + string(c.Status) + " by the administrator"}
		case StatusApproved:
			if c.BoundClientID == "" {
				c.BoundClientID = login.ClientID
				c.UpdatedAt = now
			} else if c.BoundClientID != login.ClientID {
				result = boundError()
				return nil
			}
			touch(c)
		default:
			result = &enterprise.LoginError{Kind: enterprise.LoginErrRejected, Msg: "unknown auth code status"}
		}
		return nil
	})
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return pendingError()
		}
		log.Warnf("enterprise: update auth code [%s]: %v", code, err)
	}
	return result
}

func pendingError() error {
	return &enterprise.LoginError{Kind: enterprise.LoginErrPending, Msg: "the auth code is waiting for approval by the administrator"}
}

func boundError() error {
	return &enterprise.LoginError{Kind: enterprise.LoginErrBound, Msg: "the auth code is bound to another device"}
}

// OnControlActive registers a logged-in session and sends it the current
// snapshot and recent notices. The session is removed when it is done.
func (h *Hub) OnControlActive(s Session) {
	login := s.LoginMsg()
	e := &sessionEntry{
		s:    s,
		code: enterprise.NormalizeAuthCode(login.Metas[msg.EntMetaAuthCode]),
		caps: login.Metas[msg.EntMetaCaps] != "",
	}
	if v, err := strconv.Atoi(login.Metas[msg.EntMetaCaps]); err == nil && v >= 2 {
		e.branding = true
	}
	h.mu.Lock()
	h.sessions[s] = e
	h.mu.Unlock()
	h.auditEvent("client.connect", e.code, fmt.Sprintf("device %s, host %s, run id %s", login.ClientID, login.Hostname, s.RunID()), hostOf(s.RemoteAddr()))

	go func() {
		select {
		case <-s.Done():
		case <-h.closeCh:
			return
		}
		h.mu.Lock()
		delete(h.sessions, s)
		h.mu.Unlock()
		h.auditEvent("client.disconnect", e.code, "run id "+s.RunID(), hostOf(s.RemoteAddr()))
		h.MarkDirty()
	}()

	if e.branding && h.opts.Branding != nil {
		_ = s.SendMsg(h.opts.Branding())
	}
	if e.caps {
		_ = s.SendMsg(h.buildSnapshot(e, h.opts.ListPorts()))
		notices := h.store.Notices()
		sent := 0
		for i := len(notices) - 1; i >= 0 && sent < 20; i-- {
			n := notices[i]
			if n.Target != "" && n.Target != e.code {
				continue
			}
			_ = s.SendMsg(toNoticeMsg(n))
			sent++
		}
	}
	h.MarkDirty()
}

// MarkDirty schedules a snapshot broadcast.
func (h *Hub) MarkDirty() {
	select {
	case h.dirtyCh <- struct{}{}:
	default:
	}
}

func (h *Hub) loop() {
	ticker := time.NewTicker(h.opts.SnapshotInterval)
	defer ticker.Stop()
	for {
		select {
		case <-h.closeCh:
			return
		case <-ticker.C:
			h.BroadcastSnapshot()
		case <-h.dirtyCh:
			select {
			case <-time.After(h.opts.SnapshotDebounce):
			case <-h.closeCh:
				return
			}
			// Drain a change that arrived during the debounce.
			select {
			case <-h.dirtyCh:
			default:
			}
			h.BroadcastSnapshot()
		}
	}
}

func (h *Hub) entries() []*sessionEntry {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]*sessionEntry, 0, len(h.sessions))
	for _, e := range h.sessions {
		out = append(out, e)
	}
	return out
}

// BroadcastBranding sends the look to every client that understands it.
func (h *Hub) BroadcastBranding() {
	if h.opts.Branding == nil {
		return
	}
	m := h.opts.Branding()
	for _, e := range h.entries() {
		if e.branding {
			_ = e.s.SendMsg(m)
		}
	}
}

// BroadcastSnapshot sends the current snapshot to every capable session.
func (h *Hub) BroadcastSnapshot() {
	ports := h.opts.ListPorts()
	for _, e := range h.entries() {
		if e.caps {
			_ = e.s.SendMsg(h.buildSnapshot(e, ports))
		}
	}
}

func (h *Hub) buildSnapshot(e *sessionEntry, ports []PortUse) *msg.EntSnapshot {
	runID := e.s.RunID()
	snap := &msg.EntSnapshot{
		ClientIP:   hostOf(e.s.RemoteAddr()),
		AllowPorts: h.opts.AllowPorts,
		Ports:      make([]msg.EntPort, 0, len(ports)),
	}
	for _, p := range ports {
		ep := msg.EntPort{Proto: p.Proto, Port: p.Port}
		if p.RunID == runID {
			ep.Mine = true
			ep.ProxyName = p.ProxyName
		}
		snap.Ports = append(snap.Ports, ep)
	}
	slices.SortFunc(snap.Ports, func(a, b msg.EntPort) int {
		if v := cmp.Compare(a.Proto, b.Proto); v != 0 {
			return v
		}
		return cmp.Compare(a.Port, b.Port)
	})
	return snap
}

// CodeView is an auth code with its online state.
type CodeView struct {
	AuthCode
	Online bool   `json:"online"`
	RunID  string `json:"runID,omitempty"`
}

// ListCodes returns all codes with their online state, optionally filtered
// by status.
func (h *Hub) ListCodes(status string) []CodeView {
	online := map[string]string{}
	for _, e := range h.entries() {
		online[e.code] = e.s.RunID()
	}
	codes := h.store.List()
	out := make([]CodeView, 0, len(codes))
	for _, c := range codes {
		if status != "" && string(c.Status) != status {
			continue
		}
		runID, ok := online[c.Code]
		out = append(out, CodeView{AuthCode: c, Online: ok, RunID: runID})
	}
	return out
}

// GenerateCode creates an approved, unbound auth code.
func (h *Hub) GenerateCode(note string) (AuthCode, error) {
	for range 5 {
		code, err := enterprise.GenerateAuthCode()
		if err != nil {
			return AuthCode{}, err
		}
		now := time.Now()
		c := AuthCode{
			Code:      code,
			Status:    StatusApproved,
			Source:    SourceServer,
			Note:      strings.TrimSpace(note),
			CreatedAt: now,
			UpdatedAt: now,
		}
		if err := h.store.Insert(c); errors.Is(err, ErrExists) {
			continue
		} else if err != nil {
			return AuthCode{}, err
		}
		return c, nil
	}
	return AuthCode{}, errors.New("failed to generate a unique auth code")
}

func (h *Hub) setStatus(code string, status CodeStatus, note *string) (AuthCode, error) {
	code = enterprise.NormalizeAuthCode(code)
	c, err := h.store.Update(code, func(c *AuthCode) error {
		c.Status = status
		c.UpdatedAt = time.Now()
		if note != nil {
			c.Note = strings.TrimSpace(*note)
		}
		return nil
	})
	if err != nil {
		return c, err
	}
	if status == StatusRejected || status == StatusRevoked {
		h.kick(code)
	}
	return c, nil
}

// Approve approves an auth code.
func (h *Hub) Approve(code string) (AuthCode, error) {
	return h.setStatus(code, StatusApproved, nil)
}

// Reject rejects an auth code and disconnects its device.
func (h *Hub) Reject(code string) (AuthCode, error) {
	return h.setStatus(code, StatusRejected, nil)
}

// Revoke revokes an auth code and disconnects its device.
func (h *Hub) Revoke(code string) (AuthCode, error) {
	return h.setStatus(code, StatusRevoked, nil)
}

// SetNote changes the note of an auth code.
func (h *Hub) SetNote(code, note string) (AuthCode, error) {
	return h.store.Update(enterprise.NormalizeAuthCode(code), func(c *AuthCode) error {
		c.Note = strings.TrimSpace(note)
		c.UpdatedAt = time.Now()
		return nil
	})
}

// Unbind clears the device bound to an auth code and disconnects it, so that
// another device may use the code.
func (h *Hub) Unbind(code string) (AuthCode, error) {
	code = enterprise.NormalizeAuthCode(code)
	c, err := h.store.Update(code, func(c *AuthCode) error {
		c.BoundClientID = ""
		c.UpdatedAt = time.Now()
		return nil
	})
	if err == nil {
		h.kick(code)
	}
	return c, err
}

// Delete deletes an auth code and disconnects its device.
func (h *Hub) Delete(code string) error {
	code = enterprise.NormalizeAuthCode(code)
	if err := h.store.Delete(code); err != nil {
		return err
	}
	h.kick(code)
	return nil
}

func (h *Hub) kick(code string) {
	for _, e := range h.entries() {
		if e.code == code {
			log.Infof("enterprise: disconnect client [%s] of auth code [%s]", e.s.RunID(), code)
			_ = e.s.Close()
		}
	}
}

// SendNotice stores a notice and sends it to the target sessions. An empty
// target sends it to every client.
func (h *Hub) SendNotice(level, title, content, target string) (Notice, error) {
	title = strings.TrimSpace(title)
	content = strings.TrimSpace(content)
	if title == "" && content == "" {
		return Notice{}, errors.New("notice is empty")
	}
	switch level {
	case "", "info":
		level = "info"
	case "warning", "error", "success":
	default:
		return Notice{}, fmt.Errorf("invalid notice level %q", level)
	}
	target = enterprise.NormalizeAuthCode(target)
	id, err := util.RandID()
	if err != nil {
		return Notice{}, err
	}
	n := Notice{ID: id, Level: level, Title: title, Content: content, Target: target, Time: time.Now()}
	if err := h.store.AddNotice(n); err != nil {
		return Notice{}, err
	}
	m := toNoticeMsg(n)
	for _, e := range h.entries() {
		if e.caps && (target == "" || e.code == target) {
			_ = e.s.SendMsg(m)
		}
	}
	return n, nil
}

// Notices returns the stored notices, newest first.
func (h *Hub) Notices() []Notice {
	n := h.store.Notices()
	slices.Reverse(n)
	return n
}

// Ports returns the ports in use with the auth code that owns each one.
func (h *Hub) Ports() []PortView {
	codeByRunID := map[string]string{}
	for _, e := range h.entries() {
		codeByRunID[e.s.RunID()] = e.code
	}
	ports := h.opts.ListPorts()
	out := make([]PortView, 0, len(ports))
	for _, p := range ports {
		out = append(out, PortView{Proto: p.Proto, Port: p.Port, ProxyName: p.ProxyName, RunID: p.RunID, AuthCode: codeByRunID[p.RunID]})
	}
	slices.SortFunc(out, func(a, b PortView) int {
		if v := cmp.Compare(a.Proto, b.Proto); v != 0 {
			return v
		}
		return cmp.Compare(a.Port, b.Port)
	})
	return out
}

// PortView is a port in use, for the dashboard.
type PortView struct {
	Proto     string `json:"proto"`
	Port      int    `json:"port"`
	ProxyName string `json:"proxyName"`
	RunID     string `json:"runID"`
	AuthCode  string `json:"authCode,omitempty"`
}

func toNoticeMsg(n Notice) *msg.EntNotice {
	return &msg.EntNotice{ID: n.ID, Level: n.Level, Title: n.Title, Content: n.Content, Time: n.Time.Unix()}
}

// auditEvent records an event that does not repeat, such as a connect.
func (h *Hub) auditEvent(action, target, detail, ip string) {
	if h.opts.Audit != nil {
		h.opts.Audit(action, target, detail, ip, true)
	}
}
