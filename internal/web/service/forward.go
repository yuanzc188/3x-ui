package service

import (
	"strings"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

// ForwardService manages port-forwarding rules. Each rule binds an inbound tag
// to a socks/http proxy egress. Rules are the single source of truth and are
// injected into the generated Xray config by injectForwardRules at build time;
// the stored template is never modified.
type ForwardService struct {
	settingService SettingService
}

// ForwardSettings are the panel-wide knobs of the forwarding feature, stored
// in the generic settings table.
type ForwardSettings struct {
	GlobalDomains string `json:"globalDomains" form:"globalDomains"`
	CheckUrl      string `json:"checkUrl" form:"checkUrl"`
}

// GetSettings reads both keys, falling back to defaults for unset ones.
func (s *ForwardService) GetSettings() (ForwardSettings, error) {
	domains, err := s.settingService.getString("forwardGlobalDomains")
	if err != nil {
		return ForwardSettings{}, err
	}
	checkURL, err := s.settingService.getString("forwardCheckUrl")
	if err != nil {
		return ForwardSettings{}, err
	}
	return ForwardSettings{GlobalDomains: domains, CheckUrl: effectiveSettingValue("forwardCheckUrl", checkURL)}, nil
}

// SaveSettings validates the check URL (public http/https only, SSRF-safe)
// and persists both keys. An empty URL resets to the default.
func (s *ForwardService) SaveSettings(fs ForwardSettings) error {
	checkURL := strings.TrimSpace(fs.CheckUrl)
	if checkURL == "" {
		checkURL = defaultValueMap["forwardCheckUrl"]
	}
	clean, err := SanitizePublicHTTPURL(checkURL, false)
	if err != nil {
		return err
	}
	if err := s.settingService.setString("forwardCheckUrl", clean); err != nil {
		return err
	}
	return s.settingService.setString("forwardGlobalDomains", strings.Join(splitDomains(fs.GlobalDomains), "\n"))
}

// GetAll returns every forward rule, oldest first.
func (s *ForwardService) GetAll() ([]model.ForwardRule, error) {
	db := database.GetDB()
	var rules []model.ForwardRule
	if err := db.Model(&model.ForwardRule{}).Order("id asc").Find(&rules).Error; err != nil {
		return nil, err
	}
	return rules, nil
}

// ActiveRules returns only enabled rules — used by config generation.
func (s *ForwardService) ActiveRules() ([]model.ForwardRule, error) {
	db := database.GetDB()
	var rules []model.ForwardRule
	if err := db.Model(&model.ForwardRule{}).Where("enable = ?", true).Order("id asc").Find(&rules).Error; err != nil {
		return nil, err
	}
	return rules, nil
}

// Add persists a new rule. The unique constraint on InboundTag enforces the
// "one rule per inbound" invariant at the DB layer.
func (s *ForwardService) Add(rule *model.ForwardRule) error {
	db := database.GetDB()
	return db.Create(rule).Error
}

// forwardCheckColumns are owned by the health checker; form updates must
// never overwrite them with the zero values a bound form carries.
var forwardCheckColumns = []string{"checked_at", "check_ok", "check_ip", "check_geo", "check_ms", "check_err"}

// Update overwrites every editable column of an existing rule (including
// zero values such as enable=false), leaving health-check results untouched.
func (s *ForwardService) Update(rule *model.ForwardRule) error {
	db := database.GetDB()
	return db.Model(rule).Select("*").Omit(forwardCheckColumns...).Updates(rule).Error
}

// Delete removes a rule by id.
func (s *ForwardService) Delete(id int) error {
	db := database.GetDB()
	return db.Delete(&model.ForwardRule{}, id).Error
}

// SetEnable flips only the enable flag without rewriting the rest of the row.
func (s *ForwardService) SetEnable(id int, enable bool) error {
	db := database.GetDB()
	return db.Model(&model.ForwardRule{}).Where("id = ?", id).Update("enable", enable).Error
}
