# JEV × RB Lighter BTC 永续做市 Bot — 产品设计文档

| 项 | 内容 |
|----|------|
| 版本 | v0.1 |
| 日期 | 2026-09-22 |
| 状态 | 草案 |
| 执行风格 | **A — Post-Only 周期撤挂（对齐 jev-trader）** |
| 后端 | **Go** |
| 前端 | **暂不实现** |
| 目标市场 | RB Lighter 永续 **BTC**（perp 符号 `BTC`） |
| 账户模型 | **单账户**、小仓 live |

---

## 1. 背景与目标

### 1.1 背景

- [jev-trader](https://github.com/jarrodwatts/jev-trader) 在 Monad/Kuru 上实现「每周期读盘 → JEV 选 buy/sell → post-only 限价撤挂」的实验性做市循环。
- 本项目将同一 **决策—执行节奏** 迁移到 **Robinhood Chain 上的 Lighter 永续 BTC**，用 **Go** 实现可长期运行后端服务，**不包含 Web 前端**。
- 决策层使用 **TypeSafe JEV**（`systemone` / HTTP API），输出结构化 buy/sell，不生成自然语言。

### 1.2 产品目标

| 目标 | 说明 |
|------|------|
| G1 | 单 Lighter 账户下，稳定运行 **Style A** 做市循环（撤旧单 + 挂新 post-only 单） |
| G2 | 小仓 live：可配置上限仓位、杠杆、单笔尺寸；异常时可自动 **reduce-only 减仓或暂停** |
| G3 | 可观测：结构化日志、指标、周期事件持久化，便于复盘与对账 |
| G4 | 可运维：配置热更（可选 v2）、健康检查、优雅停机 |

### 1.3 非目标（Out of Scope）

- Web / 移动端仪表盘（v0 不做；可用日志 + Prometheus + 可选只读 HTTP 快照代替）
- 多账户编排、对敲、镜像反向等双账户策略
- 绕过 Lighter / Robinhood 活动或积分规则的设计
- 高频亚秒级竞赛（不追求 Monad 300ms 级极限；周期可配置为 1–3s 级）
- Spot 市场、除 BTC 外的多品种（v0 仅 BTC；架构预留 symbol 扩展）

### 1.4 成功标准（v0）

- Testnet 或 paper 对齐运行 ≥ 24h 无 panic、无重复 nonce/重复挂单失控
- Mainnet 小仓：`|position| ≤ MAX_POSITION_BTC` 恒成立（除网络分区恢复窗口外）
- 每周期 P99 循环延迟 < 配置 `TICK_INTERVAL` 的 80%（含 JEV 调用）

---

## 2. 用户与使用场景

| 角色 | 场景 |
|------|------|
| 运营者（你自己） | 配置 API 密钥与风险参数，启动/停止进程，查看日志与仓位 |
| 开发者 | 扩展 symbol、替换 JEV 为 mock、对接新 Lighter API 版本 |

**典型部署**：单机单进程（或 systemd / Docker），连接 `api.rh.lighter.xyz`（环境可切换 testnet）。

---

## 3. 交易逻辑（Style A — Post-Only 周期撤挂）

### 3.1 核心原则

1. **每个 tick**（可配置间隔，默认 1–3s）：读订单簿 + 账户仓位 → JEV 决策 → **取消本市场当前 bot 管理的挂单** → 挂 **1 张** post-only 限价单。
2. **单周期最多 1 张新单**（与 jev-trader 一致）；不叠多张同向挂单。
3. **JEV 输出** `buy` | `sell` 表示 **本周期倾向挂 bid 侧（long）还是 ask 侧（short）**，不是强制 taker 开仓。
4. **成交改变净仓位**；下一 tick 根据最新 `position` 与 `allowed` 再决策。
5. **减仓与平仓** 通过 **反向 post-only + `reduce_only`** 实现（当 JEV 方向与减仓方向一致时）；禁止 reduce 单误开反向新仓。

### 3.2 JEV 决策语义

- **问题类型**：`choice` — 「未来 H 个 tick（或等价 ~30s）相对当前 mid，更偏向上涨还是下跌？」
- **映射**：`buy` → 挂 **long 侧** post-only（价格：best bid + `QUOTE_INSIDE_TICKS` × tick，且不 crossing）
- **映射**：`sell` → 挂 **short 侧** post-only（价格：best ask − inside ticks）
- **迟到 tick**：若上一 tick 仍在执行（JEV + 下单 RTT 超 `TICK_INTERVAL`），标记 `late`，本 tick **不挂单**（对齐 jev-trader `hold` 行为，可选配置「late 仍挂 reduce_only」为 v1）。

### 3.3 仓位与 allowed（硬规则优先于 JEV）

记 `pos` 为 BTC 净仓位（多正空负），`MAX = MAX_POSITION_BTC`。

| JEV 想要 | 仓位状态 | 本 tick 实际挂单 |
|----------|----------|------------------|
| buy | flat | post-only **long**，开多意图 |
| buy | short | post-only **long**，**reduce_only**，size = min(ORDER_SIZE, \|pos\|) |
| buy | long 且 \|pos\| < MAX | post-only long，加仓 |
| buy | long 且 \|pos\| ≥ MAX | **不挂 long**；若允许则挂 reduce short 或 skip（`capped=true`） |
| sell | 对称 | post-only short / reduce_only long / capped |

**保证金不足**：对应方向 `allowed=false`，跳过该方向；若双向都不允许 → 本 tick skip 并告警。

### 3.4 开平仓定义（产品语义）

| 术语 | 定义 |
|------|------|
| **开仓** | 从 flat 挂出非 reduce_only 的 long/short，成交后 \|pos\| > 0 |
| **加仓** | 同向非 reduce_only，\|pos\| 增加且未超 MAX |
| **减仓** | reduce_only 反向单成交，\|pos\| 下降 |
| **平仓** | reduce_only 使 pos → 0 |
| **紧急平仓** | 运维触发 `FlattenBTC`：cancel_all（本 symbol）+ reduce_only market/IOC 至 flat（需二次确认配置开关） |

### 3.5 与 jev-trader 差异摘要

| 项 | jev-trader | 本产品 |
|----|------------|--------|
| 时钟 | Monad block ~300ms | 可配置 **Ticker**（1–3s） |
| 执行 | 链上 batchUpdate | Lighter **REST/签名 SDK** |
| 仓位 | 本地 MON 库存 | Lighter **perp 净仓位** |
| 费用 | Gas（MON） | RH Chain Gas + **Funding**（持仓） |
| 符号 | MON-USDC | **BTC** perp |

---

## 4. 系统架构

### 4.1 逻辑架构

```text
┌─────────────────────────────────────────────────────────────┐
│                     cmd/bot (main)                          │
│  config load · signal · lifecycle · optional /health        │
└───────────────────────────┬─────────────────────────────────┘
                            │
        ┌───────────────────┼───────────────────┐
        ▼                   ▼                   ▼
 ┌─────────────┐    ┌─────────────┐    ┌─────────────┐
 │   clock     │    │   trader    │    │  telemetry  │
 │  Ticker     │───▶│  Loop       │───▶│ log/metrics │
 └─────────────┘    └──────┬──────┘    │  event store│
                           │           └─────────────┘
              ┌────────────┼────────────┐
              ▼            ▼            ▼
        ┌──────────┐ ┌──────────┐ ┌──────────┐
        │ market   │ │  model   │ │ exchange │
        │ reader   │ │  (JEV)   │ │  (Lighter)│
        └──────────┘ └──────────┘ └──────────┘
```

### 4.2 模块划分（Go package）

| Package | 职责 |
|---------|------|
| `cmd/bot` | 入口、CLI（cobra）、信号处理 |
| `internal/config` | 环境变量 + YAML 校验 |
| `internal/clock` | Ticker；可选与链上 block 对齐（v2） |
| `internal/lighter` | Lighter 客户端：book、account、order CRUD、签名 |
| `internal/book` | 盘口快照、mid/spread/imbalance、tick 对齐 |
| `internal/position` | 从 account 解析 BTC 仓位；allowed 计算 |
| `internal/model` | `Model` 接口；`JevModel`、`MockModel` |
| `internal/policy` | JEV action → 订单意图（side, reduce_only, size, price） |
| `internal/trader` | 主循环：late 检测、撤单、挂单、成交对账 |
| `internal/store` | JSONL 或 SQLite 周期事件（可选） |
| `internal/telemetry` | zap/log、slog、Prometheus |
| `internal/api` | 可选：仅 `GET /health`、`GET /snapshot`（无前端依赖） |

### 4.3 依赖

| 依赖 | 用途 |
|------|------|
| Lighter API / Go SDK（或自封装 HTTP+签名） | 行情、下单、撤单 |
| TypeSafe JEV HTTP `POST /v1/systemone` | 决策 |
| 可选：PostgreSQL/SQLite | 事件持久化 |

---

## 5. 主循环（Trader Loop）规格

### 5.1 单 tick 序列

```text
1. tick_start (monotonic + wall clock)
2. busy gate：若上一 tick 未完成 → late tick，emit event，return
3. book := Lighter.GetBook(BTC)
4. account := Lighter.GetAccount(include BTC position)
5. state := BuildTradeState(book, account, history mids/trades)
6. decision := Model.Decide(ctx, state)   // JEV or mock
7. intent := Policy.Map(decision, position, allowed, book)
8. if intent.Skip → emit event，return
9. cancel := Lighter.CancelBotOrders(BTC)  // 仅 cancel COI 白名单
10. order := Lighter.PlaceLimitPostOnly(intent)
11. emit TickEvent(decision, intent, order)
12. async: 轮询 open orders / fills，更新本地 resting 视图
```

### 5.2 并发模型

- **单 goroutine** 跑主循环（避免竞态）；JEV HTTP 用带 timeout 的 `context`。
- **独立 goroutine**：成交/订单状态轮询、metrics 刷新、可选 HTTP server。
- **原则**：同一 tick 内「读 → 决策 → 撤 → 挂」串行。

### 5.3 TickEvent（持久化 schema）

与 jev-trader `BlockEvent` 对齐，便于日后接任意前端：

```json
{
  "tick_id": 1024,
  "ts_ms": 1789593630676,
  "symbol": "BTC",
  "mid": 98500.5,
  "best_bid": 98500.0,
  "best_ask": 98501.0,
  "spread_bps": 1.02,
  "decision": {
    "action": "buy",
    "probabilities": { "buy": 0.77, "sell": 0.23 },
    "confidence": 0.77,
    "latency_ms": 81,
    "late": false
  },
  "intent": {
    "side": "long",
    "reduce_only": false,
    "price": 98500.1,
    "size_btc": 0.001,
    "capped": false,
    "skip": false
  },
  "order": {
    "client_order_index": "…",
    "status": "placed",
    "error": null
  },
  "position": {
    "side": "long",
    "size_btc": 0.002,
    "entry_price": 98480.0,
    "unrealized_usd": 0.41
  },
  "totals": {
    "ticks": 1024,
    "quotes": 980,
    "fills": 120,
    "late_ticks": 3,
    "jev_usd": 0.012
  }
}
```

---

## 6. JEV 集成

### 6.1 请求

- Endpoint：`https://api.typesafe.ai/v1/systemone`（可配置 `TYPESAFE_BASE_URL`）
- Auth：`Authorization: Bearer ${TYPESAFE_API_KEY}`
- Model：建议 live 使用 **`jev-1.13.0`**（pin 版本）；开发可用 `jev-latest`

### 6.2 State 字段（BuildTradeState）

| 字段 | 类型 | 说明 |
|------|------|------|
| market | string | `"BTC"` |
| tick_id | int64 | 单调递增 |
| horizon_ticks | int | 与 jev-trader HORIZON 类似 |
| mid, spread_bps | float | 来自 book |
| book_imbalance | float | 近 mid 深度不平衡 |
| returns_bps | object | last1/5/20/… 来自 mid 环形缓冲 |
| recent_mids | string | 降采样 mid 序列 |
| recent_trades | array | 最近 N 笔 public trades 摘要 |
| position | object | side, size_btc, entry, unrealized |
| funding_rate | float | 当前 BTC funding（若有） |
| allowed | object | increase_long, increase_short, reduce_long, reduce_short |

### 6.3 Questions

```json
{
  "direction": {
    "type": "choice",
    "instructions": "Over the next horizon, is price more likely up or down vs mid?",
    "criteria": {
      "buy": "Upward bias",
      "sell": "Downward bias"
    }
  }
}
```

### 6.4 MockModel

- 无 API Key 时使用：简单 momentum（last5 returns 符号）或随机，用于 CI 与 Lighter testnet。

---

## 7. Lighter 集成要点

### 7.1 环境

| 变量 | 说明 |
|------|------|
| `LIGHTER_HOST` | 如 `https://api.rh.lighter.xyz` |
| `LIGHTER_API_PRIVATE_KEY` | 签名密钥 |
| `LIGHTER_ACCOUNT_INDEX` | 账户索引 |
| `LIGHTER_API_KEY_INDEX` | API key 索引 |

### 7.2 订单

- 主路径：`order limit --post_only`（Go SDK 等价参数）
- Side：perp 使用 **long / short**
- **reduce_only**：由 `policy` 严格设置
- 维护 **client_order_index** 白名单，撤单只撤 bot 创建的 COI

### 7.3 启动检查清单

1. `market info BTC` → min size、decimals、tick
2. `account info` → 保证金、现有 BTC 仓位
3. `position leverage` → 设为配置值（如 2x cross）
4. 若存在非 bot 挂单 → 日志 WARN（不自动 cancel 非白名单单，除非配置 `CANCEL_FOREIGN_ORDERS=false` 默认）

---

## 8. 配置项（v0）

### 8.1 交易

| 键 | 默认 | 说明 |
|----|------|------|
| `SYMBOL` | BTC | perp 符号 |
| `TICK_INTERVAL` | 2s | 主循环周期 |
| `ORDER_SIZE_BTC` | 0.001 | 每 tick 挂单量（需 ≥ min） |
| `MAX_POSITION_BTC` | 0.003 | 净仓绝对值上限 |
| `QUOTE_INSIDE_TICKS` | 1 | post-only 价相对 touch 内移 tick 数 |
| `HORIZON_TICKS` | 15 | JEV 预测窗口（约 30s @ 2s） |
| `LEVERAGE` | 2 | 启动时设置 cross leverage |

### 8.2 风控

| 键 | 说明 |
|----|------|
| `MAX_DAILY_LOSS_USD` | 超则 pause + 可选 flatten |
| `MIN_SPREAD_BPS` | 过宽不挂单 |
| `JEV_MIN_CONFIDENCE` | 低于则仅允许 reduce 方向 |
| `ENABLE_EMERGENCY_FLATTEN` | 是否允许 API/信号触发清仓 |

### 8.3 JEV

| 键 | 说明 |
|----|------|
| `MODEL` | `jev` \| `mock` |
| `TYPESAFE_API_KEY` | 密钥 |
| `JEV_MODEL_ID` | 默认 `jev-1.13.0` |
| `JEV_TIMEOUT` | 如 800ms |

---

## 9. 对外接口（无前端时的运维 API）

v0 **可选**，仅 localhost 绑定：

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/health` | ok / degraded（Lighter 或 JEV 不可达） |
| GET | `/snapshot` | 最近 1 条 TickEvent + 当前 resting orders |
| POST | `/pause` | 暂停挂单（需 token） |
| POST | `/resume` | 恢复 |
| POST | `/flatten` | 需 `ENABLE_EMERGENCY_FLATTEN` + token |

**不提供**浏览器 UI；运营者用 `curl` + 日志 + Grafana。

---

## 10. 可观测性

| 类型 | 内容 |
|------|------|
| 日志 | 结构化 JSON（tick_id, action, order_status, pos, err） |
| 指标 | `tick_latency_ms`, `jev_latency_ms`, `late_ticks_total`, `orders_placed`, `fills`, `position_size_btc`, `unrealized_pnl_usd` |
| 审计 | JSONL `data/events.jsonl` 或 SQLite `tick_events` 表 |

---

## 11. 安全

- 密钥仅环境变量或本地 secret 文件，**不入库**
- 运维 API 必须 token + 127.0.0.1
- 日志脱敏：不打印 private key、完整 auth header
- 生产禁用 `MOCK` 误配置开关（启动时 `MODEL=mock` + mainnet host → fatal）

---

## 12. 部署

### 12.1 推荐形态

- Docker 单容器或 bare metal + systemd
- 单进程单账户；资源：1 CPU、512MB 起（视 SDK 而定）

### 12.2 发布流程

1. testnet / paper 回归
2. mainnet 小仓 `ORDER_SIZE=min`，观察 24h
3. 逐步放大至目标 MAX（仍为小仓）

---

## 13. 里程碑

| 阶段 | 交付 | 验收 |
|------|------|------|
| M0 | 仓库骨架、config、mock loop | mock 跑通 tick 日志 |
| M1 | Lighter 读 book + account + post-only 下单 + cancel | testnet 手动验单 |
| M2 | JEV 接入 + policy + allowed | 与 mock 切换对比 |
| M3 | 成交对账、TickEvent 持久化、metrics | 24h soak |
| M4 | 风控 pause/flatten、运维 API | 故障注入测试 |
| M5 | mainnet 小仓 live | 成功标准 §1.4 |

---

## 14. 风险与限制

| 风险 | 缓解 |
|------|------|
| Funding 侵蚀 | 监控累计 funding；限制持仓时间 |
| post-only 被拒（ crossing） | 价格 clamp 到 tick；记录 reverted |
| Lighter API 变更 | lighter 客户端版本 pin + 集成测试 |
| JEV 误判 | confidence 门控 + MAX 仓位 + 日损上限 |
| 网络分区重复下单 | 单 goroutine + COI 幂等 + 撤单优先 |

---

## 15. 附录 A — 目录结构（建议）

```text
jev-sys/
├── cmd/bot/main.go
├── internal/
│   ├── config/
│   ├── clock/
│   ├── lighter/
│   ├── book/
│   ├── position/
│   ├── model/
│   ├── policy/
│   ├── trader/
│   ├── store/
│   ├── telemetry/
│   └── api/
├── configs/example.yaml
├── docs/
│   └── PRD-jev-lighter-btc-mm.md
├── go.mod
└── Dockerfile
```

---

## 16. 附录 B — Policy 伪代码

```go
func Map(dec Decision, pos Position, book Book, cfg Config) Intent {
    wantLong := dec.Action == Buy
    size := cfg.OrderSizeBTC

    if wantLong {
        if pos.IsShort() {
            return Intent{Side: Long, ReduceOnly: true, Size: min(size, pos.Abs())}
        }
        if pos.IsLong() && pos.Abs() >= cfg.MaxPositionBTC {
            return Intent{Skip: true, Capped: true}
        }
        if !pos.AllowedIncreaseLong() {
            return Intent{Skip: true}
        }
        return Intent{Side: Long, Price: QuotePriceLong(book, cfg), Size: size}
    }
    // sell 对称 → short / reduce long
}
```

---

## 17. 文档修订记录

| 版本 | 日期 | 说明 |
|------|------|------|
| v0.1 | 2026-09-22 | 初稿：Style A、Go、无前端、BTC perp |
