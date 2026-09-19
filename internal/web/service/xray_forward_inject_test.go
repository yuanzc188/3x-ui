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

func parseInjected(t *testing.T, cfg *xray.Config) (outbounds []map[string]any, rules []map[string]any) {
	t.Helper()
	if err := json.Unmarshal(cfg.OutboundConfigs, &outbounds); err != nil {
		t.Fatalf("outbounds unparsable: %v", err)
	}
	var routing struct {
		Rules []map[string]any `json:"rules"`
	}
	if err := json.Unmarshal(cfg.RouterConfig, &routing); err != nil {
		t.Fatalf("routing unparsable: %v", err)
	}
	return outbounds, routing.Rules
}

func TestInjectForwardRules_AddsOutboundAndRoute(t *testing.T) {
	cfg := baseCfgWithInbound("in-1000-tcp")
	rules := []model.ForwardRule{{
		Id: 7, InboundTag: "in-1000-tcp", DestType: "socks",
		DestAddress: "9.9.9.9", DestPort: 1080, Enable: true,
	}}

	injectForwardRules(cfg, rules, nil)

	outbounds, routes := parseInjected(t, cfg)
	// direct + forward-out-7 + forward-block
	if len(outbounds) != 3 || outbounds[1]["protocol"] != "socks" || outbounds[1]["tag"] != "forward-out-7" || outbounds[2]["tag"] != forwardBlockTag {
		t.Fatalf("unexpected outbounds: %+v", outbounds)
	}
	// api + udp→block + all→proxy
	if len(routes) != 3 {
		t.Fatalf("expected 3 routing rules, got %d: %+v", len(routes), routes)
	}
	if routes[1]["network"] != "udp" || routes[1]["outboundTag"] != forwardBlockTag {
		t.Fatalf("udp block rule must come first: %+v", routes[1])
	}
	if routes[2]["outboundTag"] != "forward-out-7" || routes[2]["domain"] != nil {
		t.Fatalf("catch-all proxy rule wrong: %+v", routes[2])
	}
}

func TestInjectForwardRules_HttpProtocolAndAuth(t *testing.T) {
	cfg := baseCfgWithInbound("in-1000-tcp")
	rules := []model.ForwardRule{{
		Id: 1, InboundTag: "in-1000-tcp", DestType: "http",
		DestAddress: "1.1.1.1", DestPort: 8080,
		Username: "u", Password: "p", Enable: true,
	}}

	injectForwardRules(cfg, rules, nil)

	outbounds, _ := parseInjected(t, cfg)
	got := outbounds[1]
	if got["protocol"] != "http" {
		t.Fatalf("expected http protocol, got %v", got["protocol"])
	}
	servers := got["settings"].(map[string]any)["servers"].([]any)
	users := servers[0].(map[string]any)["users"].([]any)
	u := users[0].(map[string]any)
	if u["user"] != "u" || u["pass"] != "p" {
		t.Fatalf("auth not injected: %+v", u)
	}
}

func TestInjectForwardRules_WhitelistRoutes(t *testing.T) {
	global := []string{"domain:g.com"}
	cases := []struct {
		name       string
		rule       model.ForwardRule
		wantDomain []any // nil = no whitelist expected
	}{
		{"switch off", model.ForwardRule{DomainLimit: false, Domains: "domain:x.com"}, nil},
		{"on, own list wins", model.ForwardRule{DomainLimit: true, Domains: "domain:a.com\ndomain:b.com"}, []any{"domain:a.com", "domain:b.com"}},
		{"on, empty own → global", model.ForwardRule{DomainLimit: true}, []any{"domain:g.com"}},
	}
	for _, c := range cases {
		cfg := baseCfgWithInbound("in-x")
		r := c.rule
		r.Id, r.InboundTag, r.DestType, r.DestAddress, r.DestPort, r.Enable = 3, "in-x", "socks", "1.1.1.1", 1080, true
		injectForwardRules(cfg, []model.ForwardRule{r}, global)
		_, routes := parseInjected(t, cfg)
		if c.wantDomain == nil {
			if len(routes) != 3 || routes[2]["outboundTag"] != "forward-out-3" {
				t.Errorf("%s: expected plain proxy route, got %+v", c.name, routes)
			}
			continue
		}
		if len(routes) != 4 {
			t.Errorf("%s: expected 4 routes, got %d: %+v", c.name, len(routes), routes)
			continue
		}
		wl := routes[2]
		if wl["outboundTag"] != "forward-out-3" {
			t.Errorf("%s: whitelist route wrong: %+v", c.name, wl)
		}
		gotDomains, _ := wl["domain"].([]any)
		if len(gotDomains) != len(c.wantDomain) {
			t.Errorf("%s: domains %v want %v", c.name, gotDomains, c.wantDomain)
		}
		for i := range c.wantDomain {
			if gotDomains[i] != c.wantDomain[i] {
				t.Errorf("%s: domains %v want %v", c.name, gotDomains, c.wantDomain)
			}
		}
		if routes[3]["outboundTag"] != forwardBlockTag || routes[3]["domain"] != nil {
			t.Errorf("%s: trailing rule must block everything else: %+v", c.name, routes[3])
		}
	}
}

func TestInjectForwardRules_BlackholeOnce(t *testing.T) {
	cfg := baseCfgWithInbound("in-a")
	cfg.InboundConfigs = append(cfg.InboundConfigs, xray.InboundConfig{Port: 1001, Protocol: "vless", Tag: "in-b"})
	rules := []model.ForwardRule{
		{Id: 1, InboundTag: "in-a", DestType: "socks", DestAddress: "1.1.1.1", DestPort: 1, Enable: true},
		{Id: 2, InboundTag: "in-b", DestType: "socks", DestAddress: "1.1.1.2", DestPort: 1, Enable: true},
	}
	injectForwardRules(cfg, rules, nil)
	outbounds, _ := parseInjected(t, cfg)
	count := 0
	for _, ob := range outbounds {
		if ob["tag"] == forwardBlockTag {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("expected exactly one blackhole outbound, got %d", count)
	}

	// Template already has the tag → not duplicated.
	cfg2 := baseCfgWithInbound("in-a")
	cfg2.OutboundConfigs = json_util.RawMessage(`[{"protocol":"freedom","tag":"direct"},{"protocol":"blackhole","tag":"forward-block"}]`)
	injectForwardRules(cfg2, rules[:1], nil)
	outbounds2, _ := parseInjected(t, cfg2)
	if len(outbounds2) != 3 {
		t.Fatalf("blackhole duplicated: %+v", outbounds2)
	}
}

func TestInjectForwardRules_SkipsOrphanAndDisabled(t *testing.T) {
	cfg := baseCfgWithInbound("in-exists")
	rules := []model.ForwardRule{
		{Id: 1, InboundTag: "in-gone", DestType: "socks", DestAddress: "1.1.1.1", DestPort: 1, Enable: true},
		{Id: 2, InboundTag: "in-exists", DestType: "socks", DestAddress: "1.1.1.1", DestPort: 1, Enable: false},
	}
	before := string(cfg.OutboundConfigs)
	injectForwardRules(cfg, rules, nil)
	if string(cfg.OutboundConfigs) != before {
		t.Fatalf("orphan/disabled rules must not inject anything")
	}
}

func TestInjectForwardRules_LeavesCfgUntouchedOnParseError(t *testing.T) {
	cfg := baseCfgWithInbound("in-1000-tcp")
	cfg.RouterConfig = json_util.RawMessage(`{not json`)
	before := string(cfg.OutboundConfigs)
	rules := []model.ForwardRule{{Id: 1, InboundTag: "in-1000-tcp", DestType: "socks", DestAddress: "1.1.1.1", DestPort: 1, Enable: true}}
	injectForwardRules(cfg, rules, nil)
	if string(cfg.OutboundConfigs) != before {
		t.Fatalf("outbounds must stay untouched when routing is unparsable")
	}
}
