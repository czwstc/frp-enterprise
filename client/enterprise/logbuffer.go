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
	"io"
	"strings"
	"sync"
	"time"

	"github.com/fatedier/frp/pkg/util/log"
)

// LogLine is one line of the frpc log.
type LogLine struct {
	Seq   uint64
	Time  time.Time
	Level string // "trace", "debug", "info", "warn" or "error"
	Text  string
}

// LogBuffer keeps the latest log lines in memory. It is an io.Writer and a
// log.LevelWriter, so it can be given to log.InitLoggerWithWriter.
type LogBuffer struct {
	mu    sync.Mutex
	lines []LogLine
	start int
	size  int
	seq   uint64
	// mirror receives a copy of each line when set, e.g. os.Stdout.
	mirror io.Writer
}

// NewLogBuffer creates a buffer that keeps up to capacity lines.
func NewLogBuffer(capacity int, mirror io.Writer) *LogBuffer {
	if capacity <= 0 {
		capacity = 2000
	}
	return &LogBuffer{lines: make([]LogLine, capacity), mirror: mirror}
}

// Write implements io.Writer, for lines without a known level.
func (b *LogBuffer) Write(p []byte) (int, error) {
	b.add(p, "", time.Now())
	return len(p), nil
}

// WriteLog implements log.LevelWriter.
func (b *LogBuffer) WriteLog(p []byte, level log.Level, when time.Time) (int, error) {
	b.add(p, levelName(level), when)
	return len(p), nil
}

func levelName(l log.Level) string {
	switch l {
	case log.TraceLevel:
		return "trace"
	case log.DebugLevel:
		return "debug"
	case log.WarnLevel:
		return "warn"
	case log.ErrorLevel:
		return "error"
	default:
		return "info"
	}
}

// trimHeader removes the "2006-01-02 15:04:05.000 [I] " header that the
// logger writes, since the buffer keeps the time and level itself.
func trimHeader(s string) string {
	if len(s) > 24 && s[4] == '-' && s[10] == ' ' && s[23] == ' ' {
		s = s[24:]
		if len(s) > 4 && s[0] == '[' && s[2] == ']' && s[3] == ' ' {
			s = s[4:]
		}
	}
	return s
}

func (b *LogBuffer) add(p []byte, level string, when time.Time) {
	if b.mirror != nil {
		_, _ = b.mirror.Write(p)
	}
	for _, raw := range bytes.Split(bytes.TrimRight(p, "\n"), []byte("\n")) {
		text := trimHeader(strings.TrimRight(string(raw), "\r"))
		lvl := level
		if lvl == "" {
			lvl = "info"
		}
		b.mu.Lock()
		b.seq++
		line := LogLine{Seq: b.seq, Time: when, Level: lvl, Text: text}
		capacity := len(b.lines)
		if b.size < capacity {
			b.lines[(b.start+b.size)%capacity] = line
			b.size++
		} else {
			b.lines[b.start] = line
			b.start = (b.start + 1) % capacity
		}
		b.mu.Unlock()
	}
}

// Lines returns the buffered lines, oldest first.
func (b *LogBuffer) Lines() []LogLine {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]LogLine, b.size)
	for i := range b.size {
		out[i] = b.lines[(b.start+i)%len(b.lines)]
	}
	return out
}

// LastSeq returns the sequence number of the latest line, 0 when empty.
func (b *LogBuffer) LastSeq() uint64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.seq
}

// Clear removes all lines.
func (b *LogBuffer) Clear() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.start, b.size = 0, 0
}
