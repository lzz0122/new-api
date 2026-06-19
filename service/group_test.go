package service

import (
	"testing"

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
		"+:jhl":      "jhl分组",
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
		"=:拼车": "拼车分组",
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
