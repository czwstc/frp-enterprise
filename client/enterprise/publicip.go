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
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const publicIPRefresh = 5 * time.Minute

// publicIPState caches the answer of the external public IP service.
type publicIPState struct {
	url       string
	ip        string
	err       string
	fetchedAt time.Time
	fetching  bool
}

func (p PublicIPSetting) normalized() PublicIPSetting {
	switch p.Mode {
	case PublicIPAPI:
		if strings.TrimSpace(p.URL) == "" {
			p.URL = DefaultPublicIPAPI
		}
	case PublicIPOff:
	default:
		p.Mode = PublicIPServer
	}
	return p
}

// publicIPFromAPILocked returns the cached answer for u, and fetches a new
// one in the background when it is missing or old. The caller holds m.mu.
func (m *Manager) publicIPFromAPILocked(u string) (string, string) {
	s := &m.publicIP
	if s.url != u {
		*s = publicIPState{url: u}
	}
	if !s.fetching && time.Since(s.fetchedAt) > publicIPRefresh {
		s.fetching = true
		go m.fetchPublicIP(u)
	}
	if s.ip == "" && s.err == "" {
		return "", "正在查询…"
	}
	return s.ip, s.err
}

func (m *Manager) fetchPublicIP(u string) {
	ip, err := fetchPublicIP(context.Background(), u)
	m.mu.Lock()
	if m.publicIP.url == u {
		m.publicIP.fetching = false
		m.publicIP.fetchedAt = time.Now()
		if err != nil {
			m.publicIP.err = "查询失败：" + err.Error()
		} else {
			m.publicIP.ip, m.publicIP.err = ip, ""
		}
	}
	m.mu.Unlock()
	m.changed()
}

// fetchPublicIP asks u for the caller's IP. The service may answer with the
// IP in plain text, as api.ipify.org does, or in JSON under "ip", "query"
// or "origin".
func fetchPublicIP(ctx context.Context, u string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "text/plain, application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if err != nil {
		return "", err
	}
	text := strings.TrimSpace(string(body))
	if strings.HasPrefix(text, "{") {
		var obj map[string]any
		if err := json.Unmarshal([]byte(text), &obj); err != nil {
			return "", errors.New("无法解析返回的 JSON")
		}
		text = ""
		for _, k := range []string{"ip", "query", "origin"} {
			if v, ok := obj[k].(string); ok {
				text = strings.TrimSpace(strings.Split(v, ",")[0])
				break
			}
		}
	}
	if net.ParseIP(text) == nil {
		return "", errors.New("返回的不是 IP 地址")
	}
	return text, nil
}

// SetPublicIPSource changes where the public IP comes from.
func (m *Manager) SetPublicIPSource(mode, rawURL string) error {
	p := PublicIPSetting{Mode: mode, URL: strings.TrimSpace(rawURL)}
	switch mode {
	case PublicIPServer, PublicIPOff:
		p.URL = ""
	case PublicIPAPI:
		if p.URL == "" {
			p.URL = DefaultPublicIPAPI
		}
		u, err := url.Parse(p.URL)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return errors.New("查询地址必须以 http:// 或 https:// 开头")
		}
	default:
		return errors.New("公网 IP 来源无效")
	}
	m.mu.Lock()
	err := m.updateSettingsLocked(func(s *Settings) error {
		s.PublicIP = p
		return nil
	})
	m.publicIP = publicIPState{}
	m.mu.Unlock()
	m.changed()
	return err
}
