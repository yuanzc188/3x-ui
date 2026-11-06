package service

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/xray"
)

// usageReadBudget bounds one run, so a large backlog is drained over several runs.
const usageReadBudget = 32 << 20

const usageHeadBytes = 64

// usageCursor is how far the access log has been read. Head (its first bytes) tells a
// replaced file from a grown one when the new file is already past Offset.
type usageCursor struct {
	Path   string `json:"path"`
	Offset int64  `json:"offset"`
	Head   string `json:"head"`
}

// usageCollector keeps the current minute's keys so a minute split across runs is
// recorded once; mu also serialises the cron run with the drain before a truncate.
type usageCollector struct {
	mu     sync.Mutex
	minute int64
	seen   map[string]struct{}
}

var defaultUsageCollector = &usageCollector{}

// Collect appends the access log lines written since the last run to the usage files.
func (s *ClientUsageService) Collect() error {
	accessPath, err := xray.GetAccessLogPath()
	if err != nil || accessPath == "none" {
		accessPath = ""
	}
	loc := usageLocation()
	return defaultUsageCollector.collect(accessPath, usageLogDir(), loc, time.Now().In(loc))
}

func (c *usageCollector) collect(accessPath, dir string, loc *time.Location, now time.Time) (err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := pruneUsageDays(dir, now); err != nil {
		return err
	}
	if accessPath == "" {
		return nil
	}
	defer func() {
		if err != nil {
			c.seen = nil
		}
	}()
	return c.drain(accessPath, dir, loc)
}

func (c *usageCollector) drain(accessPath, dir string, loc *time.Location) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	cursorPath := filepath.Join(dir, "cursor.json")
	cur := loadUsageCursor(cursorPath, accessPath)
	file, err := openAtCursor(accessPath, &cur)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer file.Close()
	batch := usageBatch{collector: c, loc: loc, files: map[string]*bytes.Buffer{}}
	if err := scanUsage(file, &cur, &batch); err != nil {
		return err
	}
	if err := batch.flush(dir); err != nil {
		return err
	}
	return saveUsageCursor(cursorPath, cur)
}

func loadUsageCursor(path, accessPath string) usageCursor {
	var cur usageCursor
	data, err := os.ReadFile(path)
	if err != nil || json.Unmarshal(data, &cur) != nil || cur.Path != accessPath {
		return usageCursor{Path: accessPath}
	}
	return cur
}

func saveUsageCursor(path string, cur usageCursor) error {
	data, err := json.Marshal(cur)
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// openAtCursor opens the log and seeks to the cursor, restarting at 0 when the file
// was truncated or replaced since the last run.
func openAtCursor(path string, cur *usageCursor) (*os.File, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	head := make([]byte, usageHeadBytes)
	n, _ := io.ReadFull(file, head)
	info, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, err
	}
	if info.Size() < cur.Offset || !strings.HasPrefix(string(head[:n]), cur.Head) {
		cur.Offset = 0
	}
	cur.Head = string(head[:n])
	if _, err := file.Seek(cur.Offset, io.SeekStart); err != nil {
		file.Close()
		return nil, err
	}
	return file, nil
}

// scanUsage feeds whole lines to batch; a trailing line without "\n" is still being
// written, so it stays unread for the next run.
func scanUsage(r io.Reader, cur *usageCursor, batch *usageBatch) error {
	reader := bufio.NewReaderSize(io.LimitReader(r, usageReadBudget), 64<<10)
	for {
		line, err := reader.ReadString('\n')
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		cur.Offset += int64(len(line))
		batch.add(line)
	}
}

type usageBatch struct {
	collector *usageCollector
	loc       *time.Location
	files     map[string]*bytes.Buffer
}

func (b *usageBatch) add(line string) {
	rec, ok := parseUsageAccessLine(line)
	if !ok || !b.collector.firstInMinute(rec) {
		return
	}
	key := filepath.Join(rec.at.In(b.loc).Format(usageDayLayout), usageFileName(rec.email))
	buf := b.files[key]
	if buf == nil {
		buf = &bytes.Buffer{}
		b.files[key] = buf
	}
	buf.WriteString(strconv.FormatInt(rec.at.Unix(), 10) + "\t" + rec.dest + "\t" + rec.outbound + "\n")
}

func (b *usageBatch) flush(dir string) error {
	for key, buf := range b.files {
		path := filepath.Join(dir, key)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			return err
		}
		_, werr := file.Write(buf.Bytes())
		if cerr := file.Close(); werr == nil {
			werr = cerr
		}
		if werr != nil {
			return werr
		}
	}
	return nil
}

// firstInMinute reports whether rec is the first (client, destination) of its minute.
func (c *usageCollector) firstInMinute(rec usageAccess) bool {
	minute := rec.at.Unix() / 60
	if minute != c.minute || c.seen == nil {
		c.minute, c.seen = minute, map[string]struct{}{}
	}
	key := rec.email + "\x00" + rec.dest
	if _, dup := c.seen[key]; dup {
		return false
	}
	c.seen[key] = struct{}{}
	return true
}

// pruneUsageDays deletes day directories older than the retention window.
func pruneUsageDays(dir string, now time.Time) error {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	oldest := oldestUsageDay(now)
	for _, e := range entries {
		if _, perr := time.Parse(usageDayLayout, e.Name()); !e.IsDir() || perr != nil || e.Name() >= oldest {
			continue
		}
		if err := os.RemoveAll(filepath.Join(dir, e.Name())); err != nil {
			return err
		}
	}
	return nil
}

type usageAccess struct {
	at       time.Time
	email    string
	dest     string
	outbound string
}

// parseUsageAccessLine reads an accepted client line of the Xray access log:
// "<date> <time> from <src> accepted <net>:<host>:<port> [<detour>] email: <email>".
func parseUsageAccessLine(line string) (usageAccess, bool) {
	head, email, ok := strings.Cut(strings.TrimRight(line, "\r\n"), " email: ")
	if !ok || email == "" {
		return usageAccess{}, false
	}
	at, ok := parseAccessTime(head)
	if !ok {
		return usageAccess{}, false
	}
	_, rest, ok := strings.Cut(head, " accepted ")
	if !ok {
		return usageAccess{}, false
	}
	dest, detour, _ := strings.Cut(rest, " ")
	return usageAccess{at: at, email: email, dest: strings.TrimPrefix(dest, "tcp:"), outbound: detourOutbound(detour)}, true
}

// parseAccessTime reads the leading "2006/01/02 15:04:05[.µs]" Xray writes in local time.
func parseAccessTime(line string) (time.Time, bool) {
	date, rest, ok := strings.Cut(line, " ")
	clock, _, ok2 := strings.Cut(rest, " ")
	if !ok || !ok2 {
		return time.Time{}, false
	}
	at, err := time.ParseInLocation("2006/01/02 15:04:05", date+" "+clock, time.Local)
	return at, err == nil
}

// detourOutbound takes the outbound tag from "[in -> out]", "[in >> out]" or "[out]".
func detourOutbound(detour string) string {
	if !strings.HasPrefix(detour, "[") {
		return ""
	}
	inner, _, _ := strings.Cut(detour[1:], "]")
	if i := strings.LastIndex(inner, " "); i >= 0 {
		return inner[i+1:]
	}
	return inner
}
