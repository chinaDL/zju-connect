package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStartupDefaults(t *testing.T) {
	options, _, err := loadStartupOptions(nil, func() []string { return nil })
	if err != nil {
		t.Fatal(err)
	}
	cfg := options.Config
	if cfg.Protocol != "atrust" || cfg.ServerAddress != "vpn.yth.cn" || cfg.ServerPort != 443 || cfg.LoginDomain != "local" {
		t.Fatalf("startup defaults = protocol %q, server %q, port %d, login domain %q", cfg.Protocol, cfg.ServerAddress, cfg.ServerPort, cfg.LoginDomain)
	}
	if !cfg.TUNMode || !cfg.AddRoute || cfg.DNSHijack || !cfg.FakeIP {
		t.Fatalf("startup TUN defaults = mode %t, route %t, DNS hijack %t, fake IP %t; want DNS hijack disabled and others enabled",
			cfg.TUNMode, cfg.AddRoute, cfg.DNSHijack, cfg.FakeIP)
	}
	if cfg.SocksBind != "" || cfg.HTTPBind != "" {
		t.Fatalf("startup proxy defaults = %q, %q; want both disabled", cfg.SocksBind, cfg.HTTPBind)
	}
}

func TestProxyListenersCanBeEnabled(t *testing.T) {
	file := writeConfig(t, "socks_bind = \":1080\"\nhttp_bind = \":1081\"\n")
	for _, tt := range []struct {
		name      string
		args      []string
		env       []string
		wantSocks string
		wantHTTP  string
	}{
		{name: "config file", args: []string{"--config", file}, wantSocks: ":1080", wantHTTP: ":1081"},
		{name: "environment", env: []string{"ZJU_CONNECT_SOCKS_BIND=:2080", "ZJU_CONNECT_HTTP_BIND=:2081"}, wantSocks: ":2080", wantHTTP: ":2081"},
		{name: "command line", args: []string{"-socks-bind", ":3080", "-http-bind", ":3081"}, wantSocks: ":3080", wantHTTP: ":3081"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			options, _, err := loadStartupOptions(tt.args, func() []string { return tt.env })
			if err != nil {
				t.Fatal(err)
			}
			cfg := options.Config
			if cfg.SocksBind != tt.wantSocks || cfg.HTTPBind != tt.wantHTTP {
				t.Fatalf("proxy binds = %q, %q; want %q, %q", cfg.SocksBind, cfg.HTTPBind, tt.wantSocks, tt.wantHTTP)
			}
		})
	}
}

func TestTUNDefaultsCanBeOverridden(t *testing.T) {
	file := writeConfig(t, "tun_mode = false\nadd_route = false\nfake_ip = false\n")
	falseFlags := []string{"-tun-mode=false", "-add-route=false", "-fake-ip=false"}
	falseEnv := []string{
		"ZJU_CONNECT_TUN_MODE=false",
		"ZJU_CONNECT_ADD_ROUTE=false",
		"ZJU_CONNECT_FAKE_IP=false",
	}
	trueEnv := []string{
		"ZJU_CONNECT_TUN_MODE=true",
		"ZJU_CONNECT_ADD_ROUTE=true",
		"ZJU_CONNECT_FAKE_IP=true",
	}
	for _, tt := range []struct {
		name string
		args []string
		env  []string
		want bool
	}{
		{name: "default", want: true},
		{name: "config file", args: []string{"--config", file}},
		{name: "environment", env: falseEnv},
		{name: "command line", args: falseFlags},
		{name: "environment overrides file", args: []string{"--config", file}, env: trueEnv, want: true},
		{name: "command line overrides environment", args: falseFlags, env: trueEnv},
	} {
		t.Run(tt.name, func(t *testing.T) {
			options, _, err := loadStartupOptions(tt.args, func() []string { return tt.env })
			if err != nil {
				t.Fatal(err)
			}
			cfg := options.Config
			if cfg.TUNMode != tt.want || cfg.AddRoute != tt.want || cfg.FakeIP != tt.want {
				t.Fatalf("TUN options = mode %t, route %t, fake IP %t; want all %t",
					cfg.TUNMode, cfg.AddRoute, cfg.FakeIP, tt.want)
			}
		})
	}
}

func TestDNSHijackConfigPrecedence(t *testing.T) {
	file := writeConfig(t, "dns_hijack = true\n")
	for _, tt := range []struct {
		name string
		args []string
		env  []string
		want bool
	}{
		{name: "default"},
		{name: "config file", args: []string{"--config", file}, want: true},
		{name: "environment", env: []string{"ZJU_CONNECT_DNS_HIJACK=true"}, want: true},
		{name: "command line", args: []string{"-dns-hijack"}, want: true},
		{name: "environment disables file", args: []string{"--config", file}, env: []string{"ZJU_CONNECT_DNS_HIJACK=false"}},
		{name: "command line disables environment", args: []string{"-dns-hijack=false"}, env: []string{"ZJU_CONNECT_DNS_HIJACK=true"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			options, _, err := loadStartupOptions(tt.args, func() []string { return tt.env })
			if err != nil {
				t.Fatal(err)
			}
			if got := options.Config.DNSHijack; got != tt.want {
				t.Fatalf("DNSHijack = %t, want %t", got, tt.want)
			}
		})
	}
}

func TestSessionRefreshConfiguration(t *testing.T) {
	file := writeConfig(t, "session_refresh_interval = 900\n")
	for _, tt := range []struct {
		name string
		args []string
		env  []string
		want int
	}{
		{"default", nil, nil, 1800},
		{"file", []string{"--config", file}, nil, 900},
		{"environment", []string{"--config", file}, []string{"ZJU_CONNECT_SESSION_REFRESH_INTERVAL=600"}, 600},
		{"cli", []string{"--config", file, "-session-refresh-interval", "300"}, []string{"ZJU_CONNECT_SESSION_REFRESH_INTERVAL=600"}, 300},
		{"disabled", []string{"--config", file, "--session-refresh-interval=0"}, nil, 0},
	} {
		t.Run(tt.name, func(t *testing.T) {
			options, _, err := loadStartupOptions(tt.args, func() []string { return tt.env })
			if err != nil {
				t.Fatal(err)
			}
			if got := options.Config.SessionRefreshInterval; got != tt.want {
				t.Fatalf("session refresh seconds = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestConfigSourcePrecedence(t *testing.T) {
	configFile := writeConfig(t, `
protocol = "atrust"
server_address = "file.example.com"
server_port = 444
username = "file-user"
tun_mode = true
`)

	options, _, err := loadStartupOptions([]string{
		"--config", configFile,
		"--server", "cli.example.com",
		"--tun-mode=false",
	}, func() []string {
		return []string{
			"ZJU_CONNECT_SERVER_ADDRESS=env.example.com",
			"ZJU_CONNECT_SERVER_PORT=555",
			"ZJU_CONNECT_USERNAME=env-user",
		}
	})
	if err != nil {
		t.Fatal(err)
	}

	cfg := options.Config
	if cfg.ServerAddress != "cli.example.com" {
		t.Fatalf("ServerAddress = %q, want CLI value", cfg.ServerAddress)
	}
	if cfg.ServerPort != 555 {
		t.Fatalf("ServerPort = %d, want environment value", cfg.ServerPort)
	}
	if cfg.Username != "env-user" {
		t.Fatalf("Username = %q, want environment value", cfg.Username)
	}
	if cfg.Protocol != "atrust" {
		t.Fatalf("Protocol = %q, want TOML value", cfg.Protocol)
	}
	if cfg.TUNMode {
		t.Fatal("TUNMode = true, want explicit CLI false")
	}
}

func TestLegacySingleDashFlagsRemainSupported(t *testing.T) {
	options, _, err := loadStartupOptions([]string{
		"-protocol", "atrust",
		"-server=legacy.example.com",
		"-tun-mode",
	}, func() []string { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if options.Config.Protocol != "atrust" ||
		options.Config.ServerAddress != "legacy.example.com" ||
		!options.Config.TUNMode {
		t.Fatalf("legacy flags were not applied: %+v", options.Config)
	}
}

func TestLegacyArgumentNormalizationDoesNotRewriteValues(t *testing.T) {
	for _, passwordFlag := range []string{"-password", "--password"} {
		options, _, err := loadStartupOptions([]string{
			"-username", "user",
			passwordFlag, "-server",
		}, func() []string { return nil })
		if err != nil {
			t.Fatal(err)
		}
		if options.Config.Password != "-server" {
			t.Fatalf("%s value = %q, want unchanged flag-like value", passwordFlag, options.Config.Password)
		}
	}
}

func TestUnchangedCLIValuesDoNotOverrideConfig(t *testing.T) {
	configFile := writeConfig(t, `
server_port = 8443
socks_bind = "127.0.0.1:2080"
`)

	options, _, err := loadStartupOptions([]string{"--config", configFile}, func() []string { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if options.Config.ServerPort != 8443 {
		t.Fatalf("ServerPort = %d, want 8443", options.Config.ServerPort)
	}
	if options.Config.SocksBind != "127.0.0.1:2080" {
		t.Fatalf("SocksBind = %q, want TOML value", options.Config.SocksBind)
	}
}

func TestEnvironmentConfigPathAndAuthInfo(t *testing.T) {
	configFile := writeConfig(t, `
protocol = "atrust"
server_address = "vpn.example.com"
`)

	options, _, err := loadStartupOptions([]string{"--auth-info"}, func() []string {
		return []string{"ZJU_CONNECT_CONFIG=" + configFile}
	})
	if err != nil {
		t.Fatal(err)
	}
	if !options.AuthInfo {
		t.Fatal("AuthInfo = false")
	}
	if options.Config.Protocol != "atrust" || options.Config.ServerAddress != "vpn.example.com" {
		t.Fatalf("config not loaded before auth-info: %+v", options.Config)
	}
}

func TestProtocolServerWorkaround(t *testing.T) {
	for _, tt := range []struct {
		name     string
		protocol string
		server   string
		want     string
	}{
		{name: "default server remains unchanged", protocol: "atrust", want: "vpn.yth.cn"},
		{name: "aTrust rewrites EasyConnect server", protocol: "atrust", server: "rvpn.zju.edu.cn", want: "vpn.zju.edu.cn"},
		{name: "EasyConnect rewrites aTrust server", protocol: "easyconnect", server: "vpn.zju.edu.cn", want: "rvpn.zju.edu.cn"},
		{name: "aTrust server remains unchanged", protocol: "atrust", server: "vpn.zju.edu.cn", want: "vpn.zju.edu.cn"},
		{name: "EasyConnect server remains unchanged", protocol: "easyconnect", server: "rvpn.zju.edu.cn", want: "rvpn.zju.edu.cn"},
		{name: "custom server remains unchanged", protocol: "atrust", server: "vpn.example.com", want: "vpn.example.com"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			args := []string{"--protocol", tt.protocol}
			if tt.server != "" {
				args = append(args, "--server", tt.server)
			}

			options, _, err := loadStartupOptions(args, func() []string { return nil })
			if err != nil {
				t.Fatal(err)
			}
			if options.Config.ServerAddress != tt.want {
				t.Fatalf("ServerAddress = %q, want %q", options.Config.ServerAddress, tt.want)
			}
		})
	}
}

func TestCollectionCLIReplacesConfig(t *testing.T) {
	configFile := writeConfig(t, `
port_forwarding = [
  { network_type = "tcp", bind_address = "127.0.0.1:1", remote_address = "10.0.0.1:1" }
]
custom_proxy_domain = ["file.example.com"]
`)

	options, _, err := loadStartupOptions([]string{
		"--config", configFile,
		"--tcp-port-forwarding", "127.0.0.1:2-10.0.0.2:2",
		"--udp-port-forwarding", "127.0.0.1:3-10.0.0.3:3",
		"--custom-proxy-domain", "cli.example.com",
	}, func() []string { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if len(options.Config.PortForwardingList) != 2 {
		t.Fatalf("PortForwardingList = %+v", options.Config.PortForwardingList)
	}
	if got := options.Config.CustomProxyDomain; len(got) != 1 || got[0] != "cli.example.com" {
		t.Fatalf("CustomProxyDomain = %v", got)
	}
}

func TestUnknownConfigurationKeyIsRejected(t *testing.T) {
	configFile := writeConfig(t, "server_adress = \"typo.example.com\"\n")
	if _, _, err := loadStartupOptions([]string{"--config", configFile}, func() []string { return nil }); err == nil {
		t.Fatal("unknown TOML key was accepted")
	}

	if _, _, err := loadStartupOptions(nil, func() []string {
		return []string{"ZJU_CONNECT_SERVER_ADRESS=typo.example.com"}
	}); err == nil {
		t.Fatal("unknown environment key was accepted")
	}
}

func TestLegacyRemoteDNSNamesRemainSupported(t *testing.T) {
	tests := []struct {
		name    string
		config  string
		args    []string
		environ []string
		wantDNS string
	}{
		{
			name:    "TOML",
			config:  "disable_zju_dns = true\nzju_dns_server = \"192.0.2.1\"\n",
			wantDNS: "192.0.2.1",
		},
		{
			name: "environment",
			environ: []string{
				"ZJU_CONNECT_DISABLE_ZJU_DNS=true",
				"ZJU_CONNECT_ZJU_DNS_SERVER=192.0.2.2",
			},
			wantDNS: "192.0.2.2",
		},
		{
			name: "command line",
			args: []string{
				"--disable-zju-dns",
				"--zju-dns-server", "192.0.2.3",
			},
			wantDNS: "192.0.2.3",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			args := test.args
			if test.config != "" {
				args = append([]string{"--config", writeConfig(t, test.config)}, args...)
			}
			options, _, err := loadStartupOptions(args, func() []string { return test.environ })
			if err != nil {
				t.Fatal(err)
			}
			if !options.Config.DisableRemoteDNS {
				t.Fatal("DisableRemoteDNS = false, want true")
			}
			if options.Config.RemoteDNSServer != test.wantDNS {
				t.Fatalf("RemoteDNSServer = %q, want %q", options.Config.RemoteDNSServer, test.wantDNS)
			}
		})
	}
}

func TestCanonicalRemoteDNSConfigNames(t *testing.T) {
	configFile := writeConfig(t, `
disable_remote_dns = true
remote_dns_server = "192.0.2.4"
`)

	options, _, err := loadStartupOptions([]string{"--config", configFile}, func() []string { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if !options.Config.DisableRemoteDNS || options.Config.RemoteDNSServer != "192.0.2.4" {
		t.Fatalf("canonical remote DNS configuration was not applied: %+v", options.Config)
	}
}

func TestRemoteDNSAliasesPreserveSourcePrecedence(t *testing.T) {
	configFile := writeConfig(t, `
disable_zju_dns = true
zju_dns_server = "192.0.2.1"
`)

	options, _, err := loadStartupOptions([]string{
		"--config", configFile,
		"--disable-zju-dns=false",
		"--remote-dns-server", "192.0.2.3",
	}, func() []string {
		return []string{
			"ZJU_CONNECT_DISABLE_REMOTE_DNS=true",
			"ZJU_CONNECT_ZJU_DNS_SERVER=192.0.2.2",
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	if options.Config.DisableRemoteDNS {
		t.Fatal("DisableRemoteDNS = true, want explicit CLI false")
	}
	if options.Config.RemoteDNSServer != "192.0.2.3" {
		t.Fatalf("RemoteDNSServer = %q, want CLI value", options.Config.RemoteDNSServer)
	}
}

func TestEnvironmentCollectionsUseCLISyntax(t *testing.T) {
	options, _, err := loadStartupOptions(nil, func() []string {
		return []string{
			"ZJU_CONNECT_TCP_PORT_FORWARDING=127.0.0.1:2-10.0.0.2:2",
			"ZJU_CONNECT_UDP_PORT_FORWARDING=127.0.0.1:3-10.0.0.3:3",
			"ZJU_CONNECT_CUSTOM_PROXY_DOMAIN=one.example.com,two.example.com",
			"ZJU_CONNECT_CUSTOM_DNS=host.example.com:192.0.2.1",
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(options.Config.CustomProxyDomain) != 2 {
		t.Fatalf("CustomProxyDomain = %v", options.Config.CustomProxyDomain)
	}
	if len(options.Config.PortForwardingList) != 2 {
		t.Fatalf("PortForwardingList = %+v", options.Config.PortForwardingList)
	}
	if len(options.Config.CustomDNSList) != 1 || options.Config.CustomDNSList[0].IP != "192.0.2.1" {
		t.Fatalf("CustomDNSList = %+v", options.Config.CustomDNSList)
	}
}

func TestCLICollectionsOverrideEnvironmentCollections(t *testing.T) {
	options, _, err := loadStartupOptions([]string{
		"--tcp-port-forwarding", "127.0.0.1:4-10.0.0.4:4",
		"--custom-proxy-domain", "cli.example.com",
	}, func() []string {
		return []string{
			"ZJU_CONNECT_TCP_PORT_FORWARDING=127.0.0.1:2-10.0.0.2:2",
			"ZJU_CONNECT_UDP_PORT_FORWARDING=127.0.0.1:3-10.0.0.3:3",
			"ZJU_CONNECT_CUSTOM_PROXY_DOMAIN=env.example.com",
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := options.Config.PortForwardingList; len(got) != 1 || got[0].RemoteAddress != "10.0.0.4:4" {
		t.Fatalf("PortForwardingList = %+v", got)
	}
	if got := options.Config.CustomProxyDomain; len(got) != 1 || got[0] != "cli.example.com" {
		t.Fatalf("CustomProxyDomain = %v", got)
	}
}

func writeConfig(t *testing.T, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
