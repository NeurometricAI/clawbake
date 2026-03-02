package handler

import (
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/clawbake/clawbake/internal/auth"
)

func (h *Handler) RegisterRoutes(e *echo.Echo) {
	// Public
	e.GET("/healthz", h.HealthCheck)
	e.POST("/internal/notifications/instance-ready", h.NotifyInstanceReady)
	e.Static("/static", "web/static")

	// Slack bot (no auth middleware — uses Slack signature verification)
	if h.Bot != nil {
		h.Bot.RegisterRoutes(e.Group("/slack"))
	}

	if h.Auth != nil {
		// Intercept WebSocket upgrades at any path and proxy to user's instance
		e.Use(h.WebSocketMiddleware(h.Auth.RequireAuth))

		e.GET("/auth/login", h.Auth.LoginHandler)
		e.GET("/auth/callback", h.Auth.CallbackHandler)
		e.GET("/auth/logout", h.Auth.LogoutHandler)

		// Authenticated API
		api := e.Group("/api", h.Auth.RequireAuth)
		api.GET("/instances", h.ListInstances)
		api.POST("/instances", h.CreateInstance)
		api.GET("/instances/:id", h.GetInstance)
		api.DELETE("/instances/:id", h.DeleteInstance)

		// Admin API
		admin := api.Group("/admin", h.Auth.RequireAdmin)
		admin.GET("/users", h.ListUsers)
		admin.GET("/defaults", h.GetDefaults)
		admin.PUT("/defaults", h.UpdateDefaults)
		admin.GET("/defaults/gateway-config/default", h.GetDefaultGatewayConfig)

		// Reverse proxy to user's instance
		e.Any("/proxy/*", h.ProxyToInstance, h.Auth.RequireAuth)

		// UI pages
		e.GET("/", h.PageDashboard, h.Auth.OptionalAuth)

		requireAuthUI := func(next echo.HandlerFunc) echo.HandlerFunc {
			return func(c echo.Context) error {
				user := auth.UserFromContext(c.Request().Context())
				if user == nil {
					if c.Request().Header.Get("HX-Request") == "true" {
						c.Response().Header().Set("HX-Redirect", "/")
						return c.NoContent(http.StatusOK)
					}
					return c.Redirect(http.StatusFound, "/")
				}
				return next(c)
			}
		}

		ui := e.Group("/ui", h.Auth.OptionalAuth, requireAuthUI)
		ui.POST("/instances", h.PageCreateInstance)
		ui.GET("/instances/:id", h.PageInstanceDetail)
		ui.GET("/instances/:id/status", h.PageInstanceStatus)
		ui.DELETE("/instances/:id", h.PageDeleteInstance)
		ui.GET("/admin/users", h.PageAdminUsers)
		ui.GET("/admin/defaults", h.PageAdminDefaults)
		ui.POST("/admin/defaults", h.PageUpdateDefaults)
	} else if h.DevAuth != nil {
		// Intercept WebSocket upgrades at any path and proxy to user's instance
		e.Use(h.WebSocketMiddleware(h.DevAuth.RequireAuth))

		e.GET("/auth/dev-login", h.DevAuth.LoginHandler)
		e.GET("/auth/logout", h.DevAuth.LogoutHandler)

		// Authenticated API
		api := e.Group("/api", h.DevAuth.RequireAuth)
		api.GET("/instances", h.ListInstances)
		api.POST("/instances", h.CreateInstance)
		api.GET("/instances/:id", h.GetInstance)
		api.DELETE("/instances/:id", h.DeleteInstance)

		// Admin API
		admin := api.Group("/admin", h.DevAuth.RequireAdmin)
		admin.GET("/users", h.ListUsers)
		admin.GET("/defaults", h.GetDefaults)
		admin.PUT("/defaults", h.UpdateDefaults)
		admin.GET("/defaults/gateway-config/default", h.GetDefaultGatewayConfig)

		// Reverse proxy to user's instance
		e.Any("/proxy/*", h.ProxyToInstance, h.DevAuth.RequireAuth)

		// UI pages (dev mode)
		e.GET("/", h.PageDashboard, h.DevAuth.OptionalAuth)

		requireAuthUI := func(next echo.HandlerFunc) echo.HandlerFunc {
			return func(c echo.Context) error {
				user := auth.UserFromContext(c.Request().Context())
				if user == nil {
					if c.Request().Header.Get("HX-Request") == "true" {
						c.Response().Header().Set("HX-Redirect", "/")
						return c.NoContent(http.StatusOK)
					}
					return c.Redirect(http.StatusFound, "/")
				}
				return next(c)
			}
		}

		ui := e.Group("/ui", h.DevAuth.OptionalAuth, requireAuthUI)
		ui.POST("/instances", h.PageCreateInstance)
		ui.GET("/instances/:id", h.PageInstanceDetail)
		ui.GET("/instances/:id/status", h.PageInstanceStatus)
		ui.DELETE("/instances/:id", h.PageDeleteInstance)
		ui.GET("/admin/users", h.PageAdminUsers)
		ui.GET("/admin/defaults", h.PageAdminDefaults)
		ui.POST("/admin/defaults", h.PageUpdateDefaults)
	}
}
