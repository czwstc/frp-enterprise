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
	"errors"
	"fmt"
	"net"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/fatedier/frp/client"
	"github.com/fatedier/frp/pkg/config"
	"github.com/fatedier/frp/pkg/config/source"
	"github.com/fatedier/frp/pkg/config/types"
	v1 "github.com/fatedier/frp/pkg/config/v1"
	entpkg "github.com/fatedier/frp/pkg/enterprise"
	"github.com/fatedier/frp/pkg/msg"
	"github.com/fatedier/frp/pkg/util/log"
)

// State is the connection state of the client.
type State string

const (
	StateIdle       State = "idle"
	StateConnecting State = "connecting"
	StatePending    State = "pending"
	StateRunning    State = "running"
	StateRejected   State = "rejected"
	StateError      State = "error"
)

// Label returns the state in Chinese, for the UI.
func (s State) Label() string {
	switch s {
	case StateConnecting:
		return "连接中"
	case StatePending:
		return "等待审批"
	case StateRunning:
		return "运行中"
	case StateRejected:
		return "已拒绝"
	case StateError:
		return "错误"
	default:
		return "未连接"
	}
}

// ProxyStatus is a configured proxy with its working status.
type ProxyStatus struct {
	ProxySetting
	Phase      string
	Err        string
	RemoteAddr string
}

// Status is a snapshot of everything the UI shows.
type Status struct {
	State       State
	Message     string
	ServerAddr  string
	DeviceID    string
	AuthCode    string
	PublicIP    string
	LocalIPs    []string
	ConnectedAt time.Time
	AllowPorts  string
	Ports       []msg.EntPort
	Proxies     []ProxyStatus
	Notices     []msg.EntNotice
	Branding    Branding
}

// Manager runs the embedded frpc service and keeps its state.
type Manager struct {
	store *SettingsStore
	logs  *LogBuffer

	mu          sync.Mutex
	settings    Settings
	svc         *client.Service
	gen         int
	state       State
	message     string
	connectedAt time.Time
	snapshot    *msg.EntSnapshot
	notices     []msg.EntNotice

	cbMu     sync.Mutex
	onChange func()
	onNotice func(msg.EntNotice)
}

// NewManager loads the settings from store. logs may be nil.
func NewManager(store *SettingsStore, logs *LogBuffer) (*Manager, error) {
	st, err := store.Load()
	if errors.Is(err, ErrSettingsUnreadable) {
		log.Warnf("settings at %s cannot be read on this machine, starting with empty settings", store.Path())
		st, err = newSettings()
	}
	if err != nil {
		return nil, err
	}
	if logs == nil {
		logs = NewLogBuffer(0, nil)
	}
	m := &Manager{store: store, logs: logs, settings: st, state: StateIdle}
	return m, nil
}

// Logs returns the log buffer.
func (m *Manager) Logs() *LogBuffer { return m.logs }

// OnChange sets a function called after every state change, from any goroutine.
func (m *Manager) OnChange(fn func()) {
	m.cbMu.Lock()
	m.onChange = fn
	m.cbMu.Unlock()
}

// OnNotice sets a function called for every new server notice.
func (m *Manager) OnNotice(fn func(msg.EntNotice)) {
	m.cbMu.Lock()
	m.onNotice = fn
	m.cbMu.Unlock()
}

func (m *Manager) changed() {
	m.cbMu.Lock()
	fn := m.onChange
	m.cbMu.Unlock()
	if fn != nil {
		fn()
	}
}

// Settings returns a copy of the settings.
func (m *Manager) Settings() Settings {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.settings.Clone()
}

// updateSettings changes and saves the settings.
func (m *Manager) updateSettingsLocked(fn func(*Settings) error) error {
	next := m.settings.Clone()
	if err := fn(&next); err != nil {
		return err
	}
	if err := m.store.Save(next); err != nil {
		return fmt.Errorf("保存配置失败：%w", err)
	}
	m.settings = next
	return nil
}

// Login saves the connection settings and connects.
func (m *Manager) Login(serverAddr string, serverPort int, token, authCode string) error {
	m.mu.Lock()
	err := m.updateSettingsLocked(func(s *Settings) error {
		s.ServerAddr = serverAddr
		s.ServerPort = serverPort
		s.Token = token
		s.AuthCode = authCode
		if err := s.ValidateConnection(); err != nil {
			return err
		}
		s.AutoConnect = true
		return nil
	})
	m.mu.Unlock()
	if err != nil {
		return err
	}
	return m.Start()
}

// Logout disconnects and stops connecting automatically. The connection
// settings stay filled in.
func (m *Manager) Logout() error {
	m.Stop()
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.updateSettingsLocked(func(s *Settings) error {
		s.AutoConnect = false
		return nil
	})
}

// ParseServerAddr splits "host:port" or "host" (port 7000).
func ParseServerAddr(s string) (string, int, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", 0, errors.New("请输入服务端地址")
	}
	host, portStr, err := net.SplitHostPort(s)
	if err != nil {
		// No port given.
		return strings.Trim(s, "[]"), 7000, nil
	}
	var port int
	if _, err := fmt.Sscanf(portStr, "%d", &port); err != nil || port < 1 || port > 65535 {
		return "", 0, fmt.Errorf("端口无效：%s", portStr)
	}
	if host == "" {
		return "", 0, errors.New("请输入服务端地址")
	}
	return host, port, nil
}

func (m *Manager) buildCommon(st Settings) *v1.ClientCommonConfig {
	return &v1.ClientCommonConfig{
		Auth:          v1.AuthClientConfig{Method: v1.AuthMethodToken, Token: st.Token},
		User:          st.UserPrefix(),
		ClientID:      st.DeviceID,
		ServerAddr:    st.ServerAddr,
		ServerPort:    st.ServerPort,
		LoginFailExit: new(false),
		Enterprise:    v1.ClientEnterpriseConfig{AuthCode: st.AuthCode},
	}
}

func buildProxyConfigs(st Settings) []v1.ProxyConfigurer {
	out := make([]v1.ProxyConfigurer, 0, len(st.Proxies))
	for _, p := range st.Proxies {
		base := v1.ProxyBaseConfig{
			Name:         p.Name,
			Type:         p.Type,
			ProxyBackend: v1.ProxyBackend{LocalIP: p.LocalIP, LocalPort: p.LocalPort},
		}
		switch p.Type {
		case "udp":
			out = append(out, &v1.UDPProxyConfig{ProxyBaseConfig: base, RemotePort: p.RemotePort})
		default:
			base.Type = "tcp"
			out = append(out, &v1.TCPProxyConfig{ProxyBaseConfig: base, RemotePort: p.RemotePort})
		}
	}
	return config.CompleteProxyConfigurers(out)
}

// Start connects to the server with the saved settings, restarting the
// connection when it is already running.
func (m *Manager) Start() error {
	m.mu.Lock()
	old := m.detachLocked()
	st := m.settings.Clone()
	if err := st.ValidateConnection(); err != nil {
		m.mu.Unlock()
		closeService(old)
		return err
	}
	common := m.buildCommon(st)
	src := source.NewConfigSource()
	if err := src.ReplaceAll(buildProxyConfigs(st), nil); err != nil {
		m.mu.Unlock()
		closeService(old)
		return err
	}
	m.gen++
	gen := m.gen
	svc, err := client.NewService(client.ServiceOptions{
		Common:                 common,
		ConfigSourceAggregator: source.NewAggregator(src),
		EventHandler:           &serviceEvents{m: m, gen: gen},
	})
	if err != nil {
		m.state, m.message = StateError, err.Error()
		m.mu.Unlock()
		closeService(old)
		m.changed()
		return err
	}
	m.svc = svc
	m.state, m.message = StateConnecting, "正在连接 "+net.JoinHostPort(st.ServerAddr, fmt.Sprint(st.ServerPort))
	m.snapshot = nil
	m.connectedAt = time.Time{}
	m.mu.Unlock()
	closeService(old)
	m.changed()

	log.Infof("connecting to %s:%d with auth code %s", st.ServerAddr, st.ServerPort, st.AuthCode)
	go func() {
		err := svc.Run(context.Background())
		m.mu.Lock()
		if m.gen == gen && m.svc == svc {
			m.svc = nil
			if m.state != StateRejected {
				m.state = StateError
				m.message = "连接已停止"
				if err != nil {
					m.message = err.Error()
				}
			}
		}
		m.mu.Unlock()
		m.changed()
	}()
	return nil
}

// Restart reconnects with the saved settings.
func (m *Manager) Restart() error {
	return m.Start()
}

// Stop disconnects.
func (m *Manager) Stop() {
	m.mu.Lock()
	old := m.detachLocked()
	m.state, m.message = StateIdle, ""
	m.connectedAt = time.Time{}
	m.mu.Unlock()
	closeService(old)
	m.changed()
}

// detachLocked forgets the running service and returns it to be closed
// after the lock is released.
func (m *Manager) detachLocked() *client.Service {
	svc := m.svc
	m.svc = nil
	m.gen++
	return svc
}

func closeService(svc *client.Service) {
	if svc != nil {
		svc.Close()
	}
}

// stopGen stops the service of generation gen, keeping the current state.
func (m *Manager) stopGen(gen int) {
	m.mu.Lock()
	if m.gen != gen {
		m.mu.Unlock()
		return
	}
	old := m.detachLocked()
	m.mu.Unlock()
	closeService(old)
	m.changed()
}

// serviceEvents receives the events of one service generation.
type serviceEvents struct {
	m   *Manager
	gen int
}

func (e *serviceEvents) OnLoginResult(err error) {
	m := e.m
	m.mu.Lock()
	if m.gen != e.gen {
		m.mu.Unlock()
		return
	}
	stop := false
	if err == nil {
		m.state, m.message = StateRunning, ""
		m.connectedAt = time.Now()
	} else {
		switch entpkg.ParseLoginErrorKind(err.Error()) {
		case entpkg.LoginErrPending:
			m.state, m.message = StatePending, "鉴权码 "+m.settings.AuthCode+" 正在等待管理员审批，审批通过后会自动连接"
		case entpkg.LoginErrRejected:
			m.state, m.message, stop = StateRejected, "鉴权码已被管理员拒绝或吊销", true
		case entpkg.LoginErrBound:
			m.state, m.message, stop = StateRejected, "鉴权码已绑定其他设备", true
		case entpkg.LoginErrRequired, entpkg.LoginErrInvalid:
			m.state, m.message, stop = StateRejected, "鉴权码无效："+err.Error(), true
		default:
			m.state, m.message = StateConnecting, "连接失败，正在重试："+err.Error()
		}
	}
	m.mu.Unlock()
	if stop {
		go m.stopGen(e.gen)
	}
	m.changed()
}

func (e *serviceEvents) OnDisconnected() {
	m := e.m
	m.mu.Lock()
	if m.gen != e.gen {
		m.mu.Unlock()
		return
	}
	m.state, m.message = StateConnecting, "与服务端的连接已断开，正在重连"
	m.connectedAt = time.Time{}
	m.mu.Unlock()
	m.changed()
}

func (e *serviceEvents) OnEntSnapshot(snap *msg.EntSnapshot) {
	m := e.m
	m.mu.Lock()
	if m.gen != e.gen {
		m.mu.Unlock()
		return
	}
	m.snapshot = snap
	m.mu.Unlock()
	m.changed()
}

func (e *serviceEvents) OnEntBranding(b *msg.EntBranding) {
	m := e.m
	m.mu.Lock()
	if m.gen != e.gen || m.settings.Branding.Version == b.Version && m.settings.Branding.SiteName == b.SiteName &&
		m.settings.Branding.Accent == b.Accent && m.settings.Branding.Logo == b.Logo {
		m.mu.Unlock()
		return
	}
	err := m.updateSettingsLocked(func(s *Settings) error {
		s.Branding = Branding{Version: b.Version, SiteName: b.SiteName, Accent: b.Accent, Logo: b.Logo}
		return nil
	})
	m.mu.Unlock()
	if err != nil {
		log.Warnf("save branding: %v", err)
	}
	m.changed()
}

func (e *serviceEvents) OnEntNotice(n *msg.EntNotice) {
	m := e.m
	m.mu.Lock()
	if slices.ContainsFunc(m.notices, func(x msg.EntNotice) bool { return x.ID == n.ID }) {
		m.mu.Unlock()
		return
	}
	m.notices = append(m.notices, *n)
	sort.SliceStable(m.notices, func(i, j int) bool { return m.notices[i].Time > m.notices[j].Time })
	if len(m.notices) > 100 {
		m.notices = m.notices[:100]
	}
	m.mu.Unlock()
	m.cbMu.Lock()
	fn := m.onNotice
	m.cbMu.Unlock()
	if fn != nil {
		fn(*n)
	}
	m.changed()
}

// Status returns the current status.
func (m *Manager) Status() Status {
	m.mu.Lock()
	st := Status{
		State:       m.state,
		Message:     m.message,
		ServerAddr:  net.JoinHostPort(m.settings.ServerAddr, fmt.Sprint(m.settings.ServerPort)),
		DeviceID:    m.settings.DeviceID,
		AuthCode:    m.settings.AuthCode,
		ConnectedAt: m.connectedAt,
		Notices:     slices.Clone(m.notices),
		Branding:    m.settings.Branding,
	}
	if m.snapshot != nil {
		st.PublicIP = m.snapshot.ClientIP
		st.AllowPorts = m.snapshot.AllowPorts
		st.Ports = slices.Clone(m.snapshot.Ports)
	}
	svc := m.svc
	proxies := slices.Clone(m.settings.Proxies)
	m.mu.Unlock()

	running := map[string]ProxyStatus{}
	if svc != nil {
		for _, ws := range svc.AllProxyStatus() {
			running[ws.Name] = ProxyStatus{Phase: ws.Phase, Err: ws.Err, RemoteAddr: ws.RemoteAddr}
		}
	}
	for _, p := range proxies {
		ps := running[p.Name]
		ps.ProxySetting = p
		if ps.Phase == "" {
			ps.Phase = "stopped"
		}
		st.Proxies = append(st.Proxies, ps)
	}
	st.LocalIPs = LocalIPs()
	return st
}

// LocalIPs returns the non-loopback addresses of this machine.
func LocalIPs() []string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return nil
	}
	var out []string
	for _, a := range addrs {
		ipnet, ok := a.(*net.IPNet)
		if !ok || ipnet.IP.IsLoopback() || ipnet.IP.IsLinkLocalUnicast() {
			continue
		}
		out = append(out, ipnet.IP.String())
	}
	sort.Slice(out, func(i, j int) bool {
		// IPv4 first.
		a4, b4 := strings.Contains(out[i], "."), strings.Contains(out[j], ".")
		if a4 != b4 {
			return a4
		}
		return out[i] < out[j]
	})
	return out
}

// CheckRemotePort reports why proto/port cannot be used by the proxy named
// self (empty for a new proxy), or nil when it can.
func (m *Manager) CheckRemotePort(proto string, port int, self string) error {
	if port == 0 {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.checkRemotePortLocked(proto, port, self)
}

func (m *Manager) checkRemotePortLocked(proto string, port int, self string) error {
	if port == 0 {
		return nil
	}
	for _, p := range m.settings.Proxies {
		if p.Name != self && p.Type == proto && p.RemotePort == port {
			return fmt.Errorf("远端端口 %d/%s 已被本机的映射 %s 使用", port, proto, p.Name)
		}
	}
	if m.snapshot == nil {
		return nil
	}
	for _, p := range m.snapshot.Ports {
		if p.Proto != proto || p.Port != port {
			continue
		}
		if p.Mine {
			// Held by this device: either this very proxy, or another one
			// already checked against the settings above.
			continue
		}
		return fmt.Errorf("远端端口 %d/%s 已被其他客户端占用", port, proto)
	}
	if m.snapshot.AllowPorts != "" {
		ranges, err := types.NewPortsRangeSliceFromString(m.snapshot.AllowPorts)
		if err == nil && !portAllowed(ranges, port) {
			return fmt.Errorf("远端端口 %d 不在服务端允许的范围内（%s）", port, m.snapshot.AllowPorts)
		}
	}
	return nil
}

func portAllowed(ranges []types.PortsRange, port int) bool {
	for _, r := range ranges {
		if r.Single != 0 && r.Single == port {
			return true
		}
		if r.Start != 0 && port >= r.Start && port <= r.End {
			return true
		}
	}
	return false
}

// SaveProxy adds a proxy, or replaces the proxy named oldName.
func (m *Manager) SaveProxy(oldName string, p ProxySetting) error {
	if err := p.Normalize(); err != nil {
		return err
	}
	m.mu.Lock()
	err := m.updateSettingsLocked(func(s *Settings) error {
		idx := -1
		for i, x := range s.Proxies {
			if x.Name == oldName && oldName != "" {
				idx = i
			} else if x.Name == p.Name {
				return fmt.Errorf("名称 %s 已存在", p.Name)
			}
		}
		if oldName != "" && idx < 0 {
			return fmt.Errorf("映射 %s 不存在", oldName)
		}
		self := oldName
		if err := m.checkRemotePortLocked(p.Type, p.RemotePort, self); err != nil {
			return err
		}
		if idx >= 0 {
			s.Proxies[idx] = p
		} else {
			s.Proxies = append(s.Proxies, p)
		}
		return nil
	})
	m.mu.Unlock()
	if err != nil {
		return err
	}
	return m.applyProxies()
}

// DeleteProxy removes a proxy.
func (m *Manager) DeleteProxy(name string) error {
	m.mu.Lock()
	err := m.updateSettingsLocked(func(s *Settings) error {
		before := len(s.Proxies)
		s.Proxies = slices.DeleteFunc(s.Proxies, func(x ProxySetting) bool { return x.Name == name })
		if len(s.Proxies) == before {
			return fmt.Errorf("映射 %s 不存在", name)
		}
		return nil
	})
	m.mu.Unlock()
	if err != nil {
		return err
	}
	return m.applyProxies()
}

func (m *Manager) applyProxies() error {
	m.mu.Lock()
	svc := m.svc
	st := m.settings.Clone()
	m.mu.Unlock()
	defer m.changed()
	if svc == nil {
		return nil
	}
	return svc.UpdateConfigSource(m.buildCommon(st), buildProxyConfigs(st), nil)
}

// LockEnabled reports whether the UI lock is on.
func (m *Manager) LockEnabled() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.settings.Lock.Enabled
}

// VerifyPassword checks the lock password.
func (m *Manager) VerifyPassword(password string) bool {
	m.mu.Lock()
	lock := m.settings.Lock
	m.mu.Unlock()
	return lock.VerifyPassword(password)
}

// SetLockPassword turns the lock on with a new password. When the lock is
// already on, current must be the current password.
func (m *Manager) SetLockPassword(current, password string) error {
	if !m.VerifyPassword(current) {
		return errors.New("当前密码不正确")
	}
	lock, err := HashPassword(password)
	if err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.updateSettingsLocked(func(s *Settings) error {
		s.Lock = lock
		return nil
	})
}

// DisableLock turns the lock off.
func (m *Manager) DisableLock(current string) error {
	if !m.VerifyPassword(current) {
		return errors.New("当前密码不正确")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.updateSettingsLocked(func(s *Settings) error {
		s.Lock = LockSetting{}
		return nil
	})
}

// GenerateAuthCode returns a new random auth code.
func GenerateAuthCode() (string, error) {
	return entpkg.GenerateAuthCode()
}
