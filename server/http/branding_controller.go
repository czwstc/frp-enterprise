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
	"net/http"

	httppkg "github.com/fatedier/frp/pkg/util/http"
	"github.com/fatedier/frp/server/adminauth"
	"github.com/fatedier/frp/server/branding"
)

// BrandingController serves the dashboard's settings page.
type BrandingController struct {
	store *branding.Store
	audit AuditFunc
}

// NewBrandingController creates a controller.
func NewBrandingController(store *branding.Store, audit AuditFunc) *BrandingController {
	if audit == nil {
		audit = func(*http.Request, string, string, string) {}
	}
	return &BrandingController{store: store, audit: audit}
}

// APIGet handles GET /api/branding, which needs no session.
func (c *BrandingController) APIGet(*httppkg.Context) (any, error) {
	return c.store.Get(), nil
}

// APIUpdate handles PUT /api/branding.
func (c *BrandingController) APIUpdate(ctx *httppkg.Context) (any, error) {
	var req branding.Settings
	if err := ctx.BindJSON(&req); err != nil {
		return nil, httppkg.NewError(http.StatusBadRequest, "invalid request body: "+err.Error())
	}
	actor := adminauth.Actor(ctx.Req)
	if actor == "" {
		actor = "dashboard"
	}
	saved, err := c.store.Update(req, actor)
	if err != nil {
		return nil, httppkg.NewError(http.StatusBadRequest, err.Error())
	}
	c.audit(ctx.Req, "settings.branding", saved.SiteName, "theme "+saved.Theme+", accent "+saved.Accent)
	return saved, nil
}
