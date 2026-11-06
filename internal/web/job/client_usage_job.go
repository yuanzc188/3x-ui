package job

import (
	"github.com/mhsanaei/3x-ui/v3/internal/logger"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service"
)

// ClientUsageJob copies the access log lines written since its last run into the
// per-client usage files and drops days past the retention window.
type ClientUsageJob struct {
	usageService service.ClientUsageService
}

func NewClientUsageJob() *ClientUsageJob {
	return new(ClientUsageJob)
}

func (j *ClientUsageJob) Run() {
	if err := j.usageService.Collect(); err != nil {
		logger.Warning("client usage job:", err)
	}
}
