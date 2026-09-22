# jev-sys 部署指南

JEV × [RB Lighter](https://apidocs.rh.lighter.xyz/docs/get-started) BTC 永续 Style A 做市 Bot（Go 单进程）。本文覆盖 **Linux 生产部署**、Windows 本地运行要点、配置分阶段与运维检查。

> 产品设计见 [PRD-jev-lighter-btc-mm.md](./PRD-jev-lighter-btc-mm.md)。端到端流程与 Lighter/Vanta API 见 [FLOW-AND-API.md](./FLOW-AND-API.md)。仓库示例配置：`configs/example.env`（复制为 `configs/.env` 后填写，**不要提交 Git**）。

---

## 1. 环境要求

| 项 | 要求 |
|----|------|
| Go | **1.23+**（与 `go.mod` 一致） |
| 网络 | 可访问 `api.rh.lighter.xyz`（RB Lighter 主网）；使用 JEV 时需访问 `api.typesafe.ai` |
| 账户 | Lighter **account index**、API Key（索引通常 **2–254**，见 [API Keys](https://apidocs.rh.lighter.xyz/docs/api-keys)） |
| Chain ID | 主网 **466324**；测试网 **300**，Base URL 见 [Get Started](https://apidocs.rh.lighter.xyz/docs/get-started) |

文档建议低延迟部署靠近 **AWS Tokyo ap-northeast-1a**；本机跨境 RTT 较高时，可增大 `JEV_TIMEOUT` 与 `TICK_INTERVAL`。

---

## 2. Linux 部署（推荐流程）

### 2.1 创建系统用户与目录

```bash
sudo useradd -r -s /usr/sbin/nologin jevbot || true
sudo mkdir -p /opt/jev-sys/{bin,configs,data}
sudo chown -R jevbot:jevbot /opt/jev-sys
```

### 2.2 获取代码并编译

```bash
cd /tmp
git clone https://github.com/Dog-Feng/jey-sys.git
cd jey-sys
go version   # 需 >= 1.23

go build -o /opt/jev-sys/bin/bot ./cmd/bot
sudo chown jevbot:jevbot /opt/jev-sys/bin/bot
sudo chmod 750 /opt/jev-sys/bin/bot
```

### 2.3 配置文件

```bash
sudo cp configs/example.env /opt/jev-sys/configs/.env
sudo chown jevbot:jevbot /opt/jev-sys/configs/.env
sudo chmod 600 /opt/jev-sys/configs/.env
sudo -u jevbot nano /opt/jev-sys/configs/.env
```

启动时默认读取 **`configs/.env`**（相对**当前工作目录**），或通过 **`ENV_FILE`** 指定绝对路径：

```bash
export ENV_FILE=/opt/jev-sys/configs/.env
```

### 2.4 分阶段配置（建议顺序）

**阶段 A — Mock 交易所（验证进程与 HTTP）**

```env
EXCHANGE=mock
DRY_RUN=true
MODEL=mock
HTTP_LISTEN=127.0.0.1:8080
DATA_DIR=/opt/jev-sys/data
```

**阶段 B2 — Vanta（Orderly）干跑**

```env
EXCHANGE=vanta
DRY_RUN=true
VANTA_BASE_URL=https://api.orderly.org
SYMBOL=BTC
TYPESAFE_API_KEYS=key_a,key_b
```

Vanta 说明见 [Vanta API](https://vanta-6.gitbook.io/vanta-gitbook/core-concepts/api)。JEV 默认使用 `TYPESAFE_API_KEYS` 中 **第 2 个** Key（index 1）；Lighter 用 **第 1 个**。额度不足时自动轮换下一个 Key。

**阶段 B1 — Lighter RB 真实盘口 + 模拟下单（不发链上 tx）**

```env
EXCHANGE=lighter
DRY_RUN=true
MODEL=jev
LIGHTER_HOST=https://api.rh.lighter.xyz
LIGHTER_CHAIN_ID=466324
LIGHTER_ACCOUNT_INDEX=<你的账户索引>
TYPESAFE_API_KEY=<TypeSafe 控制台 Key，非 vck_>
JEV_TIMEOUT=2s
TICK_INTERVAL=3s
```

`DRY_RUN=true` 时**不会**初始化 lighter-go 签名器，也**不会**调用 `/api/v1/apikeys`；`LIGHTER_API_PRIVATE_KEY` 可留空。

**阶段 C — 小仓 Live（Post-Only Maker）**

```env
DRY_RUN=false
LIGHTER_API_KEY_INDEX=<2-254>
LIGHTER_API_PRIVATE_KEY=<API 私钥>
LIGHTER_LEVERAGE=5
LIGHTER_LEVERAGE_CROSS=true
ORDER_SIZE_BTC=0.001
MAX_POSITION_BTC=0.003
```

Live 启动时会通过 lighter-go **校验私钥与账户**（`GET /api/v1/apikeys`），并每 tick **撤旧单 + 1 张 post-only**（`nextNonce` + `sendTx`）。

### 2.5 systemd 服务

创建 `/etc/systemd/system/jev-sys.service`：

```ini
[Unit]
Description=jev-sys Lighter BTC Style A bot
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=jevbot
Group=jevbot
WorkingDirectory=/opt/jev-sys
Environment=ENV_FILE=/opt/jev-sys/configs/.env
ExecStart=/opt/jev-sys/bin/bot
Restart=on-failure
RestartSec=5
# 日志走 stdout（JSON）；journald 收集
StandardOutput=journal
StandardError=journal

# 可选：限制资源
LimitNOFILE=65535

[Install]
WantedBy=multi-user.target
```

启用与查看日志：

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now jev-sys
sudo systemctl status jev-sys
journalctl -u jev-sys -f
```

健康检查（默认仅本机监听时）：

```bash
curl -s http://127.0.0.1:8080/health
curl -s http://127.0.0.1:8080/snapshot | jq .
```

### 2.6 防火墙与安全

- **`HTTP_LISTEN`** 默认 `127.0.0.1:8080`，勿对公网暴露；需要远程观测时用 SSH 隧道或反向代理 + 鉴权（`OPS_TOKEN` 可在后续版本扩展，当前 snapshot 无鉴权）。
- **`configs/.env` 权限 600**，仅 `jevbot` 可读。
- 勿将 `TYPESAFE_API_KEY`、`LIGHTER_API_PRIVATE_KEY` 写入 Git 或截图上传。

### 2.7 升级与回滚

```bash
cd /tmp/jey-sys && git pull
go build -o /opt/jev-sys/bin/bot.new ./cmd/bot
sudo systemctl stop jev-sys
sudo mv /opt/jev-sys/bin/bot.new /opt/jev-sys/bin/bot
sudo chown jevbot:jevbot /opt/jev-sys/bin/bot
sudo systemctl start jev-sys
```

---

## 3. Windows 本地运行（简述）

```powershell
cd D:\project\jev-sys
copy configs\example.env configs\.env
# 编辑 configs\.env
go build -o bin\bot.exe .\cmd\bot
.\bin\bot.exe
```

---

## 4. 单 Tick 外部依赖（Live + JEV）

便于排查延迟与超时：

| 顺序 | 目标 | 请求 |
|------|------|------|
| 1 | RB Lighter | `GET /api/v1/orderBookOrders` |
| 2 | RB Lighter | `GET /api/v1/account` |
| 3 | TypeSafe | `POST /v1/systemone` |
| 4–5 | RB Lighter | 撤单：`nextNonce` + `POST /api/v1/sendTx` |
| 6–7 | RB Lighter | 挂单：`nextNonce` + `POST /api/v1/sendTx` |
| 8 | RB Lighter | `GET /api/v1/account`（更新仓位日志） |

日志字段 **`ms`** 为 tick 总耗时；**`ms_book`**（并行拉盘口+账户）、**`ms_jev`**、**`ms_exec`**（撤挂/读仓）便于定位瓶颈。`skip` 且无本地跟踪单时跳过撤单；`skip` 时不重复请求 account。JEV 超时可设 `JEV_TIMEOUT=2s` 或 `3s`。

---

## 5. 常见问题

| 现象 | 处理 |
|------|------|
| `json: cannot unmarshal string ... min_base_amount` | 使用仓库最新代码（`flexFloat` 解析 RB 字符串数值）并重新编译 |
| `lighter Check ... apikeys ... timeout` | DNS/网络；Live 需稳定访问 RB API；Dry-run 勿配私钥或保持 `DRY_RUN=true` |
| `jev ... context deadline exceeded` | 增大 `JEV_TIMEOUT`、`TICK_INTERVAL` |
| `order:"sim"` | `DRY_RUN=true`，正常 |
| `order:"placed"` | Live post-only 已 broadcast |
| 杠杆 | `LIGHTER_LEVERAGE`（或 `LEVERAGE`）1–100；`0` 跳过启动改杠杆；`LIGHTER_LEVERAGE_CROSS` 全仓/逐仓 |
| TypeSafe 401 | 使用 [console.typesafe.ai](https://console.typesafe.ai/keys) 的 Key，不要用 `vck_` Gateway Key |

---

## 6. 测试

```bash
go test ./...
```

---

## 7. 相关链接

- RB Lighter 入门：[Get Started](https://apidocs.rh.lighter.xyz/docs/get-started)
- lighter-go：[github.com/elliottech/lighter-go](https://github.com/elliottech/lighter-go)
- 本仓库：[github.com/Dog-Feng/jey-sys](https://github.com/Dog-Feng/jey-sys)
