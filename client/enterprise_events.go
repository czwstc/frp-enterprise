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

package client

import (
	"slices"
	"sync"

	"github.com/fatedier/frp/client/proxy"
	"github.com/fatedier/frp/pkg/msg"
)

// EventHandler receives login and enterprise events of a Service. Methods
// may be called from any goroutine and must not block.
type EventHandler interface {
	// OnLoginResult is called after every login attempt; err is nil on success.
	OnLoginResult(err error)
	// OnDisconnected is called when an established control session ends.
	OnDisconnected()
	// OnEntSnapshot is called when the server pushes a port snapshot.
	OnEntSnapshot(snapshot *msg.EntSnapshot)
	// OnEntNotice is called once for every new notice from the server.
	OnEntNotice(notice *msg.EntNotice)
}

const maxEntNotices = 100

// entState keeps the latest enterprise data pushed by the server.
type entState struct {
	mu       sync.RWMutex
	snapshot *msg.EntSnapshot
	notices  []msg.EntNotice
	handler  EventHandler
}

func newEntState(handler EventHandler) *entState {
	return &entState{handler: handler}
}

func (s *entState) loginResult(err error) {
	if s.handler != nil {
		s.handler.OnLoginResult(err)
	}
}

func (s *entState) disconnected() {
	if s.handler != nil {
		s.handler.OnDisconnected()
	}
}

func (s *entState) handleSnapshot(m msg.Message) {
	snap, ok := m.(*msg.EntSnapshot)
	if !ok {
		return
	}
	s.mu.Lock()
	s.snapshot = snap
	s.mu.Unlock()
	if s.handler != nil {
		s.handler.OnEntSnapshot(snap)
	}
}

func (s *entState) handleNotice(m msg.Message) {
	n, ok := m.(*msg.EntNotice)
	if !ok {
		return
	}
	s.mu.Lock()
	// The server resends recent notices after every login; keep each once.
	if slices.ContainsFunc(s.notices, func(x msg.EntNotice) bool { return x.ID == n.ID }) {
		s.mu.Unlock()
		return
	}
	s.notices = append(s.notices, *n)
	slices.SortStableFunc(s.notices, func(a, b msg.EntNotice) int { return int(a.Time - b.Time) })
	if len(s.notices) > maxEntNotices {
		s.notices = s.notices[len(s.notices)-maxEntNotices:]
	}
	s.mu.Unlock()
	if s.handler != nil {
		s.handler.OnEntNotice(n)
	}
}

// EntState is the enterprise data last pushed by the server.
type EntState struct {
	Snapshot *msg.EntSnapshot `json:"snapshot"`
	Notices  []msg.EntNotice  `json:"notices"`
}

func (s *entState) get() EntState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return EntState{Snapshot: s.snapshot, Notices: slices.Clone(s.notices)}
}

// EnterpriseState returns the enterprise data last pushed by the server.
func (svr *Service) EnterpriseState() EntState {
	return svr.ent.get()
}

// AllProxyStatus returns the working status of every proxy.
func (svr *Service) AllProxyStatus() []*proxy.WorkingStatus {
	return svr.getAllProxyStatus()
}
