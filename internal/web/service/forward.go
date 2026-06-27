package service

import (
	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

// ForwardService manages port-forwarding rules. Each rule binds an inbound tag
// to a socks/http proxy egress. Rules are the single source of truth and are
// injected into the generated Xray config by injectForwardRules at build time;
// the stored template is never modified.
type ForwardService struct{}

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

// Update saves an existing rule (full overwrite by primary key).
func (s *ForwardService) Update(rule *model.ForwardRule) error {
	db := database.GetDB()
	return db.Save(rule).Error
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
