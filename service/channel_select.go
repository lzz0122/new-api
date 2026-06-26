package service

import (
	"errors"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/gin-gonic/gin"
)

type RetryParam struct {
	Ctx          *gin.Context
	TokenGroup   string
	ModelName    string
	Retry        *int
	resetNextTry bool
}

func (p *RetryParam) GetRetry() int {
	if p.Retry == nil {
		return 0
	}
	return *p.Retry
}

func (p *RetryParam) SetRetry(retry int) {
	p.Retry = &retry
}

func (p *RetryParam) IncreaseRetry() {
	if p.resetNextTry {
		p.resetNextTry = false
		return
	}
	if p.Retry == nil {
		p.Retry = new(int)
	}
	*p.Retry++
}

func (p *RetryParam) ResetRetryNextTry() {
	p.resetNextTry = true
}

func requestFailedChannelKey(group string, modelName string) string {
	return group + "\x00" + modelName
}

func MarkRequestChannelFailed(c *gin.Context, group string, modelName string, channelID int) {
	if c == nil || group == "" || modelName == "" || channelID <= 0 {
		return
	}
	failures, _ := common.GetContextKeyType[map[string]map[int]struct{}](c, constant.ContextKeyFailedChannelIds)
	if failures == nil {
		failures = make(map[string]map[int]struct{})
	}
	key := requestFailedChannelKey(group, modelName)
	if failures[key] == nil {
		failures[key] = make(map[int]struct{})
	}
	failures[key][channelID] = struct{}{}
	common.SetContextKey(c, constant.ContextKeyFailedChannelIds, failures)
}

func getRequestFailedChannelIDs(c *gin.Context, group string, modelName string) map[int]struct{} {
	if c == nil || group == "" || modelName == "" {
		return nil
	}
	failures, ok := common.GetContextKeyType[map[string]map[int]struct{}](c, constant.ContextKeyFailedChannelIds)
	if !ok || len(failures) == 0 {
		return nil
	}
	source := failures[requestFailedChannelKey(group, modelName)]
	if len(source) == 0 {
		return nil
	}
	copied := make(map[int]struct{}, len(source))
	for id := range source {
		copied[id] = struct{}{}
	}
	return copied
}

func requestFailedChannelExclusions(c *gin.Context, group string, modelName string) map[int]struct{} {
	if ChannelHealthEnabled() {
		return nil
	}
	return getRequestFailedChannelIDs(c, group, modelName)
}

func setSelectedTokenGroupContext(c *gin.Context, cfg model.TokenGroupConfig, group string) {
	common.SetContextKey(c, constant.ContextKeyUsingGroup, group)
	settings := effectiveTokenGroupItem(cfg, group)
	timeoutSeconds := settings.TimeoutSeconds
	if ChannelHealthEnabled() {
		timeoutSeconds = 0
	}
	common.SetContextKey(c, constant.ContextKeyTokenGroupTimeout, timeoutSeconds)
}

func getTokenGroupPreferredSatisfiedChannel(tokenID int, group string, modelName string, excludedIDs map[int]struct{}) (*model.Channel, error) {
	if lastChannelID := getTokenGroupLastSuccessfulChannel(tokenID, group, modelName); lastChannelID > 0 {
		channel, err := model.GetRandomSatisfiedChannelFromIDsExcluding(group, modelName, 0, []int{lastChannelID}, excludedIDs)
		if err != nil || channel != nil {
			return channel, err
		}
	}
	preferredIDs := getTokenGroupPreferredChannels(tokenID, group, modelName)
	if len(preferredIDs) == 0 {
		return nil, nil
	}
	channel, err := model.GetRandomSatisfiedChannelFromIDsExcluding(group, modelName, 0, preferredIDs, excludedIDs)
	if err != nil || channel != nil {
		return channel, err
	}
	setTokenGroupPreferredChannels(tokenID, group, modelName, nil)
	return nil, nil
}

// CacheGetRandomSatisfiedChannel tries to get a random channel that satisfies the requirements.
// 尝试获取一个满足要求的随机渠道。
//
// For "auto" tokenGroup with cross-group Retry enabled:
// 对于启用了跨分组重试的 "auto" tokenGroup：
//
//   - Each group will exhaust all its priorities before moving to the next group.
//     每个分组会用完所有优先级后才会切换到下一个分组。
//
//   - Uses ContextKeyAutoGroupIndex to track current group index.
//     使用 ContextKeyAutoGroupIndex 跟踪当前分组索引。
//
//   - Uses ContextKeyAutoGroupRetryIndex to track the global Retry count when current group started.
//     使用 ContextKeyAutoGroupRetryIndex 跟踪当前分组开始时的全局重试次数。
//
//   - priorityRetry = Retry - startRetryIndex, represents the priority level within current group.
//     priorityRetry = Retry - startRetryIndex，表示当前分组内的优先级级别。
//
//   - When GetRandomSatisfiedChannel returns nil (priorities exhausted), moves to next group.
//     当 GetRandomSatisfiedChannel 返回 nil（优先级用完）时，切换到下一个分组。
//
// Example flow (2 groups, each with 2 priorities, RetryTimes=3):
// 示例流程（2个分组，每个有2个优先级，RetryTimes=3）：
//
//	Retry=0: GroupA, priority0 (startRetryIndex=0, priorityRetry=0)
//	         分组A, 优先级0
//
//	Retry=1: GroupA, priority1 (startRetryIndex=0, priorityRetry=1)
//	         分组A, 优先级1
//
//	Retry=2: GroupA exhausted → GroupB, priority0 (startRetryIndex=2, priorityRetry=0)
//	         分组A用完 → 分组B, 优先级0
//
//	Retry=3: GroupB, priority1 (startRetryIndex=2, priorityRetry=1)
//	         分组B, 优先级1
func CacheGetRandomSatisfiedChannel(param *RetryParam) (*model.Channel, string, error) {
	var channel *model.Channel
	var err error
	selectGroup := param.TokenGroup
	userGroup := common.GetContextKeyString(param.Ctx, constant.ContextKeyUserGroup)

	if cfg, ok := GetTokenGroupConfigFromContext(param.Ctx); ok && len(cfg.Groups) > 1 && param.TokenGroup != "auto" {
		tokenID := common.GetContextKeyInt(param.Ctx, constant.ContextKeyTokenId)
		startGroupIndex := 0
		stickyRecoveredIndexes := make([]int, 0)
		if lastGroupIndex, exists := common.GetContextKey(param.Ctx, constant.ContextKeyTokenGroupIndex); exists {
			if idx, ok := lastGroupIndex.(int); ok {
				startGroupIndex = idx
			}
		}
		for i := startGroupIndex; i < len(cfg.Groups); i++ {
			group := cfg.Groups[i].Group
			if cooling, reason, until := IsTokenGroupModelCooling(tokenID, group, param.ModelName, cfg); cooling {
				AppendTokenGroupFailure(param.Ctx, FormatTokenGroupCoolingReason(group, reason, until))
				if until == 0 && tokenGroupRecoveryStrategy(cfg, group) == model.TokenGroupRecoveryStrategySticky {
					stickyRecoveredIndexes = append(stickyRecoveredIndexes, i)
				}
				common.SetContextKey(param.Ctx, constant.ContextKeyTokenGroupIndex, i+1)
				param.SetRetry(0)
				continue
			}
			priorityRetry := param.GetRetry()
			if i > startGroupIndex {
				priorityRetry = 0
			}
			logger.LogDebug(param.Ctx, "Token selecting group: %s, priorityRetry: %d", group, priorityRetry)
			excludedIDs := requestFailedChannelExclusions(param.Ctx, group, param.ModelName)
			if priorityRetry == 0 {
				channel, err = getTokenGroupPreferredSatisfiedChannel(tokenID, group, param.ModelName, excludedIDs)
				if err != nil {
					return nil, group, err
				}
			}
			if channel == nil {
				channel, err = model.GetRandomSatisfiedChannelExcluding(group, param.ModelName, priorityRetry, excludedIDs)
			}
			if err != nil {
				return nil, group, err
			}
			if channel == nil {
				if ChannelHealthEnabled() {
					AppendTokenGroupFailure(param.Ctx, group+" 分组当前无可用渠道")
				}
				common.SetContextKey(param.Ctx, constant.ContextKeyTokenGroupIndex, i+1)
				param.SetRetry(0)
				continue
			}
			setSelectedTokenGroupContext(param.Ctx, cfg, group)
			selectGroup = group
			if priorityRetry >= common.RetryTimes {
				common.SetContextKey(param.Ctx, constant.ContextKeyTokenGroupIndex, i+1)
				param.SetRetry(0)
				param.ResetRetryNextTry()
			} else {
				common.SetContextKey(param.Ctx, constant.ContextKeyTokenGroupIndex, i)
			}
			return channel, selectGroup, nil
		}
		for _, i := range stickyRecoveredIndexes {
			group := cfg.Groups[i].Group
			logger.LogDebug(param.Ctx, "Token selecting sticky recovered group: %s", group)
			excludedIDs := requestFailedChannelExclusions(param.Ctx, group, param.ModelName)
			channel, err = getTokenGroupPreferredSatisfiedChannel(tokenID, group, param.ModelName, excludedIDs)
			if err != nil {
				return nil, group, err
			}
			if channel == nil {
				channel, err = model.GetRandomSatisfiedChannelExcluding(group, param.ModelName, 0, excludedIDs)
			}
			if err != nil {
				return nil, group, err
			}
			if channel == nil {
				continue
			}
			setSelectedTokenGroupContext(param.Ctx, cfg, group)
			common.SetContextKey(param.Ctx, constant.ContextKeyTokenGroupIndex, i)
			param.SetRetry(0)
			return channel, group, nil
		}
		return nil, selectGroup, nil
	}

	if param.TokenGroup == "auto" {
		if len(setting.GetAutoGroups()) == 0 {
			return nil, selectGroup, errors.New("auto groups is not enabled")
		}
		autoGroups := GetUserAutoGroup(userGroup)
		tokenID := common.GetContextKeyInt(param.Ctx, constant.ContextKeyTokenId)

		// startGroupIndex: the group index to start searching from
		// startGroupIndex: 开始搜索的分组索引
		startGroupIndex := 0
		crossGroupRetry := common.GetContextKeyBool(param.Ctx, constant.ContextKeyTokenCrossGroupRetry)

		if lastGroupIndex, exists := common.GetContextKey(param.Ctx, constant.ContextKeyAutoGroupIndex); exists {
			if idx, ok := lastGroupIndex.(int); ok {
				startGroupIndex = idx
			}
		}

		for i := startGroupIndex; i < len(autoGroups); i++ {
			autoGroup := autoGroups[i]
			// Calculate priorityRetry for current group
			// 计算当前分组的 priorityRetry
			priorityRetry := param.GetRetry()
			// If moved to a new group, reset priorityRetry and update startRetryIndex
			// 如果切换到新分组，重置 priorityRetry 并更新 startRetryIndex
			if i > startGroupIndex {
				priorityRetry = 0
			}
			logger.LogDebug(param.Ctx, "Auto selecting group: %s, priorityRetry: %d", autoGroup, priorityRetry)

			excludedIDs := requestFailedChannelExclusions(param.Ctx, autoGroup, param.ModelName)
			if priorityRetry == 0 {
				channel, err = getTokenGroupPreferredSatisfiedChannel(tokenID, autoGroup, param.ModelName, excludedIDs)
				if err != nil {
					return nil, autoGroup, err
				}
			}
			if channel == nil {
				channel, _ = model.GetRandomSatisfiedChannelExcluding(autoGroup, param.ModelName, priorityRetry, excludedIDs)
			}
			if channel == nil {
				// Current group has no available channel for this model, try next group
				// 当前分组没有该模型的可用渠道，尝试下一个分组
				logger.LogDebug(param.Ctx, "No available channel in group %s for model %s at priorityRetry %d, trying next group", autoGroup, param.ModelName, priorityRetry)
				if ChannelHealthEnabled() {
					AppendTokenGroupFailure(param.Ctx, autoGroup+" 分组当前无可用渠道")
				}
				// 重置状态以尝试下一个分组
				common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroupIndex, i+1)
				common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroupRetryIndex, 0)
				// Reset retry counter so outer loop can continue for next group
				// 重置重试计数器，以便外层循环可以为下一个分组继续
				param.SetRetry(0)
				continue
			}
			common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroup, autoGroup)
			selectGroup = autoGroup
			logger.LogDebug(param.Ctx, "Auto selected group: %s", autoGroup)

			// Prepare state for next retry
			// 为下一次重试准备状态
			if crossGroupRetry && priorityRetry >= common.RetryTimes {
				// Current group has exhausted all retries, prepare to switch to next group
				// This request still uses current group, but next retry will use next group
				// 当前分组已用完所有重试次数，准备切换到下一个分组
				// 本次请求仍使用当前分组，但下次重试将使用下一个分组
				logger.LogDebug(param.Ctx, "Current group %s retries exhausted (priorityRetry=%d >= RetryTimes=%d), preparing switch to next group for next retry", autoGroup, priorityRetry, common.RetryTimes)
				common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroupIndex, i+1)
				// Reset retry counter so outer loop can continue for next group
				// 重置重试计数器，以便外层循环可以为下一个分组继续
				param.SetRetry(0)
				param.ResetRetryNextTry()
			} else {
				// Stay in current group, save current state
				// 保持在当前分组，保存当前状态
				common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroupIndex, i)
			}
			break
		}
	} else {
		channel, err = model.GetRandomSatisfiedChannelExcluding(param.TokenGroup, param.ModelName, param.GetRetry(), requestFailedChannelExclusions(param.Ctx, param.TokenGroup, param.ModelName))
		if err != nil {
			return nil, param.TokenGroup, err
		}
	}
	return channel, selectGroup, nil
}
