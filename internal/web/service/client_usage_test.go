package service

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"
)

var usageBase = time.Date(2026, 11, 6, 10, 0, 0, 0, time.Local)

func accessLine(at time.Time, dest, detour, email string) string {
	line := at.Format("2006/01/02 15:04:05.000000") + " from 198.51.100.7:50000 accepted " + dest
	if detour != "" {
		line += " [" + detour + "]"
	}
	if email != "" {
		line += " email: " + email
	}
	return line + "\n"
}

func appendFile(t *testing.T, path, data string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.WriteString(data); err != nil {
		t.Fatal(err)
	}
}

func usageLines(t *testing.T, dir, email string) []string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, usageBase.Format(usageDayLayout), usageFileName(email)))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(strings.TrimRight(string(data), "\n"), "\n")
}

func rec(at time.Time, dest, outbound string) string {
	return strings.Join([]string{strconv.FormatInt(at.Unix(), 10), dest, outbound}, "\t")
}

func TestUsageCollect_DedupesPerClientMinuteAndHoldsPartialLine(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "access.log")
	dir := t.TempDir()
	c := &usageCollector{}
	appendFile(t, logPath,
		accessLine(usageBase, "tcp:www.tiktok.com:443", "inbound-443 -> fwd-1", "alice")+
			accessLine(usageBase.Add(20*time.Second), "tcp:www.tiktok.com:443", "inbound-443 -> fwd-1", "alice")+
			accessLine(usageBase.Add(30*time.Second), "udp:8.8.8.8:53", "inbound-443 -> forward-block", "alice")+
			accessLine(usageBase.Add(40*time.Second), "tcp:www.tiktok.com:443", "inbound-443 -> fwd-1", "bob")+
			accessLine(usageBase.Add(70*time.Second), "tcp:www.tiktok.com:443", "inbound-443 >> fwd-1", "alice")+
			accessLine(usageBase, "tcp:127.0.0.1:62789", "api -> api", "")+
			"2026/11/06 10:01:30.000000 from 198.51.100.7:1 rejected  proxy/vless: invalid request user id\n"+
			"2026/11/06 10:01:50.000000 from 198.51.100.7:50000 accepted tcp:half.example:443 [inbound-443 -> fwd-1] email: bo")

	if err := c.collect(logPath, dir, time.Local, usageBase); err != nil {
		t.Fatal(err)
	}

	wantAlice := []string{
		rec(usageBase, "www.tiktok.com:443", "fwd-1"),
		rec(usageBase.Add(30*time.Second), "udp:8.8.8.8:53", "forward-block"),
		rec(usageBase.Add(70*time.Second), "www.tiktok.com:443", "fwd-1"),
	}
	if got := usageLines(t, dir, "alice"); !reflect.DeepEqual(got, wantAlice) {
		t.Fatalf("alice = %q, want %q", got, wantAlice)
	}
	if got, want := usageLines(t, dir, "bob"), []string{rec(usageBase.Add(40*time.Second), "www.tiktok.com:443", "fwd-1")}; !reflect.DeepEqual(got, want) {
		t.Fatalf("bob = %q, want %q", got, want)
	}
	if got := usageLines(t, dir, "bo"); got != nil {
		t.Fatalf("a trailing line without its newline must wait for the next run, got %q", got)
	}
}

func TestUsageCollect_ResumesAndRestartsOnTruncateOrReplace(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "access.log")
	dir := t.TempDir()
	c := &usageCollector{}
	collect := func() {
		t.Helper()
		if err := c.collect(logPath, dir, time.Local, usageBase); err != nil {
			t.Fatal(err)
		}
	}
	appendFile(t, logPath, accessLine(usageBase, "tcp:a.example:443", "in -> out", "alice")+
		"2026/11/06 10:00:10.000000 from 198.51.100.7:50000 accepted tcp:b.exa")
	collect()
	appendFile(t, logPath, "mple:443 [in -> out] email: alice\n")
	collect()

	// Pruned to zero, then written again: the shorter file restarts at offset 0.
	if err := os.WriteFile(logPath, []byte(accessLine(usageBase.Add(2*time.Minute), "tcp:c.example:443", "in -> out", "alice")), 0o644); err != nil {
		t.Fatal(err)
	}
	collect()
	// Replaced by a file already past the old offset: only its head tells it apart.
	var replaced strings.Builder
	for i := range 3 {
		replaced.WriteString(accessLine(usageBase.Add(time.Duration(3+i)*time.Minute), "tcp:d.example:443", "in -> out", "alice"))
	}
	if err := os.WriteFile(logPath, []byte(replaced.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	collect()
	// A panel restart keeps the saved cursor, so nothing is recorded twice.
	c = &usageCollector{}
	collect()

	want := []string{
		rec(usageBase, "a.example:443", "out"),
		rec(usageBase.Add(10*time.Second), "b.example:443", "out"),
		rec(usageBase.Add(2*time.Minute), "c.example:443", "out"),
		rec(usageBase.Add(3*time.Minute), "d.example:443", "out"),
		rec(usageBase.Add(4*time.Minute), "d.example:443", "out"),
		rec(usageBase.Add(5*time.Minute), "d.example:443", "out"),
	}
	if got := usageLines(t, dir, "alice"); !reflect.DeepEqual(got, want) {
		t.Fatalf("alice =\n%q\nwant\n%q", got, want)
	}
}

func TestReadUsageDay_NewestFirstFilteredAndCapped(t *testing.T) {
	dir := t.TempDir()
	day := usageBase.Format(usageDayLayout)
	if err := os.MkdirAll(filepath.Join(dir, day), 0o755); err != nil {
		t.Fatal(err)
	}
	stored := []string{
		rec(usageBase, "www.tiktok.com:443", "fwd-1"),
		rec(usageBase.Add(time.Minute), "udp:8.8.8.8:53", "forward-block"),
		rec(usageBase.Add(2*time.Minute), "v16.TikTokCDN.com:443", "fwd-1"),
		"not a record",
	}
	if err := os.WriteFile(filepath.Join(dir, day, usageFileName("alice")), []byte(strings.Join(stored, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ms := func(d time.Duration) int64 { return usageBase.Add(d).Unix() * 1000 }
	all := []UsageLogEntry{
		{Time: ms(2 * time.Minute), Destination: "v16.TikTokCDN.com:443", Outbound: "fwd-1"},
		{Time: ms(time.Minute), Destination: "udp:8.8.8.8:53", Outbound: "forward-block", Blocked: true},
		{Time: ms(0), Destination: "www.tiktok.com:443", Outbound: "fwd-1"},
	}
	defaultMax := maxUsageEntries
	t.Cleanup(func() { maxUsageEntries = defaultMax })

	for _, tc := range []struct {
		name, email, filter string
		max                 int
		want                []UsageLogEntry
		truncated           bool
	}{
		{"newest first with blackholes flagged", "alice", "", defaultMax, all, false},
		{"filter ignores case", "alice", "TIKTOK", defaultMax, []UsageLogEntry{all[0], all[2]}, false},
		{"cap keeps the newest", "alice", "", 2, all[:2], true},
		{"client without records", "nobody", "", defaultMax, []UsageLogEntry{}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			maxUsageEntries = tc.max
			got, truncated, err := readUsageDay(dir, day, tc.email, tc.filter, map[string]bool{"forward-block": true})
			if err != nil || truncated != tc.truncated || !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %+v truncated=%v err=%v, want %+v truncated=%v", got, truncated, err, tc.want, tc.truncated)
			}
		})
	}
}

func TestResolveUsageDay(t *testing.T) {
	now := time.Date(2026, 11, 6, 15, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		in, want string
		err      error
	}{
		{"", "2026-11-06", nil},
		{"2026-11-06", "2026-11-06", nil},
		{"2026-10-08", "2026-10-08", nil},
		{"2026-10-07", "", ErrUsageDayOutOfRange},
		{"2026-11-07", "", ErrUsageDayOutOfRange},
		{"2026-11-6", "", ErrUsageDayInvalid},
		{"../../etc", "", ErrUsageDayInvalid},
	} {
		t.Run(tc.in, func(t *testing.T) {
			got, err := resolveUsageDay(tc.in, now)
			if got != tc.want || !errors.Is(err, tc.err) {
				t.Fatalf("resolveUsageDay(%q) = %q, %v; want %q, %v", tc.in, got, err, tc.want, tc.err)
			}
		})
	}
}

func TestPruneUsageDays_KeepsRetentionWindow(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"2026-11-06", "2026-10-08", "2026-10-07", "2025-01-01", "not-a-day"} {
		if err := os.MkdirAll(filepath.Join(dir, name), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "cursor.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := pruneUsageDays(dir, time.Date(2026, 11, 6, 15, 0, 0, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, e := range entries {
		got = append(got, e.Name())
	}
	if want := []string{"2026-10-08", "2026-11-06", "cursor.json", "not-a-day"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("after prune: %q, want %q", got, want)
	}
}
