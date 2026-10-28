# ZJU Connect

> 🚫 **Disclaimer**
>
> This program is provided **as is**, and the author **does not guarantee the correctness or reliability of the program**. Please judge whether the specific scenario is suitable for using this program. **The problems or consequences caused by using this program are borne by the user**!

---

[中文](README.md) | English

**This program is based on [EasierConnect](https://github.com/lyc8503/EasierConnect) (now Archived), thanks to the original author [lyc8503](https://github.com/lyc8503).**

**QQ group: 1037726410**, welcome to join the discussion.

### Usage

#### Use a GUI client

+ If you are from ZJU:
  + Windows users are recommended to use [ZJU Connect for Windows](https://github.com/mythologyli/zju-connect-for-Windows)
  + Linux/macOS users can try the [EZ4Connect](https://github.com/chenx-dust/EZ4Connect) developed by [Chenx Dust](https://github.com/chenx-dust) (support aTrust) or the [hitsz-connect-verge](https://github.com/kowyo/hitsz-connect-verge) developed by [kowyo](https://github.com/kowyo)
    Please set the server address to `rvpn.zju.edu.cn:443`
+ If you are not from ZJU:

  You can try the [EZ4Connect](https://github.com/chenx-dust/EZ4Connect) developed by [Chenx Dust](https://github.com/chenx-dust) (support aTrust) or the [hitsz-connect-verge](https://github.com/kowyo/hitsz-connect-verge) developed by [kowyo](https://github.com/kowyo)

#### Run Directly

TUN mode is enabled by default and no SOCKS5/HTTP proxy ports are opened. Run the connection commands below with administrator privileges (`sudo` on Linux/macOS or an elevated terminal on Windows). For proxies only, pass `-tun-mode=false -socks-bind :1080 -http-bind :1081`.

##### Using EasyConnect Protocol

+ If you are a ZJU user:
  1. Download the latest version for your platform from the [Release](https://github.com/mythologyli/zju-connect/releases) page.

  2. Taking macOS as an example, extract the executable file `zju-connect`

  3. macOS users need to remove security restrictions first. Run the following in your terminal: `sudo xattr -rd com.apple.quarantine zju-connect`

  4. Run the command: `./zju-connect -protocol easyconnect -server rvpn.zju.edu.cn -username <Account> -password <Password>`

  5. By default, server-provided resources are accessed through TUN and no local proxy ports are opened. Set listening addresses explicitly to enable SOCKS5/HTTP proxies.

+ If you are a non-ZJU user:

  The steps are the same as above, but try setting the running parameters as follows:

  `./zju-connect -protocol easyconnect -server <Server Address> -port <Server Port> -username xxx -password xxx -disable-zju-config -skip-domain-resource -remote-dns-server auto`

  *For details, see this [link](https://github.com/Mythologyli/zju-connect/issues/65#issuecomment-2650185322).*

  If your server requires a graphic captcha, try setting the running parameters as follows:

  `./zju-connect -protocol easyconnect -server <Server Address> -port <Server Port> -username xxx -password xxx -disable-zju-config -skip-domain-resource -remote-dns-server auto -disable-multi-line -graph-code-file graph_code.jpg`

  When logging in, the captcha image will be saved to the `graph_code.jpg` file. Please check it and manually enter the captcha code.

##### Using aTrust Protocol

+ If you are a ZJU user:
  Other steps are the same as the EasyConnect method, but set the running parameters to:
  `./zju-connect -protocol atrust -server vpn.zju.edu.cn -login-domain Radius -username <Account> -password <Password> -client-data-file client_data.json`
  Then follow the on-screen prompts. If you do not want to save the login status, you can omit the `-client-data-file` parameter.

+ If you are a non-ZJU user:
  The steps are the same as for ZJU users; please specify the login protocol according to your situation.
  **How to determine the Login Domain and Protocol?**

  1. Fetch available authentication methods: `./zju-connect -protocol atrust -server <Server Address> -port <Server Port> -auth-info`.
  2. Identify your parameters:
     The command will return a JSON array of available authentication methods. For example:
     ```json
     [{"loginDomain":"Radius","authType":"auth/psw","authName":"上网账号","loginUrl":""},{"loginDomain":"local","authType":"auth/psw","authName":"IDC运维账号","loginUrl":""},{"loginDomain":"radius93482","authType":"auth/psw","authName":"INTL ID","loginUrl":""}]
     ```
     In this example, there are three methods. To use the first method (Radius), you must append -login-domain Radius -auth-type "auth/psw" to your execution command.
  3. Supported Authentication Types: `auth/psw` (password), `auth/cas` (CAS), `auth/smsCheckCode` (SMS verification code).

#### Run as a service

[Link](docs/service_en.md)

#### Run in Docker

[Link](docs/docker_en.md)

### Warning

1. When using other proxy tools with TUN mode enabled and zju-connect as a downstream proxy, please be sure to provide the correct network diversion rules, refer to [this issue](https://github.com/Mythologyli/zju-connect/issues/57)

### TUN mode precautions

1. Need to run with administrator privileges

2. Windows system needs to go to [Wintun official website](https://www.wintun.net) to download `wintun.dll` and place it in the same directory as the executable file

3. `-tun-mode -add-route -fake-ip` are enabled by default, but DNS hijacking is disabled. By default, TUN adds routes for server-provided IP resources. To handle aTrust domain resources, explicitly pass `-dns-hijack`. For SOCKS5/HTTP proxies only, pass `-tun-mode=false -socks-bind :1080 -http-bind :1081`; this does not require administrator privileges.

4. With aTrust, direct TCP traffic involving domain names via TUN may fail if DNS hijacking or Fake IP is disabled.

### Arguments

#### General Arguments

+ `protocol`: Login protocol, supports `easyconnect`/`atrust`, default is `atrust`

+ `server`: VPN server address, default is `vpn.yth.cn`

+ `port`: VPN server port, default is `443`

+ `username`: Network account. For example: student ID

+ `password`: Network account password. (Sensitive information is recommended to be provided via env `ZJU_CONNECT_PASSWORD`)

+ `graph-code-file`: Graphic captcha file path, empty by default. In aTrust mode, an empty value starts a temporary HTTP page on a random IPv4 port and prints the local URL plus URLs for active network interfaces. On Linux, open a reachable URL from another device; macOS and Windows also open the local page automatically. If a path is set, the image is saved there and JSON is entered in the terminal. The page closes after submission or timeout; remote access requires the firewall to allow the random port.

+ `totp-secret`: TOTP secret, can be used to automatically complete TOTP verification. If the server doesn't require TOTP or you want to manually enter the code, leave blank. (Sensitive information is recommended to be provided via env `ZJU_CONNECT_TOTP_SECRET`)

+ `disable-remote-dns`: Disable remote DNS and use local DNS instead, generally no need to add this argument

+ `disable-server-config`: Disable server configuration, generally no need to add this argument

+ `socks-bind`: SOCKS5 proxy listening address, empty by default (disabled). For example, `:1080`

+ `socks-user`: SOCKS5 proxy username, leave blank if no authentication is required

+ `socks-passwd`: SOCKS5 proxy password, leave blank if no authentication is required

+ `http-bind`: HTTP proxy listening address, empty by default (disabled). For example, `:1081`

+ `shadowsocks-url`: Shadowsocks server URL. For example: `ss://aes-128-gcm:password@server:port`. Format [refer to here](https://github.com/shadowsocks/go-shadowsocks2)

+ `dial-direct-proxy`: When a URL does not match rules and switches to direct connection, use a proxy. Typically used in conjunction with other proxy tools, currently supports only HTTP proxy. For example: `http://127.0.0.1:7890`. Set to `""` to disable

+ `tcp-tunnel-mode`: TCP tunnel mode, default is `false`. When enabled, only TCP traffic can be proxied through the TCP tunnel. Since only aTrust supports TCP tunneling, this mode is ineffective under EasyConnect. Enabling this will disable TUN mode

+ `tun-mode`: TUN mode (experimental), enabled by default. Please read the TUN mode precautions above; use `-tun-mode=false` to disable it

+ `add-route`: Add routes according to the configuration issued by the server when TUN mode is enabled; enabled by default

+ `dns-ttl`: DNS cache time, default is `3600` seconds

+ `disable-keep-alive`: Disable periodic keep-alive, generally no need to add this argument

+ `keep-alive-url`: Uses HTTP keep-alive, suitable for situations where the server does not provide DNS. Set the URL to visit, for example, `https://www.cnki.net/favicon.ico`. The default is empty, in which case the server-provided DNS is used for keep-alive.

+ `remote-dns-server`: Remote DNS server address, default is `auto`. Set to `auto` to use the DNS server obtained from the server; disable remote DNS if it fails to obtain

+ `secondary-dns-server`: Standby DNS server used when the remote DNS server cannot resolve. The default `auto` uses the second server supplied by VPN policy, then falls back to `114.114.114.114`. Leave blank to use system default DNS, but it must be set when `dns-hijack` is enabled

+ `dns-server-bind`: DNS server listening address, default is empty (disabled). For example, set to `127.0.0.1:53`, then you can send DNS requests to `127.0.0.1:53`

+ `local-dns-server`: Local DNS server used to resolve the VPN server hostname, as IP or IP:port; when empty, the system DNS is used, routable DNS addresses are bound to the detected underlay interface, and local DNS stubs keep their loopback route

+ `dns-hijack`: Hijack DNS requests when TUN mode is enabled; disabled by default. Enable it explicitly to handle domain resources

+ `fake-ip`: Enable Fake IP mode; enabled by default, but effective only when DNS hijacking is also enabled. It has no effect with EasyConnect

+ `debug-dump`: Whether to enable debugging, generally no need to add this argument

+ `debug-pcap-file`: Reconstruct a PCAP from data read and written on VPN underlay TCP connections; a full capture queue blocks network I/O, kernel handshakes and retransmissions are omitted, and TLS payloads remain encrypted

+ `debug-tls-log-file`: Export TLS session secrets in NSS key log format for decrypting `debug-pcap-file` traffic in Wireshark. The file contains session secrets and must only be used for debugging and stored securely

+ `bind-interface`: Manually bind VPN underlay connections to this network interface for either EasyConnect or aTrust. A non-empty value takes precedence over automatic detection.

+ `auto-detect-interface`: Automatically detect and bind the VPN underlay interface; defaults to `false`. Set it to `true` to enable automatic detection. If disabled and `bind-interface` is empty, underlay connections use system routing. **This feature may not work correctly while another VPN with Fake IP enabled is in use.**

+ `tcp-port-forwarding`: TCP port forwarding, format is `local address-remote address,local address-remote address,...`, for example `127.0.0.1:9898-10.10.98.98:80,0.0.0.0:9899-10.10.98.98:80`. Multiple forwardings are separated by `,`

+ `udp-port-forwarding`: UDP port forwarding, format is `local address-remote address,local address-remote address,...`, for example `127.0.0.1:53-10.10.0.21:53`. Multiple forwardings are separated by `,`

+ `custom-dns`: Specify custom DNS resolution results, format is `domain:IP,domain:IP,...`, for example `www.cc98.org:10.10.98.98,appservice.zju.edu.cn:10.203.8.198`. Multiple resolutions are separated by `,`

+ `proxy-all`: Whether to proxy all traffic, generally no need to add this argument

+ `config`: Specify the configuration file, referring to `config.toml.example`

Configuration sources are merged with the precedence “built-in defaults < TOML file < environment variables < command-line flags”. Only explicitly supplied command-line flags override other sources; for example, `-tun-mode=false` disables a value enabled by the file or environment.

Every configuration key can be set with an uppercase environment variable prefixed by `ZJU_CONNECT_`. For example, `server_address` maps to `ZJU_CONNECT_SERVER_ADDRESS`, and `tun_mode` maps to `ZJU_CONNECT_TUN_MODE`. Collection values use the same names and syntax as command-line flags, for example `ZJU_CONNECT_TCP_PORT_FORWARDING=127.0.0.1:9898-10.10.98.98:80`, `ZJU_CONNECT_CUSTOM_DNS=www.cc98.org:10.10.98.98`, and `ZJU_CONNECT_CUSTOM_PROXY_DOMAIN=nature.com,science.org`.

The old `disable_zju_dns` and `zju_dns_server` names, including their command-line flags and environment variables, remain supported but are deprecated. Use `disable_remote_dns` and `remote_dns_server` instead.

#### EasyConnect Related Arguments

+ `cert-file`: p12 certificate file path, if the server requires certificate verification, this parameter needs to be configured

+ `cert-password`: Certificate password. (Sensitive information is recommended to be provided via env `ZJU_CONNECT_CERT_PASSWORD`)

+ `skip-domain-resource`: Do not use the domain resource provided by the server for split tunneling, generally no need to add this argument

+ `disable-multi-line`: Disable automatic line selection based on latency. When added, use the line specified by the `server` parameter

+ `disable-zju-config`: Disable ZJU related configuration, non-ZJU users may need to add this argument

+ `custom-proxy-domain`: Specify custom domains to use RVPN proxy, format is `domain,domain,...`, for example `nature.com,science.org`. Multiple domains are separated by `,`

+ `twf-id`: twfID login, for debugging purposes, generally no need to add this argument

#### aTrust Related Arguments

+ `auth-type`: aTrust login authentication type, supports `auth/psw` (password), `auth/cas` (CAS), `auth/smsCheckCode` (SMS verification code), default is auto detected.
+ `login-domain`: Login domain, default is `local`.
+ `client-data-file`: Client data file path, used to save login status to avoid repeated verification.
+ `cas-ticket`: CAS verification ticket, defaults to empty, which triggers interactive verification.
+ `phone`: Phone number used for SMS verification code login.
+ `update-best-nodes-interval`: Interval for updating the optimal line automatically, in seconds, default is `300`. Set to `0` to disable automatic optimal line selection.
+ `session-refresh-interval`: aTrust SID refresh interval in seconds, default `1800`. Set to `0` to disable periodic refresh. The TOML key is `session_refresh_interval`; the environment variable is `ZJU_CONNECT_SESSION_REFRESH_INTERVAL`.
+ `auth-info`: Only get aTrust authentication information without logging in, generally no need to add this argument. Can be used to check supported authentication methods.
+ `trust-device`: Trust the current device (requires logged-in `-client-data-file`), does not start the tunnel.
+ `untrust-device`: Untrust the current device (requires logged-in `-client-data-file`), does not start the tunnel.
+ `sid`: aTrust SID, for debugging purposes, generally no need to add this argument.
+ `device-id`: aTrust device ID, for debugging purposes, generally no need to add this argument.
+ `sign-key`: aTrust signature key, for debugging purposes, generally no need to add this argument.
+ `resource-file`: aTrust resource file, for debugging purposes, generally no need to add this argument.

### Schedule

#### Completed

- [x] Proxy TCP traffic
- [x] Proxy UDP traffic
- [x] SOCKS5 proxy service
- [x] HTTP proxy service
- [x] Shadowsocks proxy service
- [x] Remote DNS resolution
- [x] ZJU rules addition
- [x] Support IPv6 direct connection
- [x] DNS cache acceleration
- [x] Automatic line selection
- [x] TCP port forwarding
- [x] UDP port forwarding
- [x] Start through configuration file
- [x] Periodic keep-alive
- [x] TUN mode
- [x] Automatical DNS hijack
- [x] SMS verification
- [x] TOTP verification
- [x] Certificate verification
- [x] aTrust protocol support
- [x] Fake IP

#### To Do

### Contributors

<a href="https://github.com/mythologyli/zju-connect/graphs/contributors">
  <img src="https://contrib.rocks/image?repo=mythologyli/zju-connect" />
</a>

### Thanks

+ [EasierConnect](https://github.com/lyc8503/EasierConnect)

+ [socks2http](https://github.com/zenhack/socks2http)

+ [![image](docs/yxvm.png)](https://yxvm.com/)

  [NodeSupport](https://github.com/NodeSeekDev/NodeSupport) sponsored this project

### Star History

[![Star History Chart](https://api.star-history.com/image?repos=mythologyli/zju-connect&type=date&legend=top-left)](https://www.star-history.com/?repos=mythologyli%2Fzju-connect&type=date&legend=bottom-right)
