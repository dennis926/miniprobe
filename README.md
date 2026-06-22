# MiniProbe 🔍

> 轻量级服务器监控探针 — 参考哪吒探针 & Komari，代码简洁，快速部署，客户端自动。

---

## 特性

| 特性 | 说明 |
|------|------|
| 📊 实时监控 | CPU、内存、磁盘、网络流量（速率 + 累计）、系统负载 |
| 🚀 零依赖部署 | Go 单二进制服务端，无需安装任何运行时 |
| 🔄 自动重连 | 客户端断线 5 s 后自动重连，无需人工干预 |
| 🐍 客户端自动 | `agent.py` 自动安装 `psutil` / `websocket-client` |
| 🎨 内嵌面板 | 暗黑主题 Dashboard，响应式设计，3 s 自动刷新 |
| 🔒 Token 认证 | WebSocket 连接鉴权，防止未授权节点接入 |
| 🐧 跨平台 | 服务端：Linux / macOS / Windows；客户端：任何有 Python 3 的系统 |
| 🐳 Docker 支持 | 一条命令 `docker compose up -d` 启动服务端 |

---

## 架构

```
┌──────────────────────────────────────────────┐
│         MiniProbe Server (Go 单二进制)         │
│                                              │
│   GET  /              → 内嵌 Dashboard HTML   │
│   GET  /api/agents    → 所有节点 JSON          │
│   WS   /ws?token=xxx  → Agent WebSocket 入口  │
└────────────────────┬─────────────────────────┘
                     │ WebSocket (实时推送)
         ┌───────────┼───────────┐
   ┌─────┴────┐ ┌────┴─────┐ ┌──┴───────┐
   │ agent.py │ │ agent.py │ │ agent.py │  × N 台服务器
   │ Linux    │ │ Windows  │ │ macOS    │
   └──────────┘ └──────────┘ └──────────┘
```

---

## 快速开始

### 方式一：手动编译（推荐，3 步完成）

**第 1 步 — 编译并启动服务端**
```bash
cd server
go mod tidy          # 下载依赖（仅首次）
go build -o miniprobe-server .
./miniprobe-server -port 8080 -token mytoken
```
打开浏览器访问 `http://localhost:8080` 即可看到 Dashboard。

**第 2 步 — 在每台被监控机器上运行 Agent**
```bash
# 将 agent/agent.py 复制到目标机器，然后执行：
python3 agent.py -s ws://SERVER_IP:8080/ws -t mytoken
```
Agent 会自动安装依赖并开始上报数据，断线后自动重连。

---

### 方式二：Docker 一键部署服务端

```bash
# 默认 token=miniprobe，端口 8080
docker compose up -d

# 自定义 token 和端口
TOKEN=mysecret PORT=9090 docker compose up -d
```

客户端仍使用 `python3 agent.py` 连接。

---

### 方式三：一键脚本安装（Linux 系统服务）

```bash
# 服务端（需要 root，自动创建 systemd 服务）
bash scripts/install_server.sh --port 8080 --token mytoken

# 客户端（无需 root，注册为 systemd user 服务）
bash scripts/install_agent.sh -s ws://SERVER_IP:8080/ws -t mytoken
```

---

## 参数说明

### 服务端 `miniprobe-server`

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `-port` | `8080` | HTTP / WebSocket 监听端口 |
| `-token` | `miniprobe` | Agent 连接认证 Token |

### 客户端 `agent.py`

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `-s / --server` | `ws://localhost:8080/ws` | 服务端 WebSocket 地址 |
| `-t / --token` | `miniprobe` | 认证 Token（须与服务端一致）|
| `-i / --interval` | `3` | 上报间隔（秒）|

---

## 监控指标

| 指标 | 说明 |
|------|------|
| CPU | 使用率 % |
| 内存 | 已用 / 总量 / 使用率 % |
| 磁盘 | 已用 / 总量 / 使用率 %（根分区 / C盘）|
| 网络 | 实时入站速率、出站速率、累计入站流量 |
| 负载 | 1 / 5 / 15 分钟均值（Linux / macOS）|
| 系统 | 主机名、IP、OS、架构、运行时间 |

---

## 文件结构

```
miniprobe/
├── server/
│   ├── main.go             # Go 服务端 + 内嵌 Dashboard
│   └── go.mod              # Go 依赖（仅 gorilla/websocket）
├── agent/
│   └── agent.py            # Python 客户端（自动安装依赖）
├── scripts/
│   ├── install_server.sh   # 服务端一键安装脚本
│   ├── install_agent.sh    # 客户端一键安装脚本
│   └── miniprobe-agent.service  # Systemd 服务模板
├── Dockerfile              # 服务端 Docker 镜像
├── docker-compose.yml      # Docker Compose 配置
└── README.md
```

---

## 自定义扩展

### 添加新指标
1. 在 `agent/agent.py` 的 `collect()` 函数中新增字段
2. 在 `server/main.go` 的 `AgentInfo` 结构体中添加对应字段（JSON tag）
3. 在 `main.go` 末尾的 `dashboardHTML` 常量中更新 `buildCard()` 函数以展示新字段
4. 重新编译服务端即可

### 修改 Dashboard 样式
Dashboard HTML 完整内嵌在 `server/main.go` 末尾的 `dashboardHTML` 常量中，
直接修改后重新 `go build` 即可，无需分离静态文件。

---

## License

MIT
