package service

import (
	"encoding/json"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

// End-to-end through GetXrayConfig: rules + global setting in the DB come out
// as the expected outbounds and ordered routing rules in the generated config.
func TestGetXrayConfig_ForwardRulesEndToEnd(t *testing.T) {
	setupConflictDB(t)
	seedInboundConflict(t, "in-a", "", 44401, model.VLESS, `{"network":"tcp","security":"none"}`, `{"clients":[]}`)
	seedInboundConflict(t, "in-b", "", 44402, model.VLESS, `{"network":"tcp","security":"none"}`, `{"clients":[]}`)

	fwd := &ForwardService{}
	if err := fwd.SaveSettings(ForwardSettings{GlobalDomains: "domain:g.com\n\ndomain:g.com"}); err != nil {
		t.Fatalf("SaveSettings: %v", err)
	}
	rules := []model.ForwardRule{
		// whitelist on, no own list → global
		{InboundTag: "in-a", DestType: "socks", DestAddress: "10.0.0.1", DestPort: 1080, Username: "u", Password: "p", Enable: true, DomainLimit: true},
		// whitelist off → plain proxy
		{InboundTag: "in-b", DestType: "http", DestAddress: "10.0.0.2", DestPort: 8080, Enable: true},
		// disabled → ignored
		{InboundTag: "in-a-dup", DestType: "socks", DestAddress: "10.0.0.3", DestPort: 1, Enable: false},
	}
	for i := range rules {
		if err := fwd.Add(&rules[i]); err != nil {
			t.Fatalf("Add %s: %v", rules[i].InboundTag, err)
		}
	}

	cfg, err := (&XrayService{}).GetXrayConfig()
	if err != nil {
		t.Fatalf("GetXrayConfig: %v", err)
	}
	var outbounds []map[string]any
	if err := json.Unmarshal(cfg.OutboundConfigs, &outbounds); err != nil {
		t.Fatalf("outbounds: %v", err)
	}
	tags := map[string]map[string]any{}
	for _, ob := range outbounds {
		if tag, _ := ob["tag"].(string); tag != "" {
			tags[tag] = ob
		}
	}
	outA, outB := forwardOutTag(rules[0].Id), forwardOutTag(rules[1].Id)
	if tags[outA] == nil || tags[outB] == nil || tags[forwardBlockTag] == nil {
		t.Fatalf("missing injected outbounds, have %v", keysOf(tags))
	}
	if tags[outB]["protocol"] != "http" || tags[forwardBlockTag]["protocol"] != "blackhole" {
		t.Fatalf("wrong protocols: %v / %v", tags[outB]["protocol"], tags[forwardBlockTag]["protocol"])
	}

	var routing struct {
		Rules []map[string]any `json:"rules"`
	}
	if err := json.Unmarshal(cfg.RouterConfig, &routing); err != nil {
		t.Fatalf("routing: %v", err)
	}
	// Collect the injected rules per inbound, in order.
	perInbound := map[string][]map[string]any{}
	for _, r := range routing.Rules {
		ins, _ := r["inboundTag"].([]any)
		if len(ins) == 1 {
			if tag, _ := ins[0].(string); tag == "in-a" || tag == "in-b" {
				perInbound[tag] = append(perInbound[tag], r)
			}
		}
	}
	a := perInbound["in-a"]
	if len(a) != 3 || a[0]["network"] != "udp" || a[0]["outboundTag"] != forwardBlockTag ||
		a[1]["outboundTag"] != outA || a[2]["outboundTag"] != forwardBlockTag {
		t.Fatalf("in-a routes wrong: %+v", a)
	}
	if doms, _ := a[1]["domain"].([]any); len(doms) != 1 || doms[0] != "domain:g.com" {
		t.Fatalf("in-a whitelist should be the de-duplicated global list, got %v", a[1]["domain"])
	}
	b := perInbound["in-b"]
	if len(b) != 2 || b[0]["network"] != "udp" || b[1]["outboundTag"] != outB || b[1]["domain"] != nil {
		t.Fatalf("in-b routes wrong: %+v", b)
	}
}

func keysOf(m map[string]map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
