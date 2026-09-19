# 端口转发 v2 设计方案：域名白名单 / 健康检查 / 到期提醒 / 一键续费

日期：2026-09-19
项目：3x-ui fork（Go 后端 + Vite/React/TypeScript 前端）
前置：`docs/superpowers/specs/2026-06-12-port-forward-design.md`（v1，已在 `feat/port-forward` 实现）

## 1. 背景与目标

业务场景：服务器统一在运营者手里，每个客户一个 vless/vmess 入站（节点），客户只拿到
二维码/订阅链接。每个节点可绑定一个住宅 socks5/http 代理作为出口，用于运营 TikTok 等社媒；
未绑定的节点走服务器 IP 直出。客户线下按月/季/年续费，由运营者手动顺延。机器多为 1C1G，
单机 ≤ 30 个节点。

v1 已实现「按入站绑一个 socks5/http 出口」。v2 在此基础上补齐运营所需：

1. 域名白名单（全局 + 节点级，节点开关三态）
2. 代理健康检查（出口 IP / 地区 / 延迟，异常 TG 告警）
3. 代理供应商到期提醒
4. 客户端一键续费（+1 / +3 / +12 月）

### 决策记录

- **绑定粒度：按入站**（不按 client）。与运营者现有用法一致，v1 代码直接复用；
  一个入站一个 client，到期/流量/limitIp 照常可用；节点互相隔离，一个端口被封不影响其他。
- **白名单两级语义：节点覆盖全局**。节点开关关 → 不限制；开 + 自定义为空 → 全局；
  开 + 自定义非空 → 自定义。
- **fail-closed 天然成立**：路由只把入站流量送到代理出站，代理挂了就是连不上，
  不会回落到机房 IP。不做 balancer / 备用代理。
- **不建新表**：代理池、供应商、续费流水、客户档案都不做；供应商信息写规则备注。

## 2. 前置步骤：同步上游

main 落后上游 `MHSanaei/3x-ui` main 约 818 个提交（3 个月）。实施第一步：

1. `main` 合并上游 main（本地 main 只多 3 个文档提交，无代码冲突）
2. `feat/port-forward`（12 个提交）rebase 到新 main。预计冲突点：`internal/web/service/xray.go`、
   `internal/database/model/model.go`、`internal/database/db.go`、`frontend/src/routes.tsx`、
   `frontend/src/layouts/AppSidebar.tsx`、`internal/web/translation/*.json`——均为追加型改动
3. 后端 `go build ./... && go test ./internal/...`、前端 `npm run build` 通过后再开始 v2 开发

若 rebase 冲突超出预期，改为 merge main 进 feat 分支。

## 3. 数据模型

只扩展 `forward_rules` 表（`internal/database/model/model.go`），GORM 自动迁移加列：

```go
type ForwardRule struct {
    // ── v1 已有 ──
    Id          int
    InboundTag  string // 绑定入站 tag，唯一
    Remark      string // 面板内备注（客户不可见）：客户姓名/微信/供应商等
    DestType    string // socks | http
    DestAddress string
    DestPort    int
    Username    string
    Password    string
    Enable      bool

    // ── 域名白名单 ──
    DomainLimit bool   `json:"domainLimit" form:"domainLimit"` // 开关
    Domains     string `json:"domains" form:"domains"`         // 自定义白名单，换行分隔

    // ── 供应商到期 ──
    ExpiryTime  int64  `json:"expiryTime" form:"expiryTime"`   // 毫秒时间戳，0 = 不设

    // ── 健康检查结果（定时任务写入，API 只读，表单提交时忽略）──
    CheckedAt   int64  `json:"checkedAt"`
    CheckOK     bool   `json:"checkOk"`
    CheckIP     string `json:"checkIp"`
    CheckGeo    string `json:"checkGeo"`  // "United States / Los Angeles"
    CheckMs     int    `json:"checkMs"`
    CheckErr    string `json:"checkErr"`
}
```

全局设置复用 3x-ui 的 `settings` 键值表（`SettingService` 新增 getter/setter）：

| 键 | 默认值 | 说明 |
|---|---|---|
| `forwardGlobalDomains` | `""` | 全局白名单，换行分隔 |
| `forwardCheckUrl` | `http://ip-api.com/json/?fields=query,country,city` | 健康检查请求地址 |

域名格式原样透传给 Xray（`domain:tiktok.com`、`geosite:xxx`、纯子串、`regexp:` 均可），
面板只做去空行 / trim。

## 4. Xray 配置注入

扩展 v1 的 `injectForwardRules(cfg, rules)`（`internal/web/service/xray.go`），
新增入参：全局白名单。注入内容：

**出站**（追加到 outbounds 末尾）：
- `forward-block`：`{"protocol":"blackhole","tag":"forward-block"}`，有任意规则生效时注入一次
- `forward-out-{id}`：v1 已有

**路由**（追加到 `routing.rules` 末尾，每条规则按以下顺序，Xray 自上而下匹配）：

```
① {type:field, inboundTag:[T], network:"udp",       outboundTag:"forward-block"}
② {type:field, inboundTag:[T], domain:[白名单...],  outboundTag:"forward-out-N"}   仅白名单生效时
③a {type:field, inboundTag:[T],                      outboundTag:"forward-out-N"}   白名单不生效时
③b {type:field, inboundTag:[T],                      outboundTag:"forward-block"}   白名单生效时
```

- ① 固定注入：住宅代理基本不支持 UDP，直接 block 让 App（QUIC）回落 TCP。
- 白名单生效 = `DomainLimit && effectiveList 非空`，`effectiveList = Domains 非空 ? Domains : 全局`。
- ③b 会把未匹配域名的流量（含客户端直连纯 IP 的流量）全部 block——这是刻意的，否则白名单形同虚设。
- 白名单依赖入站开启嗅探（3x-ui 默认开）。UI 在列表中对嗅探关闭的入站显示提示，后端不干预入站配置。
- 沿用 v1 安全策略：outbounds / routing 解析失败则整体跳过注入，不动模板。

## 5. 后端

### 5.1 Service（`internal/web/service/forward.go`，v1 已有，扩展）

- `Add/Update`：新增字段入库；`Check*` 字段不接受表单输入（更新时保留旧值）。
- `GetGlobalDomains / SetGlobalDomains / GetCheckUrl / SetCheckUrl`：转发到 `SettingService`。
- `CheckOne(rule) error`：执行一次健康检查并写回 `Check*`；供定时任务与「立即检测」共用。
- `CheckAll()`：遍历启用规则，5 并发（`semaphore` 用 buffered channel），逐条 `CheckOne`，
  收集状态变化后统一发 TG。

### 5.2 健康检查实现（`internal/web/service/forward_check.go`，新文件）

- 用标准库 `net/http`：`http.Transport{Proxy: http.ProxyURL(u)}`，
  `u = socks5://user:pass@host:port` 或 `http://user:pass@host:port`。Go 原生支持 socks5 代理 URL。
  不使用 3x-ui 的 `OutboundService.TestOutbound`（它起临时 Xray 进程，1C1G 上 30 条跑不动）。
- 超时 15s。请求 `forwardCheckUrl`（经 `SanitizePublicHTTPURL` 校验防 SSRF）。
- 响应解析：优先按 JSON 取 `query`（或 `ip`）、`country`、`city`；JSON 解析失败则把 body trim
  后当作纯 IP（兼容 ipify 类接口）。
- 写回：`CheckedAt=now, CheckOK, CheckIP, CheckGeo="country / city", CheckMs, CheckErr`。
- **告警规则**（`tgbot.SendMsgToTgbotAdmins`，TG 未启用则只记日志）：
  - 正常 → 失败：发「[转发] 规则 {Remark|InboundTag} 检测失败：{err}」
  - 失败持续：不重复发
  - 失败 → 恢复：发「[转发] 规则 … 已恢复，出口 {IP}」
  - 出口 IP 变化（前后都 OK 且 IP 不同）：发「[转发] 规则 … 出口 IP 变化 {old} → {new}」
  - 首次检测（`CheckedAt==0`）不告警

### 5.3 定时任务（`internal/web/job/`）

- `forward_check_job.go`：`@every 10m` → `ForwardService.CheckAll()`。
- `forward_expiry_job.go`：`@daily` → 遍历 `ExpiryTime>0` 的规则，`ExpiryTime - now < 3 天`（含已过期）
  的汇总为一条 TG 消息发管理员。无符合项不发。
- 在 `internal/web/web.go` 的 `startTask` 注册。

### 5.4 Controller（`internal/web/controller/forward.go`，v1 已有，扩展）

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/panel/api/forward/list` | v1，返回含 Check* 字段 |
| POST | `/panel/api/forward/add` `/update/:id` `/del/:id` `/setEnable/:id` | v1 |
| POST | `/panel/api/forward/check/:id` | 立即检测一条，返回更新后的规则 |
| GET | `/panel/api/forward/settings` | `{globalDomains, checkUrl}` |
| POST | `/panel/api/forward/settings` | 保存两项；改全局白名单后 `SetToNeedRestart()` |

`add/update/setEnable/del` 后 `SetToNeedRestart()`（v1 已有）。

## 6. 前端

### 6.1 转发页（`frontend/src/pages/port-forward/`，v1 已有，扩展）

- `types.ts`：`ForwardRule` 加新字段；`ForwardSettings {globalDomains, checkUrl}`。
- `usePortForward.ts`：加 `checkNow(id)`、`settings` query、`saveSettings` mutation。
- `form/ForwardFormModal.tsx` 新增：
  - 「域名限制」Switch；开启时显示 TextArea「自定义白名单（每行一条，留空则使用全局）」
  - 「供应商到期」DatePicker（可清空 → 0）
- `list/ForwardList.tsx` 新增列：
  - 健康：`Badge` 绿/红/灰（未检测），文字 `IP · 地区 · ms`，Tooltip 显示检测时间与错误
  - 到期：`Tag`，颜色规则同客户页（无=灰、>3 天绿、≤3 天橙、已过期红）
  - 白名单：`关 / 全局 / 自定义`
  - 入站嗅探关闭 → 行内 warning 图标「嗅探未开启，白名单不生效」（从已拉取的入站列表读 `sniffing.enabled`）
  - 操作列加「立即检测」按钮（loading 态）
- `PortForwardPage.tsx`：右上角「全局设置」按钮 → `form/ForwardSettingsModal.tsx`
  （全局白名单 TextArea + 检测 URL Input）。
- i18n：`pages.portForward.*` 新增键，en-US / zh-CN。

### 6.2 客户页续费（`frontend/src/pages/clients/ClientsPage.tsx`）

- 操作列加 `Dropdown` 按钮「续费」，菜单：+1 月 / +3 月 / +12 月。
- 逻辑：`base = max(expiryTime, now)`；`new = dayjs(base).add(n, 'month').valueOf()`；
  调 `POST /panel/api/clients/update/:email`，body 为该 client 现有字段 + 新 `expiryTime`。
- `expiryTime <= 0`（永不过期 / 首次使用后计时）时按钮禁用并 Tooltip 说明。
- 成功后刷新列表并 message。

## 7. 数据流

```
表单保存 ──► Controller ──► ForwardService 写表 ──► SetToNeedRestart
                                                        │
GetXrayConfig ──► injectForwardRules(rules, globalDomains) ──► 热重载

ForwardCheckJob(10m) ──► CheckAll ──► CheckOne × N(5 并发) ──► 写 Check* ──► 状态变化 → TG
ForwardExpiryJob(daily) ──► 汇总 3 天内到期 ──► TG
「立即检测」 ──► POST /forward/check/:id ──► CheckOne ──► 返回规则

客户页「续费」 ──► POST /clients/update/:email (expiryTime 顺延) ──► 3x-ui 既有流程
```

## 8. 边界与错误处理

- **孤儿规则**（入站已删）：v1 已处理，注入时跳过、列表标记。健康检查仍照常执行（代理本身可测）。
- **全局白名单为空且节点自定义为空但开关开**：等价于不限制，列表显示「全局(空)」提示。
- **健康检查 URL 不合法**：`CheckAll` 直接返回并记 warning，不写 `CheckErr`。
- **代理认证失败 / 超时**：记为失败，`CheckErr` 存错误文本（截断 200 字）。
- **TG 未启用**：告警只写日志。
- **并发写回**：每条规则独立 `UPDATE ... WHERE id=?` 只更新 `Check*` 列，不与表单更新互相覆盖。
- **续费竞态**：前端以列表中的当前 `expiryTime` 计算；客户被同时编辑的概率可忽略。

## 9. 测试

- `injectForwardRules`：白名单四态（关 / 开+空+全局空 / 开+空+全局有 / 开+自定义）的路由输出顺序与内容；UDP block 与 blackhole 出站只注入一次。
- `forward_check.go`：用 `httptest` 起一个 HTTP 代理 + 目标，验证 JSON / 纯文本两种响应解析、超时记为失败。
- 告警判定：给定 (oldRule, newRule) 表驱动测试四种告警情形 + 首次不告警。
- 到期任务：边界 3 天、已过期、0 不设。
- 前端 `helpers.test.ts`：续费日期计算（未过期顺延 / 已过期从今起 / ≤0 禁用）、白名单三态显示文案。

## 10. 不做（YAGNI）

代理池表、供应商表、续费流水表、客户档案页、按出站流量统计、balancer / observatory
自动切换、备用代理、检测间隔可配置、`geoip:private` / bittorrent / 25 端口黑名单、
域名列表语法校验。
