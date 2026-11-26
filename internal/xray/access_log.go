package xray

import (
	"encoding/json"
	"os"
	"regexp"
	"strings"
	"time"

	corelog "github.com/xtls/xray-core/app/log"

	"github.com/mhsanaei/3x-ui/v3/internal/logger"
)

type accessLogConfig struct {
	Access      string `json:"access"`
	Level       string `json:"loglevel"`
	MaskAddress string `json:"maskAddress"`
}

func (c accessLogConfig) filePath() string {
	if strings.EqualFold(c.Level, "none") || strings.EqualFold(c.Access, "none") {
		return ""
	}
	return c.Access
}

// accessLogRuntimeConfig sends access messages through stdout so the panel can
// check the client before writing to disk. The saved config still names the
// actual log file used by the panel's readers and rotation jobs.
func accessLogRuntimeConfig(cfg *Config, logs *LogWriter, filter func([]string) ([]string, error)) (*Config, *accessLogWriter, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(cfg.LogConfig, &fields); err != nil || fields == nil {
		return cfg, nil, err
	}
	var settings accessLogConfig
	if err := json.Unmarshal(cfg.LogConfig, &settings); err != nil {
		return nil, nil, err
	}
	if settings.Access == "none" || strings.EqualFold(settings.Level, "none") {
		return cfg, nil, nil
	}
	path := settings.filePath()
	runtimeAccess := json.RawMessage(`"none"`)
	var writer *accessLogWriter
	if path != "" {
		mask4, mask6, err := corelog.ParseMaskAddress(settings.MaskAddress)
		if err != nil {
			return nil, nil, err
		}
		file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
		if err != nil {
			return nil, nil, err
		}
		if err := file.Close(); err != nil {
			return nil, nil, err
		}
		runtimeAccess = json.RawMessage(`""`)
		writer = &accessLogWriter{path: path, logs: logs, filter: filter, masked: settings.MaskAddress != "", mask4: mask4, mask6: mask6}
	}
	for key := range fields {
		if strings.EqualFold(key, "access") {
			delete(fields, key)
		}
	}
	fields["access"] = runtimeAccess
	runtimeCfg := *cfg
	raw, err := json.Marshal(fields)
	runtimeCfg.LogConfig = raw
	return &runtimeCfg, writer, err
}

type accessLogWriter struct {
	path      string
	logs      *LogWriter
	filter    func([]string) ([]string, error)
	pending   string
	lastError time.Time
	masked    bool
	mask4     int
	mask6     int
}

var coreLogTimestamp = regexp.MustCompile(`^\d{4}/\d{2}/\d{2} \d{2}:\d{2}:\d{2}(\.\d+)? `)

func isAccessLogLine(line string) bool {
	prefix := coreLogTimestamp.FindStringIndex(line)
	if prefix == nil {
		return false
	}
	body := line[prefix[1]:]
	if strings.HasPrefix(body, "from ") {
		return true
	}
	// Xray writes DNS telemetry to the same configured access sink.
	if strings.HasPrefix(body, "[Info] ") || strings.HasPrefix(body, "[Warning] ") ||
		strings.HasPrefix(body, "[Debug] ") || strings.HasPrefix(body, "[Error] ") {
		return false
	}
	return strings.Contains(body, " got answer: ") || strings.Contains(body, " cache HIT: ") ||
		strings.Contains(body, " cache OPTIMISTE: ")
}

// AccessLogClientEmail reads the identity suffix appended by Xray.
func AccessLogClientEmail(line string) string {
	i := strings.LastIndex(line, " email: ")
	if i < 0 {
		return ""
	}
	return strings.TrimRight(line[i+len(" email: "):], "\r\n")
}

// os/exec gives the same writer to stdout and stderr and invokes Write serially.
func (w *accessLogWriter) Write(p []byte) (int, error) {
	w.pending += string(p)
	var accessLines, emails []string
	for {
		line, rest, complete := strings.Cut(w.pending, "\n")
		if !complete {
			break
		}
		w.pending = rest
		if !isAccessLogLine(line) {
			_, _ = w.logs.Write([]byte(line + "\n"))
			continue
		}
		accessLines = append(accessLines, line)
		if email := AccessLogClientEmail(line); email != "" {
			emails = append(emails, email)
		}
	}
	if err := w.append(accessLines, emails); err != nil && time.Since(w.lastError) >= time.Minute {
		logger.Warning("Xray access log:", err)
		w.lastError = time.Now()
	}
	return len(p), nil
}

type accessLogText struct{ value string }

func (s accessLogText) String() string { return s.value }

func (w *accessLogWriter) append(lines, emails []string) error {
	var disabled []string
	if len(emails) != 0 {
		if w.masked {
			// Xray applies its mask to the whole line, including the email suffix.
			emails = nil
		}
		var err error
		disabled, err = w.filter(emails)
		if err != nil {
			return err // Do not write access records when the policy could not be read.
		}
	}
	blocked := make(map[string]bool, len(disabled))
	for _, email := range disabled {
		if w.masked {
			masked := corelog.MaskedMsgWrapper{Message: accessLogText{value: email}, Mask4: w.mask4, Mask6: w.mask6}
			email = masked.String()
		}
		blocked[strings.ToLower(email)] = true
	}
	var output strings.Builder
	for _, line := range lines {
		if !blocked[strings.ToLower(AccessLogClientEmail(line))] {
			output.WriteString(line)
			output.WriteByte('\n')
		}
	}
	if output.Len() == 0 {
		return nil
	}
	file, err := os.OpenFile(w.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	_, err = file.WriteString(output.String())
	closeErr := file.Close()
	if err != nil {
		return err
	}
	return closeErr
}

func (w *accessLogWriter) flush() {
	if w.pending != "" && !isAccessLogLine(w.pending) {
		_, _ = w.logs.Write([]byte(w.pending))
	}
	w.pending = ""
}
