package service

import (
	"path/filepath"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

// newForwardTestDB wires a temp sqlite db (all models auto-migrated) the same
// way setupConflictDB does in port_conflict_test.go. A real file (not :memory:)
// avoids the per-connection isolation problem of the pure-Go sqlite driver.
func newForwardTestDB(t *testing.T) {
	t.Helper()
	dbDir := t.TempDir()
	t.Setenv("XUI_DB_FOLDER", dbDir)
	if err := database.InitDB(filepath.Join(dbDir, "x-ui.db")); err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	t.Cleanup(func() { _ = database.CloseDB() })
}

func TestForwardRuleCRUD(t *testing.T) {
	newForwardTestDB(t)
	svc := ForwardService{}

	rule := &model.ForwardRule{
		InboundTag:  "in-1000-tcp",
		DestType:    "socks",
		DestAddress: "1.2.3.4",
		DestPort:    1080,
		Enable:      true,
	}
	if err := svc.Add(rule); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if rule.Id == 0 {
		t.Fatal("expected autoincrement id to be set")
	}

	all, err := svc.GetAll()
	if err != nil {
		t.Fatalf("GetAll: %v", err)
	}
	if len(all) != 1 || all[0].InboundTag != "in-1000-tcp" {
		t.Fatalf("unexpected GetAll result: %+v", all)
	}

	if err := svc.SetEnable(rule.Id, false); err != nil {
		t.Fatalf("SetEnable: %v", err)
	}
	active, err := svc.ActiveRules()
	if err != nil {
		t.Fatalf("ActiveRules: %v", err)
	}
	if len(active) != 0 {
		t.Fatalf("expected 0 active rules after disable, got %d", len(active))
	}

	if err := svc.Delete(rule.Id); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	all, _ = svc.GetAll()
	if len(all) != 0 {
		t.Fatalf("expected 0 rules after delete, got %d", len(all))
	}
}
