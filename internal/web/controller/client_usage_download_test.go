package controller

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/mhsanaei/3x-ui/v3/internal/database/dbtest"
	"github.com/mhsanaei/3x-ui/v3/internal/web/locale"
)

// A non-ASCII email must still name the file (RFC 2231), and a rejected day must come
// back as the JSON error, not as an attachment the browser would save as the log.
func TestDownloadUsageLogHTTP(t *testing.T) {
	dbtest.InitDB(t, filepath.Join(t.TempDir(), "x-ui.db"))
	t.Setenv("XUI_LOG_FOLDER", t.TempDir())
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatal(err)
	}
	today := time.Now().In(loc).Format("2006-01-02")

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("I18n", func(_ locale.I18nType, key string, _ ...string) string { return key })
		c.Next()
	})
	NewClientController(router.Group("/panel/api/clients"))

	for _, tc := range []struct {
		name, target, disposition, body string
	}{
		{
			name:        "today as an attachment named after the client",
			target:      "/panel/api/clients/usageLog/" + url.PathEscape("商户A@shop") + "/download",
			disposition: "attachment; filename*=utf-8''usage_%E5%95%86%E6%88%B7A%40shop_" + today + ".log",
			body:        "time (Asia/Shanghai)\tdestination\troute\n",
		},
		{
			name:   "day outside the retention window",
			target: "/panel/api/clients/usageLog/alice/download?day=2000-01-01",
			body:   `"success":false`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tc.target, nil))
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, body %q", rec.Code, rec.Body.String())
			}
			if got := rec.Header().Get("Content-Disposition"); got != tc.disposition {
				t.Fatalf("Content-Disposition = %q, want %q", got, tc.disposition)
			}
			if tc.disposition != "" && rec.Body.String() != tc.body {
				t.Fatalf("body = %q, want %q", rec.Body.String(), tc.body)
			}
			if tc.disposition == "" && !strings.Contains(rec.Body.String(), tc.body) {
				t.Fatalf("body = %q, want it to contain %q", rec.Body.String(), tc.body)
			}
		})
	}
}
