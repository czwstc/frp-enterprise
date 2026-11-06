package main

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/fatedier/frp/client/enterprise"
)

func (a *app) mainView(c *ui.Context) {
	t := c.Theme()
	st := a.status
	ui.Column(c).Fill().Children(func() {
		// Header.
		ui.Row(c).Padding(14, 20).Gap(t.Space(3)).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "FRP 企业客户端").FontSize(18).Bold()
			stateBadge(c, st.State)
			ui.Spacer(c)
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
				color = t.Warning
			case enterprise.StateRejected, enterprise.StateError:
				color = t.Danger
			}
			ui.Text(c, st.Message).TextColor(color).Wrap().PaddingX(20).PaddingY(2)
		}
		if a.actionErr != "" {
			ui.Text(c, a.actionErr).TextColor(t.Danger).Wrap().PaddingX(20).PaddingY(2)
		}
		noticeTab := "通知"
		if n := len(st.Notices); n > 0 {
			noticeTab = fmt.Sprintf("通知 (%d)", n)
		}
		ui.Row(c).PaddingX(20).PaddingY(6).Children(func() {
			ui.Tabs(c, &a.tab, "概览", "端口映射", "服务端端口", noticeTab, "日志", "安全")
		})
		ui.Column(c).Grow(1).Padding(8, 20, 20, 20).Children(func() {
			switch a.tab {
			case tabOverview:
				a.overviewTab(c)
			case tabProxies:
				a.proxiesTab(c)
			case tabPorts:
				a.portsTab(c)
			case tabNotices:
				a.noticesTab(c)
			case tabLogs:
				a.logsTab(c)
			case tabSecurity:
				a.securityTab(c)
			}
		})
	})
	ui.Modal(c, &a.editing, func() { a.proxyEditor(c) })
}

func (a *app) restart() {
	a.actionErr = ""
	if err := a.be.Restart(); err != nil {
		a.actionErr = "连接失败：" + err.Error()
	}
	a.refresh()
}

func infoRow(c *ui.Context, label, value string) {
	t := c.Theme()
	ui.Row(c).Gap(t.Space(4)).PaddingY(6).Children(func() {
		ui.Text(c, label).TextColor(t.TextMuted).Width(110)
		ui.Text(c, value).Selectable().Grow(1).Wrap()
	})
}

func (a *app) overviewTab(c *ui.Context) {
	t := c.Theme()
	st := a.status
	ui.Scroll(c).Grow(1).Children(func() {
		ui.Column(c).Padding(16).Radius(10).Border(1, t.Border).Children(func() {
			infoRow(c, "运行状态", st.State.Label())
			infoRow(c, "服务端", st.ServerAddr)
			publicIP := st.PublicIP
			if publicIP == "" {
				publicIP = "连接后由服务端告知"
			}
			infoRow(c, "当前公网 IP", publicIP)
			infoRow(c, "本机 IP", joinOr(st.LocalIPs, "无"))
			infoRow(c, "设备 ID", st.DeviceID)
			ui.Row(c).Gap(t.Space(4)).PaddingY(6).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "鉴权码").TextColor(t.TextMuted).Width(110)
				ui.Text(c, st.AuthCode).Selectable()
				ui.Button(c, "复制").OnClick(func() {
					c.Services().WriteClipboard(st.AuthCode)
				})
			})
			if !st.ConnectedAt.IsZero() {
				infoRow(c, "已连接", formatDuration(time.Since(st.ConnectedAt)))
				c.After(time.Second)
			}
			running := 0
			for _, p := range st.Proxies {
				if p.Phase == "running" {
					running++
				}
			}
			infoRow(c, "端口映射", fmt.Sprintf("%d 个，运行中 %d 个", len(st.Proxies), running))
			mine, others := 0, 0
			for _, p := range st.Ports {
				if p.Mine {
					mine++
				} else {
					others++
				}
			}
			infoRow(c, "服务端端口", fmt.Sprintf("本机占用 %d 个，其他客户端占用 %d 个", mine, others))
		})
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

func (a *app) proxiesTab(c *ui.Context) {
	t := c.Theme()
	st := a.status
	ui.Row(c).Gap(t.Space(2)).PaddingY(6).AlignItems(ui.Center).Children(func() {
		ui.Text(c, "本机端口映射：把本机服务通过服务端的远端端口暴露出去。").TextColor(t.TextMuted).Grow(1)
		ui.PrimaryButton(c, "新增映射").OnClick(func() { a.openEditor(nil) })
	})
	header := func() {
		ui.Row(c).PaddingY(6).PaddingX(10).Gap(t.Space(2)).Background(t.Surface).Radius(6).Children(func() {
			ui.Text(c, "名称").Bold().Width(120)
			ui.Text(c, "协议").Bold().Width(50)
			ui.Text(c, "本地地址").Bold().Width(150)
			ui.Text(c, "远端端口").Bold().Width(80)
			ui.Text(c, "状态").Bold().Grow(1)
			ui.Text(c, "操作").Bold().Width(120)
		})
	}
	header()
	ui.Scroll(c).Grow(1).Children(func() {
		if len(st.Proxies) == 0 {
			ui.Text(c, "还没有端口映射，点“新增映射”添加。").TextColor(t.TextMuted).Padding(12)
		}
		for _, p := range st.Proxies {
			ui.Row(c.Key(p.Name)).PaddingY(8).PaddingX(10).Gap(t.Space(2)).AlignItems(ui.Center).Children(func() {
				ui.Text(c, p.Name).Width(120).SingleLine()
				ui.Text(c, p.Type).Width(50)
				ui.Text(c, fmt.Sprintf("%s:%d", p.LocalIP, p.LocalPort)).Width(150).SingleLine()
				remote := strconv.Itoa(p.RemotePort)
				if p.RemotePort == 0 {
					remote = "自动"
					if p.RemoteAddr != "" {
						remote = "自动" + p.RemoteAddr
					}
				}
				ui.Text(c, remote).Width(80)
				ui.Column(c).Grow(1).Children(func() {
					color := t.TextMuted
					switch p.Phase {
					case "running":
						color = t.Success
					case "start error", "check failed":
						color = t.Danger
					}
					ui.Text(c, proxyPhaseLabel(p.Phase)).TextColor(color)
					if p.Err != "" {
						ui.Text(c, p.Err).TextColor(t.Danger).FontSize(11).Wrap()
					}
				})
				ui.Row(c).Width(120).Gap(t.Space(1)).Children(func() {
					ps := p.ProxySetting
					ui.Button(c, "编辑").Label("编辑 " + p.Name).OnClick(func() { a.openEditor(&ps) })
					ui.Button(c, "删除").Label("删除 " + p.Name).OnClick(func() {
						if err := a.be.DeleteProxy(ps.Name); err != nil {
							a.actionErr = err.Error()
						}
						a.refresh()
					})
				})
			})
			ui.Divider(c)
		}
	})
}

// remotePortError is the reason the edited remote port cannot be used.
func (a *app) remotePortError() string {
	p := a.editedProxy()
	if p.RemotePort == 0 {
		return ""
	}
	if err := a.be.CheckRemotePort(p.Type, p.RemotePort, a.editOld); err != nil {
		return err.Error()
	}
	return ""
}

func (a *app) proxyEditor(c *ui.Context) {
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
			ui.Text(c, a.edErr).TextColor(t.Danger).Wrap()
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

func (a *app) portsTab(c *ui.Context) {
	t := c.Theme()
	st := a.status
	allow := st.AllowPorts
	if allow == "" {
		allow = "不限"
	}
	ui.Text(c, "服务端上已经被转发的端口。被其他客户端占用的端口无法再映射。允许的端口范围："+allow).
		TextColor(t.TextMuted).Wrap().PaddingY(6)
	ui.Row(c).PaddingY(6).PaddingX(10).Gap(t.Space(2)).Background(t.Surface).Radius(6).Children(func() {
		ui.Text(c, "协议").Bold().Width(60)
		ui.Text(c, "端口").Bold().Width(80)
		ui.Text(c, "占用方").Bold().Grow(1)
	})
	ui.Scroll(c).Grow(1).Children(func() {
		if len(st.Ports) == 0 {
			msg := "服务端上暂无已转发的端口。"
			if st.State != enterprise.StateRunning {
				msg = "连接服务端后显示。"
			}
			ui.Text(c, msg).TextColor(t.TextMuted).Padding(12)
		}
		for _, p := range st.Ports {
			ui.Row(c.Key(fmt.Sprintf("%s/%d", p.Proto, p.Port))).PaddingY(7).PaddingX(10).Gap(t.Space(2)).AlignItems(ui.Center).Children(func() {
				ui.Text(c, p.Proto).Width(60)
				ui.Text(c, strconv.Itoa(p.Port)).Width(80)
				ui.Row(c).Grow(1).Gap(t.Space(2)).AlignItems(ui.Center).Children(func() {
					if p.Mine {
						ui.Badge(c, "本机").Background(t.Accent).TextColor(t.AccentText)
						ui.Text(c, displayProxyName(p.ProxyName)).TextColor(t.TextMuted)
					} else {
						ui.Badge(c, "其他客户端").Background(t.Warning).TextColor(ui.RGB(255, 255, 255))
					}
				})
			})
			ui.Divider(c)
		}
	})
}

// displayProxyName removes the device prefix frps adds to proxy names.
func displayProxyName(name string) string {
	if _, rest, ok := strings.Cut(name, "."); ok {
		return rest
	}
	return name
}

func (a *app) noticesTab(c *ui.Context) {
	t := c.Theme()
	st := a.status
	ui.Scroll(c).Grow(1).Gap(t.Space(2)).Children(func() {
		if len(st.Notices) == 0 {
			ui.Text(c, "暂无服务端通知。").TextColor(t.TextMuted).Padding(12)
		}
		for _, n := range st.Notices {
			ui.Column(c.Key(n.ID)).Padding(12).Gap(4).Radius(8).Border(1, t.Border).Children(func() {
				ui.Row(c).Gap(t.Space(2)).AlignItems(ui.Center).Children(func() {
					color := t.Accent
					switch n.Level {
					case "warning":
						color = t.Warning
					case "error":
						color = t.Danger
					case "success":
						color = t.Success
					}
					ui.Badge(c, noticeLevelLabel(n)).Background(color).TextColor(ui.RGB(255, 255, 255))
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

func (a *app) logsTab(c *ui.Context) {
	t := c.Theme()
	lines := a.filteredLogs()
	ui.Row(c).Gap(t.Space(2)).PaddingY(6).AlignItems(ui.Center).Children(func() {
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
			color = t.Warning
		case "error":
			color = t.Danger
		case "debug", "trace":
			color = t.TextMuted
		}
		ui.Row(c).Gap(t.Space(2)).PaddingY(2).Children(func() {
			ui.Text(c, l.Time.Format("15:04:05")).TextColor(t.TextMuted).Font("monospace").FontSize(12)
			ui.Text(c, l.Text).TextColor(color).Font("monospace").FontSize(12).Wrap().Grow(1).Selectable()
		})
	}).Grow(1).Padding(8).Radius(8).Border(1, t.Border).Children(func() {
		if len(lines) == 0 {
			ui.Text(c, "暂无日志。").TextColor(t.TextMuted)
		}
	})
}

func (a *app) securityTab(c *ui.Context) {
	t := c.Theme()
	ui.Scroll(c).Grow(1).Children(func() {
		ui.Column(c).Width(460).Gap(t.Space(3)).Padding(16).Radius(10).Border(1, t.Border).Children(func() {
			if !a.be.LockEnabled() {
				ui.Text(c, "加密锁定").FontSize(16).Bold()
				ui.Text(c, "开启后，客户端界面只显示运行状态和重启按钮，输入密码后才能查看全部内容。配置文件始终加密保存在本机。").
					TextColor(t.TextMuted).Wrap()
				ui.Form(c, func() {
					ui.Field(c, "新密码", func() { ui.TextInput(c.Key("sec-new"), &a.secNew).Password() })
					ui.Field(c, "确认密码", func() { ui.TextInput(c.Key("sec-new2"), &a.secNew2).Password() })
				})
				a.secMessages(c)
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
			ui.Text(c, "加密锁定已开启").FontSize(16).Bold()
			ui.Row(c).Gap(t.Space(2)).Children(func() {
				ui.PrimaryButton(c, "立即锁定").OnClick(func() { a.locked = true })
			})
			ui.Divider(c)
			ui.Text(c, "修改或关闭密码").Bold()
			ui.Form(c, func() {
				ui.Field(c, "当前密码", func() { ui.TextInput(c.Key("sec-cur"), &a.secCur).Password() })
				ui.Field(c, "新密码", func() { ui.TextInput(c.Key("sec-new"), &a.secNew).Password() })
				ui.Field(c, "确认密码", func() { ui.TextInput(c.Key("sec-new2"), &a.secNew2).Password() })
			})
			a.secMessages(c)
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

func (a *app) secMessages(c *ui.Context) {
	t := c.Theme()
	if a.secErr != "" {
		ui.Text(c, a.secErr).TextColor(t.Danger).Wrap()
	}
	if a.secOK != "" {
		ui.Text(c, a.secOK).TextColor(t.Success).Wrap()
	}
}
