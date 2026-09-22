# jev-sys — JEV × Lighter BTC Style A Bot (Go)

Post-only 周期撤挂做市循环（对齐 [jev-trader](https://github.com/jarrodwatts/jev-trader)），单周期 **最多 1 张**新单。

## 快速开始（Mock）

```powershell
cd D:\project\jev-sys
$env:EXCHANGE="mock"
$env:DRY_RUN="true"
$env:MODEL="mock"
go run ./cmd/bot
```

另开终端：

```powershell
curl http://127.0.0.1:8080/snapshot
```

事件写入 `data/events.jsonl`。

## 配置

见 `configs/example.env`。产品设计见 `docs/PRD-jev-lighter-btc-mm.md`。

**Linux / systemd 生产部署**见 **[docs/DEPLOY.md](docs/DEPLOY.md)**。

## JEV / TypeSafe 密钥

- Bot 请求 **`https://api.typesafe.ai/v1/systemone`**，环境变量 **`TYPESAFE_API_KEY`**
- 请在 [console.typesafe.ai/keys](https://console.typesafe.ai/keys) 创建 **TypeSafe 官方 Key**
- **`vck_` 开头的是 Vercel AI Gateway Key**，不能用于 `api.typesafe.ai`（会返回 401）
- 自检：`curl -s -o NUL -w "%{http_code}" https://api.typesafe.ai/v1/models -H "Authorization: Bearer 你的Key"` → 期望 **200**

## Lighter Live

1. `EXCHANGE=lighter`，填 `LIGHTER_*` 与账户索引。
2. `DRY_RUN=true`：读真实盘口，下单仍标记为 `sim`（不 broadcast）；**不会**初始化 lighter-go 签名器，也**不会**调用 [`/api/v1/apikeys`](https://apidocs.rh.lighter.xyz/reference/apikeys) 做密钥校验（可只填 `LIGHTER_HOST`，私钥可留空）。
3. `DRY_RUN=false`：使用 [lighter-go](https://github.com/elliottech/lighter-go) 签名 + `POST /api/v1/sendTx` 发单（已实现于 `internal/exchange/lighter/signer`）。

```powershell
$env:EXCHANGE="lighter"
$env:DRY_RUN="false"
$env:LIGHTER_HOST="https://api.rh.lighter.xyz"
$env:LIGHTER_CHAIN_ID="466324"
$env:LIGHTER_ACCOUNT_INDEX="你的账户索引"
$env:LIGHTER_API_KEY_INDEX="0"
$env:LIGHTER_API_PRIVATE_KEY="你的API私钥"
$env:LIGHTER_LEVERAGE="5"
$env:LIGHTER_LEVERAGE_CROSS="true"
go run ./cmd/bot
```

杠杆 **`LIGHTER_LEVERAGE`**（1–100，**0** 表示启动时不发改杠杆 tx；兼容旧名 **`LEVERAGE`**）。**`LIGHTER_LEVERAGE_CROSS`**：`true` 全仓 / `false` 逐仓。仅在 **`DRY_RUN=false`** 且 Lighter Live 时于启动执行一次。

## 测试

```powershell
go test ./...
```
