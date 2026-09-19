package job

import (
	"fmt"

	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/logger"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service/tgbot"
)

// ForwardCheckJob probes every enabled forward proxy and tells the Telegram
// admins about state transitions (down / recovered / egress IP changed).
type ForwardCheckJob struct {
	forwardService service.ForwardService
	tgbotService   tgbot.Tgbot
}

func NewForwardCheckJob() *ForwardCheckJob {
	return new(ForwardCheckJob)
}

func (j *ForwardCheckJob) Run() {
	events, err := j.forwardService.CheckAll()
	if err != nil {
		logger.Warning("forward check job:", err)
		return
	}
	for _, ev := range events {
		msg := forwardEventMessage(ev)
		logger.Info("forward check:", msg)
		j.tgbotService.SendMsgToTgbotAdmins(msg)
	}
}

func forwardRuleName(r model.ForwardRule) string {
	if r.Remark != "" {
		return r.Remark
	}
	return r.InboundTag
}

func forwardEventMessage(ev service.CheckEvent) string {
	name := forwardRuleName(ev.Rule)
	switch ev.Kind {
	case service.CheckEventDown:
		return fmt.Sprintf("⚠️ [转发] %s 检测失败: %s", name, ev.Rule.CheckErr)
	case service.CheckEventUp:
		return fmt.Sprintf("✅ [转发] %s 已恢复, 出口 %s", name, ev.Rule.CheckIP)
	case service.CheckEventIPChanged:
		return fmt.Sprintf("🔄 [转发] %s 出口 IP 变化, 现在 %s (%s)", name, ev.Rule.CheckIP, ev.Rule.CheckGeo)
	}
	return fmt.Sprintf("[转发] %s: %s", name, ev.Kind)
}
