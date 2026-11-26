package job

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/database/dbtest"
)

func writeLogConfig(t *testing.T, accessPath string, errorPath string) {
	t.Helper()
	binDir := t.TempDir()
	t.Setenv("XUI_BIN_FOLDER", binDir)
	configData, err := json.Marshal(map[string]any{
		"log": map[string]any{"access": accessPath, "error": errorPath},
	})
	if err != nil {
		t.Fatalf("marshal xray config: %v", err)
	}
	if err := os.WriteFile(filepath.Join(binDir, "config.json"), configData, 0o644); err != nil {
		t.Fatalf("write xray config: %v", err)
	}
}

func TestWipeXrayLogs_TruncatesEnabledLogs(t *testing.T) {
	accessLog := filepath.Join(t.TempDir(), "access.log")
	errorLog := filepath.Join(t.TempDir(), "error.log")
	if err := os.WriteFile(accessLog, []byte("2026/06/23 12:00:00 from tcp:203.0.113.10:443 accepted\n"), 0o644); err != nil {
		t.Fatalf("seed access log: %v", err)
	}
	if err := os.WriteFile(errorLog, []byte("xray warning\n"), 0o644); err != nil {
		t.Fatalf("seed error log: %v", err)
	}
	writeLogConfig(t, accessLog, errorLog)

	wipeXrayLogs()

	for _, logPath := range []string{accessLog, errorLog} {
		info, err := os.Stat(logPath)
		if err != nil {
			t.Fatalf("%s should still exist: %v", logPath, err)
		}
		if info.Size() != 0 {
			t.Fatalf("%s should be truncated to 0, got %d bytes", logPath, info.Size())
		}
	}
}

func TestWipeXrayLogs_LeavesDisabledLogsAlone(t *testing.T) {
	writeLogConfig(t, "none", "none")

	wipeXrayLogs()

	if _, err := os.Stat("none"); err == nil {
		os.Remove("none")
		t.Fatal(`wipeXrayLogs must not create a file named "none"`)
	}
}

func TestPruneXrayLogs_TruncatesOnlyOversizedLogs(t *testing.T) {
	oldMax := maxXrayLogBytes
	maxXrayLogBytes = 8
	defer func() { maxXrayLogBytes = oldMax }()

	dir := t.TempDir()
	accessLog := filepath.Join(dir, "access.log")
	errorLog := filepath.Join(dir, "error.log")
	if err := os.WriteFile(accessLog, []byte("small"), 0o644); err != nil {
		t.Fatalf("seed access log: %v", err)
	}
	if err := os.WriteFile(errorLog, []byte("large log line"), 0o644); err != nil {
		t.Fatalf("seed error log: %v", err)
	}
	writeLogConfig(t, accessLog, errorLog)

	NewPruneXrayLogsJob().Run()

	accessInfo, err := os.Stat(accessLog)
	if err != nil {
		t.Fatalf("access log should still exist: %v", err)
	}
	if accessInfo.Size() != 5 {
		t.Fatalf("small access log should be left alone, got %d bytes", accessInfo.Size())
	}
	errorInfo, err := os.Stat(errorLog)
	if err != nil {
		t.Fatalf("error log should still exist: %v", err)
	}
	if errorInfo.Size() != 0 {
		t.Fatalf("oversized error log should be truncated, got %d bytes", errorInfo.Size())
	}
}

func TestWipeXrayLogs_RecordsClientUsageBeforeTruncating(t *testing.T) {
	dbtest.InitDB(t, filepath.Join(t.TempDir(), "x-ui.db"))
	logDir := t.TempDir()
	t.Setenv("XUI_LOG_FOLDER", logDir)
	accessLog := filepath.Join(t.TempDir(), "access.log")
	line := time.Now().Format("2006/01/02 15:04:05.000000") +
		" from 198.51.100.7:50000 accepted tcp:www.tiktok.com:443 [inbound-443 -> fwd-1] email: alice\n"
	if err := os.WriteFile(accessLog, []byte(line), 0o644); err != nil {
		t.Fatalf("seed access log: %v", err)
	}
	writeLogConfig(t, accessLog, "none")

	wipeXrayLogs()

	if info, err := os.Stat(accessLog); err != nil || info.Size() != 0 {
		t.Fatalf("access log should be truncated: %v", err)
	}
	files, _ := filepath.Glob(filepath.Join(logDir, "usage", "*", "*.log"))
	if len(files) != 1 {
		t.Fatalf("want one usage file for alice, got %q", files)
	}
	data, err := os.ReadFile(files[0])
	if err != nil || !strings.Contains(string(data), "\twww.tiktok.com:443\tfwd-1\n") {
		t.Fatalf("usage file = %q, %v; want the tiktok visit recorded", data, err)
	}
}
