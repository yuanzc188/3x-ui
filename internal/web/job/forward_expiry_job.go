package job

import (
	"strings"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/logger"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service/tgbot"
)

// forwardExpiryWarn is how far ahead of the provider-side expiry the daily
// reminder starts firing; already-expired proxies are always included.
const forwardExpiryWarn = 3 * 24 * time.Hour

// ForwardExpiryJob sends one daily Telegram digest of forward proxies whose
// provider-side expiry is near or past.
type ForwardExpiryJob struct {
	forwardService service.ForwardService
	tgbotService   tgbot.Tgbot
}

func NewForwardExpiryJob() *ForwardExpiryJob {
	return new(ForwardExpiryJob)
}

func (j *ForwardExpiryJob) Run() {
	rules, err := j.forwardService.GetAll()
	if err != nil {
		logger.Warning("forward expiry job:", err)
		return
	}
	msg := forwardExpiryDigest(rules, time.Now())
	if msg == "" {
		return
	}
	logger.Info("forward expiry:", msg)
	j.tgbotService.SendMsgToTgbotAdmins(msg)
}

// forwardExpiryDigest lists rules expiring within forwardExpiryWarn (or
// already expired); "" when there is nothing to report.
func forwardExpiryDigest(rules []model.ForwardRule, now time.Time) string {
	var lines []string
	for _, r := range rules {
		if r.ExpiryTime <= 0 {
			continue
		}
		exp := time.UnixMilli(r.ExpiryTime)
		if exp.Sub(now) >= forwardExpiryWarn {
			continue
		}
		state := "即将到期"
		if exp.Before(now) {
			state = "已到期"
		}
		lines = append(lines, "- "+forwardRuleName(r)+": "+exp.Format("2006-01-02")+" "+state)
	}
	if len(lines) == 0 {
		return ""
	}
	return "📅 [转发] 代理供应商到期提醒:\n" + strings.Join(lines, "\n")
}
