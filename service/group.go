package service

import (
	"sort"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
)

const (
	usableGroupRemovePrefix = "-:"
	usableGroupAddPrefix    = "+:"
	usableGroupOnlyPrefix   = "=:"
)

func GetUserUsableGroups(userGroup string) map[string]string {
	groupsCopy := setting.GetUserUsableGroupsCopy()
	onlyMode := false
	if userGroup != "" {
		specialSettings, b := ratio_setting.GetGroupRatioSetting().GroupSpecialUsableGroup.Get(userGroup)
		if b {
			for specialGroup := range specialSettings {
				if strings.HasPrefix(specialGroup, usableGroupOnlyPrefix) {
					onlyMode = true
					break
				}
			}
			if onlyMode {
				groupsCopy = make(map[string]string)
			}
			// 处理特殊可用分组
			for specialGroup, desc := range specialSettings {
				if strings.HasPrefix(specialGroup, usableGroupRemovePrefix) {
					// 移除分组
					groupToRemove := strings.TrimPrefix(specialGroup, usableGroupRemovePrefix)
					delete(groupsCopy, groupToRemove)
				} else if strings.HasPrefix(specialGroup, usableGroupAddPrefix) {
					// 添加分组
					groupToAdd := strings.TrimPrefix(specialGroup, usableGroupAddPrefix)
					groupsCopy[groupToAdd] = desc
				} else if strings.HasPrefix(specialGroup, usableGroupOnlyPrefix) {
					// 白名单分组；存在任意白名单规则时，不继承默认用户可选分组
					groupToAdd := strings.TrimPrefix(specialGroup, usableGroupOnlyPrefix)
					groupsCopy[groupToAdd] = desc
				} else {
					// 直接添加分组
					groupsCopy[specialGroup] = desc
				}
			}
		}
		// 如果userGroup不在UserUsableGroups中，返回UserUsableGroups + userGroup
		if _, ok := groupsCopy[userGroup]; !ok && !onlyMode {
			groupsCopy[userGroup] = "用户分组"
		}
	}
	return groupsCopy
}

func GroupInUserUsableGroups(userGroup, groupName string) bool {
	_, ok := GetUserUsableGroups(userGroup)[groupName]
	return ok
}

func GetUserGroupOptions() ([]string, error) {
	groupSet := map[string]struct{}{
		"default": {},
	}
	addGroup := func(group string) {
		group = strings.TrimSpace(group)
		if group == "" {
			return
		}
		groupSet[group] = struct{}{}
	}

	userGroups, err := model.ListUserGroups()
	if err != nil {
		return nil, err
	}
	for _, group := range userGroups {
		addGroup(group)
	}
	for group := range common.GetTopupGroupRatioCopy() {
		addGroup(group)
	}
	groupRatioSetting := ratio_setting.GetGroupRatioSetting()
	for group := range groupRatioSetting.GroupGroupRatio.ReadAll() {
		addGroup(group)
	}
	for group := range groupRatioSetting.GroupSpecialUsableGroup.ReadAll() {
		addGroup(group)
	}

	groups := make([]string, 0, len(groupSet))
	for group := range groupSet {
		groups = append(groups, group)
	}
	sort.Strings(groups)
	return groups, nil
}

// GetUserAutoGroup 根据用户分组获取自动分组设置
func GetUserAutoGroup(userGroup string) []string {
	groups := GetUserUsableGroups(userGroup)
	autoGroups := make([]string, 0)
	for _, group := range setting.GetAutoGroups() {
		if _, ok := groups[group]; ok {
			autoGroups = append(autoGroups, group)
		}
	}
	return autoGroups
}

// GetUserGroupRatio 获取用户使用某个分组的倍率
// userGroup 用户分组
// group 需要获取倍率的分组
func GetUserGroupRatio(userGroup, group string) float64 {
	ratio, ok := ratio_setting.GetGroupGroupRatio(userGroup, group)
	if ok {
		return ratio
	}
	return ratio_setting.GetGroupRatio(group)
}
