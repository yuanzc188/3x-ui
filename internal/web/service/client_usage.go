package service

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/config"
	"github.com/mhsanaei/3x-ui/v3/internal/database"
)

// usageRetentionDays counts today, so the oldest kept day is today-29.
const usageRetentionDays = 30

const usageDayLayout = "2006-01-02"

// maxUsageEntries caps one response; a var so tests can shrink it.
var maxUsageEntries = 5000

var (
	ErrUsageDayInvalid    = errors.New("day must be YYYY-MM-DD")
	ErrUsageDayOutOfRange = errors.New("day is outside the usage log retention window")
)

// UsageLogEntry is one destination a client opened, deduplicated per minute.
type UsageLogEntry struct {
	Time        int64  `json:"time" example:"1735732800000"`
	Destination string `json:"destination" example:"www.tiktok.com:443"`
	Outbound    string `json:"outbound" example:"forward-inbound-443"`
	Blocked     bool   `json:"blocked" example:"false"`
}

// UsageLogDay is one client's usage on one day of the panel's time zone, newest first.
type UsageLogDay struct {
	Day           string          `json:"day" example:"2025-01-01"`
	Enabled       bool            `json:"enabled" example:"true"`
	RetentionDays int             `json:"retentionDays" example:"30"`
	Truncated     bool            `json:"truncated" example:"false"`
	Entries       []UsageLogEntry `json:"entries"`
}

// ClientUsageService records which destinations each client opened, from the Xray access
// log, into per-day per-client files, and reads them back.
type ClientUsageService struct {
	serverService ServerService
}

// Day returns email's usage on day (YYYY-MM-DD, empty = today); filter matches destinations.
func (s *ClientUsageService) Day(email, day, filter string) (*UsageLogDay, error) {
	loc := usageLocation()
	resolved, err := resolveUsageDay(day, time.Now().In(loc))
	if err != nil {
		return nil, err
	}
	enabled, _ := (&SettingService{}).GetAccessLogEnable()
	entries, truncated, err := readUsageDay(usageLogDir(), resolved, email, filter, s.blockedTags())
	if err != nil {
		return nil, err
	}
	return &UsageLogDay{
		Day:           resolved,
		Enabled:       enabled,
		RetentionDays: usageRetentionDays,
		Truncated:     truncated,
		Entries:       entries,
	}, nil
}

// UsageExport streams one client's whole day, oldest first, as a download. Unlike the
// on-screen list it has no cap and is read line by line, so a busy day costs no memory.
type UsageExport struct {
	file    *os.File
	loc     *time.Location
	match   func(dest string) bool
	blocked map[string]bool
}

// Export opens email's records of day (empty = today) for Stream; the caller must Close.
func (s *ClientUsageService) Export(email, day, filter string) (*UsageExport, string, error) {
	loc := usageLocation()
	resolved, err := resolveUsageDay(day, time.Now().In(loc))
	if err != nil {
		return nil, "", err
	}
	export, err := openUsageExport(usageLogDir(), resolved, email, filter, loc, s.blockedTags())
	return export, resolved, err
}

func openUsageExport(dir, day, email, filter string, loc *time.Location, blocked map[string]bool) (*UsageExport, error) {
	f, err := os.Open(filepath.Join(dir, day, usageFileName(email)))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	return &UsageExport{file: f, loc: loc, match: usageMatcher(filter), blocked: blocked}, nil
}

func (e *UsageExport) Close() error {
	if e.file == nil {
		return nil
	}
	return e.file.Close()
}

// Stream writes a header row, then "<time>\t<destination>\t<route>" per record.
func (e *UsageExport) Stream(w io.Writer) error {
	out := bufio.NewWriter(w)
	_, _ = fmt.Fprintf(out, "time (%s)\tdestination\troute\n", e.loc)
	if e.file == nil {
		return out.Flush()
	}
	sc := bufio.NewScanner(e.file)
	for sc.Scan() {
		entry, ok := parseUsageRecord(sc.Text())
		if !ok || !e.match(entry.Destination) {
			continue
		}
		route := entry.Outbound
		if e.blocked[route] {
			route = "BLOCKED"
		}
		at := time.UnixMilli(entry.Time).In(e.loc).Format(time.DateTime)
		_, _ = fmt.Fprintf(out, "%s\t%s\t%s\n", at, entry.Destination, route)
	}
	if err := sc.Err(); err != nil {
		return err
	}
	return out.Flush()
}

// usageMatcher is the destination filter shared by the list and the download.
func usageMatcher(filter string) func(dest string) bool {
	filter = strings.ToLower(strings.TrimSpace(filter))
	return func(dest string) bool { return filter == "" || strings.Contains(strings.ToLower(dest), filter) }
}

func (s *ClientUsageService) blockedTags() map[string]bool {
	_, blackholes := s.serverService.GetDefaultLogOutboundTags()
	tags := map[string]bool{forwardBlockTag: true}
	for _, tag := range blackholes {
		tags[tag] = true
	}
	return tags
}

// usageLocation is the panel's time zone; jobs can run before the DB is up.
func usageLocation() *time.Location {
	if database.GetDB() == nil {
		return time.Local
	}
	if loc, err := (&SettingService{}).GetTimeLocation(); err == nil && loc != nil {
		return loc
	}
	return time.Local
}

func usageLogDir() string { return filepath.Join(config.GetLogFolder(), "usage") }

// usageFileName hashes the email: emails are free text, file names are not.
func usageFileName(email string) string {
	sum := sha256.Sum256([]byte(email))
	return hex.EncodeToString(sum[:16]) + ".log"
}

func oldestUsageDay(now time.Time) string {
	return now.AddDate(0, 0, -(usageRetentionDays - 1)).Format(usageDayLayout)
}

func resolveUsageDay(day string, now time.Time) (string, error) {
	today := now.Format(usageDayLayout)
	if day == "" {
		return today, nil
	}
	parsed, err := time.Parse(usageDayLayout, day)
	if err != nil || parsed.Format(usageDayLayout) != day {
		return "", ErrUsageDayInvalid
	}
	if day > today || day < oldestUsageDay(now) {
		return "", ErrUsageDayOutOfRange
	}
	return day, nil
}

// readUsageDay returns up to maxUsageEntries matching records, newest first.
func readUsageDay(dir, day, email, filter string, blocked map[string]bool) ([]UsageLogEntry, bool, error) {
	data, err := os.ReadFile(filepath.Join(dir, day, usageFileName(email)))
	if errors.Is(err, os.ErrNotExist) {
		return []UsageLogEntry{}, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	match := usageMatcher(filter)
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	entries := make([]UsageLogEntry, 0, min(len(lines), maxUsageEntries))
	for i := len(lines) - 1; i >= 0; i-- {
		entry, ok := parseUsageRecord(lines[i])
		if !ok || !match(entry.Destination) {
			continue
		}
		if len(entries) == maxUsageEntries {
			return entries, true, nil
		}
		entry.Blocked = blocked[entry.Outbound]
		entries = append(entries, entry)
	}
	return entries, false, nil
}

// parseUsageRecord reads one "<unix seconds>\t<destination>\t<outbound>" line.
func parseUsageRecord(line string) (UsageLogEntry, bool) {
	sec, rest, ok := strings.Cut(line, "\t")
	if !ok {
		return UsageLogEntry{}, false
	}
	dest, outbound, ok := strings.Cut(rest, "\t")
	unix, err := strconv.ParseInt(sec, 10, 64)
	if !ok || err != nil {
		return UsageLogEntry{}, false
	}
	return UsageLogEntry{Time: unix * 1000, Destination: dest, Outbound: outbound}, true
}
