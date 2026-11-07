// Command frpc-gui is the desktop client of frp enterprise: it logs in to
// frps with an auth code, maps local ports, and shows the logs, the ports in
// use on the server and the server's notices.
package main

import (
	"log"
	"os"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/fatedier/frp/client/enterprise"
	frplog "github.com/fatedier/frp/pkg/util/log"
)

func main() {
	logs := enterprise.NewLogBuffer(2000, os.Stderr)
	frplog.InitLoggerWithWriter(logs, "info")

	path, err := enterprise.DefaultSettingsPath()
	if err != nil {
		log.Fatal(err)
	}
	if p := os.Getenv("FRPC_GUI_CONFIG"); p != "" {
		path = p
	}
	mgr, err := enterprise.NewManager(enterprise.NewSettingsStore(path, nil), logs)
	if err != nil {
		log.Fatal(err)
	}
	a := newApp(managerBackend{mgr})
	if a.settings.AutoConnect {
		if err := mgr.Start(); err != nil {
			a.actionErr = "自动连接失败：" + err.Error()
		}
		a.refresh()
	}

	mygo.App.WhenReady(func() {
		win := mygo.NewWindow(mygo.WindowOptions{
			Title:     AppName,
			Width:     920,
			Height:    640,
			MinWidth:  720,
			MinHeight: 480,
			StateKey:  "main",
			Content:   ui.View(a.view),
		})
		// Events of the embedded frpc arrive on its goroutines; the state
		// changes on the main thread.
		mgr.OnChange(func() { win.Update(a.refresh) })
		go func() {
			for range time.Tick(time.Second) {
				win.Update(a.refresh)
			}
		}()
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
	mgr.Stop()
}
