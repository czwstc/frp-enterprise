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

// Package audit records who did what on the frps dashboard and to clients,
// in a JSON-lines file, and serves recent entries to the dashboard.
package audit

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/fatedier/frp/pkg/util/log"
)

// Entry is one audit record.
type Entry struct {
	Time    time.Time `json:"time"`
	Actor   string    `json:"actor"`
	Action  string    `json:"action"`
	Target  string    `json:"target,omitempty"`
	Detail  string    `json:"detail,omitempty"`
	IP      string    `json:"ip,omitempty"`
	Success bool      `json:"success"`
}

// System is the actor of events frps does on its own.
const System = "system"

const defaultKeep = 5000

// Logger appends entries to a file and keeps the latest in memory.
type Logger struct {
	mu      sync.Mutex
	path    string
	f       *os.File
	entries []Entry
	keep    int
}

// Open opens or creates the audit log at path, loading its latest entries.
// An empty path keeps entries in memory only.
func Open(path string) (*Logger, error) {
	l := &Logger{path: path, keep: defaultKeep}
	if path == "" {
		return l, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	if f, err := os.Open(path); err == nil {
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 64*1024), 1024*1024)
		for sc.Scan() {
			var e Entry
			if json.Unmarshal(sc.Bytes(), &e) == nil {
				l.entries = append(l.entries, e)
				if len(l.entries) > 2*l.keep {
					l.entries = slices.Clone(l.entries[len(l.entries)-l.keep:])
				}
			}
		}
		f.Close()
		if len(l.entries) > l.keep {
			l.entries = slices.Clone(l.entries[len(l.entries)-l.keep:])
		}
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, err
	}
	l.f = f
	return l, nil
}

// Close closes the file.
func (l *Logger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.f != nil {
		err := l.f.Close()
		l.f = nil
		return err
	}
	return nil
}

// Record appends an entry, filling in the time.
func (l *Logger) Record(e Entry) {
	if l == nil {
		return
	}
	if e.Time.IsZero() {
		e.Time = time.Now()
	}
	if e.Actor == "" {
		e.Actor = System
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.entries = append(l.entries, e)
	if len(l.entries) > 2*l.keep {
		l.entries = slices.Clone(l.entries[len(l.entries)-l.keep:])
	}
	if l.f != nil {
		b, err := json.Marshal(e)
		if err == nil {
			b = append(b, '\n')
			_, err = l.f.Write(b)
		}
		if err != nil {
			log.Warnf("write audit log %s: %v", l.path, err)
		}
	}
}

// Query filters the entries.
type Query struct {
	// Q matches the actor, action, target, detail or IP, ignoring case.
	Q string
	// Action keeps entries whose action starts with it, as "auth." or
	// "code.approve".
	Action string
	Offset int
	Limit  int
}

// Page is a page of entries, newest first.
type Page struct {
	Total int     `json:"total"`
	Items []Entry `json:"items"`
}

// List returns entries matching q, newest first.
func (l *Logger) List(q Query) Page {
	l.mu.Lock()
	all := slices.Clone(l.entries)
	l.mu.Unlock()
	needle := strings.ToLower(strings.TrimSpace(q.Q))
	matched := make([]Entry, 0, len(all))
	for i := len(all) - 1; i >= 0; i-- {
		e := all[i]
		if q.Action != "" && !strings.HasPrefix(e.Action, q.Action) {
			continue
		}
		if needle != "" {
			hay := strings.ToLower(e.Actor + " " + e.Action + " " + e.Target + " " + e.Detail + " " + e.IP)
			if !strings.Contains(hay, needle) {
				continue
			}
		}
		matched = append(matched, e)
	}
	if q.Limit <= 0 || q.Limit > 500 {
		q.Limit = 100
	}
	if q.Offset < 0 {
		q.Offset = 0
	}
	page := Page{Total: len(matched), Items: []Entry{}}
	if q.Offset < len(matched) {
		end := min(q.Offset+q.Limit, len(matched))
		page.Items = matched[q.Offset:end]
	}
	return page
}
