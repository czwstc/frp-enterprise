// Copyright 2017 fatedier, fatedier@gmail.com
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

package server

import (
	"net/http"

	"github.com/gorilla/mux"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	httppkg "github.com/fatedier/frp/pkg/util/http"
	netpkg "github.com/fatedier/frp/pkg/util/net"
	"github.com/fatedier/frp/server/adminauth"
	"github.com/fatedier/frp/server/audit"
	adminapi "github.com/fatedier/frp/server/http"
)

func (svr *Service) registerRouteHandlers(helper *httppkg.RouterRegisterHelper) {
	helper.Router.HandleFunc("/healthz", healthz)

	// With dashboard accounts, the session middleware replaces basic auth and
	// the pages load without a session, to show the login page.
	authMiddleware := helper.AuthMiddleware
	if svr.adminAuth != nil {
		authMiddleware = svr.adminAuth.Middleware
		svr.adminAuth.RegisterPublic(helper.Router)
		svr.registerStaticHandlers(helper.Router, helper)
	} else {
		helper.Router.HandleFunc("/api/auth/config", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"code":200,"msg":"success","data":{"enabled":false}}`))
		}).Methods("GET")
	}

	subRouter := helper.Router.NewRoute().Subrouter()

	subRouter.Use(authMiddleware)
	subRouter.Use(httppkg.NewRequestLogger)

	// metrics
	if svr.cfg.EnablePrometheus {
		subRouter.Handle("/metrics", promhttp.Handler())
	}

	apiController := adminapi.NewController(svr.cfg, svr.clientRegistry, svr.pxyManager)

	// apis
	subRouter.HandleFunc("/api/serverinfo", httppkg.MakeHTTPHandlerFunc(apiController.APIServerInfo)).Methods("GET")
	subRouter.HandleFunc("/api/proxy/{type}", httppkg.MakeHTTPHandlerFunc(apiController.APIProxyByType)).Methods("GET")
	subRouter.HandleFunc("/api/proxy/{type}/{name}", httppkg.MakeHTTPHandlerFunc(apiController.APIProxyByTypeAndName)).Methods("GET")
	subRouter.HandleFunc("/api/proxies/{name}", httppkg.MakeHTTPHandlerFunc(apiController.APIProxyByName)).Methods("GET")
	subRouter.HandleFunc("/api/traffic/{name}", httppkg.MakeHTTPHandlerFunc(apiController.APIProxyTraffic)).Methods("GET")
	subRouter.HandleFunc("/api/clients", httppkg.MakeHTTPHandlerFunc(apiController.APIClientList)).Methods("GET")
	subRouter.HandleFunc("/api/clients/{key}", httppkg.MakeHTTPHandlerFunc(apiController.APIClientDetail)).Methods("GET")
	subRouter.HandleFunc("/api/proxies", httppkg.MakeHTTPHandlerFunc(apiController.DeleteProxies)).Methods("DELETE")

	entController := adminapi.NewEnterpriseController(svr.entHub, svr.auditDashboard)
	subRouter.HandleFunc("/api/enterprise/status", httppkg.MakeHTTPHandlerFuncV2(entController.APIStatus)).Methods("GET")
	subRouter.HandleFunc("/api/enterprise/codes", httppkg.MakeHTTPHandlerFuncV2(entController.APIListCodes)).Methods("GET")
	subRouter.HandleFunc("/api/enterprise/codes", httppkg.MakeHTTPHandlerFuncV2(entController.APIGenerateCode)).Methods("POST")
	subRouter.HandleFunc("/api/enterprise/codes/{code}", httppkg.MakeHTTPHandlerFuncV2(entController.APIDeleteCode)).Methods("DELETE")
	subRouter.HandleFunc("/api/enterprise/codes/{code}/{action}", httppkg.MakeHTTPHandlerFuncV2(entController.APICodeAction)).Methods("POST")
	subRouter.HandleFunc("/api/enterprise/notices", httppkg.MakeHTTPHandlerFuncV2(entController.APIListNotices)).Methods("GET")
	subRouter.HandleFunc("/api/enterprise/notices", httppkg.MakeHTTPHandlerFuncV2(entController.APISendNotice)).Methods("POST")
	subRouter.HandleFunc("/api/enterprise/ports", httppkg.MakeHTTPHandlerFuncV2(entController.APIPorts)).Methods("GET")

	subRouter.HandleFunc("/api/v2/users", httppkg.MakeHTTPHandlerFuncV2(apiController.APIV2UserList)).Methods("GET")
	subRouter.HandleFunc("/api/v2/system/info", httppkg.MakeHTTPHandlerFuncV2(apiController.APIV2SystemInfo)).Methods("GET")
	subRouter.HandleFunc("/api/v2/system/prune", httppkg.MakeHTTPHandlerFuncV2(apiController.APIV2SystemPrune)).Methods("POST")
	subRouter.HandleFunc("/api/v2/clients", httppkg.MakeHTTPHandlerFuncV2(apiController.APIV2ClientList)).Methods("GET")
	v2EncodedPathRouter := subRouter.NewRoute().Subrouter()
	v2EncodedPathRouter.UseEncodedPath()
	v2EncodedPathRouter.HandleFunc("/api/v2/clients/{key}", httppkg.MakeHTTPHandlerFuncV2(apiController.APIV2ClientDetail)).Methods("GET")
	subRouter.HandleFunc("/api/v2/proxies", httppkg.MakeHTTPHandlerFuncV2(apiController.APIV2ProxyList)).Methods("GET")
	v2EncodedPathRouter.HandleFunc("/api/v2/proxies/{name}/traffic", httppkg.MakeHTTPHandlerFuncV2(apiController.APIV2ProxyTraffic)).Methods("GET")
	v2EncodedPathRouter.HandleFunc("/api/v2/proxies/{name}", httppkg.MakeHTTPHandlerFuncV2(apiController.APIV2ProxyDetail)).Methods("GET")

	if svr.adminAuth != nil {
		svr.adminAuth.RegisterPrivate(subRouter)
		logController := adminapi.NewLogController(svr.auditLog)
		subRouter.HandleFunc("/api/admin/audit", httppkg.MakeHTTPHandlerFuncV2(logController.APIAudit)).Methods("GET")
		subRouter.HandleFunc("/api/admin/logs", httppkg.MakeHTTPHandlerFuncV2(logController.APIRuntimeLogs)).Methods("GET")
		return
	}

	// view
	svr.registerStaticHandlers(subRouter, helper)
}

func (svr *Service) registerStaticHandlers(r *mux.Router, helper *httppkg.RouterRegisterHelper) {
	r.Handle("/favicon.ico", http.FileServer(helper.AssetsFS)).Methods("GET")
	r.PathPrefix("/static/").Handler(
		netpkg.MakeHTTPGzipHandler(http.StripPrefix("/static/", http.FileServer(helper.AssetsFS))),
	).Methods("GET")
	r.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/static/", http.StatusMovedPermanently)
	})
}

// auditDashboard records an action a dashboard user did.
func (svr *Service) auditDashboard(r *http.Request, action, target, detail string) {
	actor := adminauth.Actor(r)
	if actor == "" {
		actor = "dashboard"
	}
	svr.auditLog.Record(audit.Entry{Actor: actor, Action: action, Target: target, Detail: detail, IP: adminauth.ClientIP(r), Success: true})
}

func healthz(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(200)
}
