package job

import (
	"strings"
	"testing"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service"
)

func TestForwardExpiryDigest(t *testing.T) {
	now := time.Date(2026, 9, 19, 12, 0, 0, 0, time.UTC)
	day := 24 * time.Hour
	rules := []model.ForwardRule{
		{Remark: "unset"}, // 0 → skipped
		{Remark: "far", ExpiryTime: now.Add(10 * day).UnixMilli()},      // skipped
		{Remark: "edge", ExpiryTime: now.Add(3 * day).UnixMilli()},      // exactly 3d → skipped
		{Remark: "soon", ExpiryTime: now.Add(2 * day).UnixMilli()},      // included
		{InboundTag: "in-9", ExpiryTime: now.Add(-1 * day).UnixMilli()}, // expired, name falls back to tag
	}
	got := forwardExpiryDigest(rules, now)
	for _, want := range []string{"soon", "in-9", "已到期", "即将到期"} {
		if !strings.Contains(got, want) {
			t.Errorf("digest missing %q:\n%s", want, got)
		}
	}
	for _, absent := range []string{"unset", "far", "edge"} {
		if strings.Contains(got, absent) {
			t.Errorf("digest must not contain %q:\n%s", absent, got)
		}
	}
	if forwardExpiryDigest(rules[:3], now) != "" {
		t.Fatal("nothing due → empty digest")
	}
}

func TestForwardEventMessage(t *testing.T) {
	r := model.ForwardRule{InboundTag: "in-1", CheckIP: "1.2.3.4", CheckGeo: "US / LA", CheckErr: "timeout"}
	if m := forwardEventMessage(service.CheckEvent{Rule: r, Kind: service.CheckEventDown}); !strings.Contains(m, "in-1") || !strings.Contains(m, "timeout") {
		t.Fatalf("down message: %s", m)
	}
	r.Remark = "客户A"
	if m := forwardEventMessage(service.CheckEvent{Rule: r, Kind: service.CheckEventIPChanged}); !strings.Contains(m, "客户A") || !strings.Contains(m, "1.2.3.4") {
		t.Fatalf("ip changed message: %s", m)
	}
}
