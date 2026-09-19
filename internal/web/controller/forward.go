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
	g.POST("/check/:id", a.check)
	g.GET("/settings", a.getSettings)
	g.POST("/settings", a.saveSettings)
}

func (a *ForwardController) list(c *gin.Context) {
	rules, err := a.forwardService.GetAllWithStatus()
	if err != nil {
		jsonMsg(c, I18nWeb(c, "pages.portForward.toasts.obtain"), err)
		return
	}
	jsonObj(c, rules, nil)
}

// check probes one rule right now and returns it with fresh Check* fields.
func (a *ForwardController) check(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	rule, err := a.forwardService.GetByID(id)
	if err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	fs, err := a.forwardService.GetSettings()
	if err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	checkURL, err := service.SanitizePublicHTTPURL(fs.CheckUrl, false)
	if err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	a.forwardService.CheckOne(rule, checkURL)
	jsonMsgObj(c, I18nWeb(c, "pages.portForward.toasts.checkDone"), rule, nil)
}

func (a *ForwardController) getSettings(c *gin.Context) {
	fs, err := a.forwardService.GetSettings()
	if err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	jsonObj(c, fs, nil)
}

// saveSettings persists both knobs; a changed global whitelist alters the
// generated routing, so xray is flagged for a (hot) reload.
func (a *ForwardController) saveSettings(c *gin.Context) {
	fs, ok := middleware.BindAndValidate[service.ForwardSettings](c)
	if !ok {
		return
	}
	before, err := a.forwardService.GetSettings()
	if err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	if err := a.forwardService.SaveSettings(*fs); err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	jsonMsg(c, I18nWeb(c, "pages.portForward.toasts.settingsSaved"), nil)
	if after, err := a.forwardService.GetSettings(); err == nil && after.GlobalDomains != before.GlobalDomains {
		a.xrayService.SetToNeedRestart()
	}
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
