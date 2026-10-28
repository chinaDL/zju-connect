package configs

import "testing"

func TestDefault(t *testing.T) {
	cfg := Default()
	if cfg.Protocol != "atrust" {
		t.Fatalf("Protocol = %q, want atrust", cfg.Protocol)
	}
	if cfg.ServerAddress != "vpn.yth.cn" {
		t.Fatalf("ServerAddress = %q, want vpn.yth.cn", cfg.ServerAddress)
	}
	if cfg.ServerPort != 443 {
		t.Fatalf("ServerPort = %d, want 443", cfg.ServerPort)
	}
	if cfg.LoginDomain != "local" {
		t.Fatalf("LoginDomain = %q, want local", cfg.LoginDomain)
	}
	if cfg.SocksBind != "" || cfg.HTTPBind != "" {
		t.Fatalf("proxy defaults = %q, %q; want both disabled", cfg.SocksBind, cfg.HTTPBind)
	}
	if !cfg.TUNMode || !cfg.AddRoute || cfg.DNSHijack || !cfg.FakeIP {
		t.Fatalf("TUN defaults = mode %t, route %t, DNS hijack %t, fake IP %t; want DNS hijack disabled and others enabled",
			cfg.TUNMode, cfg.AddRoute, cfg.DNSHijack, cfg.FakeIP)
	}
}
