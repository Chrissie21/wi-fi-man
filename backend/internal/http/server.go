package http

import (
	"html/template"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"wifi-man/backend/internal/config"
	"wifi-man/backend/internal/http/handlers"
	"wifi-man/backend/internal/http/middleware"
)

type Dependencies struct {
	Cfg                config.Config
	Portal             *handlers.PortalHandler
	Admin              *handlers.AdminHandler
	Accounting         *handlers.AccountingHandler
	Payment            *handlers.PaymentHandler
	Gateway            *handlers.GatewayHandler
	Health             *handlers.HealthHandler
	PortalRateLimit    gin.HandlerFunc
	WebhookRateLimiter gin.HandlerFunc
}

func NewRouter(dep Dependencies) *gin.Engine {
	r := gin.New()
	r.Use(gin.Recovery(), middleware.RequestID(), middleware.StructuredLogger())
	if dep.Cfg.MetricsEnabled {
		r.Use(middleware.Metrics())
	}

	tmpl := template.Must(template.ParseGlob("templates/*.tmpl"))
	r.SetHTMLTemplate(tmpl)
	r.Static("/static", "web/static")

	r.GET("/healthz", dep.Health.Health)
	if dep.Cfg.MetricsEnabled {
		r.GET("/metrics", gin.WrapH(promhttp.Handler()))
	}

	r.GET("/portal", dep.Portal.Page)
	r.POST("/portal/redeem", dep.Portal.RedeemFragment)
	r.POST("/portal/purchase", dep.Portal.PurchaseFragment)
	r.GET("/portal/plans/fragment", dep.Portal.PlanCardsFragment)

	v1 := r.Group("/v1")
	{
		portal := v1.Group("/portal")
		if dep.PortalRateLimit != nil {
			portal.Use(dep.PortalRateLimit)
		}
		portal.POST("/redeem", dep.Portal.Redeem)
		portal.POST("/resume", dep.Portal.Resume)
		portal.GET("/plans", dep.Portal.ListPlans)
		portal.POST("/purchase", dep.Portal.Purchase)

		admin := v1.Group("/admin")
		admin.Use(middleware.RequireRole(dep.Cfg.AdminTokenHeader, "super_admin", "cashier", "support"))
		admin.GET("/plans", dep.Admin.ListPlans)
		admin.POST("/plans", dep.Admin.CreatePlan)
		admin.POST("/tokens/generate", dep.Admin.GenerateTokens)
		admin.POST("/tokens/:id/revoke", dep.Admin.RevokeToken)
		admin.GET("/sessions/active", dep.Admin.ActiveSessions)
		admin.GET("/reports/sales", dep.Admin.SalesReport)

		accounting := v1.Group("/accounting")
		accounting.POST("/start", dep.Accounting.Start)
		accounting.POST("/interim", dep.Accounting.Interim)
		accounting.POST("/stop", dep.Accounting.Stop)

		gateway := v1.Group("/gateway")
		gateway.POST("/disconnect", dep.Gateway.Disconnect)

		payments := v1.Group("/payments")
		if dep.WebhookRateLimiter != nil {
			payments.POST("/mobilemoney/webhook", dep.WebhookRateLimiter, dep.Payment.MobileMoneyWebhook)
		} else {
			payments.POST("/mobilemoney/webhook", dep.Payment.MobileMoneyWebhook)
		}
		payments.POST("/manual/record", dep.Payment.ManualRecord)
	}

	r.GET("/", func(c *gin.Context) {
		c.Redirect(http.StatusFound, "/portal")
	})

	return r
}
