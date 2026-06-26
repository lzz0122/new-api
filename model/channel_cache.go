package model

import (
	"errors"
	"fmt"
	"math/rand"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
)

var group2model2channels map[string]map[string][]int // enabled channel
var channelsIDM map[int]*Channel                     // all channels include disabled
var channelSyncLock sync.RWMutex

func InitChannelCache() {
	if !common.MemoryCacheEnabled {
		return
	}
	newChannelId2channel := make(map[int]*Channel)
	var channels []*Channel
	DB.Find(&channels)
	for _, channel := range channels {
		newChannelId2channel[channel.Id] = channel
	}
	var abilities []*Ability
	DB.Find(&abilities)
	groups := make(map[string]bool)
	for _, ability := range abilities {
		groups[ability.Group] = true
	}
	newGroup2model2channels := make(map[string]map[string][]int)
	for group := range groups {
		newGroup2model2channels[group] = make(map[string][]int)
	}
	for _, channel := range channels {
		if channel.Status != common.ChannelStatusEnabled {
			continue // skip disabled channels
		}
		groups := strings.Split(channel.Group, ",")
		for _, group := range groups {
			models := strings.Split(channel.Models, ",")
			for _, model := range models {
				if _, ok := newGroup2model2channels[group][model]; !ok {
					newGroup2model2channels[group][model] = make([]int, 0)
				}
				newGroup2model2channels[group][model] = append(newGroup2model2channels[group][model], channel.Id)
			}
		}
	}

	// sort by priority
	for group, model2channels := range newGroup2model2channels {
		for model, channels := range model2channels {
			sort.Slice(channels, func(i, j int) bool {
				return newChannelId2channel[channels[i]].GetPriority() > newChannelId2channel[channels[j]].GetPriority()
			})
			newGroup2model2channels[group][model] = channels
		}
	}

	channelSyncLock.Lock()
	group2model2channels = newGroup2model2channels
	//channelsIDM = newChannelId2channel
	for i, channel := range newChannelId2channel {
		if channel.ChannelInfo.IsMultiKey {
			channel.Keys = channel.GetKeys()
			if channel.ChannelInfo.MultiKeyMode == constant.MultiKeyModePolling {
				if oldChannel, ok := channelsIDM[i]; ok {
					// 存在旧的渠道，如果是多key且轮询，保留轮询索引信息
					if oldChannel.ChannelInfo.IsMultiKey && oldChannel.ChannelInfo.MultiKeyMode == constant.MultiKeyModePolling {
						channel.ChannelInfo.MultiKeyPollingIndex = oldChannel.ChannelInfo.MultiKeyPollingIndex
					}
				}
			}
		}
	}
	channelsIDM = newChannelId2channel
	channelSyncLock.Unlock()
	common.SysLog("channels synced from database")
}

func SyncChannelCache(frequency int) {
	for {
		time.Sleep(time.Duration(frequency) * time.Second)
		common.SysLog("syncing channels from database")
		InitChannelCache()
	}
}

func GetRandomSatisfiedChannel(group string, model string, retry int) (*Channel, error) {
	// if memory cache is disabled, get channel directly from database
	if !common.MemoryCacheEnabled {
		return GetChannel(group, model, retry)
	}

	channelSyncLock.RLock()
	defer channelSyncLock.RUnlock()

	// First, try to find channels with the exact model name.
	channels := group2model2channels[group][model]

	// If no channels found, try to find channels with the normalized model name.
	if len(channels) == 0 {
		normalizedModel := ratio_setting.FormatMatchingModelName(model)
		channels = group2model2channels[group][normalizedModel]
	}

	if len(channels) == 0 {
		return nil, nil
	}
	channels = filterRoutableChannelIDs(group, channels)
	if len(channels) == 0 {
		return nil, nil
	}

	if len(channels) == 1 {
		if channel, ok := channelsIDM[channels[0]]; ok {
			return channel, nil
		}
		return nil, fmt.Errorf("数据库一致性错误，渠道# %d 不存在，请联系管理员修复", channels[0])
	}

	uniquePriorities := make(map[int]bool)
	for _, channelId := range channels {
		if channel, ok := channelsIDM[channelId]; ok {
			uniquePriorities[int(channel.GetPriority())] = true
		} else {
			return nil, fmt.Errorf("数据库一致性错误，渠道# %d 不存在，请联系管理员修复", channelId)
		}
	}
	var sortedUniquePriorities []int
	for priority := range uniquePriorities {
		sortedUniquePriorities = append(sortedUniquePriorities, priority)
	}
	sort.Sort(sort.Reverse(sort.IntSlice(sortedUniquePriorities)))

	if retry >= len(uniquePriorities) {
		retry = len(uniquePriorities) - 1
	}
	targetPriority := int64(sortedUniquePriorities[retry])

	// get the priority for the given retry number
	var sumWeight = 0
	var targetChannels []*Channel
	for _, channelId := range channels {
		if channel, ok := channelsIDM[channelId]; ok {
			if channel.GetPriority() == targetPriority {
				sumWeight += channel.GetWeight()
				targetChannels = append(targetChannels, channel)
			}
		} else {
			return nil, fmt.Errorf("数据库一致性错误，渠道# %d 不存在，请联系管理员修复", channelId)
		}
	}

	if len(targetChannels) == 0 {
		return nil, errors.New(fmt.Sprintf("no channel found, group: %s, model: %s, priority: %d", group, model, targetPriority))
	}

	// smoothing factor and adjustment
	smoothingFactor := 1
	smoothingAdjustment := 0

	if sumWeight == 0 {
		// when all channels have weight 0, set sumWeight to the number of channels and set smoothing adjustment to 100
		// each channel's effective weight = 100
		sumWeight = len(targetChannels) * 100
		smoothingAdjustment = 100
	} else if sumWeight/len(targetChannels) < 10 {
		// when the average weight is less than 10, set smoothing factor to 100
		smoothingFactor = 100
	}

	// Calculate the total weight of all channels up to endIdx
	totalWeight := sumWeight * smoothingFactor

	// Generate a random value in the range [0, totalWeight)
	randomWeight := rand.Intn(totalWeight)

	// Find a channel based on its weight
	for _, channel := range targetChannels {
		randomWeight -= channel.GetWeight()*smoothingFactor + smoothingAdjustment
		if randomWeight < 0 {
			return channel, nil
		}
	}
	// return null if no channel is not found
	return nil, errors.New("channel not found")
}

func GetRandomSatisfiedChannelExcluding(group string, model string, retry int, excludedIDs map[int]struct{}) (*Channel, error) {
	if len(excludedIDs) == 0 {
		return GetRandomSatisfiedChannel(group, model, retry)
	}
	channels, err := GetSatisfiedChannels(group, model)
	if err != nil || len(channels) == 0 {
		return nil, err
	}
	filtered := make([]*Channel, 0, len(channels))
	for _, channel := range channels {
		if _, excluded := excludedIDs[channel.Id]; excluded {
			continue
		}
		filtered = append(filtered, channel)
	}
	if len(filtered) == 0 {
		return nil, nil
	}
	return getRandomChannelFromCandidates(filtered, retry)
}

func GetSatisfiedChannels(group string, model string) ([]*Channel, error) {
	if !common.MemoryCacheEnabled {
		return getSatisfiedChannelsFromDB(group, model)
	}

	channelSyncLock.RLock()
	defer channelSyncLock.RUnlock()

	channelIDs := group2model2channels[group][model]
	if len(channelIDs) == 0 {
		normalizedModel := ratio_setting.FormatMatchingModelName(model)
		channelIDs = group2model2channels[group][normalizedModel]
	}
	if len(channelIDs) == 0 {
		return nil, nil
	}
	channelIDs = filterRoutableChannelIDs(group, channelIDs)
	if len(channelIDs) == 0 {
		return nil, nil
	}

	channels := make([]*Channel, 0, len(channelIDs))
	for _, channelID := range channelIDs {
		channel, ok := channelsIDM[channelID]
		if !ok {
			return nil, fmt.Errorf("数据库一致性错误，渠道# %d 不存在，请联系管理员修复", channelID)
		}
		channels = append(channels, channel)
	}
	return channels, nil
}

func GetRandomSatisfiedChannelFromIDs(group string, model string, retry int, preferredIDs []int) (*Channel, error) {
	if len(preferredIDs) == 0 {
		return nil, nil
	}
	preferred := make(map[int]struct{}, len(preferredIDs))
	for _, id := range preferredIDs {
		preferred[id] = struct{}{}
	}
	channels, err := GetSatisfiedChannels(group, model)
	if err != nil || len(channels) == 0 {
		return nil, err
	}
	filtered := make([]*Channel, 0, len(channels))
	for _, channel := range channels {
		if _, ok := preferred[channel.Id]; ok {
			filtered = append(filtered, channel)
		}
	}
	if len(filtered) == 0 {
		return nil, nil
	}
	return getRandomChannelFromCandidates(filtered, retry)
}

func GetRandomSatisfiedChannelFromIDsExcluding(group string, model string, retry int, preferredIDs []int, excludedIDs map[int]struct{}) (*Channel, error) {
	if len(excludedIDs) == 0 {
		return GetRandomSatisfiedChannelFromIDs(group, model, retry, preferredIDs)
	}
	if len(preferredIDs) == 0 {
		return nil, nil
	}
	preferred := make(map[int]struct{}, len(preferredIDs))
	for _, id := range preferredIDs {
		preferred[id] = struct{}{}
	}
	channels, err := GetSatisfiedChannels(group, model)
	if err != nil || len(channels) == 0 {
		return nil, err
	}
	filtered := make([]*Channel, 0, len(channels))
	for _, channel := range channels {
		if _, ok := preferred[channel.Id]; !ok {
			continue
		}
		if _, excluded := excludedIDs[channel.Id]; excluded {
			continue
		}
		filtered = append(filtered, channel)
	}
	if len(filtered) == 0 {
		return nil, nil
	}
	return getRandomChannelFromCandidates(filtered, retry)
}

func getSatisfiedChannelsFromDB(group string, modelName string) ([]*Channel, error) {
	abilities, err := getSatisfiedAbilities(group, modelName)
	if err != nil || len(abilities) == 0 {
		return nil, err
	}
	channelIDs := make([]int, 0, len(abilities))
	seen := make(map[int]struct{}, len(abilities))
	for _, ability := range abilities {
		if _, ok := seen[ability.ChannelId]; ok {
			continue
		}
		seen[ability.ChannelId] = struct{}{}
		channelIDs = append(channelIDs, ability.ChannelId)
	}
	channelsByID := make(map[int]*Channel, len(channelIDs))
	var channels []*Channel
	if err := DB.Where("id IN ?", channelIDs).Find(&channels).Error; err != nil {
		return nil, err
	}
	for _, channel := range channels {
		channelsByID[channel.Id] = channel
	}
	ordered := make([]*Channel, 0, len(channelIDs))
	for _, id := range channelIDs {
		channel, ok := channelsByID[id]
		if !ok {
			return nil, fmt.Errorf("数据库一致性错误，渠道# %d 不存在，请联系管理员修复", id)
		}
		ordered = append(ordered, channel)
	}
	return filterRoutableChannels(group, ordered), nil
}

func filterRoutableChannelIDs(group string, channelIDs []int) []int {
	if len(channelIDs) == 0 || !operation_setting.GetChannelHealthSetting().Enabled {
		return channelIDs
	}
	threshold := GetChannelHealthGroupFailureThreshold(group)
	if threshold <= 0 {
		return channelIDs
	}
	var states []ChannelHealthState
	err := DB.Where("channel_id IN ?", channelIDs).Find(&states).Error
	if err != nil {
		return channelIDs
	}
	globalStateMap := make(map[int]ChannelHealthState, len(states))
	for _, state := range states {
		globalStateMap[state.ChannelId] = state
	}
	var groupStates []ChannelHealthGroupState
	if err := DB.Where("channel_id IN ? AND group_name = ?", channelIDs, strings.TrimSpace(group)).Find(&groupStates).Error; err != nil {
		return channelIDs
	}
	groupStateMap := make(map[int]ChannelHealthGroupState, len(groupStates))
	for _, state := range groupStates {
		groupStateMap[state.ChannelId] = state
	}
	unhealthySet := make(map[int]struct{}, len(channelIDs))
	for _, channelID := range channelIDs {
		globalState, hasGlobalState := globalStateMap[channelID]
		groupState, hasGroupState := groupStateMap[channelID]
		var globalPtr *ChannelHealthState
		if hasGlobalState {
			globalPtr = &globalState
		}
		var groupPtr *ChannelHealthGroupState
		if hasGroupState {
			groupPtr = &groupState
		}
		if IsChannelHealthUnavailableForGroupState(globalPtr, groupPtr, group, threshold) {
			unhealthySet[channelID] = struct{}{}
		}
	}
	if len(unhealthySet) == 0 {
		return channelIDs
	}
	filtered := make([]int, 0, len(channelIDs))
	for _, id := range channelIDs {
		if _, unhealthy := unhealthySet[id]; unhealthy {
			continue
		}
		filtered = append(filtered, id)
	}
	return filtered
}

func filterRoutableChannels(group string, channels []*Channel) []*Channel {
	if len(channels) == 0 || !operation_setting.GetChannelHealthSetting().Enabled {
		return channels
	}
	channelIDs := make([]int, 0, len(channels))
	for _, channel := range channels {
		if channel != nil {
			channelIDs = append(channelIDs, channel.Id)
		}
	}
	routableIDs := filterRoutableChannelIDs(group, channelIDs)
	if len(routableIDs) == len(channelIDs) {
		return channels
	}
	routableSet := make(map[int]struct{}, len(routableIDs))
	for _, id := range routableIDs {
		routableSet[id] = struct{}{}
	}
	filtered := make([]*Channel, 0, len(channels))
	for _, channel := range channels {
		if channel == nil {
			continue
		}
		if _, ok := routableSet[channel.Id]; ok {
			filtered = append(filtered, channel)
		}
	}
	return filtered
}

func getSatisfiedAbilities(group string, modelName string) ([]Ability, error) {
	var abilities []Ability
	query := DB.Where(commonGroupCol+" = ? and model = ? and enabled = ?", group, modelName, true).
		Order("priority DESC").Order("weight DESC")
	if err := query.Find(&abilities).Error; err != nil {
		return nil, err
	}
	if len(abilities) > 0 {
		return abilities, nil
	}
	normalizedModel := ratio_setting.FormatMatchingModelName(modelName)
	if normalizedModel == modelName {
		return abilities, nil
	}
	query = DB.Where(commonGroupCol+" = ? and model = ? and enabled = ?", group, normalizedModel, true).
		Order("priority DESC").Order("weight DESC")
	if err := query.Find(&abilities).Error; err != nil {
		return nil, err
	}
	return abilities, nil
}

func getRandomChannelFromCandidates(candidates []*Channel, retry int) (*Channel, error) {
	if len(candidates) == 0 {
		return nil, nil
	}
	if len(candidates) == 1 {
		return candidates[0], nil
	}
	uniquePriorities := make(map[int]bool)
	for _, channel := range candidates {
		uniquePriorities[int(channel.GetPriority())] = true
	}
	var sortedUniquePriorities []int
	for priority := range uniquePriorities {
		sortedUniquePriorities = append(sortedUniquePriorities, priority)
	}
	sort.Sort(sort.Reverse(sort.IntSlice(sortedUniquePriorities)))

	if retry >= len(uniquePriorities) {
		retry = len(uniquePriorities) - 1
	}
	targetPriority := int64(sortedUniquePriorities[retry])

	var sumWeight int
	var targetChannels []*Channel
	for _, channel := range candidates {
		if channel.GetPriority() == targetPriority {
			sumWeight += channel.GetWeight()
			targetChannels = append(targetChannels, channel)
		}
	}
	if len(targetChannels) == 0 {
		return nil, errors.New(fmt.Sprintf("no channel found, priority: %d", targetPriority))
	}

	smoothingFactor := 1
	smoothingAdjustment := 0
	if sumWeight == 0 {
		sumWeight = len(targetChannels) * 100
		smoothingAdjustment = 100
	} else if sumWeight/len(targetChannels) < 10 {
		smoothingFactor = 100
	}
	totalWeight := sumWeight * smoothingFactor
	randomWeight := rand.Intn(totalWeight)
	for _, channel := range targetChannels {
		randomWeight -= channel.GetWeight()*smoothingFactor + smoothingAdjustment
		if randomWeight < 0 {
			return channel, nil
		}
	}
	return nil, errors.New("channel not found")
}

func CacheGetChannel(id int) (*Channel, error) {
	if !common.MemoryCacheEnabled {
		return GetChannelById(id, true)
	}
	channelSyncLock.RLock()
	defer channelSyncLock.RUnlock()

	c, ok := channelsIDM[id]
	if !ok {
		return nil, fmt.Errorf("渠道# %d，已不存在", id)
	}
	return c, nil
}

func CacheGetChannelInfo(id int) (*ChannelInfo, error) {
	if !common.MemoryCacheEnabled {
		channel, err := GetChannelById(id, true)
		if err != nil {
			return nil, err
		}
		return &channel.ChannelInfo, nil
	}
	channelSyncLock.RLock()
	defer channelSyncLock.RUnlock()

	c, ok := channelsIDM[id]
	if !ok {
		return nil, fmt.Errorf("渠道# %d，已不存在", id)
	}
	return &c.ChannelInfo, nil
}

func CacheUpdateChannelStatus(id int, status int) {
	if !common.MemoryCacheEnabled {
		return
	}
	channelSyncLock.Lock()
	defer channelSyncLock.Unlock()
	var target *Channel
	if channel, ok := channelsIDM[id]; ok {
		channel.Status = status
		target = channel
	}

	removeChannelFromGroupModelCacheLocked(id)
	if status == common.ChannelStatusEnabled && target != nil {
		addChannelToGroupModelCacheLocked(target)
	}
}

func removeChannelFromGroupModelCacheLocked(id int) {
	for group, model2channels := range group2model2channels {
		for modelName, channels := range model2channels {
			filtered := channels[:0]
			for _, channelId := range channels {
				if channelId != id {
					filtered = append(filtered, channelId)
				}
			}
			group2model2channels[group][modelName] = filtered
		}
	}
}

func addChannelToGroupModelCacheLocked(channel *Channel) {
	if channel == nil || channel.Status != common.ChannelStatusEnabled {
		return
	}
	if group2model2channels == nil {
		group2model2channels = make(map[string]map[string][]int)
	}
	groups := strings.Split(channel.Group, ",")
	models := strings.Split(channel.Models, ",")
	for _, group := range groups {
		group = strings.TrimSpace(group)
		if group == "" {
			continue
		}
		if group2model2channels[group] == nil {
			group2model2channels[group] = make(map[string][]int)
		}
		for _, modelName := range models {
			modelName = strings.TrimSpace(modelName)
			if modelName == "" {
				continue
			}
			channels := group2model2channels[group][modelName]
			exists := false
			for _, id := range channels {
				if id == channel.Id {
					exists = true
					break
				}
			}
			if !exists {
				channels = append(channels, channel.Id)
			}
			sort.Slice(channels, func(i, j int) bool {
				left := channelsIDM[channels[i]]
				right := channelsIDM[channels[j]]
				if left == nil || right == nil {
					return channels[i] < channels[j]
				}
				return left.GetPriority() > right.GetPriority()
			})
			group2model2channels[group][modelName] = channels
		}
	}
}

func CacheUpdateChannel(channel *Channel) {
	if !common.MemoryCacheEnabled {
		return
	}
	channelSyncLock.Lock()
	defer channelSyncLock.Unlock()
	if channel == nil {
		return
	}

	if channelsIDM == nil {
		channelsIDM = make(map[int]*Channel)
	}
	if oldChannel, ok := channelsIDM[channel.Id]; ok {
		logger.LogDebug(nil, "CacheUpdateChannel before: id=%d, name=%s, status=%d, polling_index=%d", channel.Id, channel.Name, channel.Status, oldChannel.ChannelInfo.MultiKeyPollingIndex)
	}
	channelsIDM[channel.Id] = channel
	logger.LogDebug(nil, "CacheUpdateChannel after: id=%d, name=%s, status=%d, polling_index=%d", channel.Id, channel.Name, channel.Status, channel.ChannelInfo.MultiKeyPollingIndex)
}
