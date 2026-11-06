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

package msg

// Enterprise login metadata keys carried in Login.Metas.
const (
	// EntMetaAuthCode carries the per-device auth code.
	EntMetaAuthCode = "ent.auth_code"
	// EntMetaCaps declares that the client understands enterprise messages.
	// The server only sends EntSnapshot/EntNotice to clients that set it,
	// because older clients drop the connection on unknown message types.
	EntMetaCaps = "ent.caps"
	// EntCapsV1 is the current value of EntMetaCaps.
	EntCapsV1 = "1"
)

// Wire type identifiers for enterprise messages. They are kept far away from
// upstream identifiers to avoid collisions when merging upstream changes.
const (
	TypeEntSnapshot byte = 'E'
	TypeEntNotice   byte = 'N'

	V2TypeEntSnapshot uint16 = 1001
	V2TypeEntNotice   uint16 = 1002
)

// EntPort describes one port in use on the server.
type EntPort struct {
	Proto     string `json:"proto"`
	Port      int    `json:"port"`
	Mine      bool   `json:"mine,omitempty"`
	ProxyName string `json:"proxy_name,omitempty"`
}

// EntSnapshot is pushed by the server to enterprise clients after login and
// whenever the set of ports in use changes.
type EntSnapshot struct {
	// ClientIP is the address of the client as observed by the server.
	ClientIP string `json:"client_ip,omitempty"`
	// AllowPorts is a human readable description of the allowed port ranges,
	// e.g. "6000-7000,8080". Empty means unrestricted.
	AllowPorts string    `json:"allow_ports,omitempty"`
	Ports      []EntPort `json:"ports,omitempty"`
}

// EntNotice is a notification sent by the server administrator.
type EntNotice struct {
	ID      string `json:"id"`
	Level   string `json:"level,omitempty"`
	Title   string `json:"title,omitempty"`
	Content string `json:"content,omitempty"`
	// Time is a unix timestamp in seconds.
	Time int64 `json:"time"`
}
