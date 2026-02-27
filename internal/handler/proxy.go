package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	"github.com/labstack/echo/v4"

	v1alpha1 "github.com/clawbake/clawbake/api/v1alpha1"
	"github.com/clawbake/clawbake/internal/auth"
	"github.com/clawbake/clawbake/internal/k8s"
)

const (
	proxyPrefixWeb = "/proxy/web"
	proxyPrefixTUI = "/proxy/tui"
	portWeb        = 18789
	portTUI        = 7681
)

type proxyRoute struct {
	port   int
	prefix string
}

// classifyProxyPath determines backend port and route prefix from the request path.
func classifyProxyPath(path string) proxyRoute {
	if strings.HasPrefix(path, proxyPrefixTUI) {
		return proxyRoute{port: portTUI, prefix: proxyPrefixTUI}
	}
	return proxyRoute{port: portWeb, prefix: proxyPrefixWeb}
}

// stripRoutePrefix removes the given prefix from the path, returning "/" if nothing remains.
func stripRoutePrefix(path, prefix string) string {
	if len(path) > len(prefix) {
		return path[len(prefix):]
	}
	return "/"
}

func isWebSocketUpgrade(r *http.Request) bool {
	return strings.EqualFold(r.Header.Get("Upgrade"), "websocket")
}

// WebSocketMiddleware intercepts WebSocket upgrade requests at any path and proxies
// them to the authenticated user's openclaw instance.
func (h *Handler) WebSocketMiddleware(requireAuth echo.MiddlewareFunc) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			if !isWebSocketUpgrade(c.Request()) {
				return next(c)
			}
			// Run auth middleware first, then proxy
			return requireAuth(func(c echo.Context) error {
				user := auth.UserFromContext(c.Request().Context())
				if user == nil {
					return echo.NewHTTPError(http.StatusUnauthorized, "authentication required")
				}
				userID, _ := user.ID.Value()
				uid, _ := userID.(string)

				instance, err := k8s.GetInstance(c.Request().Context(), h.K8s, h.Config.KubeNamespace, uid)
				if err != nil {
					return echo.NewHTTPError(http.StatusNotFound, "no instance found")
				}
				if instance.Status.Phase != v1alpha1.PhaseRunning {
					return echo.NewHTTPError(http.StatusServiceUnavailable, "instance is not running")
				}

				ns := instance.Status.Namespace
				if ns == "" {
					ns = fmt.Sprintf("clawbake-%s", uid)
				}

				route := classifyProxyPath(c.Request().URL.Path)
				target, _ := url.Parse(fmt.Sprintf("http://openclaw.%s.svc.cluster.local:%d", ns, route.port))

				token := ""
				if route.port == portWeb {
					token = instance.Spec.GatewayToken
				}

				return h.proxyWebSocket(c, target, token, route.prefix)
			})(c)
		}
	}
}

func (h *Handler) ProxyToInstance(c echo.Context) error {
	reqPath := c.Request().URL.Path

	// Redirect bare /proxy/ to /proxy/web/
	if reqPath == "/proxy" || reqPath == "/proxy/" {
		return c.Redirect(http.StatusFound, "/proxy/web/")
	}

	user := auth.UserFromContext(c.Request().Context())
	userID, _ := user.ID.Value()
	uid, _ := userID.(string)

	instance, err := k8s.GetInstance(c.Request().Context(), h.K8s, h.Config.KubeNamespace, uid)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "no instance found")
	}

	if instance.Status.Phase != v1alpha1.PhaseRunning {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "instance is not running")
	}

	ns := instance.Status.Namespace
	if ns == "" {
		ns = fmt.Sprintf("clawbake-%s", uid)
	}

	route := classifyProxyPath(reqPath)
	target, _ := url.Parse(fmt.Sprintf("http://openclaw.%s.svc.cluster.local:%d", ns, route.port))

	// Only inject token for web UI
	token := ""
	if route.port == portWeb {
		token = instance.Spec.GatewayToken
	}

	if isWebSocketUpgrade(c.Request()) {
		return h.proxyWebSocket(c, target, token, route.prefix)
	}

	// Inject token as query param so the control UI uses it for WebSocket auth.
	requestPath := stripRoutePrefix(reqPath, route.prefix)
	if route.port == portWeb && requestPath == "/" && token != "" && c.QueryParam("token") == "" {
		return c.Redirect(http.StatusFound, fmt.Sprintf("/proxy/web/?token=%s", url.QueryEscape(token)))
	}

	proxy := &httputil.ReverseProxy{
		Director: func(req *http.Request) {
			req.URL.Scheme = target.Scheme
			req.URL.Host = target.Host
			req.Host = target.Host
			req.URL.Path = stripRoutePrefix(req.URL.Path, route.prefix)
			if token != "" {
				req.Header.Set("X-OpenClaw-Token", token)
			}
		},
		ModifyResponse: func(resp *http.Response) error {
			// Rewrite basePath in control UI config so WebSocket URLs go through /proxy/web
			if requestPath != "/__openclaw/control-ui-config.json" {
				return nil
			}
			body, err := io.ReadAll(resp.Body)
			resp.Body.Close()
			if err != nil {
				return err
			}
			var cfg map[string]any
			if err := json.Unmarshal(body, &cfg); err != nil {
				resp.Body = io.NopCloser(bytes.NewReader(body))
				return nil
			}
			cfg["basePath"] = "/proxy/web"
			modified, err := json.Marshal(cfg)
			if err != nil {
				resp.Body = io.NopCloser(bytes.NewReader(body))
				return nil
			}
			resp.Body = io.NopCloser(bytes.NewReader(modified))
			resp.ContentLength = int64(len(modified))
			resp.Header.Set("Content-Length", fmt.Sprintf("%d", len(modified)))
			return nil
		},
	}

	proxy.ServeHTTP(c.Response(), c.Request())
	return nil
}

func (h *Handler) proxyWebSocket(c echo.Context, target *url.URL, token string, prefix string) error {
	req := c.Request()

	// Connect to backend
	backendConn, err := net.Dial("tcp", target.Host)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadGateway, "failed to connect to instance")
	}
	defer backendConn.Close()

	// Build the upgrade request for the backend.
	// Strip the route prefix if present, otherwise forward the path as-is.
	path := req.URL.Path
	if strings.HasPrefix(path, prefix) {
		path = stripRoutePrefix(path, prefix)
	}
	if req.URL.RawQuery != "" {
		path += "?" + req.URL.RawQuery
	}

	outReq, _ := http.NewRequest(req.Method, path, nil)
	outReq.Header = req.Header.Clone()
	outReq.Host = target.Host
	// Rewrite Origin to match the backend so the gateway's origin check passes
	outReq.Header.Set("Origin", fmt.Sprintf("http://%s", target.Host))
	if token != "" {
		outReq.Header.Set("X-OpenClaw-Token", token)
	}

	if err := outReq.Write(backendConn); err != nil {
		return echo.NewHTTPError(http.StatusBadGateway, "failed to send upgrade request")
	}

	// Hijack the client connection
	hijacker, ok := c.Response().Writer.(http.Hijacker)
	if !ok {
		return echo.NewHTTPError(http.StatusInternalServerError, "hijack not supported")
	}
	clientConn, clientBuf, err := hijacker.Hijack()
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to hijack connection")
	}
	defer clientConn.Close()

	// Pipe data bidirectionally — the backend's 101 response flows to the client,
	// then WebSocket frames flow in both directions.
	errc := make(chan error, 2)
	go func() {
		_, err := io.Copy(clientConn, backendConn)
		errc <- err
	}()
	go func() {
		// Flush any data buffered by the HTTP server's reader
		if n := clientBuf.Reader.Buffered(); n > 0 {
			buf := make([]byte, n)
			clientBuf.Read(buf)
			backendConn.Write(buf)
		}
		_, err := io.Copy(backendConn, clientConn)
		errc <- err
	}()

	<-errc
	return nil
}
