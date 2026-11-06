// Copyright 2023 The frp Authors
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

package validation

import (
	"fmt"
	"net/url"
	"slices"
	"strings"

	"github.com/samber/lo"

	v1 "github.com/fatedier/frp/pkg/config/v1"
)

func (v *ConfigValidator) ValidateServerConfig(c *v1.ServerConfig) (Warning, error) {
	var (
		warnings Warning
		errs     error
	)
	if !slices.Contains(SupportedAuthMethods, c.Auth.Method) {
		errs = AppendError(errs, fmt.Errorf("invalid auth method, optional values are %v", SupportedAuthMethods))
	}
	if !lo.Every(SupportedAuthAdditionalScopes, c.Auth.AdditionalScopes) {
		errs = AppendError(errs, fmt.Errorf("invalid auth additional scopes, optional values are %v", SupportedAuthAdditionalScopes))
	}

	errs = AppendError(errs, v.validateAuthTokenSource(c.Auth.Token, c.Auth.TokenSource))

	if err := validateLogConfig(&c.Log); err != nil {
		errs = AppendError(errs, err)
	}

	if err := validateWebServerConfig(&c.WebServer); err != nil {
		errs = AppendError(errs, err)
	}
	if err := validateMetricsServerConfig(&c.MetricsServer, &c.WebServer); err != nil {
		errs = AppendError(errs, err)
	}
	if c.Transport.TLS.UseSystemCa {
		errs = AppendError(errs, fmt.Errorf("transport.tls.useSystemCa is only supported in frpc transport.tls"))
	}

	errs = AppendError(errs, ValidatePort(c.BindPort, "bindPort"))
	errs = AppendError(errs, ValidatePort(c.KCPBindPort, "kcpBindPort"))
	errs = AppendError(errs, ValidatePort(c.QUICBindPort, "quicBindPort"))
	errs = AppendError(errs, ValidatePort(c.VhostHTTPPort, "vhostHTTPPort"))
	errs = AppendError(errs, ValidatePort(c.VhostHTTPSPort, "vhostHTTPSPort"))
	errs = AppendError(errs, ValidatePort(c.TCPMuxHTTPConnectPort, "tcpMuxHTTPConnectPort"))
	if c.Transport.MaxPoolCount < 0 {
		errs = AppendError(errs, fmt.Errorf("invalid transport.maxPoolCount, must be non-negative"))
	}

	errs = AppendError(errs, validateAdminAuthConfig(c))

	for _, p := range c.HTTPPlugins {
		if !lo.Every(SupportedHTTPPluginOps, p.Ops) {
			errs = AppendError(errs, fmt.Errorf("invalid http plugin ops, optional values are %v", SupportedHTTPPluginOps))
		}
	}
	return warnings, errs
}

func validateAdminAuthConfig(c *v1.ServerConfig) error {
	a := &c.Enterprise.Admin
	if !a.Enabled {
		return nil
	}
	var errs error
	if c.WebServer.Port <= 0 {
		errs = AppendError(errs, fmt.Errorf("enterprise.admin requires webServer.port"))
	}
	// A missing environment variable in {{ .Envs.X }} renders as "<no value>",
	// which must never become a password.
	for field, v := range map[string]string{
		"localUser.username":     a.LocalUser.Username,
		"localUser.password":     a.LocalUser.Password,
		"localUser.passwordHash": a.LocalUser.PasswordHash,
		"oidc.issuer":            a.OIDC.Issuer,
		"oidc.clientID":          a.OIDC.ClientID,
		"oidc.clientSecret":      a.OIDC.ClientSecret,
		"oidc.redirectURL":       a.OIDC.RedirectURL,
		"oidc.displayName":       a.OIDC.DisplayName,
	} {
		if strings.Contains(v, "<no value>") {
			errs = AppendError(errs, fmt.Errorf("enterprise.admin.%s comes from an environment variable that is not set", field))
		}
	}
	if a.LocalUser.Password == "" && a.LocalUser.PasswordHash == "" {
		errs = AppendError(errs, fmt.Errorf("enterprise.admin.localUser.password or passwordHash is required"))
	}
	if a.LocalUser.Password != "" && len(a.LocalUser.Password) < 8 {
		errs = AppendError(errs, fmt.Errorf("enterprise.admin.localUser.password must be at least 8 characters"))
	}
	if o := &a.OIDC; o.OIDCEnabled() {
		if o.ClientID == "" {
			errs = AppendError(errs, fmt.Errorf("enterprise.admin.oidc.clientID is required"))
		}
		if o.RedirectURL == "" {
			errs = AppendError(errs, fmt.Errorf("enterprise.admin.oidc.redirectURL is required"))
		} else if u, err := url.Parse(o.RedirectURL); err != nil || u.Path == "" || u.Path == "/" || (u.Scheme != "http" && u.Scheme != "https") {
			errs = AppendError(errs, fmt.Errorf("enterprise.admin.oidc.redirectURL must be an absolute http(s) URL with a path"))
		}
	}
	return errs
}
