package service

import (
	"reflect"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

func TestSplitDomains(t *testing.T) {
	got := splitDomains("  domain:tiktok.com \n\ngeosite:tiktok\r\ndomain:tiktok.com\n")
	want := []string{"domain:tiktok.com", "geosite:tiktok"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
	if splitDomains("") != nil {
		t.Fatal("empty input must yield nil")
	}
}

func TestEffectiveDomains(t *testing.T) {
	global := []string{"domain:g.com"}
	cases := []struct {
		name   string
		rule   model.ForwardRule
		global []string
		want   []string
	}{
		{"switch off ignores everything", model.ForwardRule{DomainLimit: false, Domains: "domain:x.com"}, global, nil},
		{"on, empty own, empty global → unrestricted", model.ForwardRule{DomainLimit: true}, nil, nil},
		{"on, empty own → global", model.ForwardRule{DomainLimit: true, Domains: " \n"}, global, global},
		{"on, own overrides global", model.ForwardRule{DomainLimit: true, Domains: "domain:a.com\ndomain:b.com"}, global, []string{"domain:a.com", "domain:b.com"}},
	}
	for _, c := range cases {
		if got := effectiveDomains(c.rule, c.global); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: got %v want %v", c.name, got, c.want)
		}
	}
}
