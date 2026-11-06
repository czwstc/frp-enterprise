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

// Package adminauth implements the frps dashboard accounts: one local user
// protected by a TOTP second factor and recovery codes, and users of an OIDC
// provider, matched by user name and approved by an administrator.
package adminauth

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"
)

// UserStatus is the state of an OIDC user.
type UserStatus string

const (
	UserPending  UserStatus = "pending"
	UserApproved UserStatus = "approved"
	UserRejected UserStatus = "rejected"
	UserDisabled UserStatus = "disabled"
)

// User is a dashboard user signing in with OIDC.
type User struct {
	Username    string     `json:"username"`
	Status      UserStatus `json:"status"`
	Email       string     `json:"email,omitempty"`
	Subject     string     `json:"subject,omitempty"`
	Note        string     `json:"note,omitempty"`
	RequestedAt time.Time  `json:"requestedAt,omitzero"`
	CreatedAt   time.Time  `json:"createdAt"`
	DecidedBy   string     `json:"decidedBy,omitempty"`
	DecidedAt   time.Time  `json:"decidedAt,omitzero"`
	LastLoginAt time.Time  `json:"lastLoginAt,omitzero"`
	LastLoginIP string     `json:"lastLoginIP,omitempty"`
}

// LocalState is the second factor of the local user.
type LocalState struct {
	TOTPSecret     string    `json:"totpSecret,omitempty"`
	TOTPEnabled    bool      `json:"totpEnabled"`
	LastTOTPStep   int64     `json:"lastTOTPStep,omitempty"`
	RecoveryHashes []string  `json:"recoveryHashes,omitempty"`
	EnrolledAt     time.Time `json:"enrolledAt,omitzero"`
	LastLoginAt    time.Time `json:"lastLoginAt,omitzero"`
	LastLoginIP    string    `json:"lastLoginIP,omitempty"`
}

type storeData struct {
	Local LocalState       `json:"local"`
	Users map[string]*User `json:"users"`
}

var (
	errUserNotFound = errors.New("用户不存在")
	errUserExists   = errors.New("用户已存在")
)

// userKey matches user names without regard to case.
func userKey(name string) string { return strings.ToLower(strings.TrimSpace(name)) }

// store keeps users and the local second factor in a JSON file.
type store struct {
	mu   sync.Mutex
	path string
	data storeData
}

func openStore(path string) (*store, error) {
	s := &store{path: path, data: storeData{Users: map[string]*User{}}}
	if path == "" {
		return s, nil
	}
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	if len(b) > 0 {
		if err := json.Unmarshal(b, &s.data); err != nil {
			return nil, fmt.Errorf("parse admin store %s: %w", path, err)
		}
	}
	if s.data.Users == nil {
		s.data.Users = map[string]*User{}
	}
	return s, nil
}

func (s *store) saveLocked() error {
	if s.path == "" {
		return nil
	}
	b, err := json.MarshalIndent(&s.data, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".frps_admin-*.tmp")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(name, s.path)
}

// update runs fn on a copy of the data and saves it; an error from fn
// discards the change.
func (s *store) update(fn func(d *storeData) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, err := json.Marshal(&s.data)
	if err != nil {
		return err
	}
	var next storeData
	if err := json.Unmarshal(b, &next); err != nil {
		return err
	}
	if next.Users == nil {
		next.Users = map[string]*User{}
	}
	if err := fn(&next); err != nil {
		return err
	}
	prev := s.data
	s.data = next
	if err := s.saveLocked(); err != nil {
		s.data = prev
		return err
	}
	return nil
}

func (s *store) local() LocalState {
	s.mu.Lock()
	defer s.mu.Unlock()
	l := s.data.Local
	l.RecoveryHashes = slices.Clone(l.RecoveryHashes)
	return l
}

func (s *store) user(name string) (User, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	u, ok := s.data.Users[userKey(name)]
	if !ok {
		return User{}, false
	}
	return *u, true
}

func (s *store) users() []User {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]User, 0, len(s.data.Users))
	for _, u := range s.data.Users {
		out = append(out, *u)
	}
	slices.SortFunc(out, func(a, b User) int {
		order := func(st UserStatus) int {
			switch st {
			case UserPending:
				return 0
			case UserApproved:
				return 1
			case UserDisabled:
				return 2
			default:
				return 3
			}
		}
		if d := order(a.Status) - order(b.Status); d != 0 {
			return d
		}
		return strings.Compare(userKey(a.Username), userKey(b.Username))
	})
	return out
}

func (s *store) countPending() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for _, u := range s.data.Users {
		if u.Status == UserPending {
			n++
		}
	}
	return n
}
