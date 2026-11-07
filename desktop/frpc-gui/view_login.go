package main

import (
	"github.com/egoist/mygo/ui"

	"github.com/fatedier/frp/client/enterprise"
)

func (a *app) loginView(c *ui.Context, p palette) {
	t := c.Theme()
	ui.Column(c).Fill().Center().Padding(24).Children(func() {
		card(c, p).Width(440).Gap(t.Space(4)).Padding(28).Children(func() {
			ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
				a.brandMark(c, p)
				ui.Column(c).Children(func() {
					ui.Text(c, AppName).Bold().FontSize(15)
					ui.Text(c, siteName(a.status.Branding)).FontSize(11.5).TextColor(t.TextMuted)
				})
			})
			ui.Text(c, "输入服务端地址、TOKEN 和鉴权码后连接。没有鉴权码时可以点“生成”，再把它发给管理员审批。").
				TextColor(t.TextMuted).Wrap()

			ui.Field(c, "服务端地址", func() {
				ui.TextInput(c.Key("addr"), &a.loginAddr).Placeholder("frp.example.com:7000")
			})
			ui.Field(c, "TOKEN", func() {
				ui.TextInput(c.Key("token"), &a.loginToken).Password().Placeholder("服务端 auth.token")
			})
			ui.Field(c, "鉴权码", func() {
				ui.Row(c).Gap(t.Space(2)).Children(func() {
					ui.TextInput(c.Key("code"), &a.loginCode).Placeholder("XXXX-XXXX-XXXX-XXXX").Grow(1)
					ui.Button(c, "生成").Tooltip("在本机生成一个鉴权码，提交给管理员审批").OnClick(func() {
						code, err := enterprise.GenerateAuthCode()
						if err != nil {
							a.loginErr = err.Error()
							return
						}
						a.loginCode = code
						a.codeGenerated = true
					})
				})
			}).Description(a.codeHint())

			if a.loginErr != "" {
				ui.Text(c, a.loginErr).TextColor(p.red).Wrap()
			}
			ui.Row(c).Justify(ui.End).Gap(t.Space(2)).Children(func() {
				if a.codeGenerated {
					ui.Button(c, "复制鉴权码").OnClick(func() {
						c.Services().WriteClipboard(a.loginCode)
					})
				}
				ui.PrimaryButton(c, "连接").OnClick(a.connect)
			})
		})
	})
}

func (a *app) codeHint() string {
	if a.codeGenerated {
		return "已生成新的鉴权码，请把它发给管理员审批；审批通过后会自动连接。"
	}
	return "管理员发给你的鉴权码，或在本机生成后提交审批。"
}

func (a *app) connect() {
	host, port, err := enterprise.ParseServerAddr(a.loginAddr)
	if err != nil {
		a.loginErr = err.Error()
		return
	}
	if err := a.be.Login(host, port, a.loginToken, a.loginCode); err != nil {
		a.loginErr = err.Error()
		return
	}
	a.loginErr = ""
	a.codeGenerated = false
	a.page = pageMain
	a.tab = tabOverview
	a.refresh()
}

func (a *app) lockedView(c *ui.Context, p palette) {
	t := c.Theme()
	ui.Column(c).Fill().Center().Padding(24).Children(func() {
		card(c, p).Width(340).Gap(t.Space(4)).Padding(24).AlignItems(ui.Center).Children(func() {
			a.brandMark(c, p)
			ui.Text(c, AppName).Bold().FontSize(15)
			ui.Text(c, "客户端已锁定").TextColor(t.TextMuted)
			ui.Row(c).Gap(t.Space(2)).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "运行状态")
				stateBadge(c, a.status.State)
			})
			dotMatrix(c, a.history, stateColor(t, a.status.State), p.dotOff, 22)
			if a.actionErr != "" {
				ui.Text(c, a.actionErr).TextColor(p.red).Wrap()
			}
			ui.Button(c, "重启").FillWidth().OnClick(func() {
				a.actionErr = ""
				if err := a.be.Restart(); err != nil {
					a.actionErr = "重启失败：" + err.Error()
				}
				a.refresh()
			})
			ui.Divider(c)
			ui.TextInput(c.Key("unlock"), &a.unlockPw).Password().Placeholder("输入密码解锁").Label("解锁密码").FillWidth().OnSubmit(a.unlock)
			if a.unlockErr != "" {
				ui.Text(c, a.unlockErr).TextColor(p.red)
			}
			ui.PrimaryButton(c, "解锁").FillWidth().OnClick(a.unlock)
		})
	})
}

func (a *app) unlock() {
	if !a.be.VerifyPassword(a.unlockPw) {
		a.unlockErr = "密码不正确"
		a.unlockPw = ""
		return
	}
	a.locked = false
	a.unlockPw = ""
	a.unlockErr = ""
	a.actionErr = ""
}
