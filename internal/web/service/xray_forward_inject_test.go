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

func TestInjectForwardRules_LeavesCfgUntouchedOnParseError(t *testing.T) {
	cfg := baseCfgWithInbound("in-1000-tcp")
	cfg.OutboundConfigs = json_util.RawMessage(`this is not valid json`)
	origOut := string(cfg.OutboundConfigs)
	origRouting := string(cfg.RouterConfig)

	rules := []model.ForwardRule{{
		Id: 1, InboundTag: "in-1000-tcp", DestType: "socks",
		DestAddress: "9.9.9.9", DestPort: 1080, Enable: true,
	}}

	injectForwardRules(cfg, rules)

	if string(cfg.OutboundConfigs) != origOut {
		t.Fatalf("OutboundConfigs must be untouched on parse error, got %q", string(cfg.OutboundConfigs))
	}
	if string(cfg.RouterConfig) != origRouting {
		t.Fatalf("RouterConfig must be untouched on parse error, got %q", string(cfg.RouterConfig))
	}
}
