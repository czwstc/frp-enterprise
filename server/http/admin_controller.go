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

package http

import (
	"strconv"
	"strings"

	httppkg "github.com/fatedier/frp/pkg/util/http"
	"github.com/fatedier/frp/pkg/util/log"
	"github.com/fatedier/frp/server/audit"
)

// LogController serves the audit log and the runtime log of frps.
type LogController struct {
	audit *audit.Logger
}

// NewLogController creates a controller.
func NewLogController(a *audit.Logger) *LogController {
	return &LogController{audit: a}
}

func queryInt(ctx *httppkg.Context, name string, def int) int {
	if v, err := strconv.Atoi(ctx.Query(name)); err == nil {
		return v
	}
	return def
}

// APIAudit handles GET /api/admin/audit?q=&action=&offset=&limit=.
func (c *LogController) APIAudit(ctx *httppkg.Context) (any, error) {
	return c.audit.List(audit.Query{
		Q:      ctx.Query("q"),
		Action: ctx.Query("action"),
		Offset: queryInt(ctx, "offset", 0),
		Limit:  queryInt(ctx, "limit", 50),
	}), nil
}

// RuntimeLogLine is one line of the frps log.
type RuntimeLogLine struct {
	Seq   uint64 `json:"seq"`
	Time  int64  `json:"time"`
	Level string `json:"level"`
	Text  string `json:"text"`
}

// APIRuntimeLogs handles GET /api/admin/logs?level=&q=&limit=, newest last.
func (c *LogController) APIRuntimeLogs(ctx *httppkg.Context) (any, error) {
	buf := log.Memory()
	if buf == nil {
		return []RuntimeLogLine{}, nil
	}
	minLevel := map[string]int{"": 0, "trace": 0, "debug": 1, "info": 2, "warn": 3, "error": 4}
	rank := map[string]int{"trace": 0, "debug": 1, "info": 2, "warn": 3, "error": 4}
	want := minLevel[ctx.Query("level")]
	q := strings.ToLower(ctx.Query("q"))
	limit := queryInt(ctx, "limit", 500)
	if limit <= 0 || limit > 2000 {
		limit = 500
	}
	lines := buf.Lines()
	out := make([]RuntimeLogLine, 0, len(lines))
	for _, l := range lines {
		if rank[l.Level] < want {
			continue
		}
		if q != "" && !strings.Contains(strings.ToLower(l.Text), q) {
			continue
		}
		out = append(out, RuntimeLogLine{Seq: l.Seq, Time: l.Time.UnixMilli(), Level: l.Level, Text: l.Text})
	}
	if len(out) > limit {
		out = out[len(out)-limit:]
	}
	return out, nil
}
