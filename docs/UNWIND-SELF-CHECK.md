# 双账户 UNWIND 流程自检

## 状态

| phase | 条件 | 下单 |
|-------|------|------|
| `normal` | 默认 | JEV buy→A long，sell→B short |
| `unwind` | A≥MAX 且 B≤-MAX（连续 N tick 确认） | A short RO + B long RO，每 tick 0～2 单 |
| → `normal` | 双仓 ≤ FLAT_EPS（连续 M tick） | 下一 tick 起恢复 JEV |

## 代码路径

```
trader.onTick
  → GetBook + GetAccount(A) + GetAccount(B)
  → JEV Decide（UNWIND 下仍调用，不下 NORMAL 单）
  → tryEnterUnwind（仅 normal）
  → if phase==unwind → runUnwindTick → return
  → else 原 policy.Map + 单腿 Place
```

## 自检清单

- [ ] `LIGHTER_DUAL=true` 且 `LIGHTER_DUAL_UNWIND=true` 启动日志含 `lighter_dual_unwind`
- [ ] 双满 2 tick（默认）后出现 `unwind_start`
- [ ] UNWIND tick 日志：`phase=unwind`, `exec_leg=unwind_both`, `order_a` / `order_b`
- [ ] UNWIND 期间无 `A_long` / `B_short` 加仓 intent
- [ ] 双零 2 tick 后出现 `unwind_complete`，随后恢复 `exec_leg=A_long|B_short`
- [ ] `min_base` 失败时 `order_*_err=size below min_base`，phase 仍为 unwind 直至手工或成交

## 关键文件

- `internal/policy/unwind.go` — 意图与双满/双零判定
- `internal/trader/dual_unwind.go` — 状态机与执行
- `internal/config/lighter_dual.go` — 环境变量
