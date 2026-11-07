package main

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/fatedier/frp/client/enterprise"
)

type navItem struct {
	tab   int
	icon  string
	label string
	color func(p palette) ui.Color
	count string
	alert bool
}

func (a *app) navGroups() []struct {
	label string
	items []navItem
} {
	st := a.status
	running := 0
	for _, px := range st.Proxies {
		if px.Phase == "running" {
			running++
		}
	}
	mapCount := ""
	if len(st.Proxies) > 0 {
		mapCount = strconv.Itoa(len(st.Proxies))
	}
	portCount := ""
	if len(st.Ports) > 0 {
		portCount = strconv.Itoa(len(st.Ports))
	}
	noticeCount := ""
	if len(st.Notices) > 0 {
		noticeCount = strconv.Itoa(len(st.Notices))
	}
	return []struct {
		label string
		items []navItem
	}{
		{"连接", []navItem{
			{tabOverview, "home", "概览", func(p palette) ui.Color { return p.blue }, "", false},
			{tabProxies, "link", "端口映射", func(p palette) ui.Color { return p.green }, mapCount, running < len(st.Proxies)},
			{tabPorts, "grid", "服务端端口", func(p palette) ui.Color { return p.teal }, portCount, false},
		}},
		{"消息", []navItem{
			{tabNotices, "bell", "通知", func(p palette) ui.Color { return p.amber }, noticeCount, false},
			{tabLogs, "doc", "日志", func(p palette) ui.Color { return p.orange }, "", false},
		}},
		{"设备", []navItem{
			{tabSecurity, "lock", "安全", func(p palette) ui.Color { return p.violet }, "", false},
			{tabPrefs, "gear", "设置", func(p palette) ui.Color { return p.gray }, "", false},
		}},
	}
}

var tabTitles = map[int]string{
	tabOverview: "概览", tabProxies: "端口映射", tabPorts: "服务端端口",
	tabNotices: "通知", tabLogs: "日志", tabSecurity: "安全", tabPrefs: "设置",
}

func (a *app) mainView(c *ui.Context, p palette) {
	ui.Row(c).Fill().AlignItems(ui.Stretch).Children(func() {
		a.sidebar(c, p)
		ui.Column(c).Grow(1).Background(p.canvas).Children(func() {
			a.topbar(c, p)
			ui.Column(c).Grow(1).Padding(4, 20, 20, 20).Gap(12).Children(func() {
				switch a.tab {
				case tabOverview:
					a.overviewTab(c, p)
				case tabProxies:
					a.proxiesTab(c, p)
				case tabPorts:
					a.portsTab(c, p)
				case tabNotices:
					a.noticesTab(c, p)
				case tabLogs:
					a.logsTab(c, p)
				case tabSecurity:
					a.securityTab(c, p)
				case tabPrefs:
					a.prefsTab(c, p)
				}
			})
		})
	})
	ui.Modal(c, &a.editing, func() { a.proxyEditor(c, p) })
}

func (a *app) sidebar(c *ui.Context, p palette) {
	t := c.Theme()
	ui.Column(c).Width(210).Background(p.side).Padding(12, 10).Gap(14).Children(func() {
		ui.Column(c).Padding(4, 4).Children(func() {
			a.brandMark(c, p)
		})
		for _, g := range a.navGroups() {
			ui.Column(c).Gap(1).Children(func() {
				ui.Text(c, g.label).FontSize(11).Bold().TextColor(t.TextMuted).Padding(4, 8)
				for _, it := range g.items {
					on := a.tab == it.tab
					btn := ui.ButtonBase(c.Key(it.label)).Label(it.label).Padding(6, 8).Radius(7).OnClick(func() { a.tab = it.tab })
					if on {
						btn.Background(t.Accent).TextColor(ui.RGB(255, 255, 255))
					} else if btn.Hovered() {
						btn.Background(p.sideHover)
					}
					btn.Children(func() {
						ui.Row(c).FillWidth().Gap(9).AlignItems(ui.Center).Children(func() {
							iconSquare(c, it.icon, it.color(p), 18)
							ui.Text(c, it.label).Grow(1)
							if it.count != "" {
								col := t.TextMuted
								if on {
									col = ui.RGBA(255, 255, 255, 0.85)
								}
								if it.alert {
									ui.Text(c, it.count).FontSize(11).Bold().TextColor(ui.RGB(255, 255, 255)).Background(p.red).Padding(0, 6).Radius(9)
								} else {
									ui.Text(c, it.count).FontSize(11.5).TextColor(col)
								}
							}
						})
					})
				}
			})
		}
		ui.Spacer(c)
		ui.Column(c).Padding(8).Gap(2).Children(func() {
			ui.Text(c, siteName(a.status.Branding)).FontSize(11).TextColor(t.TextMuted)
			ui.Text(c, a.status.ServerAddr).FontSize(11).TextColor(t.TextMuted).SingleLine()
		})
	})
}

func (a *app) topbar(c *ui.Context, p palette) {
	t := c.Theme()
	st := a.status
	ui.Column(c).Padding(14, 20, 8, 20).Gap(6).Children(func() {
		ui.Row(c).Gap(t.Space(2)).AlignItems(ui.Center).Children(func() {
			ui.Column(c).Grow(1).Children(func() {
				ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
					ui.Text(c, tabTitles[a.tab]).FontSize(17).Bold()
					stateBadge(c, st.State)
				})
				ui.Text(c, st.ServerAddr+" · 设备 "+st.DeviceID).FontSize(11.5).TextColor(t.TextMuted)
			})
			switch st.State {
			case enterprise.StateIdle, enterprise.StateRejected, enterprise.StateError:
				ui.PrimaryButton(c, "连接").OnClick(a.restart)
			default:
				ui.Button(c, "重启").OnClick(a.restart)
				ui.Button(c, "断开").OnClick(func() {
					a.be.Stop()
					a.refresh()
				})
			}
			if a.be.LockEnabled() {
				ui.Button(c, "锁定").OnClick(func() { a.locked = true })
			}
			ui.Button(c, "退出登录").OnClick(func() {
				if err := a.be.Logout(); err != nil {
					a.actionErr = err.Error()
					return
				}
				a.refresh()
				a.fillLoginForm()
				a.page = pageLogin
			})
		})
		if st.Message != "" {
			color := t.TextMuted
			switch st.State {
			case enterprise.StatePending:
				color = p.amber
			case enterprise.StateRejected, enterprise.StateError:
				color = p.red
			}
			ui.Text(c, st.Message).TextColor(color).Wrap()
		}
		if a.actionErr != "" {
			ui.Text(c, a.actionErr).TextColor(p.red).Wrap()
		}
	})
}

func (a *app) restart() {
	a.actionErr = ""
	if err := a.be.Restart(); err != nil {
		a.actionErr = "连接失败：" + err.Error()
	}
	a.refresh()
}

func kv(c *ui.Context, label, value string) {
	ui.Column(c).Grow(1).Basis(0).Gap(2).Children(func() {
		ui.Text(c, label).FontSize(11).TextColor(c.Theme().TextMuted)
		ui.Text(c, value).Bold().Wrap().Selectable()
	})
}

func (a *app) overviewTab(c *ui.Context, p palette) {
	t := c.Theme()
	st := a.status
	ui.Scroll(c).Grow(1).Gap(12).Children(func() {
		card(c, p).Gap(12).Children(func() {
			ui.Row(c).Gap(14).AlignItems(ui.Center).Children(func() {
				col := stateColor(t, st.State)
				bg := col
				bg.A = 36
				icon := "x"
				switch st.State {
				case enterprise.StateRunning:
					icon = "check"
				case enterprise.StateConnecting, enterprise.StatePending:
					icon = "pause"
				}
				ui.Column(c).Size(52, 52).Radius(26).Background(bg).Center().TextColor(col).Children(func() {
					ui.Icon(c, icons[icon]).Size(24, 24)
				})
				ui.Column(c).Grow(1).Gap(2).Children(func() {
					ui.Text(c, st.State.Label()).FontSize(20).Bold()
					detail := "鉴权码 " + st.AuthCode
					if !st.ConnectedAt.IsZero() {
						detail = "已连接 " + formatDuration(time.Since(st.ConnectedAt)) + " · " + detail
						c.After(time.Second)
					}
					ui.Text(c, detail).FontSize(11.5).TextColor(t.TextMuted).Selectable()
				})
				ui.Column(c).Width(180).Children(func() {
					dotMatrix(c, a.history, col, p.dotOff, 34)
				})
			})
			publicIP := st.PublicIP
			switch {
			case st.PublicIPSource.Mode == enterprise.PublicIPOff:
				publicIP = "已关闭"
			case publicIP == "" && st.PublicIPError != "":
				publicIP = st.PublicIPError
			case publicIP == "":
				publicIP = "连接后由服务端告知"
			}
			running := 0
			for _, px := range st.Proxies {
				if px.Phase == "running" {
					running++
				}
			}
			mine, others := 0, 0
			for _, pt := range st.Ports {
				if pt.Mine {
					mine++
				} else {
					others++
				}
			}
			ui.Row(c).Gap(12).Children(func() {
				kv(c, "当前公网 IP · "+publicIPSourceLabel(st.PublicIPSource), publicIP)
				ui.Column(c).Grow(1).Basis(0).Gap(2).Children(func() {
					label := "本机 IP"
					if n := len(st.LocalIPs); n > 1 {
						label = fmt.Sprintf("本机 IP（共 %d 个）", n)
					}
					ui.Text(c, label).FontSize(11).TextColor(t.TextMuted)
					switch len(st.LocalIPs) {
					case 0:
						ui.Text(c, "无").Bold()
					case 1:
						ui.Text(c, st.LocalIPs[0]).Bold().Selectable()
					default:
						ui.Select(c.Key("local-ip"), &a.localIP, st.LocalIPs).Label("本机 IP").MaxWidth(220)
					}
				})
				kv(c, "端口映射", fmt.Sprintf("%d 个，运行中 %d 个", len(st.Proxies), running))
				kv(c, "服务端端口", fmt.Sprintf("本机 %d · 其他 %d", mine, others))
			})
		})

		ui.Row(c).Gap(12).AlignItems(ui.Stretch).Children(func() {
			card(c, p).Grow(1).Basis(0).Children(func() {
				cardHeader(c, "link", p.green, "端口映射", nil)
				if len(st.Proxies) == 0 {
					ui.Text(c, "还没有端口映射。").TextColor(t.TextMuted)
				}
				for i, px := range st.Proxies {
					if i == 4 {
						ui.Text(c, fmt.Sprintf("还有 %d 个…", len(st.Proxies)-4)).FontSize(11.5).TextColor(t.TextMuted)
						break
					}
					a.proxyLine(c, p, px, false)
				}
			})
			card(c, p).Grow(1).Basis(0).Children(func() {
				cardHeader(c, "bell", p.amber, "服务端通知", nil)
				if len(st.Notices) == 0 {
					ui.Text(c, "暂无通知。").TextColor(t.TextMuted)
				}
				for i, n := range st.Notices {
					if i == 3 {
						break
					}
					ui.Column(c.Key(n.ID)).Gap(2).Children(func() {
						ui.Text(c, n.Title).Bold()
						if n.Content != "" {
							ui.Text(c, n.Content).FontSize(11.5).TextColor(t.TextMuted).MaxLines(2)
						}
					})
				}
			})
		})
	})
}

// proxyLine is one mapping: status dot, name, addresses and type.
func (a *app) proxyLine(c *ui.Context, p palette, px enterprise.ProxyStatus, actions bool) {
	t := c.Theme()
	ui.Row(c.Key(px.Name)).Gap(10).AlignItems(ui.Center).PaddingY(6).Children(func() {
		dot := t.TextMuted
		switch px.Phase {
		case "running":
			dot = p.green
		case "start error", "check failed":
			dot = p.red
		case "wait start", "new":
			dot = p.amber
		}
		ui.Column(c).Size(8, 8).Radius(4).Background(dot)
		ui.Column(c).Grow(1).Gap(2).Children(func() {
			ui.Text(c, px.Name).Bold().SingleLine()
			remote := ":" + strconv.Itoa(px.RemotePort)
			if px.RemotePort == 0 {
				remote = "自动"
				if px.RemoteAddr != "" {
					remote = px.RemoteAddr
				}
			}
			ui.Text(c, fmt.Sprintf("%s:%d → %s", px.LocalIP, px.LocalPort, remote)).Font("monospace").FontSize(11.5).TextColor(t.TextMuted)
			if px.Err != "" {
				ui.Text(c, px.Err).FontSize(11).TextColor(p.red).Wrap()
			}
		})
		tag(c, strings.ToUpper(px.Type), p.green)
		if actions {
			ui.Text(c, proxyPhaseLabel(px.Phase)).FontSize(11.5).TextColor(dot).Width(56)
			ps := px.ProxySetting
			ui.Button(c, "编辑").Label("编辑 " + px.Name).OnClick(func() { a.openEditor(&ps) })
			ui.Button(c, "删除").Label("删除 " + px.Name).OnClick(func() {
				if err := a.be.DeleteProxy(ps.Name); err != nil {
					a.actionErr = err.Error()
				}
				a.refresh()
			})
		}
	})
}

func (a *app) openEditor(p *enterprise.ProxySetting) {
	a.editing = true
	a.edErr = ""
	if p == nil {
		a.editOld = ""
		a.edName = ""
		a.edProto = 0
		a.edLocalIP = "127.0.0.1"
		a.edLocalPort = 0
		a.edRemote = 0
		return
	}
	a.editOld = p.Name
	a.edName = p.Name
	a.edProto = 0
	if p.Type == "udp" {
		a.edProto = 1
	}
	a.edLocalIP = p.LocalIP
	a.edLocalPort = float64(p.LocalPort)
	a.edRemote = float64(p.RemotePort)
}

func (a *app) editedProxy() enterprise.ProxySetting {
	proto := "tcp"
	if a.edProto == 1 {
		proto = "udp"
	}
	return enterprise.ProxySetting{
		Name:       strings.TrimSpace(a.edName),
		Type:       proto,
		LocalIP:    strings.TrimSpace(a.edLocalIP),
		LocalPort:  int(a.edLocalPort),
		RemotePort: int(a.edRemote),
	}
}

func (a *app) proxiesTab(c *ui.Context, p palette) {
	t := c.Theme()
	st := a.status
	ui.Row(c).Gap(t.Space(2)).AlignItems(ui.Center).Children(func() {
		ui.Text(c, "把本机服务通过服务端的远端端口暴露出去。").TextColor(t.TextMuted).Grow(1)
		ui.PrimaryButton(c, "新增映射").OnClick(func() { a.openEditor(nil) })
	})
	card(c, p).Grow(1).Children(func() {
		ui.Scroll(c).Grow(1).Children(func() {
			if len(st.Proxies) == 0 {
				ui.Text(c, "还没有端口映射，点“新增映射”添加。").TextColor(t.TextMuted).Padding(12)
			}
			for i, px := range st.Proxies {
				if i > 0 {
					ui.Divider(c)
				}
				a.proxyLine(c, p, px, true)
			}
		})
	})
}

// remotePortError is the reason the edited remote port cannot be used.
func (a *app) remotePortError() string {
	px := a.editedProxy()
	if px.RemotePort == 0 {
		return ""
	}
	if err := a.be.CheckRemotePort(px.Type, px.RemotePort, a.editOld); err != nil {
		return err.Error()
	}
	return ""
}

func (a *app) proxyEditor(c *ui.Context, p palette) {
	t := c.Theme()
	title := "新增端口映射"
	if a.editOld != "" {
		title = "编辑端口映射 " + a.editOld
	}
	ui.Column(c).Width(420).Gap(t.Space(3)).Children(func() {
		ui.Text(c, title).FontSize(16).Bold()
		portErr := a.remotePortError()
		ui.Form(c, func() {
			ui.Field(c, "名称", func() {
				ui.TextInput(c.Key("ed-name"), &a.edName).Placeholder("例如 ssh").AutoFocus()
			})
			ui.Field(c, "协议", func() {
				ui.Segmented(c, &a.edProto, "TCP", "UDP")
			})
			ui.Field(c, "本地 IP", func() {
				ui.TextInput(c.Key("ed-ip"), &a.edLocalIP)
			})
			ui.Field(c, "本地端口", func() {
				ui.NumberInput(c.Key("ed-lport"), &a.edLocalPort, 0, 65535, 1)
			})
			ui.Field(c, "远端端口", func() {
				ui.NumberInput(c.Key("ed-rport"), &a.edRemote, 0, 65535, 1)
			}).Description("0 表示由服务端自动分配").Error(portErr)
		})
		if a.edErr != "" {
			ui.Text(c, a.edErr).TextColor(p.red).Wrap()
		}
		ui.Row(c).Justify(ui.End).Gap(t.Space(2)).Children(func() {
			ui.Button(c, "取消").OnClick(func() { a.editing = false })
			ui.PrimaryButton(c, "保存").Disabled(portErr != "").OnClick(func() {
				if err := a.be.SaveProxy(a.editOld, a.editedProxy()); err != nil {
					a.edErr = err.Error()
					return
				}
				a.editing = false
				a.refresh()
			})
		})
	})
}

func (a *app) portsTab(c *ui.Context, p palette) {
	t := c.Theme()
	st := a.status
	allow := st.AllowPorts
	if allow == "" {
		allow = "不限"
	}
	card(c, p).Children(func() {
		cardHeader(c, "grid", p.teal, "服务端已转发的端口", func() {
			ui.Text(c, "允许范围 "+allow).FontSize(11.5).TextColor(t.TextMuted)
		})
		if len(st.Ports) == 0 {
			msg := "服务端上暂无已转发的端口。"
			if st.State != enterprise.StateRunning {
				msg = "连接服务端后显示。"
			}
			ui.Text(c, msg).TextColor(t.TextMuted)
		}
		ui.Row(c).Wrap().Gap(6).Children(func() {
			for _, pt := range st.Ports {
				label := fmt.Sprintf("%d/%s", pt.Port, pt.Proto)
				chip := ui.Text(c.Key(label), label).Font("monospace").FontSize(11.5).Padding(3, 8).Radius(6)
				if pt.Mine {
					chip.Background(p.accentSoft).TextColor(t.Accent).Bold().Tooltip("本机：" + displayProxyName(pt.ProxyName))
				} else {
					chip.Background(p.side).TextColor(t.TextMuted).Tooltip("其他客户端")
				}
			}
		})
		ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "本机").FontSize(11).Bold().TextColor(t.Accent).Background(p.accentSoft).Padding(1, 7).Radius(6)
			ui.Text(c, "其他客户端").FontSize(11).TextColor(t.TextMuted).Background(p.side).Padding(1, 7).Radius(6)
			ui.Text(c, "被其他客户端占用的端口无法再映射。").FontSize(11.5).TextColor(t.TextMuted)
		})
	})
}

// displayProxyName removes the device prefix frps adds to proxy names.
func displayProxyName(name string) string {
	if _, rest, ok := strings.Cut(name, "."); ok {
		return rest
	}
	return name
}

func (a *app) noticesTab(c *ui.Context, p palette) {
	t := c.Theme()
	st := a.status
	ui.Scroll(c).Grow(1).Gap(10).Children(func() {
		if len(st.Notices) == 0 {
			ui.Text(c, "暂无服务端通知。").TextColor(t.TextMuted).Padding(12)
		}
		for _, n := range st.Notices {
			card(c.Key(n.ID), p).Children(func() {
				ui.Row(c).Gap(t.Space(2)).AlignItems(ui.Center).Children(func() {
					color := p.blue
					switch n.Level {
					case "warning":
						color = p.amber
					case "error":
						color = p.red
					case "success":
						color = p.green
					}
					tag(c, noticeLevelLabel(n), color)
					ui.Text(c, n.Title).Bold().Grow(1)
					ui.Text(c, time.Unix(n.Time, 0).Format("2006-01-02 15:04")).TextColor(t.TextMuted).FontSize(12)
				})
				if n.Content != "" {
					ui.Text(c, n.Content).Wrap().Selectable()
				}
			})
		}
	})
}

var logFilters = []string{"全部", "信息", "警告", "错误"}

func (a *app) filteredLogs() []enterprise.LogLine {
	if a.logFilter == 0 {
		return a.logs
	}
	out := make([]enterprise.LogLine, 0, len(a.logs))
	for _, l := range a.logs {
		switch a.logFilter {
		case 1:
			if l.Level == "info" || l.Level == "warn" || l.Level == "error" {
				out = append(out, l)
			}
		case 2:
			if l.Level == "warn" || l.Level == "error" {
				out = append(out, l)
			}
		case 3:
			if l.Level == "error" {
				out = append(out, l)
			}
		}
	}
	return out
}

func (a *app) logsTab(c *ui.Context, p palette) {
	t := c.Theme()
	lines := a.filteredLogs()
	ui.Row(c).Gap(t.Space(2)).AlignItems(ui.Center).Children(func() {
		ui.Segmented(c, &a.logFilter, logFilters...).Label("日志级别")
		ui.Spacer(c)
		ui.Button(c, "复制全部").OnClick(func() {
			var b strings.Builder
			for _, l := range lines {
				fmt.Fprintf(&b, "%s [%s] %s\n", l.Time.Format("2006-01-02 15:04:05"), l.Level, l.Text)
			}
			c.Services().WriteClipboard(b.String())
		})
		ui.Button(c, "清空").OnClick(func() {
			a.be.ClearLogs()
			a.refresh()
		})
	})
	ui.List(c, &a.logList, len(lines), func(i int) {
		l := lines[i]
		color := t.Text
		switch l.Level {
		case "warn":
			color = p.amber
		case "error":
			color = p.red
		case "debug", "trace":
			color = t.TextMuted
		}
		ui.Row(c).Gap(t.Space(2)).PaddingY(2).Children(func() {
			ui.Text(c, l.Time.Format("15:04:05")).TextColor(t.TextMuted).Font("monospace").FontSize(12)
			ui.Text(c, l.Text).TextColor(color).Font("monospace").FontSize(12).Wrap().Grow(1).Selectable()
		})
	}).Grow(1).Padding(10).Radius(12).Background(p.card).Border(1, p.line).Children(func() {
		if len(lines) == 0 {
			ui.Text(c, "暂无日志。").TextColor(t.TextMuted)
		}
	})
}

func (a *app) securityTab(c *ui.Context, p palette) {
	t := c.Theme()
	ui.Scroll(c).Grow(1).Children(func() {
		card(c, p).Width(480).Gap(t.Space(3)).Padding(16).Children(func() {
			if !a.be.LockEnabled() {
				cardHeader(c, "lock", p.violet, "加密锁定", nil)
				ui.Text(c, "开启后，客户端界面只显示运行状态和重启按钮，输入密码后才能查看全部内容。配置文件始终加密保存在本机。").
					TextColor(t.TextMuted).Wrap()
				ui.Form(c, func() {
					ui.Field(c, "新密码", func() { ui.TextInput(c.Key("sec-new"), &a.secNew).Password() })
					ui.Field(c, "确认密码", func() { ui.TextInput(c.Key("sec-new2"), &a.secNew2).Password() })
				})
				a.secMessages(c, p)
				ui.Row(c).Justify(ui.End).Children(func() {
					ui.PrimaryButton(c, "开启锁定").OnClick(func() {
						if a.secNew != a.secNew2 {
							a.secErr, a.secOK = "两次输入的密码不一致", ""
							return
						}
						if err := a.be.SetLockPassword("", a.secNew); err != nil {
							a.secErr, a.secOK = err.Error(), ""
							return
						}
						a.secNew, a.secNew2, a.secErr, a.secOK = "", "", "", "已开启锁定，点顶部的“锁定”或重新打开客户端后生效"
					})
				})
				return
			}
			cardHeader(c, "lock", p.violet, "加密锁定已开启", func() {
				ui.PrimaryButton(c, "立即锁定").OnClick(func() { a.locked = true })
			})
			ui.Divider(c)
			ui.Text(c, "修改或关闭密码").Bold()
			ui.Form(c, func() {
				ui.Field(c, "当前密码", func() { ui.TextInput(c.Key("sec-cur"), &a.secCur).Password() })
				ui.Field(c, "新密码", func() { ui.TextInput(c.Key("sec-new"), &a.secNew).Password() })
				ui.Field(c, "确认密码", func() { ui.TextInput(c.Key("sec-new2"), &a.secNew2).Password() })
			})
			a.secMessages(c, p)
			ui.Row(c).Justify(ui.End).Gap(t.Space(2)).Children(func() {
				ui.Button(c, "关闭锁定").OnClick(func() {
					if err := a.be.DisableLock(a.secCur); err != nil {
						a.secErr, a.secOK = err.Error(), ""
						return
					}
					a.secCur, a.secNew, a.secNew2, a.secErr, a.secOK = "", "", "", "", "已关闭锁定"
				})
				ui.PrimaryButton(c, "修改密码").OnClick(func() {
					if a.secNew != a.secNew2 {
						a.secErr, a.secOK = "两次输入的密码不一致", ""
						return
					}
					if err := a.be.SetLockPassword(a.secCur, a.secNew); err != nil {
						a.secErr, a.secOK = err.Error(), ""
						return
					}
					a.secCur, a.secNew, a.secNew2, a.secErr, a.secOK = "", "", "", "", "密码已修改"
				})
			})
		})
	})
}

func (a *app) secMessages(c *ui.Context, p palette) {
	if a.secErr != "" {
		ui.Text(c, a.secErr).TextColor(p.red).Wrap()
	}
	if a.secOK != "" {
		ui.Text(c, a.secOK).TextColor(p.green).Wrap()
	}
}

func publicIPSourceLabel(p enterprise.PublicIPSetting) string {
	switch p.Mode {
	case enterprise.PublicIPAPI:
		host := strings.TrimPrefix(strings.TrimPrefix(p.URL, "https://"), "http://")
		host, _, _ = strings.Cut(host, "/")
		return host
	case enterprise.PublicIPOff:
		return "未显示"
	default:
		return "服务端告知"
	}
}

func (a *app) loadPrefsForm() {
	a.prefsInit = true
	src := a.settings.PublicIP
	switch src.Mode {
	case enterprise.PublicIPAPI:
		a.ipMode = 1
	case enterprise.PublicIPOff:
		a.ipMode = 2
	default:
		a.ipMode = 0
	}
	a.ipURL = src.URL
	if a.ipURL == "" {
		a.ipURL = enterprise.DefaultPublicIPAPI
	}
}

func (a *app) prefsTab(c *ui.Context, p palette) {
	t := c.Theme()
	st := a.status
	ui.Scroll(c).Grow(1).Children(func() {
		card(c, p).Width(560).Gap(t.Space(3)).Padding(16).Children(func() {
			cardHeader(c, "globe", p.blue, "公网 IP 来源", nil)
			ui.Text(c, "“服务端告知”是 frps 看到的本机连接来源地址，不需要访问外部网站；frps 部署在同一内网时它会是内网地址。"+
				"“外部 API”向指定网址查询，返回纯文本 IP 或含 ip 字段的 JSON 都可以。").TextColor(t.TextMuted).Wrap()
			ui.Form(c, func() {
				ui.Field(c, "来源", func() {
					ui.Segmented(c, &a.ipMode, "服务端告知", "外部 API", "不显示")
				})
				if a.ipMode == 1 {
					ui.Field(c, "查询地址", func() {
						ui.TextInput(c.Key("ip-url"), &a.ipURL).Placeholder(enterprise.DefaultPublicIPAPI)
					}).Description("例如 https://api.ipify.org、https://ifconfig.me/ip、https://ipinfo.io/ip")
				}
				ui.Field(c, "当前结果", func() {
					v := st.PublicIP
					if v == "" {
						v = st.PublicIPError
					}
					if v == "" {
						v = "—"
					}
					ui.Text(c, v).Selectable()
				})
			})
			if a.prefsErr != "" {
				ui.Text(c, a.prefsErr).TextColor(p.red).Wrap()
			}
			if a.prefsMsg != "" {
				ui.Text(c, a.prefsMsg).TextColor(p.green)
			}
			ui.Row(c).Justify(ui.End).Children(func() {
				ui.PrimaryButton(c, "保存").OnClick(func() {
					mode := []string{enterprise.PublicIPServer, enterprise.PublicIPAPI, enterprise.PublicIPOff}[a.ipMode]
					if err := a.be.SetPublicIPSource(mode, a.ipURL); err != nil {
						a.prefsErr, a.prefsMsg = err.Error(), ""
						return
					}
					a.prefsErr, a.prefsMsg = "", "已保存"
					a.refresh()
				})
			})
		})
	})
}
