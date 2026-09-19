package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/logger"
)

const (
	forwardCheckTimeout  = 15 * time.Second
	forwardCheckParallel = 5 // ponytail: fixed fan-out sized for 1C1G boxes
	forwardCheckErrMax   = 200
	forwardCheckBodyMax  = 64 << 10
)

// Check event kinds, produced by classifyCheck and consumed by the TG alerting job.
const (
	CheckEventDown      = "down"
	CheckEventUp        = "up"
	CheckEventIPChanged = "ipChanged"
)

// CheckEvent is a state transition worth telling the operator about.
type CheckEvent struct {
	Rule model.ForwardRule
	Kind string
}

// forwardProxyURL builds the proxy URL net/http understands for the rule:
// socks5://[user:pass@]host:port or http://[user:pass@]host:port.
func forwardProxyURL(r model.ForwardRule) *url.URL {
	scheme := "socks5"
	if r.DestType == "http" {
		scheme = "http"
	}
	u := &url.URL{Scheme: scheme, Host: net.JoinHostPort(r.DestAddress, strconv.Itoa(r.DestPort))}
	if r.Username != "" || r.Password != "" {
		u.User = url.UserPassword(r.Username, r.Password)
	}
	return u
}

// probeProxy fetches checkURL through the rule's proxy and reports the egress
// IP, a "country / city" label (may be empty) and the round-trip latency.
func probeProxy(r model.ForwardRule, checkURL string) (ip, geo string, ms int, err error) {
	client := &http.Client{
		Timeout: forwardCheckTimeout,
		Transport: &http.Transport{
			Proxy:             http.ProxyURL(forwardProxyURL(r)),
			DisableKeepAlives: true,
		},
	}
	start := time.Now()
	resp, err := client.Get(checkURL)
	if err != nil {
		return "", "", 0, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, forwardCheckBodyMax))
	if err != nil {
		return "", "", 0, err
	}
	ms = int(time.Since(start).Milliseconds())
	if resp.StatusCode != http.StatusOK {
		return "", "", ms, fmt.Errorf("check url returned HTTP %d", resp.StatusCode)
	}
	ip, geo = parseCheckBody(body)
	if ip == "" {
		return "", "", ms, errors.New("check url response contains no IP")
	}
	return ip, geo, ms, nil
}

// parseCheckBody accepts ip-api style JSON ({query|ip, country, city}) or a
// plain-text body that is just the IP (ipify style).
func parseCheckBody(body []byte) (ip, geo string) {
	var j struct {
		Query   string `json:"query"`
		IP      string `json:"ip"`
		Country string `json:"country"`
		City    string `json:"city"`
	}
	if err := json.Unmarshal(body, &j); err != nil {
		text := strings.TrimSpace(string(body))
		if net.ParseIP(text) != nil {
			return text, ""
		}
		return "", ""
	}
	ip = j.Query
	if ip == "" {
		ip = j.IP
	}
	var parts []string
	for _, p := range []string{j.Country, j.City} {
		if p = strings.TrimSpace(p); p != "" {
			parts = append(parts, p)
		}
	}
	return strings.TrimSpace(ip), strings.Join(parts, " / ")
}

// classifyCheck compares the stored state with the fresh one. The very first
// check never alerts; a failure that persists is reported only once.
func classifyCheck(old, now model.ForwardRule) string {
	if old.CheckedAt == 0 {
		return ""
	}
	switch {
	case old.CheckOK && !now.CheckOK:
		return CheckEventDown
	case !old.CheckOK && now.CheckOK:
		return CheckEventUp
	case old.CheckOK && now.CheckOK && old.CheckIP != now.CheckIP:
		return CheckEventIPChanged
	}
	return ""
}

// CheckOne probes the rule, persists the result into the Check* columns only
// (never touching editable fields) and returns the event kind, "" for none.
// The rule is updated in place with the fresh result.
func (s *ForwardService) CheckOne(r *model.ForwardRule, checkURL string) string {
	old := *r
	ip, geo, ms, err := probeProxy(*r, checkURL)
	r.CheckedAt = time.Now().UnixMilli()
	r.CheckOK = err == nil
	r.CheckIP, r.CheckGeo, r.CheckMs, r.CheckErr = ip, geo, ms, ""
	if err != nil {
		r.CheckErr = truncate(err.Error(), forwardCheckErrMax)
	}
	db := database.GetDB()
	if dbErr := db.Model(&model.ForwardRule{}).Where("id = ?", r.Id).Updates(map[string]any{
		"checked_at": r.CheckedAt, "check_ok": r.CheckOK, "check_ip": r.CheckIP,
		"check_geo": r.CheckGeo, "check_ms": r.CheckMs, "check_err": r.CheckErr,
	}).Error; dbErr != nil {
		logger.Warning("forward check: persist result failed:", dbErr)
	}
	return classifyCheck(old, *r)
}

// CheckAll probes every enabled rule with bounded concurrency and returns the
// state transitions. A bad check URL aborts the whole run without touching rows.
func (s *ForwardService) CheckAll() ([]CheckEvent, error) {
	fs, err := s.GetSettings()
	if err != nil {
		return nil, err
	}
	checkURL, err := SanitizePublicHTTPURL(fs.CheckUrl, false)
	if err != nil {
		return nil, fmt.Errorf("forward check url rejected: %w", err)
	}
	rules, err := s.ActiveRules()
	if err != nil {
		return nil, err
	}

	var (
		mu     sync.Mutex
		wg     sync.WaitGroup
		events []CheckEvent
		sem    = make(chan struct{}, forwardCheckParallel)
	)
	for i := range rules {
		wg.Add(1)
		sem <- struct{}{}
		go func(r *model.ForwardRule) {
			defer wg.Done()
			defer func() { <-sem }()
			if kind := s.CheckOne(r, checkURL); kind != "" {
				mu.Lock()
				events = append(events, CheckEvent{Rule: *r, Kind: kind})
				mu.Unlock()
			}
		}(&rules[i])
	}
	wg.Wait()
	return events, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
