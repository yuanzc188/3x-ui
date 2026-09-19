package service

import (
	"strings"

	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

// splitDomains turns a newline-separated list into trimmed, de-duplicated
// entries in their original order. Entries are Xray domain matchers and are
// passed through verbatim (domain:, geosite:, regexp:, plain substrings).
func splitDomains(raw string) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, line := range strings.Split(raw, "\n") {
		d := strings.TrimSpace(line)
		if d == "" {
			continue
		}
		if _, dup := seen[d]; dup {
			continue
		}
		seen[d] = struct{}{}
		out = append(out, d)
	}
	return out
}

// effectiveDomains resolves the rule's three-state whitelist: switch off →
// nil (unrestricted); on with its own list → that list; on without → global.
// A nil/empty result always means "no whitelist".
func effectiveDomains(rule model.ForwardRule, global []string) []string {
	if !rule.DomainLimit {
		return nil
	}
	if own := splitDomains(rule.Domains); len(own) > 0 {
		return own
	}
	return global
}
