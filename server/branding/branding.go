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

// Package branding keeps the look of the frps dashboard and the desktop
// clients: the site name, the logo, the theme, the accent color and the link
// in the dashboard's top right corner.
package branding

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

// Link modes for the top right corner.
const (
	LinkCustom = "custom"
	LinkGitHub = "github"
	LinkNone   = "none"
)

// TopLink is the link in the dashboard's top right corner.
type TopLink struct {
	Mode  string `json:"mode"`
	URL   string `json:"url,omitempty"`
	Text  string `json:"text,omitempty"`
	Image string `json:"image,omitempty"`
}

// Settings is the branding of the dashboard and the clients.
type Settings struct {
	SiteName string `json:"siteName"`
	Subtitle string `json:"subtitle"`
	// Logo is a data URL of a PNG, JPEG, WebP or SVG image; empty uses the
	// built-in mark.
	Logo string `json:"logo,omitempty"`
	// LogoMode is "both" (logo and name) or "logo" (a wide logo alone).
	LogoMode string `json:"logoMode"`
	// LogoBackdrop keeps a light logo readable on a light background:
	// "halo" (a dark glow traced around it), "plate" (a dark rounded plate)
	// or "none".
	LogoBackdrop string `json:"logoBackdrop"`
	// Theme is "system", "light" or "dark".
	Theme string `json:"theme"`
	// Accent is a #rrggbb color.
	Accent    string    `json:"accent"`
	TopLink   TopLink   `json:"topLink"`
	Version   int64     `json:"version"`
	UpdatedBy string    `json:"updatedBy,omitempty"`
	UpdatedAt time.Time `json:"updatedAt,omitzero"`
}

// Defaults is the built-in Huiding branding.
func Defaults() Settings {
	return Settings{
		SiteName:     "HDIT FRPE",
		Subtitle:     "内网穿透控制台",
		LogoMode:     "logo",
		LogoBackdrop: "halo",
		Theme:        "system",
		Accent:       "#1f5eff",
		TopLink:      TopLink{Mode: LinkCustom, URL: "https://www.huiding-group.com/", Text: "汇鼎科技官网"},
	}
}

// MaxImageBytes limits an uploaded image, decoded.
const MaxImageBytes = 512 * 1024

var (
	hexColor = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)
	imageURL = regexp.MustCompile(`^data:image/(png|jpeg|webp|svg\+xml);base64,([A-Za-z0-9+/=]+)$`)
)

func checkText(field, s string, limit int) error {
	if utf8.RuneCountInString(s) > limit {
		return fmt.Errorf("%s不能超过 %d 个字符", field, limit)
	}
	if strings.ContainsAny(s, "\r\n<>") {
		return fmt.Errorf("%s不能包含换行或尖括号", field)
	}
	return nil
}

func checkImage(field, s string) error {
	if s == "" {
		return nil
	}
	m := imageURL.FindStringSubmatch(s)
	if m == nil {
		return fmt.Errorf("%s必须是 PNG、JPEG、WebP 或 SVG 图片", field)
	}
	raw, err := base64.StdEncoding.DecodeString(m[2])
	if err != nil {
		return fmt.Errorf("%s不是有效的图片数据", field)
	}
	if len(raw) > MaxImageBytes {
		return fmt.Errorf("%s不能超过 %d KB", field, MaxImageBytes/1024)
	}
	return nil
}

// Normalize trims the settings and checks them.
func (s *Settings) Normalize() error {
	s.SiteName = strings.TrimSpace(s.SiteName)
	s.Subtitle = strings.TrimSpace(s.Subtitle)
	s.TopLink.URL = strings.TrimSpace(s.TopLink.URL)
	s.TopLink.Text = strings.TrimSpace(s.TopLink.Text)
	s.Accent = strings.ToLower(strings.TrimSpace(s.Accent))
	if s.SiteName == "" {
		return errors.New("站点名称不能为空")
	}
	if err := checkText("站点名称", s.SiteName, 40); err != nil {
		return err
	}
	if err := checkText("副标题", s.Subtitle, 60); err != nil {
		return err
	}
	if err := checkText("链接提示文字", s.TopLink.Text, 30); err != nil {
		return err
	}
	if err := checkImage("左上角图标", s.Logo); err != nil {
		return err
	}
	if err := checkImage("右上角图标", s.TopLink.Image); err != nil {
		return err
	}
	switch s.LogoMode {
	case "both", "logo":
	case "":
		s.LogoMode = "both"
	default:
		return errors.New("图标显示方式无效")
	}
	switch s.LogoBackdrop {
	case "halo", "plate", "none":
	case "":
		s.LogoBackdrop = "halo"
	default:
		return errors.New("图标底衬无效")
	}
	switch s.Theme {
	case "system", "light", "dark":
	case "":
		s.Theme = "system"
	default:
		return errors.New("主题无效")
	}
	if !hexColor.MatchString(s.Accent) {
		return errors.New("强调色必须是 #rrggbb 格式")
	}
	switch s.TopLink.Mode {
	case LinkCustom:
		u, err := url.Parse(s.TopLink.URL)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return errors.New("链接地址必须以 http:// 或 https:// 开头")
		}
		if s.TopLink.Text == "" && s.TopLink.Image == "" {
			return errors.New("自定义链接需要提示文字或图标")
		}
	case LinkGitHub, LinkNone:
	default:
		return errors.New("右上角链接的显示方式无效")
	}
	return nil
}

// Store keeps the branding in a JSON file. An empty path keeps it in memory.
type Store struct {
	mu       sync.RWMutex
	path     string
	cur      Settings
	watchers []func(Settings)
}

// Open loads the branding from path, or starts from the defaults.
func Open(path string) (*Store, error) {
	s := &Store{path: path, cur: Defaults()}
	if path == "" {
		return s, nil
	}
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	var st Settings
	if err := json.Unmarshal(b, &st); err != nil {
		return nil, fmt.Errorf("parse branding %s: %w", path, err)
	}
	if err := st.Normalize(); err != nil {
		return nil, fmt.Errorf("branding %s: %w", path, err)
	}
	s.cur = st
	return s, nil
}

// Get returns the current branding.
func (s *Store) Get() Settings {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cur
}

// OnChange registers fn, called after every saved change.
func (s *Store) OnChange(fn func(Settings)) {
	s.mu.Lock()
	s.watchers = append(s.watchers, fn)
	s.mu.Unlock()
}

// Update checks and saves new branding.
func (s *Store) Update(next Settings, actor string) (Settings, error) {
	if err := next.Normalize(); err != nil {
		return Settings{}, err
	}
	s.mu.Lock()
	next.Version = s.cur.Version + 1
	next.UpdatedBy = actor
	next.UpdatedAt = time.Now()
	if err := s.saveLocked(next); err != nil {
		s.mu.Unlock()
		return Settings{}, err
	}
	s.cur = next
	watchers := append([]func(Settings){}, s.watchers...)
	s.mu.Unlock()
	for _, fn := range watchers {
		fn(next)
	}
	return next, nil
}

func (s *Store) saveLocked(st Settings) error {
	if s.path == "" {
		return nil
	}
	b, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".frps_branding-*.tmp")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(name, s.path)
}
