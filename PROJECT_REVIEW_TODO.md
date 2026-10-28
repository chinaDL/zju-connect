# 项目理解进度

## 任务：建立 zju-connect 项目地图

- [x] 阅读仓库约束并梳理项目结构与关键运行路径。
  - [x] 检查仓库约束、README、模块依赖和已有恢复文件；未发现仓库内 AGENTS.md 或 TODO 文件。依赖信息显示为 Go 1.25.6，配置使用 koanf。验证：目录与文件读取；结果：完成，无测试（只读调查）。
  - [x] 梳理启动入口和配置初始化。`main.go` 负责生命周期编排；`init.go` 用 pflag + koanf 合并默认值、TOML、环境变量、CLI 参数并校验。验证：源码阅读；结果：完成，无测试（只读调查）。
  - [x] 梳理两套 VPN 协议客户端、统一网络接口、解析/拨号/网络栈路径。`client.Client` 统一 EasyConnect/aTrust；`dial.Dialer` 按策略选 VPN/直连；gVisor/TUN/TCP tunnel 提供数据通道。验证：源码阅读；结果：完成，无测试（只读调查）。
  - [x] 检查已有测试与文档，确定当前验证方式和可见改进候选项。已发现 47 个测试文件，GitHub Actions 构建多平台二进制。验证：`go test ./...`；结果：失败，`client/atrust/auth/session_context_test.go:28` 调用不存在的 `(*Session).do`；`auth.go` 实现的是 `Session.request`，且认证代码均调用该方法，故失败原因是测试与当前 API 不一致。其余可编译包测试通过；测试进程已退出。未修改业务代码或测试。
  - [x] 汇总项目地图、关键文件入口和后续改进切入点。项目由启动编排、协议客户端、网络栈、规则解析拨号、代理服务组成；可见改进方向包括修复测试/API不一致、把启动编排拆成可测试阶段、按用户提供的 Go 约束评估 Cobra/Viper/.env/zap 适配。验证：源码、README、测试和 CI 阅读；完成。

## 恢复位置

本轮理解任务已完成；全量测试当前受 `session_context_test.go` 编译错误阻塞。后续从用户指定的改进方向开始，不重复本轮已完成的只读调查。

## 本轮结论

- 只新增/更新本恢复文件；未改业务源码、配置、文档或测试。
- 测试命令：`go test ./...`。结果：失败于 `client/atrust/auth/session_context_test.go:28`，原因是调用不存在的 `(*Session).do`；当前实现为 `(*Session).request`。除该包编译失败外，其余包测试通过。

## 任务：aTrust 图形验证码可远程访问

- [x] 核查现有验证码入口和限制。`client/authchallenge/captcha.go` 当前监听 `127.0.0.1:0` 并在所有平台尝试打开浏览器；aTrust 点选验证码进入此路径，EasyConnect 图片文字验证码使用文件加终端输入。验证：源码和文档阅读；完成；未改文件。
- [x] 实现 `0.0.0.0:0` 临时监听、逐个输出非回环 IPv4 网卡地址与本机地址、随机访问路径、macOS/Windows 自动打开浏览器，并保证提交/超时后关闭服务。已修改：`client/authchallenge/captcha.go`。验证：源码检查；功能测试在下一项执行。
- [x] 添加 HTTP 流程、访问限制、地址列表和超时测试，修复已知陈旧测试调用以恢复全量测试基线。已修改：`client/authchallenge/captcha_test.go`、`client/atrust/auth/session_context_test.go`。测试发现 `Session.request` 覆盖调用方 context，导致 `TestRefreshCancellation` 失败；已修复 `client/atrust/auth/auth.go` 使调用方/会话取消都生效。验证：`go test ./client/atrust/auth ./client/authchallenge` 通过；测试启动的 HTTP 服务均已关闭。
- [x] 更新验证码使用文档，运行测试并确认进程清理、工作区状态。已修改：`README.md`、`README_en.md`、`config.toml.example`、`docs/docker.md`、`docs/docker_en.md`；补充测试时发现响应体需在关闭时释放组合 context，已调整 `client/atrust/auth/auth.go` 并在 `session_context_test.go` 添加流式响应测试。验证：`go test ./...`、`go test -race ./client/authchallenge ./client/atrust/auth`、`go test -count=20 ./client/authchallenge`、`CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build ./...`、`CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build ./...`、`git diff --check` 全部通过；`pgrep` 未发现遗留测试或 zju-connect 进程。首次 Linux 交叉编译未设 `CGO_ENABLED=0`，因 macOS C 编译器无法编译 Linux runtime/cgo 失败；按 CI 配置重跑成功。

恢复位置：本次验证码任务已完成，无 `[~]` 项。后续新任务从用户提出的下一项改进开始，不重复本次已完成步骤。

## 任务：更新连接参数默认值

- [x] 将内置默认值改为 `-login-domain local -protocol atrust -server vpn.yth.cn -port 443`，同步示例与文档。已修改：`configs/config.go`、`configs/config_test.go`、`init_test.go`、`config.toml.example`、`README.md`、`README_en.md`。保留此前验证码功能改动；ZJU 与 EasyConnect 文档示例改为显式指定相应协议/服务器/登录域。验证：`go test ./configs .` 通过。
- [x] 运行全量测试，核对差异与遗留进程，记录结果。验证：`go test ./...` 通过；`git diff --check` 通过；`pgrep -fl 'authchallenge\.test|auth\.test|zju-connect'` 未发现遗留进程。错误：无。

恢复位置：本次默认值任务已完成，无 `[~]` 项。后续任务按用户新要求开始；此前验证码改动及本次默认值改动均已保留在工作区。

## 任务：默认启用 TUN 与自动路由、DNS 劫持、Fake IP

- [x] 更新内置默认值与示例配置，并同步使用说明。已修改 `configs/config.go`、`config.toml.example`、`README.md`、`README_en.md`、`docs/docker.md`、`docs/docker_en.md`、`docs/service.md`、`docs/service_en.md`。验证：源码与文档对照，四项默认启用，Docker/OpenWrt 代理场景注明 `tun_mode = false`；正式测试在后续项执行。
- [x] 更新配置测试，验证四项默认启用以及文件、环境变量、CLI 显式关闭仍有效。已修改 `configs/config_test.go`、`init_test.go`。验证：`go test ./configs .` 通过；`git diff --check` 通过。格式检查命令 `gofmt -l ...` 因 shell 中找不到 `gofmt` 失败（外部工具路径问题），下一项定位 Go 工具链后重试。
- [x] 运行全量测试，检查格式、差异和测试进程清理。已修改 `main.go`，限定 Fake IP 路由只添加于支持 TCP 隧道的客户端；已同步修订 macOS 服务权限说明。验证：`go test ./configs .`、`go test ./...`、`CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build ./...`、`CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build ./...`、`git diff --check` 全部通过；`/Users/whaz/sdk/go1.26.0/bin/gofmt -l configs/config.go configs/config_test.go init_test.go main.go` 无输出；测试完成后 `pgrep` 未发现遗留测试或 zju-connect 进程。错误：仅首次直接调用 `gofmt` 因 PATH 缺失失败，已改用 GOROOT 下的工具完成验证。

恢复位置：本次任务已完成，无 `[~]` 项；此前任务和工作区现有改动均保留。

## 任务：默认关闭 SOCKS5 与 HTTP 监听

- [x] 修改内置默认值、示例配置与文档中的代理启动说明。已修改 `configs/config.go`、`config.toml.example`、`README.md`、`README_en.md`、`docs/docker.md`、`docs/docker_en.md`。验证：对照 `main.go`，bind 为空时不启动对应服务；Docker 代理示例显式设置两个监听地址。测试在后续项执行。
- [x] 更新配置测试，确认默认关闭、显式设置端口可重新启用。已修改 `configs/config_test.go`、`init_test.go`。验证：`go test ./configs .` 通过；`git diff --check` 通过；已用 Go 1.26.0 的 `gofmt -w init_test.go` 完成格式化。
- [x] 运行全量测试、格式与差异检查，确认没有遗留测试进程。验证：`go test ./configs .`、`go test ./...`、`git diff --check` 均通过；`/Users/whaz/sdk/go1.26.0/bin/gofmt -l configs/config.go configs/config_test.go init_test.go` 无输出；测试结束后 `pgrep -fl 'authchallenge\.test|auth\.test|zju-connect|go test'` 无结果。错误：无。

恢复位置：本次任务已完成，无 `[~]` 项；保留此前所有工作区改动。

## 任务：默认关闭 DNS 劫持

- [x] 修改内置与示例默认值，调整 Fake IP 路由条件并同步文档。已修改 `configs/config.go`、`config.toml.example`、`main.go`、`README.md`、`README_en.md`。验证：源码对照，`DNSHijack` 回到零值 `false`，Fake IP 网段路由需 DNS 劫持同时开启；测试在下一项执行。
- [x] 调整默认值与配置覆盖测试，验证显式启用 DNS 劫持。已修改 `configs/config_test.go`、`init_test.go`。验证：`go test ./configs .` 通过；`gofmt -l` 无输出。全仓库 `git diff --check` 报出本轮开始前已有的 `Makefile` 与 `使用方法.txt` 尾部空行，下一项只核对本轮修改的文件并保留这些现有改动。
- [x] 运行全量测试、限定范围的格式与差异检查，确认进程清理。验证：`go test ./configs .`、`go test ./...` 通过；`git diff --check -- PROJECT_REVIEW_TODO.md configs/config.go configs/config_test.go config.toml.example init_test.go main.go README.md README_en.md` 通过；Go 1.26.0 `gofmt -l` 无输出；测试结束后 `pgrep -fl 'authchallenge\.test|auth\.test|zju-connect|go test'` 无结果。全仓库差异检查仍显示本轮开始前已有的 `Makefile`、`使用方法.txt` 尾部空行，未改动这些文件。

恢复位置：本次任务已完成，无 `[~]` 项；保留已有工作区改动，包括本轮开始前出现的其他文件。
