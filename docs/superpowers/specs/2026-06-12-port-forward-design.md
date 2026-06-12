# 端口转发功能设计方案

日期：2026-06-12
项目：3x-ui fork（Go 后端 + Vite/React/TypeScript 前端）

## 1. 背景与目标

当前要把某个入站的流量转发到一个 socks5 代理，需要手动编辑 Xray 配置：
加一个 `socks` 出站、再加一条 `routing` 规则（`inboundTag → outboundTag`），非常繁琐。

目标：提供一个可视化页面，选中一个已有入站、填入 socks5 目标，面板自动生成
对应的 socks 出站 + 路由规则，并热重载生效。

### 典型链路

客户端 → (vless/vmess 等代理入站，加密+认证) → 本机 → (socks5 出站) → 目标。

转发功能负责「出」的那一段：为代理入站指定一个 socks5 出口。
**不使用 dokodemo-door/tunnel 作为转发入站**——实际转发用 vless/vmess 等代理协议入站，
目标地址由客户端请求决定，服务端再经 socks5 转出。

## 2. 架构总览

转发规则做成独立模块，唯一真相源是新表 `forward_rules`。在 `GetXrayConfig()`
组装运行配置时，把启用的规则**注入**为 socks 出站 + 路由规则；**绝不改动存储的
`xrayTemplateConfig`**。

完全复用代码库已有的注入范式：`injectPanelEgress`（internal/web/service/xray.go:301）
与 `mergeSubscriptionOutbounds`（xray.go:392）——它们都在生成配置上追加 outbound/routing
而不动模板，且都可被热重载应用（gRPC 增删 outbound + reload routing，不停 Xray 进程）。

## 3. 数据模型

新表 `forward_rules`（`internal/database/model/model.go`，并在迁移处注册自动建表）：

```go
type ForwardRule struct {
    Id          int    `json:"id" gorm:"primaryKey;autoIncrement"`
    InboundTag  string `json:"inboundTag" form:"inboundTag" gorm:"unique"` // 绑定的入站 tag，唯一
    Remark      string `json:"remark" form:"remark"`                        // 备注
    DestAddress string `json:"destAddress" form:"destAddress"`              // socks5 地址
    DestPort    int    `json:"destPort" form:"destPort"`                    // socks5 端口
    Username    string `json:"username" form:"username"`                    // 可选认证
    Password    string `json:"password" form:"password"`                    // 可选认证
    Enable      bool   `json:"enable" form:"enable" gorm:"default:true"`    // 启用开关
}
```

约定：
- 出站 tag：`forward-out-{id}`（自增 id，天然唯一）。
- 路由规则：`{ "type": "field", "inboundTag": [InboundTag], "outboundTag": "forward-out-{id}" }`。
- `InboundTag` 加唯一约束，从数据库层保证「同一入站只允许一条转发规则」。

## 4. 后端

### 4.1 Service：`internal/web/service/forward.go`

`ForwardService` 提供：`GetAll() / Add() / Update() / Delete() / SetEnable()`。
所有写操作完成后调用 `xrayService.SetToNeedRestart()` 触发热重载流程。

### 4.2 注入：`internal/web/service/xray.go`

新增 `injectForwardRules(cfg *xray.Config, rules []model.ForwardRule)`，
在 `GetXrayConfig()` 第 283 行（`injectPanelEgress` 之后、`return` 之前）调用。

逻辑（沿用现有 inject 的安全策略——解析失败就不动对应字段）：

1. 先解析 `cfg.OutboundConfigs`（RawMessage → []any），失败则跳过整个注入。
2. 收集当前生成配置里的入站 tag 集合（`cfg.InboundConfigs[].Tag`）。
3. 遍历启用规则：若 `InboundTag` 不在入站集合中（孤儿规则），**跳过**（不报错）。
4. 为每条有效规则追加 socks 出站（含可选 auth）、并向 `cfg.RouterConfig` 的 `rules`
   追加一条 `inboundTag → forward-out-{id}` 规则。
5. 回写 `cfg.OutboundConfigs` 与 `cfg.RouterConfig`。

出站要先于路由规则引用它存在——同一函数内先 append 出站再 append 路由，与
现有 helper 一致。

### 4.3 Controller：`internal/web/controller/forward.go`

照 `inbound.go` 的 `BindAndValidate` 范式注册路由：

- `GET  /panel/api/forward/list`
- `POST /panel/api/forward/add`
- `POST /panel/api/forward/update/:id`
- `POST /panel/api/forward/del/:id`
- `POST /panel/api/forward/setEnable/:id`

写操作后若 `needRestart` 则调 `xrayService.SetToNeedRestart()`。

### 4.4 入站下拉数据

复用已有的 `InboundService.GetInboundTags()` / inbounds 列表接口，前端拉取
入站列表填充下拉框，显示「端口 + tag + 协议」。下拉**显示所有入站**，不做协议过滤。

## 5. 前端（新页面 `frontend/src/pages/port-forward/`）

照 inbounds 页范式拆分：

- `PortForwardPage.tsx`：容器，组织 message、Modal、表格。
- `usePortForward.ts`：React Query 拉取规则列表 + mutations（增删改、启用）。
- `list/ForwardList.tsx`：Ant Design Table，列含入站、socks5 目标、备注、
  启用开关、编辑/删除操作；孤儿规则（入站已失效）行内标记。
- `form/ForwardFormModal.tsx`：表单 = 入站下拉 / socks5 地址 / 端口 /
  可选用户名密码 / 备注。保存前校验「该入站是否已有规则」，已有则提示去编辑。

接线：

- `routes.tsx`：新增懒加载路由 `/port-forward`。
- `layouts/AppSidebar.tsx`：菜单加一项，图标用 `SwapOutlined`。
- `api/queryKeys.ts`：新增 `portForward` 缓存键。
- i18n：在 `internal/web/translation/*.json` 各语言加 `pages.portForward.*` 文案键。

## 6. 数据流

填表保存 → Controller → Service 写 `forward_rules` 表 → `SetToNeedRestart`
→ 下次 `GetXrayConfig()` 调 `injectForwardRules` 注入 → Xray 热重载（gRPC 增
outbound + reload routing，不停进程）。

## 7. 边界处理

- **孤儿规则**：绑定的入站被删除后，注入时检测到 `InboundTag` 不在入站集合则跳过
  （不报错、不影响其它规则）；列表 UI 标记「入站已失效」，引导用户删除或改绑。
- **一入站一规则**：`forward_rules.InboundTag` 唯一约束（DB 层）+ 前端保存校验（UX 层）
  双重保证；路由按 inboundTag 自上而下匹配，避免一对多产生的歧义。
- **tag 冲突**：`forward-out-{id}` 用自增 id，天然唯一，不与用户手写出站冲突。
- **模板解析失败**：注入函数遇到 outbounds/routing 无法解析时，保持该字段原样、
  不注入，沿用 `mergeSubscriptionOutbounds` 的既有安全策略。

## 8. 不做的事（YAGNI）

- 不在本页创建入站（已确认只绑定已有入站）。
- 不支持非 socks5 的转发目标（如直连 IP、http 代理）——本期只做 socks5。
- 不做多个入站复用同一 socks5 出站的去重优化——每条规则一个独立出站，删除干净。
