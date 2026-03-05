package handler

import "testing"

func TestClassifyProxyPath(t *testing.T) {
	tests := []struct {
		path       string
		wantPort   int
		wantPrefix string
	}{
		{"/proxy/web/", portWeb, proxyPrefixWeb},
		{"/proxy/web/some/path", portWeb, proxyPrefixWeb},
		{"/proxy/tui/", portTUI, proxyPrefixTUI},
		{"/proxy/tui/ws", portTUI, proxyPrefixTUI},
		{"/proxy/shell/", portShell, proxyPrefixShell},
		{"/proxy/shell/ws", portShell, proxyPrefixShell},
		{"/proxy/unknown", portWeb, proxyPrefixWeb},
		{"/proxy/", portWeb, proxyPrefixWeb},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			route := classifyProxyPath(tt.path)
			if route.port != tt.wantPort {
				t.Errorf("classifyProxyPath(%q).port = %d, want %d", tt.path, route.port, tt.wantPort)
			}
			if route.prefix != tt.wantPrefix {
				t.Errorf("classifyProxyPath(%q).prefix = %q, want %q", tt.path, route.prefix, tt.wantPrefix)
			}
		})
	}
}

func TestStripRoutePrefix(t *testing.T) {
	tests := []struct {
		path   string
		prefix string
		want   string
	}{
		{"/proxy/web/", proxyPrefixWeb, "/"},
		{"/proxy/web/foo/bar", proxyPrefixWeb, "/foo/bar"},
		{"/proxy/tui/", proxyPrefixTUI, "/"},
		{"/proxy/tui/ws", proxyPrefixTUI, "/ws"},
		{"/proxy/shell/", proxyPrefixShell, "/"},
		{"/proxy/shell/ws", proxyPrefixShell, "/ws"},
		{"/proxy/web", proxyPrefixWeb, "/"},
		{"/proxy/tui", proxyPrefixTUI, "/"},
		{"/proxy/shell", proxyPrefixShell, "/"},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := stripRoutePrefix(tt.path, tt.prefix)
			if got != tt.want {
				t.Errorf("stripRoutePrefix(%q, %q) = %q, want %q", tt.path, tt.prefix, got, tt.want)
			}
		})
	}
}
