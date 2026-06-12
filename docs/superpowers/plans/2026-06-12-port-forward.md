# 端口转发功能 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在 3x-ui 面板加一个「端口转发」页面，把某个已有入站的流量一键绑定到 socks5 或 http 代理出口，自动生成 Xray 出站 + 路由规则并热重载。

**Architecture:** 转发规则存独立表 `forward_rules`（唯一真相源），在 `GetXrayConfig()` 构建运行配置时注入成 socks/http 出站 + 路由规则，绝不改动存储的 `xrayTemplateConfig`。完全复用现有 `injectPanelEgress` / `mergeSubscriptionOutbounds` 的注入范式与 gRPC 热重载机制。

**Tech Stack:** Go + GORM + Gin（后端）；React 19 + Vite + Ant Design 6 + TanStack Query + react-i18next（前端）。

**Spec:** `docs/superpowers/specs/2026-06-12-port-forward-design.md`

**约定（贯穿全程）：**
- 出站 tag：`forward-out-{id}`（自增 id，天然唯一）
- 一个入站只允许一条转发规则：DB 唯一约束（`InboundTag`）+ 前端保存校验
- 目标类型 `DestType`：`socks`（socks5）或 `http`，决定注入出站的 `protocol`
- 孤儿规则（入站已删）：注入时跳过，不报错；列表 UI 标记

---

## File Structure

**后端（Go）**
- `internal/database/model/model.go` — 新增 `ForwardRule` 结构体（修改）
- `internal/database/db.go` — `initModels()` 注册 `&model.ForwardRule{}`（修改）
- `internal/web/service/forward.go` — `ForwardService` CRUD + 校验（新建）
- `internal/web/service/xray.go` — 新增 `injectForwardRules` 并在 `GetXrayConfig` 调用（修改）
- `internal/web/controller/forward.go` — `ForwardController` + 路由（新建）
- `internal/web/controller/api.go` — 注册 `/panel/api/forward` 路由组（修改）
- `internal/web/controller/spa.go` — 新增 `GET /port-forward` SPA shell 路由（修改）
- `internal/web/translation/en-US.json`、`zh-CN.json` — 文案键（修改）

**前端（TypeScript / React）**
- `frontend/src/pages/port-forward/types.ts` — `ForwardRule` 类型（新建）
- `frontend/src/pages/port-forward/helpers.ts` — 纯逻辑辅助（新建）
- `frontend/src/pages/port-forward/usePortForward.ts` — React Query hook（新建）
- `frontend/src/pages/port-forward/form/ForwardFormModal.tsx` — 表单弹窗（新建）
- `frontend/src/pages/port-forward/list/ForwardList.tsx` — 列表表格（新建）
- `frontend/src/pages/port-forward/PortForwardPage.tsx` — 页面容器（新建）
- `frontend/src/api/queryKeys.ts` — 新增 `portForward` 缓存键（修改）
- `frontend/src/routes.tsx` — 新增懒加载路由（修改）
- `frontend/src/layouts/AppSidebar.tsx` — 新增菜单项（修改）

**测试**
- `internal/web/service/forward_test.go` — Service CRUD（新建）
- `internal/web/service/xray_forward_inject_test.go` — 注入逻辑（新建）
- `frontend/src/pages/port-forward/helpers.test.ts` — 纯逻辑（新建）

---

## Task 1: 数据模型 ForwardRule + 自动迁移

**Files:**
- Modify: `internal/database/model/model.go`
- Modify: `internal/database/db.go:60-77`
- Test: `internal/web/service/forward_test.go`

- [ ] **Step 1: 写失败测试（建表 + 基本 CRUD）**

新建 `internal/web/service/forward_test.go`：

```go
package service

import (
	"path/filepath"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

// newForwardTestDB wires a temp sqlite db (all models auto-migrated) the same
// way setupConflictDB does in port_conflict_test.go. A real file (not :memory:)
// avoids the per-connection isolation problem of the pure-Go sqlite driver.
func newForwardTestDB(t *testing.T) {
	t.Helper()
	dbDir := t.TempDir()
	t.Setenv("XUI_DB_FOLDER", dbDir)
	if err := database.InitDB(filepath.Join(dbDir, "x-ui.db")); err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	t.Cleanup(func() { _ = database.CloseDB() })
}

func TestForwardRuleCRUD(t *testing.T) {
	newForwardTestDB(t)
	svc := ForwardService{}

	rule := &model.ForwardRule{
		InboundTag:  "in-1000-tcp",
		DestType:    "socks",
		DestAddress: "1.2.3.4",
		DestPort:    1080,
		Enable:      true,
	}
	if err := svc.Add(rule); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if rule.Id == 0 {
		t.Fatal("expected autoincrement id to be set")
	}

	all, err := svc.GetAll()
	if err != nil {
		t.Fatalf("GetAll: %v", err)
	}
	if len(all) != 1 || all[0].InboundTag != "in-1000-tcp" {
		t.Fatalf("unexpected GetAll result: %+v", all)
	}

	if err := svc.SetEnable(rule.Id, false); err != nil {
		t.Fatalf("SetEnable: %v", err)
	}
	active, err := svc.ActiveRules()
	if err != nil {
		t.Fatalf("ActiveRules: %v", err)
	}
	if len(active) != 0 {
		t.Fatalf("expected 0 active rules after disable, got %d", len(active))
	}

	if err := svc.Delete(rule.Id); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	all, _ = svc.GetAll()
	if len(all) != 0 {
		t.Fatalf("expected 0 rules after delete, got %d", len(all))
	}
}
```

> 注：DB 初始化照搬 `internal/web/service/port_conflict_test.go` 的 `setupConflictDB`（temp 目录 + `database.InitDB` + `database.CloseDB`）。`database.InitDB(dbPath string) error` 与 `database.CloseDB() error` 均已存在。

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./internal/web/service/ -run TestForwardRuleCRUD -v`
Expected: 编译失败，`undefined: model.ForwardRule` 和 `undefined: ForwardService`（ForwardService 在 Task 2 建；本步先让 model 编译通过）。

- [ ] **Step 3: 新增 ForwardRule 模型**

在 `internal/database/model/model.go` 末尾追加（紧邻其它 model 结构体）：

```go
// ForwardRule binds an inbound (by tag) to a socks/http proxy egress.
// Rules are the source of truth for the port-forwarding feature; they are
// injected into the generated Xray config at build time and never written
// into the stored xrayTemplateConfig. One rule per inbound (Tag is unique).
type ForwardRule struct {
	Id          int    `json:"id" gorm:"primaryKey;autoIncrement"`
	InboundTag  string `json:"inboundTag" form:"inboundTag" gorm:"unique" validate:"required"`
	Remark      string `json:"remark" form:"remark"`
	DestType    string `json:"destType" form:"destType" validate:"required,oneof=socks http"`
	DestAddress string `json:"destAddress" form:"destAddress" validate:"required"`
	DestPort    int    `json:"destPort" form:"destPort" validate:"required,min=1,max=65535"`
	Username    string `json:"username" form:"username"`
	Password    string `json:"password" form:"password"`
	Enable      bool   `json:"enable" form:"enable" gorm:"default:true"`
}
```

- [ ] **Step 4: 注册自动迁移**

在 `internal/database/db.go` 的 `initModels()` 里，`models := []any{...}` 切片末尾（`&model.OutboundSubscription{},` 之后）加一行：

```go
		&model.ForwardRule{},
```

- [ ] **Step 5: 运行测试确认通过**

Run: `go test ./internal/web/service/ -run TestForwardRuleCRUD -v`
Expected: 仍会因 `ForwardService` 未定义而编译失败 —— 这是预期的，Task 2 补上。先确认 `model.ForwardRule` 已无报错（错误信息里不再出现 `model.ForwardRule`）。

- [ ] **Step 6: 提交**

```bash
git add internal/database/model/model.go internal/database/db.go internal/web/service/forward_test.go
git commit -m "feat(forward): add ForwardRule model and auto-migration

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Task 2: ForwardService（CRUD + 校验）

**Files:**
- Create: `internal/web/service/forward.go`
- Test: `internal/web/service/forward_test.go`（Task 1 已建，本任务让它通过）

- [ ] **Step 1: 实现 ForwardService**

新建 `internal/web/service/forward.go`：

```go
package service

import (
	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

// ForwardService manages port-forwarding rules. Each rule binds an inbound tag
// to a socks/http proxy egress. Rules are the single source of truth and are
// injected into the generated Xray config by injectForwardRules at build time;
// the stored template is never modified.
type ForwardService struct{}

// GetAll returns every forward rule, oldest first.
func (s *ForwardService) GetAll() ([]model.ForwardRule, error) {
	db := database.GetDB()
	var rules []model.ForwardRule
	if err := db.Model(&model.ForwardRule{}).Order("id asc").Find(&rules).Error; err != nil {
		return nil, err
	}
	return rules, nil
}

// ActiveRules returns only enabled rules — used by config generation.
func (s *ForwardService) ActiveRules() ([]model.ForwardRule, error) {
	db := database.GetDB()
	var rules []model.ForwardRule
	if err := db.Model(&model.ForwardRule{}).Where("enable = ?", true).Order("id asc").Find(&rules).Error; err != nil {
		return nil, err
	}
	return rules, nil
}

// Add persists a new rule. The unique constraint on InboundTag enforces the
// "one rule per inbound" invariant at the DB layer.
func (s *ForwardService) Add(rule *model.ForwardRule) error {
	db := database.GetDB()
	return db.Create(rule).Error
}

// Update saves an existing rule (full overwrite by primary key).
func (s *ForwardService) Update(rule *model.ForwardRule) error {
	db := database.GetDB()
	return db.Save(rule).Error
}

// Delete removes a rule by id.
func (s *ForwardService) Delete(id int) error {
	db := database.GetDB()
	return db.Delete(&model.ForwardRule{}, id).Error
}

// SetEnable flips only the enable flag without rewriting the rest of the row.
func (s *ForwardService) SetEnable(id int, enable bool) error {
	db := database.GetDB()
	return db.Model(&model.ForwardRule{}).Where("id = ?", id).Update("enable", enable).Error
}
```

- [ ] **Step 2: 运行测试确认通过**

Run: `go test ./internal/web/service/ -run TestForwardRuleCRUD -v`
Expected: PASS

- [ ] **Step 3: 提交**

```bash
git add internal/web/service/forward.go
git commit -m "feat(forward): add ForwardService CRUD

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Task 3: 构建时注入 injectForwardRules

**Files:**
- Modify: `internal/web/service/xray.go`（新增函数 + 在 `GetXrayConfig` 调用）
- Test: `internal/web/service/xray_forward_inject_test.go`

- [ ] **Step 1: 写失败测试**

新建 `internal/web/service/xray_forward_inject_test.go`：

```go
package service

import (
	"encoding/json"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/util/json_util"
	"github.com/mhsanaei/3x-ui/v3/internal/xray"
)

func baseCfgWithInbound(tag string) *xray.Config {
	return &xray.Config{
		InboundConfigs:  []xray.InboundConfig{{Port: 1000, Protocol: "vless", Tag: tag}},
		OutboundConfigs: json_util.RawMessage(`[{"protocol":"freedom","tag":"direct"}]`),
		RouterConfig:    json_util.RawMessage(`{"rules":[{"type":"field","inboundTag":["api"],"outboundTag":"api"}]}`),
	}
}

func TestInjectForwardRules_AddsOutboundAndRoute(t *testing.T) {
	cfg := baseCfgWithInbound("in-1000-tcp")
	rules := []model.ForwardRule{{
		Id: 7, InboundTag: "in-1000-tcp", DestType: "socks",
		DestAddress: "9.9.9.9", DestPort: 1080, Enable: true,
	}}

	injectForwardRules(cfg, rules)

	var outbounds []map[string]any
	if err := json.Unmarshal(cfg.OutboundConfigs, &outbounds); err != nil {
		t.Fatalf("outbounds unparsable: %v", err)
	}
	if len(outbounds) != 2 {
		t.Fatalf("expected 2 outbounds, got %d", len(outbounds))
	}
	got := outbounds[1]
	if got["protocol"] != "socks" || got["tag"] != "forward-out-7" {
		t.Fatalf("unexpected injected outbound: %+v", got)
	}

	var routing map[string]any
	if err := json.Unmarshal(cfg.RouterConfig, &routing); err != nil {
		t.Fatalf("routing unparsable: %v", err)
	}
	rulesArr := routing["rules"].([]any)
	if len(rulesArr) != 2 {
		t.Fatalf("expected 2 routing rules, got %d", len(rulesArr))
	}
	last := rulesArr[1].(map[string]any)
	if last["outboundTag"] != "forward-out-7" {
		t.Fatalf("unexpected routing rule: %+v", last)
	}
}

func TestInjectForwardRules_HttpProtocolAndAuth(t *testing.T) {
	cfg := baseCfgWithInbound("in-1000-tcp")
	rules := []model.ForwardRule{{
		Id: 1, InboundTag: "in-1000-tcp", DestType: "http",
		DestAddress: "1.1.1.1", DestPort: 8080,
		Username: "u", Password: "p", Enable: true,
	}}

	injectForwardRules(cfg, rules)

	var outbounds []map[string]any
	json.Unmarshal(cfg.OutboundConfigs, &outbounds)
	got := outbounds[1]
	if got["protocol"] != "http" {
		t.Fatalf("expected http protocol, got %v", got["protocol"])
	}
	servers := got["settings"].(map[string]any)["servers"].([]any)
	server := servers[0].(map[string]any)
	if server["users"] == nil {
		t.Fatalf("expected users for auth, got none")
	}
}

func TestInjectForwardRules_SkipsOrphanAndDisabled(t *testing.T) {
	cfg := baseCfgWithInbound("in-1000-tcp")
	rules := []model.ForwardRule{
		{Id: 1, InboundTag: "does-not-exist", DestType: "socks", DestAddress: "9.9.9.9", DestPort: 1080, Enable: true},
		{Id: 2, InboundTag: "in-1000-tcp", DestType: "socks", DestAddress: "9.9.9.9", DestPort: 1080, Enable: false},
	}

	injectForwardRules(cfg, rules)

	var outbounds []map[string]any
	json.Unmarshal(cfg.OutboundConfigs, &outbounds)
	if len(outbounds) != 1 {
		t.Fatalf("orphan + disabled rules must inject nothing, got %d outbounds", len(outbounds))
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./internal/web/service/ -run TestInjectForwardRules -v`
Expected: FAIL — `undefined: injectForwardRules`

- [ ] **Step 3: 实现 injectForwardRules**

在 `internal/web/service/xray.go` 中，紧跟 `mergeSubscriptionOutbounds` 函数之后（约 413 行）追加：

```go
// injectForwardRules appends, for each enabled forward rule whose inbound exists
// in the generated config, a socks/http outbound (tag forward-out-{id}) plus a
// routing rule sending that inbound's traffic to it. Like injectPanelEgress it
// works only on the generated config — the stored template is never modified —
// and the additions are hot-appliable, so toggling a rule never restarts core.
//
// Safety: if the template's outbounds or routing section is unparsable, the
// function returns without touching the config, mirroring mergeSubscriptionOutbounds.
func injectForwardRules(cfg *xray.Config, rules []model.ForwardRule) {
	if len(rules) == 0 {
		return
	}

	// Existing inbound tags — orphan rules (inbound deleted) are skipped.
	inboundTags := make(map[string]struct{}, len(cfg.InboundConfigs))
	for i := range cfg.InboundConfigs {
		inboundTags[cfg.InboundConfigs[i].Tag] = struct{}{}
	}

	var outbounds []any
	if len(cfg.OutboundConfigs) > 0 {
		if err := json.Unmarshal(cfg.OutboundConfigs, &outbounds); err != nil {
			logger.Warning("forward rules: outbounds unparsable, skipping injection:", err)
			return
		}
	}

	routing := map[string]any{}
	if len(cfg.RouterConfig) > 0 {
		if err := json.Unmarshal(cfg.RouterConfig, &routing); err != nil {
			logger.Warning("forward rules: routing unparsable, skipping injection:", err)
			return
		}
	}
	rulesArr, _ := routing["rules"].([]any)

	added := 0
	for _, r := range rules {
		if !r.Enable {
			continue
		}
		if _, ok := inboundTags[r.InboundTag]; !ok {
			continue
		}
		protocol := "socks"
		if r.DestType == "http" {
			protocol = "http"
		}
		server := map[string]any{"address": r.DestAddress, "port": r.DestPort}
		if r.Username != "" || r.Password != "" {
			server["users"] = []any{map[string]any{"user": r.Username, "pass": r.Password}}
		}
		outTag := fmt.Sprintf("forward-out-%d", r.Id)
		outbounds = append(outbounds, map[string]any{
			"protocol": protocol,
			"tag":      outTag,
			"settings": map[string]any{"servers": []any{server}},
		})
		rulesArr = append(rulesArr, map[string]any{
			"type":        "field",
			"inboundTag":  []any{r.InboundTag},
			"outboundTag": outTag,
		})
		added++
	}
	if added == 0 {
		return
	}

	newOut, err := json.MarshalIndent(outbounds, "", "  ")
	if err != nil {
		logger.Warning("forward rules: failed to rebuild outbounds, skipping injection:", err)
		return
	}
	cfg.OutboundConfigs = json_util.RawMessage(newOut)

	routing["rules"] = rulesArr
	newRouting, err := json.Marshal(routing)
	if err != nil {
		logger.Warning("forward rules: failed to rebuild routing, skipping injection:", err)
		return
	}
	cfg.RouterConfig = json_util.RawMessage(newRouting)
}
```

> 导入检查：`xray.go` 需要 `encoding/json`、`fmt`、`logger`、`json_util`、`model`。前几个该文件已在用；若 `fmt` 或 `json_util` 未导入，编译器会报错，按提示补 import（`json_util` 路径：`github.com/mhsanaei/3x-ui/v3/internal/util/json_util`）。

- [ ] **Step 4: 在 GetXrayConfig 调用注入**

在 `internal/web/service/xray.go` 的 `GetXrayConfig()` 里，找到 `injectPanelEgress` 那段（约 283 行）之后、`return xrayConfig, nil` 之前，插入：

```go
	// Inject port-forwarding rules: each enabled rule becomes a socks/http
	// outbound + a routing rule. Source of truth is the forward_rules table;
	// the stored template is never modified.
	if forwardRules, err := (&ForwardService{}).ActiveRules(); err != nil {
		logger.Warning("read forward rules failed:", err)
	} else if len(forwardRules) > 0 {
		injectForwardRules(xrayConfig, forwardRules)
	}

	return xrayConfig, nil
```

（即把原来的 `return xrayConfig, nil` 替换为上面这一整段。）

- [ ] **Step 5: 运行测试确认通过**

Run: `go test ./internal/web/service/ -run TestInjectForwardRules -v`
Expected: PASS（三个子测试全过）

- [ ] **Step 6: 跑整个 service 包确保没破坏别的**

Run: `go test ./internal/web/service/ -run 'TestForwardRuleCRUD|TestInjectForwardRules' -v && go build ./...`
Expected: PASS + 构建无错

- [ ] **Step 7: 提交**

```bash
git add internal/web/service/xray.go internal/web/service/xray_forward_inject_test.go
git commit -m "feat(forward): inject forward rules into generated xray config

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Task 4: ForwardController + 路由注册 + SPA 路由

**Files:**
- Create: `internal/web/controller/forward.go`
- Modify: `internal/web/controller/api.go:68-92`
- Modify: `internal/web/controller/spa.go:36-43`

- [ ] **Step 1: 实现 ForwardController**

新建 `internal/web/controller/forward.go`：

```go
package controller

import (
	"strconv"

	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/web/middleware"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service"

	"github.com/gin-gonic/gin"
)

// ForwardController handles CRUD for port-forwarding rules under /panel/api/forward.
type ForwardController struct {
	forwardService service.ForwardService
	xrayService    service.XrayService
}

// NewForwardController creates a ForwardController and registers its routes.
func NewForwardController(g *gin.RouterGroup) *ForwardController {
	a := &ForwardController{}
	a.initRouter(g)
	return a
}

func (a *ForwardController) initRouter(g *gin.RouterGroup) {
	g.GET("/list", a.list)
	g.POST("/add", a.add)
	g.POST("/update/:id", a.update)
	g.POST("/del/:id", a.del)
	g.POST("/setEnable/:id", a.setEnable)
}

func (a *ForwardController) list(c *gin.Context) {
	rules, err := a.forwardService.GetAll()
	if err != nil {
		jsonMsg(c, I18nWeb(c, "pages.portForward.toasts.obtain"), err)
		return
	}
	jsonObj(c, rules, nil)
}

func (a *ForwardController) add(c *gin.Context) {
	rule, ok := middleware.BindAndValidate[model.ForwardRule](c)
	if !ok {
		return
	}
	if err := a.forwardService.Add(rule); err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	jsonMsgObj(c, I18nWeb(c, "pages.portForward.toasts.createSuccess"), rule, nil)
	a.xrayService.SetToNeedRestart()
}

func (a *ForwardController) update(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	rule := &model.ForwardRule{Id: id}
	if !middleware.BindAndValidateInto(c, rule) {
		return
	}
	rule.Id = id
	if err := a.forwardService.Update(rule); err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	jsonMsgObj(c, I18nWeb(c, "pages.portForward.toasts.updateSuccess"), rule, nil)
	a.xrayService.SetToNeedRestart()
}

func (a *ForwardController) del(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	if err := a.forwardService.Delete(id); err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	jsonMsgObj(c, I18nWeb(c, "pages.portForward.toasts.deleteSuccess"), id, nil)
	a.xrayService.SetToNeedRestart()
}

func (a *ForwardController) setEnable(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	type form struct {
		Enable bool `json:"enable" form:"enable"`
	}
	var f form
	if err := c.ShouldBind(&f); err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	if err := a.forwardService.SetEnable(id, f.Enable); err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	jsonMsg(c, I18nWeb(c, "pages.portForward.toasts.updateSuccess"), nil)
	a.xrayService.SetToNeedRestart()
}
```

> `jsonMsg` / `jsonObj` / `jsonMsgObj` / `I18nWeb` 均为 controller 包内现有 helper（见 `inbound.go` 用法），无需新建。

- [ ] **Step 2: 注册路由组**

在 `internal/web/controller/api.go` 的 `initRouter`，`a.xraySettingController = NewXraySettingController(api)` 那行之后（约 88 行）加：

```go
	// Port-forwarding rules — /panel/api/forward/*
	forward := api.Group("/forward")
	NewForwardController(forward)
```

> 仿照 `NewGroupController(clients)` 的写法，不存到结构体字段，避免改 `APIController` 定义。

- [ ] **Step 3: 注册 SPA shell 路由**

在 `internal/web/controller/spa.go` 的 `initRouter`，那串 `g.GET(...)` 页面路由里（约 36-43 行），`g.GET("/xray", a.panelSPA)` 之后加一行：

```go
	g.GET("/port-forward", a.panelSPA)
```

> 没有这行，浏览器直接刷新 `/panel/port-forward` 会 404（前端 SPA 路由需要后端把该路径也交给 index.html）。

- [ ] **Step 4: 构建确认通过**

Run: `go build ./... && go vet ./internal/web/controller/`
Expected: 无错误输出

- [ ] **Step 5: 提交**

```bash
git add internal/web/controller/forward.go internal/web/controller/api.go internal/web/controller/spa.go
git commit -m "feat(forward): add ForwardController and register routes

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Task 5: 后端文案（en-US / zh-CN）

**Files:**
- Modify: `internal/web/translation/en-US.json`
- Modify: `internal/web/translation/zh-CN.json`

- [ ] **Step 1: en-US 加菜单键**

在 `internal/web/translation/en-US.json` 的 `"menu"` 对象里，`"xray"` 一项后面加：

```json
    "portForward": "Port Forward",
```

- [ ] **Step 2: en-US 加页面键**

在 `en-US.json` 的 `"pages"` 对象里新增一个 `"portForward"` 子对象（与 `"xray"` 平级）：

```json
    "portForward": {
      "title": "Port Forward",
      "addRule": "Add Rule",
      "inbound": "Inbound",
      "destType": "Target Type",
      "destAddress": "Address",
      "destPort": "Port",
      "username": "Username",
      "password": "Password",
      "remark": "Remark",
      "enable": "Enable",
      "orphanInbound": "Inbound missing",
      "duplicateInbound": "This inbound already has a forward rule. Edit the existing one instead.",
      "toasts": {
        "obtain": "Failed to load forward rules",
        "createSuccess": "Forward rule created",
        "updateSuccess": "Forward rule updated",
        "deleteSuccess": "Forward rule deleted"
      }
    },
```

- [ ] **Step 3: zh-CN 加同样的键**

在 `internal/web/translation/zh-CN.json` 的 `"menu"` 里加：

```json
    "portForward": "端口转发",
```

在 `zh-CN.json` 的 `"pages"` 里加：

```json
    "portForward": {
      "title": "端口转发",
      "addRule": "新增规则",
      "inbound": "入站",
      "destType": "目标类型",
      "destAddress": "地址",
      "destPort": "端口",
      "username": "用户名",
      "password": "密码",
      "remark": "备注",
      "enable": "启用",
      "orphanInbound": "入站已失效",
      "duplicateInbound": "该入站已有转发规则，请直接编辑已有规则。",
      "toasts": {
        "obtain": "加载转发规则失败",
        "createSuccess": "转发规则已创建",
        "updateSuccess": "转发规则已更新",
        "deleteSuccess": "转发规则已删除"
      }
    },
```

- [ ] **Step 4: 校验 JSON 合法**

Run: `python3 -c "import json; json.load(open('internal/web/translation/en-US.json')); json.load(open('internal/web/translation/zh-CN.json')); print('ok')"`
Expected: `ok`

- [ ] **Step 5: 提交**

```bash
git add internal/web/translation/en-US.json internal/web/translation/zh-CN.json
git commit -m "feat(forward): add en-US and zh-CN translations

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Task 6: 前端类型、纯逻辑辅助、查询键、数据 hook

**Files:**
- Create: `frontend/src/pages/port-forward/types.ts`
- Create: `frontend/src/pages/port-forward/helpers.ts`
- Create: `frontend/src/pages/port-forward/helpers.test.ts`
- Modify: `frontend/src/api/queryKeys.ts`
- Create: `frontend/src/pages/port-forward/usePortForward.ts`

- [ ] **Step 1: 写失败测试（纯逻辑）**

新建 `frontend/src/pages/port-forward/helpers.test.ts`：

```ts
import { describe, expect, it } from 'vitest';

import { forwardSaveUrl, inboundAlreadyBound } from './helpers';
import type { ForwardRule } from './types';

const rule = (over: Partial<ForwardRule>): ForwardRule => ({
  id: 0, inboundTag: 'in-1000-tcp', destType: 'socks',
  destAddress: '1.2.3.4', destPort: 1080, enable: true, ...over,
});

describe('forwardSaveUrl', () => {
  it('uses add endpoint when id is absent', () => {
    expect(forwardSaveUrl(rule({ id: 0 }))).toBe('/panel/api/forward/add');
  });
  it('uses update endpoint when id is present', () => {
    expect(forwardSaveUrl(rule({ id: 5 }))).toBe('/panel/api/forward/update/5');
  });
});

describe('inboundAlreadyBound', () => {
  const existing = [rule({ id: 1, inboundTag: 'in-1000-tcp' })];
  it('flags a new rule binding an already-bound inbound', () => {
    expect(inboundAlreadyBound(existing, 'in-1000-tcp', 0)).toBe(true);
  });
  it('does not flag when editing the same rule', () => {
    expect(inboundAlreadyBound(existing, 'in-1000-tcp', 1)).toBe(false);
  });
  it('does not flag a free inbound', () => {
    expect(inboundAlreadyBound(existing, 'in-2000-tcp', 0)).toBe(false);
  });
});
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd frontend && npx vitest run src/pages/port-forward/helpers.test.ts`
Expected: FAIL — 找不到 `./helpers` / `./types`

- [ ] **Step 3: 建类型**

新建 `frontend/src/pages/port-forward/types.ts`：

```ts
export type ForwardDestType = 'socks' | 'http';

export interface ForwardRule {
  id: number;
  inboundTag: string;
  remark?: string;
  destType: ForwardDestType;
  destAddress: string;
  destPort: number;
  username?: string;
  password?: string;
  enable: boolean;
}
```

- [ ] **Step 4: 建纯逻辑辅助**

新建 `frontend/src/pages/port-forward/helpers.ts`：

```ts
import type { ForwardRule } from './types';

// forwardSaveUrl picks the add vs update endpoint based on whether the rule
// already has a persisted id.
export function forwardSaveUrl(rule: Pick<ForwardRule, 'id'>): string {
  return rule.id ? `/panel/api/forward/update/${rule.id}` : '/panel/api/forward/add';
}

// inboundAlreadyBound reports whether another rule (not editingId) already binds
// inboundTag — enforces "one forward rule per inbound" in the UI before saving.
export function inboundAlreadyBound(
  rules: ForwardRule[],
  inboundTag: string,
  editingId: number,
): boolean {
  return rules.some((r) => r.inboundTag === inboundTag && r.id !== editingId);
}
```

- [ ] **Step 5: 运行测试确认通过**

Run: `cd frontend && npx vitest run src/pages/port-forward/helpers.test.ts`
Expected: PASS

- [ ] **Step 6: 加查询键**

在 `frontend/src/api/queryKeys.ts` 的 `keys` 对象里，`xray: {...}` 之后加：

```ts
  portForward: {
    root: () => ['portForward'] as const,
    list: () => ['portForward', 'list'] as const,
  },
```

- [ ] **Step 7: 建数据 hook**

新建 `frontend/src/pages/port-forward/usePortForward.ts`：

```ts
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';

import { HttpUtil } from '@/utils';
import { keys } from '@/api/queryKeys';
import { forwardSaveUrl } from './helpers';
import type { ForwardRule } from './types';

async function fetchRules(): Promise<ForwardRule[]> {
  const msg = await HttpUtil.get('/panel/api/forward/list', undefined, { silent: true });
  if (!msg?.success) throw new Error(msg?.msg || 'Failed to fetch forward rules');
  return Array.isArray(msg.obj) ? (msg.obj as ForwardRule[]) : [];
}

export function usePortForward() {
  const qc = useQueryClient();
  const invalidate = () => qc.invalidateQueries({ queryKey: keys.portForward.root() });

  const query = useQuery({ queryKey: keys.portForward.list(), queryFn: fetchRules });

  const saveMut = useMutation({
    mutationFn: (rule: Partial<ForwardRule>) =>
      HttpUtil.post(forwardSaveUrl({ id: rule.id ?? 0 }), rule),
    onSuccess: (msg) => { if (msg?.success) invalidate(); },
  });
  const removeMut = useMutation({
    mutationFn: (id: number) => HttpUtil.post(`/panel/api/forward/del/${id}`),
    onSuccess: (msg) => { if (msg?.success) invalidate(); },
  });
  const enableMut = useMutation({
    mutationFn: ({ id, enable }: { id: number; enable: boolean }) =>
      HttpUtil.post(`/panel/api/forward/setEnable/${id}`, { enable }),
    onSuccess: (msg) => { if (msg?.success) invalidate(); },
  });

  return {
    rules: query.data ?? [],
    loading: query.isFetching,
    error: (query.error as Error | null) ?? null,
    refresh: invalidate,
    save: (rule: Partial<ForwardRule>) => saveMut.mutateAsync(rule),
    remove: (id: number) => removeMut.mutateAsync(id),
    setEnable: (id: number, enable: boolean) => enableMut.mutateAsync({ id, enable }),
  };
}
```

- [ ] **Step 8: 提交**

```bash
git add frontend/src/pages/port-forward/types.ts frontend/src/pages/port-forward/helpers.ts frontend/src/pages/port-forward/helpers.test.ts frontend/src/pages/port-forward/usePortForward.ts frontend/src/api/queryKeys.ts
git commit -m "feat(forward): frontend types, helpers, query keys, data hook

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Task 7: 表单弹窗 ForwardFormModal

**Files:**
- Create: `frontend/src/pages/port-forward/form/ForwardFormModal.tsx`

- [ ] **Step 1: 实现表单弹窗**

新建 `frontend/src/pages/port-forward/form/ForwardFormModal.tsx`：

```tsx
import { useEffect, useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { Form, Input, InputNumber, Modal, Select, message } from 'antd';

import { useInboundOptions } from '@/api/queries/useInboundOptions';
import { inboundAlreadyBound } from '../helpers';
import type { ForwardRule } from '../types';

interface Props {
  open: boolean;
  editing: ForwardRule | null;
  rules: ForwardRule[];
  onCancel: () => void;
  onSave: (rule: Partial<ForwardRule>) => Promise<unknown>;
}

const emptyValues: Partial<ForwardRule> = {
  destType: 'socks', enable: true, destPort: 1080,
};

export default function ForwardFormModal({ open, editing, rules, onCancel, onSave }: Props) {
  const { t } = useTranslation();
  const [form] = Form.useForm<ForwardRule>();
  const { data: inbounds = [] } = useInboundOptions();

  useEffect(() => {
    if (!open) return;
    form.setFieldsValue(editing ?? emptyValues);
  }, [open, editing, form]);

  const inboundOptions = useMemo(
    () =>
      inbounds
        .filter((ib) => !!ib.tag)
        .map((ib) => ({
          value: ib.tag as string,
          label: `${ib.port ?? ''} · ${ib.tag}${ib.remark ? ` (${ib.remark})` : ''}`,
        })),
    [inbounds],
  );

  const handleOk = async () => {
    const values = await form.validateFields();
    const editingId = editing?.id ?? 0;
    if (inboundAlreadyBound(rules, values.inboundTag, editingId)) {
      message.error(t('pages.portForward.duplicateInbound'));
      return;
    }
    await onSave({ ...editing, ...values });
    onCancel();
  };

  return (
    <Modal
      open={open}
      title={editing ? t('edit') : t('pages.portForward.addRule')}
      onOk={handleOk}
      onCancel={onCancel}
      destroyOnHidden
    >
      <Form form={form} layout="vertical" initialValues={emptyValues}>
        <Form.Item name="inboundTag" label={t('pages.portForward.inbound')} rules={[{ required: true }]}>
          <Select options={inboundOptions} showSearch optionFilterProp="label" />
        </Form.Item>
        <Form.Item name="destType" label={t('pages.portForward.destType')} rules={[{ required: true }]}>
          <Select
            options={[
              { value: 'socks', label: 'SOCKS5' },
              { value: 'http', label: 'HTTP' },
            ]}
          />
        </Form.Item>
        <Form.Item name="destAddress" label={t('pages.portForward.destAddress')} rules={[{ required: true }]}>
          <Input placeholder="1.2.3.4" />
        </Form.Item>
        <Form.Item name="destPort" label={t('pages.portForward.destPort')} rules={[{ required: true }]}>
          <InputNumber min={1} max={65535} style={{ width: '100%' }} />
        </Form.Item>
        <Form.Item name="username" label={t('pages.portForward.username')}>
          <Input autoComplete="off" />
        </Form.Item>
        <Form.Item name="password" label={t('pages.portForward.password')}>
          <Input.Password autoComplete="new-password" />
        </Form.Item>
        <Form.Item name="remark" label={t('pages.portForward.remark')}>
          <Input />
        </Form.Item>
      </Form>
    </Modal>
  );
}
```

> `useInboundOptions()`（`@/api/queries/useInboundOptions`）已存在，返回的每项含 `tag/port/remark/protocol` —— 直接用作入站下拉数据源，下拉显示所有入站、不过滤。`t('edit')`、`t('delete')` 为仓库现有顶层通用键（en-US.json 已含 `"edit": "Edit"`、`"delete": "Delete"`），直接用。

- [ ] **Step 2: 类型检查**

Run: `cd frontend && npx tsc --noEmit`
Expected: 无 `port-forward/form/ForwardFormModal.tsx` 相关报错

- [ ] **Step 3: 提交**

```bash
git add frontend/src/pages/port-forward/form/ForwardFormModal.tsx
git commit -m "feat(forward): add ForwardFormModal

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Task 8: 列表表格 ForwardList

**Files:**
- Create: `frontend/src/pages/port-forward/list/ForwardList.tsx`

- [ ] **Step 1: 实现列表**

新建 `frontend/src/pages/port-forward/list/ForwardList.tsx`：

```tsx
import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { Button, Popconfirm, Space, Switch, Table, Tag } from 'antd';
import type { ColumnsType } from 'antd/es/table';

import { useInboundOptions } from '@/api/queries/useInboundOptions';
import type { ForwardRule } from '../types';

interface Props {
  rules: ForwardRule[];
  loading: boolean;
  onEdit: (rule: ForwardRule) => void;
  onDelete: (id: number) => void;
  onToggle: (id: number, enable: boolean) => void;
}

export default function ForwardList({ rules, loading, onEdit, onDelete, onToggle }: Props) {
  const { t } = useTranslation();
  const { data: inbounds = [] } = useInboundOptions();
  const knownTags = useMemo(() => new Set(inbounds.map((ib) => ib.tag).filter(Boolean)), [inbounds]);

  const columns: ColumnsType<ForwardRule> = [
    {
      title: t('pages.portForward.inbound'),
      dataIndex: 'inboundTag',
      render: (tag: string) =>
        knownTags.has(tag) ? (
          <span>{tag}</span>
        ) : (
          <Space>
            <span>{tag}</span>
            <Tag color="error">{t('pages.portForward.orphanInbound')}</Tag>
          </Space>
        ),
    },
    {
      title: t('pages.portForward.destType'),
      dataIndex: 'destType',
      render: (v: string) => (v === 'http' ? 'HTTP' : 'SOCKS5'),
    },
    {
      title: t('pages.portForward.destAddress'),
      render: (_, r) => `${r.destAddress}:${r.destPort}`,
    },
    { title: t('pages.portForward.remark'), dataIndex: 'remark' },
    {
      title: t('pages.portForward.enable'),
      dataIndex: 'enable',
      render: (enable: boolean, r) => (
        <Switch checked={enable} onChange={(v) => onToggle(r.id, v)} />
      ),
    },
    {
      title: '',
      key: 'actions',
      render: (_, r) => (
        <Space>
          <Button size="small" onClick={() => onEdit(r)}>{t('edit')}</Button>
          <Popconfirm title={t('delete')} onConfirm={() => onDelete(r.id)}>
            <Button size="small" danger>{t('delete')}</Button>
          </Popconfirm>
        </Space>
      ),
    },
  ];

  return (
    <Table
      rowKey="id"
      loading={loading}
      columns={columns}
      dataSource={rules}
      pagination={false}
    />
  );
}
```

> `t('edit')` / `t('delete')` 为现有通用键。`ColumnsType` 从 `antd/es/table` 引入，与仓库其它表格一致（若仓库用别的导入路径，按其它 list 组件的写法对齐）。

- [ ] **Step 2: 类型检查**

Run: `cd frontend && npx tsc --noEmit`
Expected: 无 `ForwardList.tsx` 相关报错

- [ ] **Step 3: 提交**

```bash
git add frontend/src/pages/port-forward/list/ForwardList.tsx
git commit -m "feat(forward): add ForwardList table with orphan marker

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Task 9: 页面容器 PortForwardPage

**Files:**
- Create: `frontend/src/pages/port-forward/PortForwardPage.tsx`

- [ ] **Step 1: 实现页面容器**

新建 `frontend/src/pages/port-forward/PortForwardPage.tsx`：

```tsx
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Button, Card, Space } from 'antd';
import { PlusOutlined } from '@ant-design/icons';

import { usePortForward } from './usePortForward';
import ForwardList from './list/ForwardList';
import ForwardFormModal from './form/ForwardFormModal';
import type { ForwardRule } from './types';

export default function PortForwardPage() {
  const { t } = useTranslation();
  const { rules, loading, save, remove, setEnable } = usePortForward();
  const [modalOpen, setModalOpen] = useState(false);
  const [editing, setEditing] = useState<ForwardRule | null>(null);

  const openAdd = () => { setEditing(null); setModalOpen(true); };
  const openEdit = (rule: ForwardRule) => { setEditing(rule); setModalOpen(true); };

  return (
    <Card
      title={t('pages.portForward.title')}
      extra={
        <Space>
          <Button type="primary" icon={<PlusOutlined />} onClick={openAdd}>
            {t('pages.portForward.addRule')}
          </Button>
        </Space>
      }
    >
      <ForwardList
        rules={rules}
        loading={loading}
        onEdit={openEdit}
        onDelete={(id) => remove(id)}
        onToggle={(id, enable) => setEnable(id, enable)}
      />
      <ForwardFormModal
        open={modalOpen}
        editing={editing}
        rules={rules}
        onCancel={() => setModalOpen(false)}
        onSave={save}
      />
    </Card>
  );
}
```

- [ ] **Step 2: 类型检查**

Run: `cd frontend && npx tsc --noEmit`
Expected: 无 `PortForwardPage.tsx` 相关报错

- [ ] **Step 3: 提交**

```bash
git add frontend/src/pages/port-forward/PortForwardPage.tsx
git commit -m "feat(forward): add PortForwardPage container

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Task 10: 接线（路由 + 侧边栏菜单）

**Files:**
- Modify: `frontend/src/routes.tsx`
- Modify: `frontend/src/layouts/AppSidebar.tsx`

- [ ] **Step 1: 加路由**

在 `frontend/src/routes.tsx`：

懒加载声明区（其它 `const XxxPage = lazy(...)` 旁）加：

```tsx
const PortForwardPage = lazy(() => import('@/pages/port-forward/PortForwardPage'));
```

`children` 数组里（`{ path: 'xray', ... }` 之后）加：

```tsx
      { path: 'port-forward', element: withSuspense(<PortForwardPage />) },
```

- [ ] **Step 2: 加菜单图标类型**

在 `frontend/src/layouts/AppSidebar.tsx`：

`IconName` 联合类型末尾加 `| 'forward'`：

```tsx
type IconName = 'dashboard' | 'inbound' | 'team' | 'groups' | 'setting' | 'tool' | 'cluster' | 'logout' | 'apidocs' | 'outbound' | 'forward';
```

`iconByName` map 里加（`SwapOutlined` 文件顶部已 import，直接用）：

```tsx
  forward: SwapOutlined,
```

- [ ] **Step 3: 加菜单项**

在 `AppSidebar.tsx` 的 `tabs` 数组里，`{ key: '/inbounds', ... }` 之后插入：

```tsx
    { key: '/port-forward', icon: 'forward', title: t('menu.portForward') },
```

- [ ] **Step 4: 类型检查 + 构建**

Run: `cd frontend && npx tsc --noEmit && npm run build`
Expected: 构建成功，无类型错误

- [ ] **Step 5: 提交**

```bash
git add frontend/src/routes.tsx frontend/src/layouts/AppSidebar.tsx
git commit -m "feat(forward): wire route and sidebar menu entry

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Task 11: 全量验证 + 手动冒烟

**Files:** 无（验证）

- [ ] **Step 1: 后端全测 + 构建**

Run: `go build ./... && go test ./internal/web/service/ -run 'Forward' -v`
Expected: 构建无错；`TestForwardRuleCRUD` 与三个 `TestInjectForwardRules` 全 PASS

- [ ] **Step 2: 前端测试 + 构建 + lint**

Run: `cd frontend && npx vitest run src/pages/port-forward/ && npm run build && npx eslint src/pages/port-forward/`
Expected: 测试 PASS、构建成功、lint 无错

- [ ] **Step 3: 手动冒烟（启动面板）**

Run（按仓库 README 的本地启动方式，通常）：`go run main.go`
然后浏览器登录面板：
1. 左侧菜单出现「端口转发 / Port Forward」入口。
2. 点「新增规则」→ 入站下拉能选到已建的 vless/vmess 入站 → 目标类型选 SOCKS5 → 填 socks5 地址端口 → 保存。
3. 列表出现该规则，启用开关可切换。
4. 用 `x-ui` 命令或面板的 Xray 配置查看，确认生成配置里多了 `forward-out-{id}` 出站和对应 `inboundTag → forward-out-{id}` 路由规则；存储的 `xrayTemplateConfig` 未被改动。
5. 删除该入站后回到本页，规则行显示「入站已失效」标记，且 Xray 仍能正常启动（孤儿规则被跳过）。

Expected: 以上全部符合。

- [ ] **Step 4: 最终提交（若冒烟中有微调）**

```bash
git add -A
git commit -m "test(forward): manual smoke verification adjustments

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Self-Review 备注（已核对）

- **Spec 覆盖**：数据模型(Task1) / Service(Task2) / 注入(Task3) / Controller+路由+SPA(Task4) / 文案(Task5) / 前端 hook(Task6) / 表单(Task7) / 列表(Task8) / 页面(Task9) / 接线(Task10) / 验证(Task11) —— spec 每节都有对应任务。
- **socks5 + http**：`DestType` 贯穿 model → 注入 protocol → 表单下拉 → 列表显示。
- **一入站一规则**：DB 唯一约束（Task1 model）+ 前端 `inboundAlreadyBound`（Task6/7）。
- **孤儿规则**：注入跳过（Task3 测试覆盖）+ 列表标记（Task8）。
- **不污染模板**：注入只改生成配置（Task3 注释与设计一致）。
- **类型一致**：`ForwardRule` 字段在 Go model、TS type、表单、列表、注入间命名统一（inboundTag/destType/destAddress/destPort/username/password/remark/enable）。
- **占位符扫描**：无 TBD/TODO；每个代码步骤含完整代码。
