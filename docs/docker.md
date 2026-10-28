## Docker 运行

以下端口映射示例用于 SOCKS5/HTTP 代理，请在挂载的 `config.toml` 中设置 `tun_mode = false`、`socks_bind = ":1080"`、`http_bind = ":1081"`。默认启用的 TUN 模式需要额外的容器网络权限和设备访问。

```zsh
$ docker run -d --name zju-connect -v $PWD/config.toml:/home/nonroot/config.toml -p 1080:1080 -p 1081:1081 --restart unless-stopped mythologyli/zju-connect
```

也可以使用 Docker Compose。创建 `docker-compose.yml` 文件，内容如下：

```yaml
services:
   zju-connect:
      image: mythologyli/zju-connect
      container_name: zju-connect
      restart: unless-stopped
      ports:
         - 1080:1080
         - 1081:1081
      volumes:
         - ./config.toml:/home/nonroot/config.toml
```

另外，你还可以使用 [configs top-level elements](https://docs.docker.com/compose/compose-file/08-configs/) 将 zju-connect 的配置文件直接写入 docker-compose.yml，如下：

```yaml
services:
   zju-connect:
      container_name: zju-connect
      image: mythologyli/zju-connect
      restart: unless-stopped
      ports: [1080:1080, 1081:1081]
      configs: [{ source: zju-connect-config, target: /home/nonroot/config.toml }]

configs:
   zju-connect-config:
      content: |
         username = ""
         password = ""
         tun_mode = false
         socks_bind = ":1080"
         http_bind = ":1081"
         # other configs ...
```

并在同目录下运行

```zsh
docker compose up -d
```

### aTrust 协议登录和数据持久化

aTrust 协议客户端需要在容器内持久化登录凭据（`client_data.json`），可参考以下示例配置，将凭据文件保存在 Docker 卷中：

```
services:
   zju-connect:
      container_name: zju-connect
      image: mythologyli/zju-connect
      restart: unless-stopped
      ports:
         - 1080:1080
         - 1081:1081
      configs:
         - source: zju-connect-config
           target: /home/nonroot/config.toml
      volumes:
         - zju-connect-data:/home/nonroot/data
configs:
   zju-connect-config:
      content: |
         username = ""
         password = ""
         tun_mode = false
         socks_bind = ":1080"
         http_bind = ":1081"
         client_data_file = "/home/nonroot/data/client_data.json"
         # other configs ...
volumes:
   zju-connect-data:
```

使用 aTrust 协议初次登录时需要人工验证。在 Linux 上，先将此服务的 `ports` 临时替换为 `network_mode: host`，再以交互模式运行，使随机分配的验证码端口能从主机网络访问：

```zsh
docker compose run --rm -it zju-connect
```

以下以 ZJU 服务端为例：

**1. 图形验证码**

程序会监听所有 IPv4 网卡上的随机端口，并打印本机和各活动网卡的地址。请从日志中选择可从浏览器设备连通的地址；防火墙也需要放行该端口：

```
Captcha local URL: http://127.0.0.1:40855/captcha/<随机令牌>/
Captcha network URL: http://192.168.1.10:40855/captcha/<随机令牌>/
```

访问日志中显示的地址完成图形验证码验证后，程序会提示输入短信验证码：

**2. 短信验证码**

```
2026/03/25 09:10:57 SMS message sent successfully: 验证码已发送到您的手机：***, 请查收！
zju-connect-1  | 2026/03/25 09:11:50 Please enter the SMS verification code:
```

出现上述提示后，请输入收到的短信验证码并回车。登录成功后会显示如下日志：

```
2026/03/25 09:12:12 Client data saved to /home/nonroot/data/client_data.json
2026/03/25 09:12:12 VPN client started
```

完成登录后按 `Ctrl+C` 退出容器，之后就可以使用 `docker compose up -d` 正常启动了，登录状态会被持久化在 Docker 卷中，无需重复验证。
