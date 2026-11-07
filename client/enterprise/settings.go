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

// Package enterprise runs an embedded frpc for the desktop client: it keeps
// the encrypted settings, the connection state, the logs and the data pushed
// by the server.
package enterprise

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"golang.org/x/crypto/scrypt"

	entpkg "github.com/fatedier/frp/pkg/enterprise"
	"github.com/fatedier/frp/pkg/util/util"
)

// ProxySetting is one port mapping configured in the client.
type ProxySetting struct {
	Name       string `json:"name"`
	Type       string `json:"type"`
	LocalIP    string `json:"localIP"`
	LocalPort  int    `json:"localPort"`
	RemotePort int    `json:"remotePort"`
}

// LockSetting holds the hashed UI lock password.
type LockSetting struct {
	Enabled bool   `json:"enabled"`
	Salt    []byte `json:"salt,omitempty"`
	Hash    []byte `json:"hash,omitempty"`
}

// Settings is everything the desktop client stores.
type Settings struct {
	ServerAddr string `json:"serverAddr"`
	ServerPort int    `json:"serverPort"`
	Token      string `json:"token"`
	AuthCode   string `json:"authCode"`
	// DeviceID identifies this installation; auth codes are bound to it.
	DeviceID string `json:"deviceID"`
	// AutoConnect starts the connection when the app opens.
	AutoConnect bool           `json:"autoConnect"`
	Proxies     []ProxySetting `json:"proxies"`
	Lock        LockSetting    `json:"lock"`
	// Branding is the look last received from the server.
	Branding Branding `json:"branding,omitzero"`
}

// Branding is the look the server's administrator chose.
type Branding struct {
	Version  int64  `json:"version"`
	SiteName string `json:"siteName,omitempty"`
	Accent   string `json:"accent,omitempty"`
	// Logo is a data URL, or empty for the built-in mark.
	Logo string `json:"logo,omitempty"`
}

// Clone returns a deep copy.
func (s Settings) Clone() Settings {
	out := s
	out.Proxies = append([]ProxySetting(nil), s.Proxies...)
	out.Lock.Salt = append([]byte(nil), s.Lock.Salt...)
	out.Lock.Hash = append([]byte(nil), s.Lock.Hash...)
	return out
}

var proxyNameRe = regexp.MustCompile(`^[A-Za-z0-9_-]{1,32}$`)

// Normalize fills defaults and validates a proxy setting.
func (p *ProxySetting) Normalize() error {
	p.Name = strings.TrimSpace(p.Name)
	p.Type = strings.ToLower(strings.TrimSpace(p.Type))
	p.LocalIP = strings.TrimSpace(p.LocalIP)
	if p.Type == "" {
		p.Type = "tcp"
	}
	if p.LocalIP == "" {
		p.LocalIP = "127.0.0.1"
	}
	if !proxyNameRe.MatchString(p.Name) {
		return errors.New("名称只能包含字母、数字、- 和 _，长度 1-32")
	}
	if p.Type != "tcp" && p.Type != "udp" {
		return errors.New("协议只能是 tcp 或 udp")
	}
	if p.LocalPort < 1 || p.LocalPort > 65535 {
		return errors.New("本地端口必须在 1-65535 之间")
	}
	if p.RemotePort < 0 || p.RemotePort > 65535 {
		return errors.New("远端端口必须在 0-65535 之间（0 表示由服务端分配）")
	}
	return nil
}

// ValidateConnection checks the fields needed to connect.
func (s *Settings) ValidateConnection() error {
	s.ServerAddr = strings.TrimSpace(s.ServerAddr)
	s.AuthCode = entpkg.NormalizeAuthCode(s.AuthCode)
	if s.ServerAddr == "" {
		return errors.New("请输入服务端地址")
	}
	if s.ServerPort < 1 || s.ServerPort > 65535 {
		return errors.New("服务端端口必须在 1-65535 之间")
	}
	if err := entpkg.ValidateAuthCode(s.AuthCode); err != nil {
		return fmt.Errorf("鉴权码无效：%v", err)
	}
	return nil
}

// UserPrefix is the frp user name of this device. frps prefixes proxy names
// with it, so that proxies of different devices never clash.
func (s *Settings) UserPrefix() string {
	id := s.DeviceID
	if len(id) > 8 {
		id = id[:8]
	}
	return "d" + id
}

const (
	scryptN      = 1 << 15
	scryptR      = 8
	scryptP      = 1
	keyLen       = 32
	settingsVer  = 1
	keyContext   = "frpc-enterprise-settings:"
	settingsName = "config.enc"
)

type encryptedFile struct {
	Version int    `json:"v"`
	Salt    []byte `json:"salt"`
	Nonce   []byte `json:"nonce"`
	Data    []byte `json:"data"`
}

// ErrSettingsUnreadable means the settings file exists but cannot be
// decrypted, for example after it was copied to another machine.
var ErrSettingsUnreadable = errors.New("settings file cannot be decrypted on this machine")

// SettingsStore reads and writes the settings, encrypted with a key bound to
// this machine.
type SettingsStore struct {
	path      string
	machineID func() string
}

// DefaultSettingsPath returns the settings file in the user config directory.
func DefaultSettingsPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "frpc-enterprise", settingsName), nil
}

// NewSettingsStore creates a store for path. machineID may be nil to use
// the machine ID of the system.
func NewSettingsStore(path string, machineID func() string) *SettingsStore {
	if machineID == nil {
		machineID = MachineID
	}
	return &SettingsStore{path: path, machineID: machineID}
}

// Path returns the settings file path.
func (s *SettingsStore) Path() string { return s.path }

func (s *SettingsStore) key(salt []byte) ([]byte, error) {
	return scrypt.Key([]byte(keyContext+s.machineID()), salt, scryptN, scryptR, scryptP, keyLen)
}

// Load reads the settings. A missing file gives empty settings with a new
// device ID.
func (s *SettingsStore) Load() (Settings, error) {
	b, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return newSettings()
	}
	if err != nil {
		return Settings{}, err
	}
	var f encryptedFile
	if err := json.Unmarshal(b, &f); err != nil || f.Version != settingsVer {
		return Settings{}, ErrSettingsUnreadable
	}
	key, err := s.key(f.Salt)
	if err != nil {
		return Settings{}, err
	}
	gcm, err := newGCM(key)
	if err != nil {
		return Settings{}, err
	}
	plain, err := gcm.Open(nil, f.Nonce, f.Data, []byte(keyContext))
	if err != nil {
		return Settings{}, ErrSettingsUnreadable
	}
	var st Settings
	if err := json.Unmarshal(plain, &st); err != nil {
		return Settings{}, ErrSettingsUnreadable
	}
	if st.DeviceID == "" {
		if st.DeviceID, err = util.RandID(); err != nil {
			return Settings{}, err
		}
	}
	return st, nil
}

func newSettings() (Settings, error) {
	id, err := util.RandID()
	if err != nil {
		return Settings{}, err
	}
	return Settings{ServerPort: 7000, DeviceID: id}, nil
}

// Save encrypts and writes the settings atomically.
func (s *SettingsStore) Save(st Settings) error {
	plain, err := json.Marshal(st)
	if err != nil {
		return err
	}
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return err
	}
	key, err := s.key(salt)
	if err != nil {
		return err
	}
	gcm, err := newGCM(key)
	if err != nil {
		return err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return err
	}
	f := encryptedFile{
		Version: settingsVer,
		Salt:    salt,
		Nonce:   nonce,
		Data:    gcm.Seal(nil, nonce, plain, []byte(keyContext)),
	}
	b, err := json.Marshal(&f)
	if err != nil {
		return err
	}
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".config-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, 0o600); err != nil {
		return err
	}
	return os.Rename(tmpName, s.path)
}

func newGCM(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

// HashPassword derives the lock hash of a password with a new salt.
func HashPassword(password string) (LockSetting, error) {
	if len(password) < 4 {
		return LockSetting{}, errors.New("密码至少 4 位")
	}
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return LockSetting{}, err
	}
	hash, err := scrypt.Key([]byte(password), salt, scryptN, scryptR, scryptP, keyLen)
	if err != nil {
		return LockSetting{}, err
	}
	return LockSetting{Enabled: true, Salt: salt, Hash: hash}, nil
}

// VerifyPassword checks a password against the lock hash.
func (l LockSetting) VerifyPassword(password string) bool {
	if !l.Enabled || len(l.Hash) == 0 {
		return true
	}
	hash, err := scrypt.Key([]byte(password), l.Salt, scryptN, scryptR, scryptP, keyLen)
	if err != nil {
		return false
	}
	return subtle.ConstantTimeCompare(hash, l.Hash) == 1
}
