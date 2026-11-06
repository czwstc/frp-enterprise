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
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	httppkg "github.com/fatedier/frp/pkg/util/http"
	"github.com/fatedier/frp/server/enterprise"
)

// AuditFunc records an action done through the dashboard.
type AuditFunc func(r *http.Request, action, target, detail string)

// EnterpriseController serves the enterprise auth code and notice APIs.
type EnterpriseController struct {
	hub   *enterprise.Hub
	audit AuditFunc
}

// NewEnterpriseController creates a controller. hub may be nil when the
// enterprise feature is disabled, and audit nil when nothing is recorded.
func NewEnterpriseController(hub *enterprise.Hub, audit AuditFunc) *EnterpriseController {
	if audit == nil {
		audit = func(*http.Request, string, string, string) {}
	}
	return &EnterpriseController{hub: hub, audit: audit}
}

// EnterpriseStatus tells the dashboard whether the feature is enabled.
type EnterpriseStatus struct {
	Enabled bool `json:"enabled"`
	Pending int  `json:"pending"`
}

func (c *EnterpriseController) requireHub() error {
	if c.hub == nil {
		return httppkg.NewError(http.StatusNotFound, "enterprise login is not enabled, set enterprise.enabled = true in frps config")
	}
	return nil
}

func entError(err error) error {
	if errors.Is(err, enterprise.ErrNotFound) {
		return httppkg.NewError(http.StatusNotFound, err.Error())
	}
	return httppkg.NewError(http.StatusBadRequest, err.Error())
}

// APIStatus handles GET /api/enterprise/status.
func (c *EnterpriseController) APIStatus(*httppkg.Context) (any, error) {
	if c.hub == nil {
		return EnterpriseStatus{}, nil
	}
	return EnterpriseStatus{Enabled: true, Pending: c.hub.Store().CountByStatus(enterprise.StatusPending)}, nil
}

// APIListCodes handles GET /api/enterprise/codes?status=.
func (c *EnterpriseController) APIListCodes(ctx *httppkg.Context) (any, error) {
	if err := c.requireHub(); err != nil {
		return nil, err
	}
	return c.hub.ListCodes(strings.ToLower(ctx.Query("status"))), nil
}

type generateCodeReq struct {
	Note string `json:"note"`
}

// APIGenerateCode handles POST /api/enterprise/codes.
func (c *EnterpriseController) APIGenerateCode(ctx *httppkg.Context) (any, error) {
	if err := c.requireHub(); err != nil {
		return nil, err
	}
	var req generateCodeReq
	if body, _ := ctx.Body(); len(strings.TrimSpace(string(body))) > 0 {
		if err := json.Unmarshal(body, &req); err != nil {
			return nil, httppkg.NewError(http.StatusBadRequest, "invalid request body: "+err.Error())
		}
	}
	code, err := c.hub.GenerateCode(req.Note)
	if err != nil {
		return nil, err
	}
	c.audit(ctx.Req, "code.generate", code.Code, code.Note)
	return code, nil
}

// APICodeAction handles POST /api/enterprise/codes/{code}/{action}.
func (c *EnterpriseController) APICodeAction(ctx *httppkg.Context) (any, error) {
	if err := c.requireHub(); err != nil {
		return nil, err
	}
	code := ctx.Param("code")
	var (
		res enterprise.AuthCode
		err error
	)
	switch ctx.Param("action") {
	case "approve":
		res, err = c.hub.Approve(code)
	case "reject":
		res, err = c.hub.Reject(code)
	case "revoke":
		res, err = c.hub.Revoke(code)
	case "unbind":
		res, err = c.hub.Unbind(code)
	case "note":
		var req generateCodeReq
		if err := ctx.BindJSON(&req); err != nil {
			return nil, httppkg.NewError(http.StatusBadRequest, "invalid request body: "+err.Error())
		}
		res, err = c.hub.SetNote(code, req.Note)
	default:
		return nil, httppkg.NewError(http.StatusNotFound, "unknown action")
	}
	if err != nil {
		return nil, entError(err)
	}
	c.audit(ctx.Req, "code."+ctx.Param("action"), res.Code, res.Hostname)
	return res, nil
}

// APIDeleteCode handles DELETE /api/enterprise/codes/{code}.
func (c *EnterpriseController) APIDeleteCode(ctx *httppkg.Context) (any, error) {
	if err := c.requireHub(); err != nil {
		return nil, err
	}
	if err := c.hub.Delete(ctx.Param("code")); err != nil {
		return nil, entError(err)
	}
	c.audit(ctx.Req, "code.delete", ctx.Param("code"), "")
	return map[string]string{"code": ctx.Param("code")}, nil
}

type sendNoticeReq struct {
	Level   string `json:"level"`
	Title   string `json:"title"`
	Content string `json:"content"`
	Target  string `json:"target"`
}

// APIListNotices handles GET /api/enterprise/notices.
func (c *EnterpriseController) APIListNotices(*httppkg.Context) (any, error) {
	if err := c.requireHub(); err != nil {
		return nil, err
	}
	return c.hub.Notices(), nil
}

// APISendNotice handles POST /api/enterprise/notices.
func (c *EnterpriseController) APISendNotice(ctx *httppkg.Context) (any, error) {
	if err := c.requireHub(); err != nil {
		return nil, err
	}
	var req sendNoticeReq
	if err := ctx.BindJSON(&req); err != nil {
		return nil, httppkg.NewError(http.StatusBadRequest, "invalid request body: "+err.Error())
	}
	n, err := c.hub.SendNotice(req.Level, req.Title, req.Content, req.Target)
	if err != nil {
		return nil, entError(err)
	}
	target := n.Target
	if target == "" {
		target = "all"
	}
	c.audit(ctx.Req, "notice.send", target, n.Title)
	return n, nil
}

// APIPorts handles GET /api/enterprise/ports.
func (c *EnterpriseController) APIPorts(*httppkg.Context) (any, error) {
	if err := c.requireHub(); err != nil {
		return nil, err
	}
	return c.hub.Ports(), nil
}
