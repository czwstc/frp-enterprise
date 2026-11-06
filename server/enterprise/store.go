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

// Package enterprise implements per-device auth code login, approval and
// notifications for frps.
package enterprise

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"
)

// CodeStatus is the approval status of an auth code.
type CodeStatus string

const (
	StatusPending  CodeStatus = "pending"
	StatusApproved CodeStatus = "approved"
	StatusRejected CodeStatus = "rejected"
	StatusRevoked  CodeStatus = "revoked"
)

// CodeSource tells who generated an auth code.
type CodeSource string

const (
	SourceServer CodeSource = "server"
	SourceClient CodeSource = "client"
)

// AuthCode is one auth code and the device bound to it.
type AuthCode struct {
	Code          string     `json:"code"`
	Status        CodeStatus `json:"status"`
	Source        CodeSource `json:"source"`
	Note          string     `json:"note,omitempty"`
	BoundClientID string     `json:"boundClientID,omitempty"`
	Hostname      string     `json:"hostname,omitempty"`
	OS            string     `json:"os,omitempty"`
	Arch          string     `json:"arch,omitempty"`
	Version       string     `json:"version,omitempty"`
	LastIP        string     `json:"lastIP,omitempty"`
	CreatedAt     time.Time  `json:"createdAt"`
	UpdatedAt     time.Time  `json:"updatedAt"`
	LastSeenAt    time.Time  `json:"lastSeenAt,omitzero"`
}

// Notice is a notification sent by the administrator.
type Notice struct {
	ID      string    `json:"id"`
	Level   string    `json:"level,omitempty"`
	Title   string    `json:"title,omitempty"`
	Content string    `json:"content,omitempty"`
	Target  string    `json:"target,omitempty"`
	Time    time.Time `json:"time"`
}

// ErrNotFound is returned for unknown auth codes.
var ErrNotFound = errors.New("auth code not found")

// ErrExists is returned when inserting an auth code that already exists.
var ErrExists = errors.New("auth code already exists")

const maxNotices = 50

type storeData struct {
	Codes   map[string]*AuthCode `json:"codes"`
	Notices []Notice             `json:"notices"`
}

// Store keeps auth codes and notices, persisted to a JSON file.
// An empty path keeps everything in memory.
type Store struct {
	mu   sync.Mutex
	path string
	data storeData
}

// OpenStore loads the store from path, creating an empty one if the file
// does not exist.
func OpenStore(path string) (*Store, error) {
	s := &Store{path: path, data: storeData{Codes: map[string]*AuthCode{}}}
	if path == "" {
		return s, nil
	}
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read enterprise store: %w", err)
	}
	if len(b) > 0 {
		if err := json.Unmarshal(b, &s.data); err != nil {
			return nil, fmt.Errorf("parse enterprise store %s: %w", path, err)
		}
	}
	if s.data.Codes == nil {
		s.data.Codes = map[string]*AuthCode{}
	}
	return s, nil
}

// saveLocked writes the store atomically. The caller holds s.mu.
func (s *Store) saveLocked() error {
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
	tmp, err := os.CreateTemp(dir, ".frps_enterprise-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
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
	return os.Rename(tmpName, s.path)
}

// Get returns a copy of the auth code.
func (s *Store) Get(code string) (AuthCode, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.data.Codes[code]
	if !ok {
		return AuthCode{}, false
	}
	return *c, true
}

// List returns copies of all auth codes, newest first.
func (s *Store) List() []AuthCode {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]AuthCode, 0, len(s.data.Codes))
	for _, c := range s.data.Codes {
		out = append(out, *c)
	}
	slices.SortFunc(out, func(a, b AuthCode) int {
		if v := b.CreatedAt.Compare(a.CreatedAt); v != 0 {
			return v
		}
		if a.Code < b.Code {
			return -1
		}
		return 1
	})
	return out
}

// CountByStatus returns the number of codes with the status.
func (s *Store) CountByStatus(status CodeStatus) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for _, c := range s.data.Codes {
		if c.Status == status {
			n++
		}
	}
	return n
}

// Insert adds a new auth code.
func (s *Store) Insert(c AuthCode) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.data.Codes[c.Code]; ok {
		return ErrExists
	}
	cp := c
	s.data.Codes[c.Code] = &cp
	if err := s.saveLocked(); err != nil {
		delete(s.data.Codes, c.Code)
		return err
	}
	return nil
}

// Update changes an auth code with fn and saves the store. When fn returns an
// error the change is discarded and the error is returned.
func (s *Store) Update(code string, fn func(c *AuthCode) error) (AuthCode, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cur, ok := s.data.Codes[code]
	if !ok {
		return AuthCode{}, ErrNotFound
	}
	next := *cur
	if err := fn(&next); err != nil {
		return *cur, err
	}
	prev := *cur
	*cur = next
	if err := s.saveLocked(); err != nil {
		*cur = prev
		return prev, err
	}
	return next, nil
}

// Delete removes an auth code.
func (s *Store) Delete(code string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cur, ok := s.data.Codes[code]
	if !ok {
		return ErrNotFound
	}
	delete(s.data.Codes, code)
	if err := s.saveLocked(); err != nil {
		s.data.Codes[code] = cur
		return err
	}
	return nil
}

// AddNotice appends a notice, keeping only the latest maxNotices.
func (s *Store) AddNotice(n Notice) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	prev := s.data.Notices
	s.data.Notices = append(slices.Clone(prev), n)
	if len(s.data.Notices) > maxNotices {
		s.data.Notices = s.data.Notices[len(s.data.Notices)-maxNotices:]
	}
	if err := s.saveLocked(); err != nil {
		s.data.Notices = prev
		return err
	}
	return nil
}

// Notices returns the notices, oldest first.
func (s *Store) Notices() []Notice {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.data.Notices)
}
