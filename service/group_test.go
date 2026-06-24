package service

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
)

func TestGetUserUsableGroupsMergesDefaultAndSpecialRules(t *testing.T) {
	restoreUserUsableGroups, restoreSpecialUsableGroups := snapshotGroupSettings(t)
	defer restoreUserUsableGroups()
	defer restoreSpecialUsableGroups()

	if err := setting.UpdateUserUsableGroupsByJSONString(`{"default":"默认分组","拼车":"拼车分组"}`); err != nil {
		t.Fatalf("UpdateUserUsableGroupsByJSONString() error = %v", err)
	}
	special := ratio_setting.GetGroupRatioSetting().GroupSpecialUsableGroup
	special.Clear()
	special.Set("jhl", map[string]string{
		"+:jhl":     "jhl分组",
		"-:default": "remove",
	})

	got := GetUserUsableGroups("jhl")
	if _, ok := got["default"]; ok {
		t.Fatalf("default should be removed, got %#v", got)
	}
	if got["拼车"] != "拼车分组" {
		t.Fatalf("拼车 should remain from default usable groups, got %#v", got)
	}
	if got["jhl"] != "jhl分组" {
		t.Fatalf("jhl should be added by special rule, got %#v", got)
	}
}

func TestGetUserUsableGroupsOnlyRulesOverrideDefaults(t *testing.T) {
	restoreUserUsableGroups, restoreSpecialUsableGroups := snapshotGroupSettings(t)
	defer restoreUserUsableGroups()
	defer restoreSpecialUsableGroups()

	if err := setting.UpdateUserUsableGroupsByJSONString(`{"default":"默认分组","vip":"VIP分组","拼车":"拼车分组"}`); err != nil {
		t.Fatalf("UpdateUserUsableGroupsByJSONString() error = %v", err)
	}
	special := ratio_setting.GetGroupRatioSetting().GroupSpecialUsableGroup
	special.Clear()
	special.Set("jhl", map[string]string{
		"=:拼车":  "拼车分组",
		"=:jhl": "jhl分组",
	})

	got := GetUserUsableGroups("jhl")
	if len(got) != 2 {
		t.Fatalf("only rules should return exactly two groups, got %#v", got)
	}
	if got["拼车"] != "拼车分组" || got["jhl"] != "jhl分组" {
		t.Fatalf("only rules returned unexpected groups, got %#v", got)
	}
	if _, ok := got["default"]; ok {
		t.Fatalf("only rules should hide default, got %#v", got)
	}
	if _, ok := got["vip"]; ok {
		t.Fatalf("only rules should hide vip, got %#v", got)
	}
}

func TestGetUserGroupOptionsUsesUserGroupsNotPricingGroups(t *testing.T) {
	truncate(t)
	restoreTopupGroupRatio := snapshotTopupGroupRatio(t)
	restoreGroupGroupRatio := snapshotGroupGroupRatio(t)
	restoreSpecialUsableGroups := snapshotSpecialUsableGroups(t)
	defer restoreTopupGroupRatio()
	defer restoreGroupGroupRatio()
	defer restoreSpecialUsableGroups()

	if err := common.UpdateTopupGroupRatioByJSONString(`{"xhj":1,"default":1}`); err != nil {
		t.Fatalf("UpdateTopupGroupRatioByJSONString() error = %v", err)
	}
	groupRatioSetting := ratio_setting.GetGroupRatioSetting()
	groupRatioSetting.GroupGroupRatio.Clear()
	groupRatioSetting.GroupGroupRatio.Set("jhl", map[string]float64{
		"拼车": 0.8,
	})
	groupRatioSetting.GroupSpecialUsableGroup.Clear()
	groupRatioSetting.GroupSpecialUsableGroup.Set("zz", map[string]string{
		"=:lzz_plus": "",
		"=:拼车":       "",
	})

	if err := model.DB.Create(&model.User{
		Id:       1001,
		Username: "lzz-user",
		Group:    "lzz",
	}).Error; err != nil {
		t.Fatalf("create user error = %v", err)
	}

	groups, err := GetUserGroupOptions()
	if err != nil {
		t.Fatalf("GetUserGroupOptions() error = %v", err)
	}
	groupSet := make(map[string]struct{}, len(groups))
	for _, group := range groups {
		groupSet[group] = struct{}{}
	}

	for _, group := range []string{"default", "lzz", "xhj", "jhl", "zz"} {
		if _, ok := groupSet[group]; !ok {
			t.Fatalf("expected user group %q in options, got %#v", group, groups)
		}
	}
	for _, pricingGroup := range []string{"拼车", "lzz_plus"} {
		if _, ok := groupSet[pricingGroup]; ok {
			t.Fatalf("pricing group %q should not be a user group option, got %#v", pricingGroup, groups)
		}
	}
}

func snapshotGroupSettings(t *testing.T) (func(), func()) {
	t.Helper()

	userUsableGroupsJSON := setting.UserUsableGroups2JSONString()
	specialUsableGroups := ratio_setting.GetGroupRatioSetting().GroupSpecialUsableGroup.ReadAll()

	restoreUserUsableGroups := func() {
		if err := setting.UpdateUserUsableGroupsByJSONString(userUsableGroupsJSON); err != nil {
			t.Fatalf("restore UserUsableGroups error = %v", err)
		}
	}
	restoreSpecialUsableGroups := func() {
		target := ratio_setting.GetGroupRatioSetting().GroupSpecialUsableGroup
		target.Clear()
		target.AddAll(specialUsableGroups)
	}
	return restoreUserUsableGroups, restoreSpecialUsableGroups
}

func snapshotTopupGroupRatio(t *testing.T) func() {
	t.Helper()
	topupGroupRatioJSON := common.TopupGroupRatio2JSONString()
	return func() {
		if err := common.UpdateTopupGroupRatioByJSONString(topupGroupRatioJSON); err != nil {
			t.Fatalf("restore TopupGroupRatio error = %v", err)
		}
	}
}

func snapshotGroupGroupRatio(t *testing.T) func() {
	t.Helper()
	groupGroupRatio := ratio_setting.GetGroupRatioSetting().GroupGroupRatio.ReadAll()
	return func() {
		target := ratio_setting.GetGroupRatioSetting().GroupGroupRatio
		target.Clear()
		target.AddAll(groupGroupRatio)
	}
}

func snapshotSpecialUsableGroups(t *testing.T) func() {
	t.Helper()
	specialUsableGroups := ratio_setting.GetGroupRatioSetting().GroupSpecialUsableGroup.ReadAll()
	return func() {
		target := ratio_setting.GetGroupRatioSetting().GroupSpecialUsableGroup
		target.Clear()
		target.AddAll(specialUsableGroups)
	}
}
