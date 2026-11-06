package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/fatedier/frp/client/enterprise"
	"github.com/fatedier/frp/pkg/msg"
)

// backend is what the views need from the embedded frpc. The real one is
// *enterprise.Manager; tests use a fake.
type backend interface {
	Settings() enterprise.Settings
	Status() enterprise.Status
	Logs() []enterprise.LogLine
	ClearLogs()
	Login(addr string, port int, token, code string) error
	Restart() error
	Stop()
	Logout() error
	SaveProxy(oldName string, p enterprise.ProxySetting) error
	DeleteProxy(name string) error
	CheckRemotePort(proto string, port int, self string) error
	LockEnabled() bool
	VerifyPassword(pw string) bool
	SetLockPassword(current, pw string) error
	DisableLock(current string) error
}

// managerBackend adapts *enterprise.Manager to backend.
type managerBackend struct {
	*enterprise.Manager
}

func (b managerBackend) Logs() []enterprise.LogLine { return b.Manager.Logs().Lines() }
func (b managerBackend) ClearLogs()                 { b.Manager.Logs().Clear() }

type page int

const (
	pageLogin page = iota
	pageMain
)

const (
	tabOverview = iota
	tabProxies
	tabPorts
	tabNotices
	tabLogs
	tabSecurity
)

// app is the state the window shows. All fields are read and written on the
// main thread: background events reach it through refresh, run by
// Window.Update.
type app struct {
	be backend

	page     page
	locked   bool
	settings enterprise.Settings
	status   enterprise.Status
	logs     []enterprise.LogLine
	logSeq   uint64

	// login form
	loginAddr, loginToken, loginCode string
	loginErr                         string
	codeGenerated                    bool

	tab int

	// proxy editor
	editing     bool
	editOld     string
	edName      string
	edProto     int // 0 tcp, 1 udp
	edLocalIP   string
	edLocalPort float64
	edRemote    float64
	edErr       string

	// unlock form
	unlockPw  string
	unlockErr string

	// security tab
	secCur, secNew, secNew2 string
	secErr, secOK           string

	// logs tab
	logFilter int
	logList   ui.ListState

	// notices seen, for toasts about new ones
	seenNotices map[string]bool
	toasts      []string

	actionErr string
}

func newApp(be backend) *app {
	a := &app{be: be, seenNotices: map[string]bool{}}
	a.logList.FollowEnd = true
	// Notices received before the window opened are not toasted.
	for _, n := range be.Status().Notices {
		a.seenNotices[n.ID] = true
	}
	a.refresh()
	a.locked = be.LockEnabled()
	if a.settings.AutoConnect {
		a.page = pageMain
	}
	a.fillLoginForm()
	return a
}

func (a *app) fillLoginForm() {
	s := a.settings
	if s.ServerAddr != "" {
		a.loginAddr = fmt.Sprintf("%s:%d", s.ServerAddr, s.ServerPort)
	}
	a.loginToken = s.Token
	a.loginCode = s.AuthCode
}

// refresh reloads the state from the backend. It runs on the main thread.
func (a *app) refresh() {
	a.settings = a.be.Settings()
	a.status = a.be.Status()
	logs := a.be.Logs()
	if len(logs) == 0 || len(a.logs) == 0 || logs[len(logs)-1].Seq != a.logSeq || len(logs) != len(a.logs) {
		a.logs = logs
		if len(logs) > 0 {
			a.logSeq = logs[len(logs)-1].Seq
		}
	}
	for _, n := range a.status.Notices {
		if a.seenNotices[n.ID] {
			continue
		}
		a.seenNotices[n.ID] = true
		text := n.Title
		if text == "" {
			text = n.Content
		}
		a.toasts = append(a.toasts, "服务端通知："+text)
	}
}

func (a *app) view(c *ui.Context) {
	// A locked window shows nothing of the notices.
	if !a.locked {
		for _, t := range a.toasts {
			c.Toast(t)
		}
	}
	a.toasts = nil

	switch {
	case a.locked:
		a.lockedView(c)
	case a.page == pageLogin:
		a.loginView(c)
	default:
		a.mainView(c)
	}
}

func stateColor(t *ui.Theme, s enterprise.State) ui.Color {
	switch s {
	case enterprise.StateRunning:
		return t.Success
	case enterprise.StatePending, enterprise.StateConnecting:
		return t.Warning
	case enterprise.StateRejected, enterprise.StateError:
		return t.Danger
	default:
		return t.TextMuted
	}
}

func stateBadge(c *ui.Context, s enterprise.State) {
	t := c.Theme()
	ui.Badge(c, s.Label()).Background(stateColor(t, s)).TextColor(ui.RGB(255, 255, 255))
}

func formatDuration(d time.Duration) string {
	d = d.Round(time.Second)
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	s := int(d.Seconds()) % 60
	switch {
	case h > 0:
		return fmt.Sprintf("%d 小时 %d 分", h, m)
	case m > 0:
		return fmt.Sprintf("%d 分 %d 秒", m, s)
	default:
		return fmt.Sprintf("%d 秒", s)
	}
}

func noticeLevelLabel(n msg.EntNotice) string {
	switch n.Level {
	case "warning":
		return "警告"
	case "error":
		return "紧急"
	case "success":
		return "完成"
	default:
		return "通知"
	}
}

func proxyPhaseLabel(phase string) string {
	switch phase {
	case "running":
		return "运行中"
	case "start error":
		return "启动失败"
	case "wait start", "new":
		return "启动中"
	case "check failed":
		return "检查失败"
	case "closed":
		return "已关闭"
	default:
		return "未运行"
	}
}

func joinOr(items []string, empty string) string {
	if len(items) == 0 {
		return empty
	}
	return strings.Join(items, ", ")
}
