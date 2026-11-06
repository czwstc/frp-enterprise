package main

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/fatedier/frp/client/enterprise"
	"github.com/fatedier/frp/pkg/msg"
)

type fakeBackend struct {
	settings enterprise.Settings
	status   enterprise.Status
	logs     []enterprise.LogLine
	password string
	taken    map[string]bool // "tcp/6000"

	logins   []string
	restarts int
	stops    int
}

func newFake() *fakeBackend {
	return &fakeBackend{
		settings: enterprise.Settings{ServerPort: 7000, DeviceID: "abcdef0123456789"},
		status:   enterprise.Status{State: enterprise.StateIdle},
		taken:    map[string]bool{},
	}
}

func (f *fakeBackend) Settings() enterprise.Settings { return f.settings }
func (f *fakeBackend) Status() enterprise.Status {
	st := f.status
	st.Proxies = nil
	for _, p := range f.settings.Proxies {
		st.Proxies = append(st.Proxies, enterprise.ProxyStatus{ProxySetting: p, Phase: "running"})
	}
	return st
}
func (f *fakeBackend) Logs() []enterprise.LogLine { return f.logs }
func (f *fakeBackend) ClearLogs()                 { f.logs = nil }
func (f *fakeBackend) Login(addr string, port int, token, code string) error {
	s := enterprise.Settings{ServerAddr: addr, ServerPort: port, AuthCode: code}
	if err := s.ValidateConnection(); err != nil {
		return err
	}
	f.logins = append(f.logins, fmt.Sprintf("%s:%d %s %s", addr, port, token, s.AuthCode))
	f.settings.ServerAddr, f.settings.ServerPort, f.settings.Token, f.settings.AuthCode = addr, port, token, s.AuthCode
	f.settings.AutoConnect = true
	f.status.State = enterprise.StateConnecting
	return nil
}
func (f *fakeBackend) Restart() error {
	f.restarts++
	f.status.State = enterprise.StateConnecting
	return nil
}
func (f *fakeBackend) Stop()         { f.stops++; f.status.State = enterprise.StateIdle }
func (f *fakeBackend) Logout() error { f.settings.AutoConnect = false; f.Stop(); return nil }
func (f *fakeBackend) SaveProxy(old string, p enterprise.ProxySetting) error {
	if err := p.Normalize(); err != nil {
		return err
	}
	if err := f.CheckRemotePort(p.Type, p.RemotePort, old); err != nil {
		return err
	}
	for i, x := range f.settings.Proxies {
		if x.Name == old {
			f.settings.Proxies[i] = p
			return nil
		}
	}
	f.settings.Proxies = append(f.settings.Proxies, p)
	return nil
}
func (f *fakeBackend) DeleteProxy(name string) error {
	for i, x := range f.settings.Proxies {
		if x.Name == name {
			f.settings.Proxies = append(f.settings.Proxies[:i], f.settings.Proxies[i+1:]...)
			return nil
		}
	}
	return errors.New("not found")
}
func (f *fakeBackend) CheckRemotePort(proto string, port int, _ string) error {
	if f.taken[fmt.Sprintf("%s/%d", proto, port)] {
		return fmt.Errorf("远端端口 %d/%s 已被其他客户端占用", port, proto)
	}
	return nil
}
func (f *fakeBackend) LockEnabled() bool             { return f.password != "" }
func (f *fakeBackend) VerifyPassword(pw string) bool { return f.password == "" || pw == f.password }
func (f *fakeBackend) SetLockPassword(cur, pw string) error {
	if !f.VerifyPassword(cur) {
		return errors.New("当前密码不正确")
	}
	if len(pw) < 4 {
		return errors.New("密码至少 4 位")
	}
	f.password = pw
	return nil
}
func (f *fakeBackend) DisableLock(cur string) error {
	if !f.VerifyPassword(cur) {
		return errors.New("当前密码不正确")
	}
	f.password = ""
	return nil
}

// connectedFake is a backend logged in and running, with data in every tab.
func connectedFake() *fakeBackend {
	f := newFake()
	f.settings.ServerAddr = "frp.example.com"
	f.settings.Token = "super-secret-token"
	f.settings.AuthCode = "ABCD-EFGH-JKLM-NPQR"
	f.settings.AutoConnect = true
	f.settings.Proxies = []enterprise.ProxySetting{{Name: "ssh", Type: "tcp", LocalIP: "127.0.0.1", LocalPort: 22, RemotePort: 6022}}
	f.status = enterprise.Status{
		State:      enterprise.StateRunning,
		ServerAddr: "frp.example.com:7000",
		DeviceID:   f.settings.DeviceID,
		AuthCode:   f.settings.AuthCode,
		PublicIP:   "203.0.113.9",
		LocalIPs:   []string{"192.168.1.20"},
		Ports: []msg.EntPort{
			{Proto: "tcp", Port: 6022, Mine: true, ProxyName: "dabcdef01.ssh"},
			{Proto: "tcp", Port: 6080},
		},
		Notices: []msg.EntNotice{{ID: "n1", Level: "warning", Title: "维护通知", Content: "今晚 22 点维护", Time: time.Now().Unix()}},
	}
	f.taken["tcp/6080"] = true
	f.logs = []enterprise.LogLine{{Seq: 1, Time: time.Now(), Level: "info", Text: "login to server success, get run id [x]"}}
	return f
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func TestLoginView(t *testing.T) {
	f := newFake()
	a := newApp(f)
	tt := ui.NewTester(a.view, 920, 640)
	if !tt.HasText("FRP 企业客户端") || !tt.HasText("鉴权码") {
		t.Fatalf("login page texts %q", tt.Texts())
	}

	must(t, tt.Click("连接"))
	if !tt.HasText("请输入服务端地址") {
		t.Errorf("no address error, texts %q", tt.Texts())
	}

	must(t, tt.Click("生成"))
	if len(a.loginCode) != 19 || !tt.HasText("请把它发给管理员审批") {
		t.Errorf("generated code %q, texts %q", a.loginCode, tt.Texts())
	}

	must(t, tt.Click("服务端地址"))
	tt.Type("10.0.0.5:7001")
	must(t, tt.Click("TOKEN"))
	tt.Type("tok")
	must(t, tt.Click("连接"))
	if len(f.logins) != 1 || f.logins[0] != "10.0.0.5:7001 tok "+a.loginCode {
		t.Fatalf("logins %q", f.logins)
	}
	if a.page != pageMain || !tt.HasText("连接中") || !tt.HasText("概览") {
		t.Errorf("page %d texts %q", a.page, tt.Texts())
	}
	if strings.Contains(strings.Join(tt.Texts(), "\n"), "tok\n") {
		t.Error("token shown in clear text")
	}
}

func TestLockedViewHidesEverything(t *testing.T) {
	f := connectedFake()
	f.password = "pa55"
	a := newApp(f)
	// A notice arriving while locked must not show either.
	f.status.Notices = append(f.status.Notices, msg.EntNotice{ID: "n9", Title: "机密通知", Time: time.Now().Unix()})
	a.refresh()
	tt := ui.NewTester(a.view, 920, 640)
	if !a.locked {
		t.Fatal("app should start locked")
	}
	all := strings.Join(tt.Texts(), "\n")
	for _, secret := range []string{"super-secret-token", "ABCD-EFGH", "203.0.113.9", "6022", "ssh", "维护通知", "机密通知", "login to server", "frp.example.com"} {
		if strings.Contains(all, secret) {
			t.Errorf("locked view shows %q: %q", secret, tt.Texts())
		}
	}
	if !tt.HasText("运行中") || !tt.HasText("重启") {
		t.Errorf("locked view must show status and restart, texts %q", tt.Texts())
	}

	must(t, tt.Click("重启"))
	if f.restarts != 1 {
		t.Errorf("restarts %d", f.restarts)
	}
	must(t, tt.Click("解锁密码"))
	tt.Type("wrong")
	must(t, tt.Click("解锁"))
	if !a.locked || !tt.HasText("密码不正确") {
		t.Errorf("wrong password unlocked, texts %q", tt.Texts())
	}

	must(t, tt.Click("解锁密码"))
	tt.Type("pa55")
	must(t, tt.Click("解锁"))
	if a.locked {
		t.Fatal("still locked after the right password")
	}
	if !tt.HasText("203.0.113.9") || !tt.HasText("ABCD-EFGH-JKLM-NPQR") {
		t.Errorf("overview after unlock, texts %q", tt.Texts())
	}

	// Lock again from the header.
	must(t, tt.Click("锁定"))
	if !a.locked || tt.HasText("203.0.113.9") {
		t.Errorf("lock button did not lock, texts %q", tt.Texts())
	}
}

func TestProxyEditorRejectsTakenPort(t *testing.T) {
	f := connectedFake()
	a := newApp(f)
	tt := ui.NewTester(a.view, 920, 640)
	must(t, tt.Click("端口映射"))
	if !tt.HasText("ssh") || !tt.HasText("127.0.0.1:22") || !tt.HasText("运行中") {
		t.Fatalf("proxies tab texts %q", tt.Texts())
	}

	must(t, tt.Click("新增映射"))
	if !a.editing {
		t.Fatal("editor did not open")
	}
	tt.Type("web")
	a.edLocalPort = 8080
	a.edRemote = 6080
	tt.Frame()
	if !tt.HasText("已被其他客户端占用") {
		t.Fatalf("no conflict error, texts %q", tt.Texts())
	}
	must(t, tt.Click("保存"))
	if len(f.settings.Proxies) != 1 || !a.editing {
		t.Fatalf("saved a taken port: %+v", f.settings.Proxies)
	}

	a.edRemote = 6081
	tt.Frame()
	if tt.HasText("已被其他客户端占用") {
		t.Fatalf("conflict error stays, texts %q", tt.Texts())
	}
	must(t, tt.Click("保存"))
	if a.editing || len(f.settings.Proxies) != 2 || f.settings.Proxies[1].Name != "web" || f.settings.Proxies[1].RemotePort != 6081 {
		t.Fatalf("proxies %+v editing %v err %q", f.settings.Proxies, a.editing, a.edErr)
	}
	if !tt.HasText("web") {
		t.Errorf("new proxy not listed, texts %q", tt.Texts())
	}

	must(t, tt.Click("删除 ssh"))
	if len(f.settings.Proxies) != 1 || f.settings.Proxies[0].Name != "web" {
		t.Errorf("delete: %+v", f.settings.Proxies)
	}
}

func TestPortsNoticesLogsTabs(t *testing.T) {
	f := connectedFake()
	a := newApp(f)
	tt := ui.NewTester(a.view, 920, 640)

	must(t, tt.Click("服务端端口"))
	if !tt.HasText("6080") || !tt.HasText("其他客户端") || !tt.HasText("本机") {
		t.Errorf("ports tab texts %q", tt.Texts())
	}

	must(t, tt.Click("通知 (1)"))
	if !tt.HasText("维护通知") || !tt.HasText("今晚 22 点维护") {
		t.Errorf("notices tab texts %q", tt.Texts())
	}

	must(t, tt.Click("日志"))
	if !tt.HasText("login to server success") {
		t.Errorf("logs tab texts %q", tt.Texts())
	}

	// A new notice shows as a toast.
	f.status.Notices = append(f.status.Notices, msg.EntNotice{ID: "n2", Title: "新版本发布", Time: time.Now().Unix()})
	a.refresh()
	tt.Frame()
	if !tt.HasText("服务端通知：新版本发布") {
		t.Errorf("no toast, texts %q", tt.Texts())
	}
}

func TestSecurityTabEnablesLock(t *testing.T) {
	f := connectedFake()
	a := newApp(f)
	tt := ui.NewTester(a.view, 920, 640)
	must(t, tt.Click("安全"))
	must(t, tt.Click("新密码"))
	tt.Type("abcd")
	must(t, tt.Click("确认密码"))
	tt.Type("abce")
	must(t, tt.Click("开启锁定"))
	if f.password != "" || !tt.HasText("两次输入的密码不一致") {
		t.Fatalf("mismatch accepted, texts %q", tt.Texts())
	}
	a.secNew2 = "abcd"
	tt.Frame()
	must(t, tt.Click("开启锁定"))
	if f.password != "abcd" {
		t.Fatalf("lock not enabled: %q", a.secErr)
	}
	must(t, tt.Click("立即锁定"))
	if !a.locked {
		t.Fatal("not locked")
	}
}

func TestLogoutReturnsToLogin(t *testing.T) {
	f := connectedFake()
	a := newApp(f)
	tt := ui.NewTester(a.view, 920, 640)
	must(t, tt.Click("退出登录"))
	if a.page != pageLogin || f.settings.AutoConnect || f.stops != 1 {
		t.Fatalf("page %d autoconnect %v stops %d", a.page, f.settings.AutoConnect, f.stops)
	}
	if a.loginAddr != "frp.example.com:7000" || a.loginCode != "ABCD-EFGH-JKLM-NPQR" {
		t.Errorf("login form not prefilled: %q %q", a.loginAddr, a.loginCode)
	}
}
