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

package v1

import (
	"github.com/samber/lo"

	"github.com/fatedier/frp/pkg/config/types"
	"github.com/fatedier/frp/pkg/util/util"
)

type ServerConfig struct {
	APIMetadata

	Auth AuthServerConfig `json:"auth,omitempty"`
	// BindAddr specifies the address that the server binds to. By default,
	// this value is "0.0.0.0".
	BindAddr string `json:"bindAddr,omitempty"`
	// BindPort specifies the port that the server listens on. By default, this
	// value is 7000.
	BindPort int `json:"bindPort,omitempty"`
	// KCPBindPort specifies the KCP port that the server listens on. If this
	// value is 0, the server will not listen for KCP connections.
	KCPBindPort int `json:"kcpBindPort,omitempty"`
	// QUICBindPort specifies the QUIC port that the server listens on.
	// Set this value to 0 will disable this feature.
	QUICBindPort int `json:"quicBindPort,omitempty"`
	// ProxyBindAddr specifies the address that the proxy binds to. This value
	// may be the same as BindAddr.
	ProxyBindAddr string `json:"proxyBindAddr,omitempty"`
	// VhostHTTPPort specifies the port that the server listens for HTTP Vhost
	// requests. If this value is 0, the server will not listen for HTTP
	// requests.
	VhostHTTPPort int `json:"vhostHTTPPort,omitempty"`
	// VhostHTTPTimeout specifies the response header timeout for the Vhost
	// HTTP server, in seconds. By default, this value is 60.
	VhostHTTPTimeout int64 `json:"vhostHTTPTimeout,omitempty"`
	// VhostHTTPSPort specifies the port that the server listens for HTTPS
	// Vhost requests. If this value is 0, the server will not listen for HTTPS
	// requests.
	VhostHTTPSPort int `json:"vhostHTTPSPort,omitempty"`
	// TCPMuxHTTPConnectPort specifies the port that the server listens for TCP
	// HTTP CONNECT requests. If the value is 0, the server will not multiplex TCP
	// requests on one single port. If it's not - it will listen on this value for
	// HTTP CONNECT requests.
	TCPMuxHTTPConnectPort int `json:"tcpmuxHTTPConnectPort,omitempty"`
	// If TCPMuxPassthrough is true, frps won't do any update on traffic.
	TCPMuxPassthrough bool `json:"tcpmuxPassthrough,omitempty"`
	// SubDomainHost specifies the domain that will be attached to sub-domains
	// requested by the client when using Vhost proxying. For example, if this
	// value is set to "frps.com" and the client requested the subdomain
	// "test", the resulting URL would be "test.frps.com".
	SubDomainHost string `json:"subDomainHost,omitempty"`
	// Custom404Page specifies a path to a custom 404 page to display. If this
	// value is "", a default page will be displayed.
	Custom404Page string `json:"custom404Page,omitempty"`

	SSHTunnelGateway SSHTunnelGateway `json:"sshTunnelGateway,omitempty"`

	WebServer     WebServerConfig     `json:"webServer,omitempty"`
	MetricsServer MetricsServerConfig `json:"metricsServer,omitempty"`
	// EnablePrometheus will export prometheus metrics on webserver address
	// in /metrics api.
	EnablePrometheus bool `json:"enablePrometheus,omitempty"`

	Log LogConfig `json:"log,omitempty"`

	Transport ServerTransportConfig `json:"transport,omitempty"`

	// DetailedErrorsToClient defines whether to send the specific error (with
	// debug info) to frpc. By default, this value is true.
	DetailedErrorsToClient *bool `json:"detailedErrorsToClient,omitempty"`
	// MaxPortsPerClient specifies the maximum number of ports a single client
	// may proxy to. If this value is 0, no limit will be applied.
	MaxPortsPerClient int64 `json:"maxPortsPerClient,omitempty"`
	// UserConnTimeout specifies the maximum time to wait for a work
	// connection. By default, this value is 10.
	UserConnTimeout int64 `json:"userConnTimeout,omitempty"`
	// UDPPacketSize specifies the UDP packet size
	// By default, this value is 1500
	UDPPacketSize int64 `json:"udpPacketSize,omitempty"`
	// NatHoleAnalysisDataReserveHours specifies the hours to reserve nat hole analysis data.
	NatHoleAnalysisDataReserveHours int64 `json:"natholeAnalysisDataReserveHours,omitempty"`

	AllowPorts []types.PortsRange `json:"allowPorts,omitempty"`

	HTTPPlugins []HTTPPluginOptions `json:"httpPlugins,omitempty"`

	// Enterprise enables per-device auth code login and approval.
	Enterprise ServerEnterpriseConfig `json:"enterprise,omitempty"`
}

// ServerEnterpriseConfig configures the enterprise login of frps.
type ServerEnterpriseConfig struct {
	// Enabled requires every client to log in with an approved auth code.
	Enabled bool `json:"enabled,omitempty"`
	// StorePath is the JSON file holding auth codes and notices. By default,
	// this value is "./frps_enterprise.json".
	StorePath string `json:"storePath,omitempty"`
	// BrandingPath is the JSON file holding the dashboard's and the clients'
	// look, set on the dashboard's settings page. By default, this value is
	// "./frps_branding.json".
	BrandingPath string `json:"brandingPath,omitempty"`
	// Admin replaces the dashboard's basic auth with user accounts: one local
	// user protected by TOTP, and OIDC users approved by an administrator.
	Admin AdminAuthConfig `json:"admin,omitempty"`
}

// AdminAuthConfig configures the dashboard's user accounts.
type AdminAuthConfig struct {
	// Enabled turns on dashboard accounts. It requires webServer.port.
	Enabled bool `json:"enabled,omitempty"`
	// StorePath is the JSON file holding users and the local user's second
	// factor. By default, this value is "./frps_admin.json".
	StorePath string `json:"storePath,omitempty"`
	// AuditLogPath is the JSON-lines audit log. By default, this value is
	// "./frps_audit.log".
	AuditLogPath string `json:"auditLogPath,omitempty"`
	// SessionTTLHours is how long a dashboard session lasts. By default, 12.
	SessionTTLHours int `json:"sessionTTLHours,omitempty"`
	// TOTPIssuer is the name authenticator apps show. By default,
	// "FRP Enterprise".
	TOTPIssuer string `json:"totpIssuer,omitempty"`
	// LocalUser is the single local account.
	LocalUser AdminLocalUserConfig `json:"localUser,omitempty"`
	// OIDC lets the users of an OpenID Connect provider sign in.
	OIDC AdminOIDCConfig `json:"oidc,omitempty"`
}

// AdminLocalUserConfig is the single local dashboard account.
type AdminLocalUserConfig struct {
	// Username of the local account. By default, "admin".
	Username string `json:"username,omitempty"`
	// Password in clear text, or PasswordHash as a bcrypt hash.
	Password     string `json:"password,omitempty"`
	PasswordHash string `json:"passwordHash,omitempty"`
}

// AdminOIDCConfig configures sign-in with an OpenID Connect provider.
type AdminOIDCConfig struct {
	// Issuer is the provider URL; its /.well-known/openid-configuration is
	// read at startup. OIDC sign-in is off when it is empty.
	Issuer       string `json:"issuer,omitempty"`
	ClientID     string `json:"clientID,omitempty"`
	ClientSecret string `json:"clientSecret,omitempty"`
	// RedirectURL is the callback URL registered at the provider. frps
	// serves the callback at its path.
	RedirectURL string `json:"redirectURL,omitempty"`
	// Scopes requested. By default, ["openid", "email"].
	Scopes []string `json:"scopes,omitempty"`
	// UsernameClaim names the claim matched against user names. By default,
	// the first of "preferred_username", "username" and "email" present.
	UsernameClaim string `json:"usernameClaim,omitempty"`
	// DisplayName labels the sign-in button. By default, "SSO".
	DisplayName string `json:"displayName,omitempty"`
	// TrustedCaFile adds a CA to verify the provider's certificate.
	TrustedCaFile string `json:"trustedCaFile,omitempty"`
	// InsecureSkipVerify disables verification of the provider's certificate.
	InsecureSkipVerify bool `json:"insecureSkipVerify,omitempty"`
}

// OIDCEnabled reports whether OIDC sign-in is configured.
func (c *AdminOIDCConfig) OIDCEnabled() bool { return c.Issuer != "" }

func (c *ServerConfig) Complete() error {
	if err := c.Auth.Complete(); err != nil {
		return err
	}
	c.Log.Complete()
	c.Transport.Complete()
	c.WebServer.Complete()
	c.MetricsServer.Complete()
	c.SSHTunnelGateway.Complete()
	if c.Enterprise.Enabled {
		c.Enterprise.StorePath = util.EmptyOr(c.Enterprise.StorePath, "./frps_enterprise.json")
	}
	c.Enterprise.BrandingPath = util.EmptyOr(c.Enterprise.BrandingPath, "./frps_branding.json")
	if a := &c.Enterprise.Admin; a.Enabled {
		a.StorePath = util.EmptyOr(a.StorePath, "./frps_admin.json")
		a.AuditLogPath = util.EmptyOr(a.AuditLogPath, "./frps_audit.log")
		a.SessionTTLHours = util.EmptyOr(a.SessionTTLHours, 12)
		a.TOTPIssuer = util.EmptyOr(a.TOTPIssuer, "FRP Enterprise")
		a.LocalUser.Username = util.EmptyOr(a.LocalUser.Username, "admin")
		if a.OIDC.OIDCEnabled() {
			if len(a.OIDC.Scopes) == 0 {
				a.OIDC.Scopes = []string{"openid", "email"}
			}
			a.OIDC.DisplayName = util.EmptyOr(a.OIDC.DisplayName, "SSO")
		}
	}

	c.BindAddr = util.EmptyOr(c.BindAddr, "0.0.0.0")
	c.BindPort = util.EmptyOr(c.BindPort, 7000)
	if c.ProxyBindAddr == "" {
		c.ProxyBindAddr = c.BindAddr
	}

	if c.WebServer.Port > 0 {
		c.WebServer.Addr = util.EmptyOr(c.WebServer.Addr, "0.0.0.0")
	}

	c.VhostHTTPTimeout = util.EmptyOr(c.VhostHTTPTimeout, 60)
	c.DetailedErrorsToClient = util.EmptyOr(c.DetailedErrorsToClient, new(true))
	c.UserConnTimeout = util.EmptyOr(c.UserConnTimeout, 10)
	c.UDPPacketSize = util.EmptyOr(c.UDPPacketSize, 1500)
	c.NatHoleAnalysisDataReserveHours = util.EmptyOr(c.NatHoleAnalysisDataReserveHours, 7*24)
	return nil
}

type AuthServerConfig struct {
	Method           AuthMethod           `json:"method,omitempty"`
	AdditionalScopes []AuthScope          `json:"additionalScopes,omitempty"`
	Token            string               `json:"token,omitempty"`
	TokenSource      *ValueSource         `json:"tokenSource,omitempty"`
	OIDC             AuthOIDCServerConfig `json:"oidc,omitempty"`
}

func (c *AuthServerConfig) Complete() error {
	c.Method = util.EmptyOr(c.Method, "token")
	return nil
}

type AuthOIDCServerConfig struct {
	// Issuer specifies the issuer to verify OIDC tokens with. This issuer
	// will be used to load public keys to verify signature and will be compared
	// with the issuer claim in the OIDC token.
	Issuer string `json:"issuer,omitempty"`
	// Audience specifies the audience OIDC tokens should contain when validated.
	// If this value is empty, audience ("client ID") verification will be skipped.
	Audience string `json:"audience,omitempty"`
	// SkipExpiryCheck specifies whether to skip checking if the OIDC token is
	// expired.
	SkipExpiryCheck bool `json:"skipExpiryCheck,omitempty"`
	// SkipIssuerCheck specifies whether to skip checking if the OIDC token's
	// issuer claim matches the issuer specified in OidcIssuer.
	SkipIssuerCheck bool `json:"skipIssuerCheck,omitempty"`
}

type ServerTransportConfig struct {
	// TCPMux toggles TCP stream multiplexing. This allows multiple requests
	// from a client to share a single TCP connection. By default, this value
	// is true.
	// $HideFromDoc
	TCPMux *bool `json:"tcpMux,omitempty"`
	// TCPMuxKeepaliveInterval specifies the keep alive interval for TCP stream multiplier.
	// If TCPMux is true, heartbeat of application layer is unnecessary because it can only rely on heartbeat in TCPMux.
	TCPMuxKeepaliveInterval int64 `json:"tcpMuxKeepaliveInterval,omitempty"`
	// TCPKeepAlive specifies the interval between keep-alive probes for an active network connection between frpc and frps.
	// If negative, keep-alive probes are disabled.
	TCPKeepAlive int64 `json:"tcpKeepalive,omitempty"`
	// MaxPoolCount specifies the maximum pool size for each proxy. By default,
	// this value is 5. Negative values are invalid.
	MaxPoolCount int64 `json:"maxPoolCount,omitempty"`
	// HeartBeatTimeout specifies the maximum time to wait for a heartbeat
	// before terminating the connection. It is not recommended to change this
	// value. By default, this value is 90. Set negative value to disable it.
	HeartbeatTimeout int64 `json:"heartbeatTimeout,omitempty"`
	// QUIC options.
	QUIC QUICOptions `json:"quic,omitempty"`
	// TLS specifies TLS settings for the connection from the client.
	TLS TLSServerConfig `json:"tls,omitempty"`
}

func (c *ServerTransportConfig) Complete() {
	c.TCPMux = util.EmptyOr(c.TCPMux, new(true))
	c.TCPMuxKeepaliveInterval = util.EmptyOr(c.TCPMuxKeepaliveInterval, 30)
	c.TCPKeepAlive = util.EmptyOr(c.TCPKeepAlive, 7200)
	c.MaxPoolCount = util.EmptyOr(c.MaxPoolCount, 5)
	if lo.FromPtr(c.TCPMux) {
		// If TCPMux is enabled, heartbeat of application layer is unnecessary because we can rely on heartbeat in tcpmux.
		c.HeartbeatTimeout = util.EmptyOr(c.HeartbeatTimeout, -1)
	} else {
		c.HeartbeatTimeout = util.EmptyOr(c.HeartbeatTimeout, 90)
	}
	c.QUIC.Complete()
	if c.TLS.TrustedCaFile != "" {
		c.TLS.Force = true
	}
}

type TLSServerConfig struct {
	// Force specifies whether to only accept TLS-encrypted connections.
	Force bool `json:"force,omitempty"`

	TLSConfig
}

type SSHTunnelGateway struct {
	BindPort              int    `json:"bindPort,omitempty"`
	PrivateKeyFile        string `json:"privateKeyFile,omitempty"`
	AutoGenPrivateKeyPath string `json:"autoGenPrivateKeyPath,omitempty"`
	AuthorizedKeysFile    string `json:"authorizedKeysFile,omitempty"`
}

func (c *SSHTunnelGateway) Complete() {
	c.AutoGenPrivateKeyPath = util.EmptyOr(c.AutoGenPrivateKeyPath, "./.autogen_ssh_key")
}
