package service

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

// fakeHTTPProxy is an HTTP forward proxy: it answers every proxied request
// itself instead of dialing upstream, so probeProxy can be exercised offline.
func fakeHTTPProxy(t *testing.T, body string, status int) model.ForwardRule {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !r.URL.IsAbs() {
			t.Errorf("expected an absolute proxied URL, got %q", r.URL)
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	host, port, _ := strings.Cut(strings.TrimPrefix(srv.URL, "http://"), ":")
	var p int
	for _, c := range port {
		p = p*10 + int(c-'0')
	}
	return model.ForwardRule{Id: 1, InboundTag: "in", DestType: "http", DestAddress: host, DestPort: p, Enable: true}
}

const checkTarget = "http://check.invalid/json"

func TestProbeProxy_JSON(t *testing.T) {
	r := fakeHTTPProxy(t, `{"query":"203.0.113.5","country":"United States","city":"Los Angeles"}`, 200)
	ip, geo, ms, err := probeProxy(r, checkTarget)
	if err != nil {
		t.Fatalf("probe: %v", err)
	}
	if ip != "203.0.113.5" || geo != "United States / Los Angeles" || ms < 0 {
		t.Fatalf("got ip=%q geo=%q ms=%d", ip, geo, ms)
	}
}

func TestProbeProxy_PlainText(t *testing.T) {
	r := fakeHTTPProxy(t, "  198.51.100.9\n", 200)
	ip, geo, _, err := probeProxy(r, checkTarget)
	if err != nil || ip != "198.51.100.9" || geo != "" {
		t.Fatalf("got ip=%q geo=%q err=%v", ip, geo, err)
	}
}

func TestProbeProxy_Failures(t *testing.T) {
	if _, _, _, err := probeProxy(fakeHTTPProxy(t, "not an ip", 200), checkTarget); err == nil {
		t.Fatal("garbage body must fail")
	}
	if _, _, _, err := probeProxy(fakeHTTPProxy(t, "", 502), checkTarget); err == nil {
		t.Fatal("non-200 must fail")
	}
	dead := model.ForwardRule{DestType: "socks", DestAddress: "127.0.0.1", DestPort: 1}
	if _, _, _, err := probeProxy(dead, checkTarget); err == nil {
		t.Fatal("unreachable proxy must fail")
	}
}

func TestForwardProxyURL(t *testing.T) {
	u := forwardProxyURL(model.ForwardRule{DestType: "socks", DestAddress: "1.2.3.4", DestPort: 1080, Username: "u", Password: "p:x"})
	if u.String() != "socks5://u:p%3Ax@1.2.3.4:1080" {
		t.Fatalf("socks url = %s", u)
	}
	u = forwardProxyURL(model.ForwardRule{DestType: "http", DestAddress: "::1", DestPort: 8080})
	if u.String() != "http://[::1]:8080" {
		t.Fatalf("http url = %s", u)
	}
}

func TestClassifyCheck(t *testing.T) {
	ok := func(ip string) model.ForwardRule { return model.ForwardRule{CheckedAt: 1, CheckOK: true, CheckIP: ip} }
	bad := model.ForwardRule{CheckedAt: 1, CheckOK: false}
	cases := []struct {
		name, want string
		old, now   model.ForwardRule
	}{
		{"first check never alerts", "", model.ForwardRule{}, bad},
		{"ok → fail", CheckEventDown, ok("1.1.1.1"), bad},
		{"fail persists", "", bad, bad},
		{"fail → ok", CheckEventUp, bad, ok("1.1.1.1")},
		{"ip changed", CheckEventIPChanged, ok("1.1.1.1"), ok("2.2.2.2")},
		{"steady", "", ok("1.1.1.1"), ok("1.1.1.1")},
	}
	for _, c := range cases {
		if got := classifyCheck(c.old, c.now); got != c.want {
			t.Errorf("%s: got %q want %q", c.name, got, c.want)
		}
	}
}

func TestCheckOne_PersistsOnlyCheckColumns(t *testing.T) {
	newForwardTestDB(t)
	svc := ForwardService{}
	r := fakeHTTPProxy(t, `{"query":"203.0.113.7"}`, 200)
	r.Id = 0
	r.Remark = "keep me"
	if err := svc.Add(&r); err != nil {
		t.Fatalf("Add: %v", err)
	}
	// Simulate a concurrent edit of an editable field between load and check.
	if err := svc.Update(&model.ForwardRule{Id: r.Id, InboundTag: r.InboundTag, DestType: r.DestType, DestAddress: r.DestAddress, DestPort: r.DestPort, Enable: true, Remark: "edited meanwhile"}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if kind := svc.CheckOne(&r, checkTarget); kind != "" {
		t.Fatalf("first check must not produce an event, got %q", kind)
	}
	rules, _ := svc.GetAll()
	got := rules[0]
	if !got.CheckOK || got.CheckIP != "203.0.113.7" || got.CheckedAt == 0 {
		t.Fatalf("check result not persisted: %+v", got)
	}
	if got.Remark != "edited meanwhile" {
		t.Fatalf("CheckOne must not overwrite editable fields: remark=%q", got.Remark)
	}
	// Second run with a now-dead proxy → "down".
	r.DestPort = 1
	if kind := svc.CheckOne(&r, checkTarget); kind != CheckEventDown {
		t.Fatalf("expected down event, got %q", kind)
	}
}
