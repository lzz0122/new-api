package service

import (
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const channelHealthProbeStaleSeconds int64 = 300

type ChannelHealthContext struct {
	Group       string
	Model       string
	TokenID     int
	RequestID   string
	ChannelName string
}

type UserChannelStatusResponse struct {
	Groups    []UserChannelStatusGroup `json:"groups"`
	UpdatedAt int64                    `json:"updated_at"`
}

type UserChannelStatusGroup struct {
	Group            string                  `json:"group"`
	GroupName        string                  `json:"group_name"`
	FailureThreshold int                     `json:"failure_threshold"`
	Total            int                     `json:"total"`
	AvailableCount   int                     `json:"available_count"`
	ErrorCount       int                     `json:"error_count"`
	DisplayStatus    string                  `json:"display_status"`
	Channels         []UserChannelStatusItem `json:"channels"`
}

type UserChannelStatusItem struct {
	ChannelID                 int                                  `json:"channel_id"`
	ChannelName               string                               `json:"channel_name"`
	ChannelStatus             int                                  `json:"channel_status"`
	HealthStatus              string                               `json:"health_status"`
	DisplayStatus             string                               `json:"display_status"`
	NextProbeAt               int64                                `json:"next_probe_at"`
	NextProbeRemainingSeconds int64                                `json:"next_probe_remaining_seconds"`
	FailureCount              int                                  `json:"failure_count"`
	FailureThreshold          int                                  `json:"failure_threshold"`
	ProbeIntervalSeconds      int                                  `json:"probe_interval_seconds"`
	LastFailureAt             int64                                `json:"last_failure_at"`
	LastSuccessAt             int64                                `json:"last_success_at"`
	AutoProbeEnabled          bool                                 `json:"auto_probe_enabled"`
	CanProbe                  bool                                 `json:"can_probe"`
	Models                    []string                             `json:"models"`
	ProbeModels               []string                             `json:"probe_models"`
	ProbeModelResults         []model.ChannelHealthProbeModelState `json:"probe_model_results"`
}

func ChannelHealthEnabled() bool {
	return operation_setting.GetChannelHealthSetting().Enabled
}

func ShouldUseLegacyChannelAutoDisable() bool {
	return !ChannelHealthEnabled()
}

func ShouldRecordChannelHealthFailure(err *types.NewAPIError) bool {
	if !ChannelHealthEnabled() || err == nil {
		return false
	}
	if types.IsSkipRetryError(err) {
		return false
	}
	if types.IsChannelError(err) {
		return true
	}
	if operation_setting.ShouldDisableByStatusCode(err.StatusCode) {
		return true
	}
	if operation_setting.ShouldRetryByStatusCode(err.StatusCode) {
		return true
	}
	if err.StatusCode == 0 {
		return true
	}

	lowerMessage := strings.ToLower(err.Error())
	search, _ := AcSearch(lowerMessage, operation_setting.AutomaticDisableKeywords, true)
	return search
}

func RecordChannelSuccess(c *gin.Context, channelID int) {
	if !ChannelHealthEnabled() || channelID <= 0 {
		return
	}
	ctx := channelHealthContextFromGin(c, "")
	_, _ = recordChannelHealthSuccess(channelID, ctx, model.ChannelHealthEventRecovered, false)
}

func RecordChannelFailure(c *gin.Context, channel *model.Channel, err *types.NewAPIError) (*model.ChannelHealthState, bool) {
	if channel == nil || channel.Id <= 0 || !ShouldRecordChannelHealthFailure(err) {
		return nil, false
	}

	ctx := channelHealthContextFromGin(c, channel.Name)
	state, markedUnhealthy, dbErr := recordChannelHealthFailure(channel.Id, ctx, err)
	if dbErr != nil {
		common.SysLog(fmt.Sprintf("failed to record channel health failure: channel_id=%d, error=%v", channel.Id, dbErr))
		return nil, false
	}

	if state != nil &&
		state.Status == model.ChannelHealthStatusUnhealthy &&
		channelHealthUnavailableForAllAffectedGroups(state) &&
		common.AutomaticDisableChannelEnabled &&
		channel.GetAutoBan() {
		DisableChannel(*types.NewChannelError(
			channel.Id,
			channel.Type,
			channel.Name,
			channel.ChannelInfo.IsMultiKey,
			channelUsingKeyFromGin(c),
			channel.GetAutoBan(),
		), err.MaskSensitiveErrorWithStatusCode())
	}

	return decorateChannelHealthState(state), markedUnhealthy
}

func MarkChannelHealthProbing(channelID int, manual bool) (*model.ChannelHealthState, error) {
	if !ChannelHealthEnabled() || channelID <= 0 {
		return nil, nil
	}
	now := common.GetTimestamp()
	threshold := model.GetChannelHealthMinimumPositiveFailureThreshold(channelID)
	var result *model.ChannelHealthState
	err := model.DB.Transaction(func(tx *gorm.DB) error {
		state, err := getOrCreateChannelHealthStateForUpdate(tx, channelID, now, threshold)
		if err != nil {
			return err
		}
		fromStatus := state.Status
		state.Status = model.ChannelHealthStatusProbing
		state.FailureThreshold = threshold
		state.LastProbeAt = now
		state.ProbeStartedAt = now
		state.NextProbeAt = 0
		state.ProbeAttempts++
		state.UpdatedAt = now
		if manual {
			state.ManualProbeRequestedAt = now
		}
		if err := tx.Save(state).Error; err != nil {
			return err
		}
		eventType := model.ChannelHealthEventManualProbe
		if !manual {
			eventType = model.ChannelHealthEventFailureRecorded
		}
		if err := createChannelHealthEvent(tx, state, ChannelHealthContext{}, eventType, fromStatus, state.Status, 0, "", ""); err != nil {
			return err
		}
		result = state
		return nil
	})
	return decorateChannelHealthState(result), err
}

func CancelChannelHealthProbe(channelID int) {
	if !ChannelHealthEnabled() || channelID <= 0 {
		return
	}
	now := common.GetTimestamp()
	threshold := model.GetChannelHealthMinimumPositiveFailureThreshold(channelID)
	_ = model.DB.Transaction(func(tx *gorm.DB) error {
		state, err := getOrCreateChannelHealthStateForUpdate(tx, channelID, now, threshold)
		if err != nil {
			return err
		}
		if state.Status != model.ChannelHealthStatusProbing {
			return nil
		}
		state.ProbeStartedAt = 0
		state.FailureThreshold = threshold
		switch {
		case threshold > 0 && state.FailureCount >= threshold:
			state.Status = model.ChannelHealthStatusUnhealthy
			state.NextProbeAt = nextProbeAtForUnhealthyState(state, now)
		case state.FailureCount > 0:
			state.Status = model.ChannelHealthStatusSuspect
			state.NextProbeAt = 0
		default:
			state.Status = model.ChannelHealthStatusHealthy
			state.NextProbeAt = 0
		}
		state.UpdatedAt = now
		return tx.Save(state).Error
	})
}

func RecordChannelProbeSuccess(c *gin.Context, channel *model.Channel) (*model.ChannelHealthState, error) {
	if channel == nil || channel.Id <= 0 || !ChannelHealthEnabled() {
		return nil, nil
	}
	ctx := channelHealthContextFromGin(c, channel.Name)
	state, err := recordChannelHealthSuccess(channel.Id, ctx, model.ChannelHealthEventProbeSuccess, true)
	if err != nil {
		return nil, err
	}
	if channel.Status == common.ChannelStatusAutoDisabled && common.AutomaticEnableChannelEnabled {
		EnableChannel(channel.Id, channelUsingKeyFromGin(c), channel.Name)
	}
	return decorateChannelHealthState(state), nil
}

func RecordChannelProbeFailure(c *gin.Context, channel *model.Channel, err *types.NewAPIError) (*model.ChannelHealthState, error) {
	if channel == nil || channel.Id <= 0 || err == nil || !ChannelHealthEnabled() {
		return nil, nil
	}
	ctx := channelHealthContextFromGin(c, channel.Name)
	state, _, dbErr := recordChannelHealthFailureWithEvent(channel.Id, ctx, err, model.ChannelHealthEventProbeFailure, true)
	if dbErr != nil {
		return nil, dbErr
	}
	if state != nil &&
		state.Status == model.ChannelHealthStatusUnhealthy &&
		channelHealthUnavailableForAllAffectedGroups(state) &&
		common.AutomaticDisableChannelEnabled &&
		channel.Status == common.ChannelStatusEnabled &&
		channel.GetAutoBan() {
		DisableChannel(*types.NewChannelError(
			channel.Id,
			channel.Type,
			channel.Name,
			channel.ChannelInfo.IsMultiKey,
			channelUsingKeyFromGin(c),
			channel.GetAutoBan(),
		), err.MaskSensitiveErrorWithStatusCode())
	}
	return decorateChannelHealthState(state), nil
}

func MarkChannelHealthHealthy(channel *model.Channel) (*model.ChannelHealthState, error) {
	if channel == nil || channel.Id <= 0 {
		return nil, errors.New("invalid channel")
	}
	if channel.Status == common.ChannelStatusManuallyDisabled {
		return nil, errors.New("manually disabled channel cannot be marked healthy")
	}
	if !ChannelHealthEnabled() {
		if channel.Status == common.ChannelStatusAutoDisabled {
			EnableChannel(channel.Id, "", channel.Name)
		}
		return defaultChannelHealthState(channel.Id), nil
	}
	now := common.GetTimestamp()
	threshold := model.GetChannelHealthMinimumPositiveFailureThreshold(channel.Id)
	ctx := ChannelHealthContext{ChannelName: channel.Name}
	var result *model.ChannelHealthState
	err := model.DB.Transaction(func(tx *gorm.DB) error {
		state, err := getOrCreateChannelHealthStateForUpdate(tx, channel.Id, now, threshold)
		if err != nil {
			return err
		}
		fromStatus := state.Status
		state.Status = model.ChannelHealthStatusHealthy
		state.FailureCount = 0
		state.FailureThreshold = threshold
		state.LastSuccessAt = now
		state.NextProbeAt = 0
		state.ProbeStartedAt = 0
		state.LastStatusCode = http.StatusOK
		state.LastErrorCode = ""
		state.LastError = ""
		state.LastModel = ""
		state.LastGroup = ""
		state.LastTokenId = 0
		state.LastRequestId = ""
		state.UpdatedAt = now
		if err := tx.Save(state).Error; err != nil {
			return err
		}
		if err := resetChannelHealthGroupStatesTx(tx, channel.Id, now); err != nil {
			return err
		}
		if err := tx.Where("channel_id = ?", channel.Id).Delete(&model.ChannelHealthProbeModelState{}).Error; err != nil {
			return err
		}
		if err := createChannelHealthEvent(tx, state, ctx, model.ChannelHealthEventManualHealthy, fromStatus, state.Status, http.StatusOK, "", ""); err != nil {
			return err
		}
		if fromStatus == model.ChannelHealthStatusUnhealthy || fromStatus == model.ChannelHealthStatusProbing || fromStatus == model.ChannelHealthStatusSuspect {
			if err := createChannelHealthEvent(tx, state, ctx, model.ChannelHealthEventRecovered, fromStatus, state.Status, http.StatusOK, "", ""); err != nil {
				return err
			}
		}
		result = state
		return nil
	})
	if err != nil {
		return nil, err
	}
	if channel.Status == common.ChannelStatusAutoDisabled {
		EnableChannel(channel.Id, "", channel.Name)
	}
	return AttachChannelHealthProbeModelResults(decorateChannelHealthState(result), channel), nil
}

func RecordChannelProbeModelSuccess(channelID int, modelName string) error {
	if channelID <= 0 || strings.TrimSpace(modelName) == "" {
		return nil
	}
	now := common.GetTimestamp()
	state := model.ChannelHealthProbeModelState{
		ChannelId:     channelID,
		Model:         strings.TrimSpace(modelName),
		Status:        model.ChannelHealthStatusHealthy,
		LastProbeAt:   now,
		LastSuccessAt: now,
		UpdatedAt:     now,
	}
	var existing model.ChannelHealthProbeModelState
	err := model.DB.Where("channel_id = ? AND model = ?", state.ChannelId, state.Model).First(&existing).Error
	if err == nil {
		existing.Status = state.Status
		existing.LastProbeAt = now
		existing.LastSuccessAt = now
		existing.LastStatusCode = http.StatusOK
		existing.LastErrorCode = ""
		existing.LastError = ""
		existing.UpdatedAt = now
		return model.DB.Save(&existing).Error
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	state.LastStatusCode = http.StatusOK
	return model.DB.Create(&state).Error
}

func RecordChannelProbeModelFailure(channelID int, modelName string, apiErr *types.NewAPIError) error {
	if channelID <= 0 || strings.TrimSpace(modelName) == "" || apiErr == nil {
		return nil
	}
	now := common.GetTimestamp()
	modelName = strings.TrimSpace(modelName)
	var existing model.ChannelHealthProbeModelState
	err := model.DB.Where("channel_id = ? AND model = ?", channelID, modelName).First(&existing).Error
	if err == nil {
		existing.Status = model.ChannelHealthStatusUnhealthy
		existing.LastProbeAt = now
		existing.LastFailureAt = now
		existing.LastStatusCode = apiErr.StatusCode
		existing.LastErrorCode = trimForColumn(string(apiErr.GetErrorCode()), 128)
		existing.LastError = common.LocalLogPreview(apiErr.MaskSensitiveErrorWithStatusCode())
		existing.UpdatedAt = now
		return model.DB.Save(&existing).Error
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	state := model.ChannelHealthProbeModelState{
		ChannelId:      channelID,
		Model:          modelName,
		Status:         model.ChannelHealthStatusUnhealthy,
		LastProbeAt:    now,
		LastFailureAt:  now,
		LastStatusCode: apiErr.StatusCode,
		LastErrorCode:  trimForColumn(string(apiErr.GetErrorCode()), 128),
		LastError:      common.LocalLogPreview(apiErr.MaskSensitiveErrorWithStatusCode()),
		UpdatedAt:      now,
	}
	return model.DB.Create(&state).Error
}

func ListDueChannelHealthStates(limit int) ([]model.ChannelHealthState, error) {
	if !ChannelHealthEnabled() {
		return nil, nil
	}
	if limit <= 0 {
		limit = operation_setting.GetChannelHealthSetting().ProbeBatchSize
	}
	now := common.GetTimestamp()
	staleProbeBefore := now - channelHealthProbeStaleSeconds
	var states []model.ChannelHealthState
	err := model.DB.
		Where(
			"(status = ? AND ((probe_interval_seconds IS NOT NULL AND probe_interval_seconds > 0) OR (probe_interval_seconds IS NULL AND ? > 0)) AND ((next_probe_at > 0 AND next_probe_at <= ?) OR (next_probe_at <= 0 AND (CASE WHEN updated_at >= last_probe_at AND updated_at >= last_failure_at THEN updated_at WHEN last_probe_at > last_failure_at THEN last_probe_at WHEN last_failure_at > updated_at THEN last_failure_at ELSE updated_at END) + CASE WHEN probe_interval_seconds IS NOT NULL THEN probe_interval_seconds ELSE ? END <= ?))) OR (status = ? AND probe_started_at > 0 AND probe_started_at <= ?)",
			model.ChannelHealthStatusUnhealthy,
			operation_setting.GetChannelHealthSetting().ProbeIntervalSeconds,
			now,
			operation_setting.GetChannelHealthSetting().ProbeIntervalSeconds,
			now,
			model.ChannelHealthStatusProbing,
			staleProbeBefore,
		).
		Order("next_probe_at ASC, last_failure_at ASC").
		Limit(limit).
		Find(&states).Error
	if err != nil {
		return nil, err
	}
	for i := range states {
		decorateChannelHealthState(&states[i])
	}
	return states, nil
}

func AttachChannelHealth(channels []*model.Channel) {
	if len(channels) == 0 {
		return
	}
	ids := make([]int, 0, len(channels))
	for _, channel := range channels {
		if channel != nil && channel.Id > 0 {
			ids = append(ids, channel.Id)
		}
	}
	healthMap, err := GetChannelHealthMap(ids)
	if err != nil {
		common.SysLog("failed to attach channel health: " + err.Error())
		return
	}
	for _, channel := range channels {
		if channel == nil || channel.Id <= 0 {
			continue
		}
		threshold := model.GetChannelHealthMinimumPositiveFailureThreshold(channel.Id)
		if state, ok := healthMap[channel.Id]; ok {
			health := decorateChannelHealthState(&state)
			health.FailureThreshold = threshold
			channel.Health = health
		} else {
			health := decorateChannelHealthState(defaultChannelHealthState(channel.Id))
			health.FailureThreshold = threshold
			channel.Health = health
		}
	}
}

func AttachSingleChannelHealth(channel *model.Channel) {
	if channel == nil {
		return
	}
	AttachChannelHealth([]*model.Channel{channel})
}

func GetChannelHealthMap(channelIDs []int) (map[int]model.ChannelHealthState, error) {
	result := make(map[int]model.ChannelHealthState, len(channelIDs))
	if len(channelIDs) == 0 {
		return result, nil
	}
	var states []model.ChannelHealthState
	if err := model.DB.Where("channel_id IN ?", channelIDs).Find(&states).Error; err != nil {
		return result, err
	}
	for _, state := range states {
		result[state.ChannelId] = state
	}
	return result, nil
}

func GetChannelHealthGroupStateMap(channelIDs []int, groups []string) (map[int]map[string]model.ChannelHealthGroupState, error) {
	result := make(map[int]map[string]model.ChannelHealthGroupState, len(channelIDs))
	if len(channelIDs) == 0 || len(groups) == 0 {
		return result, nil
	}
	normalizedGroups := make([]string, 0, len(groups))
	seenGroups := make(map[string]struct{}, len(groups))
	for _, group := range groups {
		group = strings.TrimSpace(group)
		if group == "" {
			continue
		}
		if _, ok := seenGroups[group]; ok {
			continue
		}
		seenGroups[group] = struct{}{}
		normalizedGroups = append(normalizedGroups, group)
	}
	if len(normalizedGroups) == 0 {
		return result, nil
	}
	var states []model.ChannelHealthGroupState
	if err := model.DB.Where("channel_id IN ? AND group_name IN ?", channelIDs, normalizedGroups).Find(&states).Error; err != nil {
		return result, err
	}
	for _, state := range states {
		if result[state.ChannelId] == nil {
			result[state.ChannelId] = make(map[string]model.ChannelHealthGroupState)
		}
		result[state.ChannelId][state.GroupName] = state
	}
	return result, nil
}

func GetChannelHealthProbeModelStateMap(channelIDs []int) (map[int]map[string]model.ChannelHealthProbeModelState, error) {
	result := make(map[int]map[string]model.ChannelHealthProbeModelState, len(channelIDs))
	if len(channelIDs) == 0 {
		return result, nil
	}
	var states []model.ChannelHealthProbeModelState
	if err := model.DB.Where("channel_id IN ?", channelIDs).Find(&states).Error; err != nil {
		return result, err
	}
	for _, state := range states {
		if result[state.ChannelId] == nil {
			result[state.ChannelId] = make(map[string]model.ChannelHealthProbeModelState)
		}
		result[state.ChannelId][state.Model] = state
	}
	return result, nil
}

func channelModelList(models string) []string {
	parts := strings.Split(models, ",")
	result := make([]string, 0, len(parts))
	for _, modelName := range parts {
		modelName = strings.TrimSpace(modelName)
		if modelName == "" {
			continue
		}
		result = append(result, modelName)
	}
	return result
}

func effectiveChannelProbeModelsFromState(state *model.ChannelHealthState, channelModels []string, testModel string) []string {
	configured := model.DecodeChannelHealthProbeModels("")
	if state != nil {
		configured = model.DecodeChannelHealthProbeModels(state.ProbeModels)
	}
	configured = model.NormalizeChannelHealthProbeModels(configured, channelModels)
	if len(configured) > 0 {
		return configured
	}
	testModel = strings.TrimSpace(testModel)
	if testModel != "" {
		if models := model.NormalizeChannelHealthProbeModels([]string{testModel}, channelModels); len(models) > 0 {
			return models
		}
	}
	return model.NormalizeChannelHealthProbeModels(channelModels, nil)
}

func EffectiveChannelProbeModels(channel *model.Channel, state *model.ChannelHealthState) []string {
	if channel == nil {
		return nil
	}
	testModel := ""
	if channel.TestModel != nil {
		testModel = *channel.TestModel
	}
	return effectiveChannelProbeModelsFromState(state, channel.GetModels(), testModel)
}

func AttachChannelHealthProbeModelResults(state *model.ChannelHealthState, channel *model.Channel) *model.ChannelHealthState {
	if state == nil || channel == nil {
		return state
	}
	probeModels := EffectiveChannelProbeModels(channel, state)
	state.ConfiguredProbeModels = probeModels
	resultMap, err := GetChannelHealthProbeModelStateMap([]int{channel.Id})
	if err != nil {
		common.SysLog("failed to attach channel probe model results: " + err.Error())
		return state
	}
	state.ProbeModelResults = probeModelResultsForDisplay(channel.Id, probeModels, resultMap)
	return state
}

func probeModelResultsForDisplay(channelID int, probeModels []string, resultMap map[int]map[string]model.ChannelHealthProbeModelState) []model.ChannelHealthProbeModelState {
	if len(probeModels) == 0 {
		return nil
	}
	results := make([]model.ChannelHealthProbeModelState, 0, len(probeModels))
	byModel := resultMap[channelID]
	for _, modelName := range probeModels {
		if result, ok := byModel[modelName]; ok {
			results = append(results, result)
			continue
		}
		results = append(results, model.ChannelHealthProbeModelState{
			ChannelId: channelID,
			Model:     modelName,
			Status:    "",
		})
	}
	return results
}

func GetUserChannelStatus(userGroup string, includeAll bool) (UserChannelStatusResponse, error) {
	usableGroups, err := channelStatusVisibleGroups(userGroup, includeAll)
	if err != nil {
		return UserChannelStatusResponse{}, err
	}
	groups := make([]string, 0, len(usableGroups))
	for group := range usableGroups {
		groups = append(groups, group)
	}
	sort.Strings(groups)
	if len(groups) == 0 {
		return UserChannelStatusResponse{Groups: []UserChannelStatusGroup{}, UpdatedAt: common.GetTimestamp()}, nil
	}
	groupThresholds := model.GetChannelHealthGroupFailureThresholds(groups)

	type visibleChannelRow struct {
		PricingGroup  string `gorm:"column:pricing_group"`
		Model         string `gorm:"column:model"`
		ChannelID     int    `gorm:"column:channel_id"`
		ChannelName   string `gorm:"column:channel_name"`
		ChannelStatus int    `gorm:"column:channel_status"`
		ChannelModels string `gorm:"column:channel_models"`
		TestModel     string `gorm:"column:test_model"`
	}
	var rows []visibleChannelRow
	err = model.DB.Table("abilities").
		Select(
			model.QualifiedGroupColumn("abilities")+" AS pricing_group, abilities.model, channels.id AS channel_id, channels.name AS channel_name, channels.status AS channel_status, channels.models AS channel_models, channels.test_model AS test_model",
		).
		Joins("JOIN channels ON channels.id = abilities.channel_id").
		Where(model.QualifiedGroupColumn("abilities")+" IN ?", groups).
		Order(model.QualifiedGroupColumn("abilities") + " ASC, channels.name ASC, channels.id ASC, abilities.model ASC").
		Scan(&rows).Error
	if err != nil {
		return UserChannelStatusResponse{}, err
	}

	channelIDs := make([]int, 0)
	channelIDSet := make(map[int]struct{})
	for _, row := range rows {
		if row.ChannelID <= 0 {
			continue
		}
		if _, ok := channelIDSet[row.ChannelID]; !ok {
			channelIDSet[row.ChannelID] = struct{}{}
			channelIDs = append(channelIDs, row.ChannelID)
		}
	}
	healthMap, err := GetChannelHealthMap(channelIDs)
	if err != nil {
		return UserChannelStatusResponse{}, err
	}
	groupStateMap, err := GetChannelHealthGroupStateMap(channelIDs, groups)
	if err != nil {
		return UserChannelStatusResponse{}, err
	}
	probeResultMap, err := GetChannelHealthProbeModelStateMap(channelIDs)
	if err != nil {
		return UserChannelStatusResponse{}, err
	}

	groupMap := make(map[string]*UserChannelStatusGroup, len(groups))
	channelMap := make(map[string]*UserChannelStatusItem)
	modelSetMap := make(map[string]map[string]struct{})
	for _, group := range groups {
		groupMap[group] = &UserChannelStatusGroup{
			Group:            group,
			GroupName:        usableGroups[group],
			FailureThreshold: groupThresholds[group],
			Channels:         []UserChannelStatusItem{},
		}
	}

	for _, row := range rows {
		groupStatus := groupMap[row.PricingGroup]
		if groupStatus == nil {
			continue
		}
		itemKey := fmt.Sprintf("%s:%d", row.PricingGroup, row.ChannelID)
		item := channelMap[itemKey]
		if item == nil {
			state, ok := healthMap[row.ChannelID]
			if !ok {
				state = *defaultChannelHealthState(row.ChannelID)
			}
			decorated := decorateChannelHealthState(&state)
			groupThreshold := groupThresholds[row.PricingGroup]
			var groupStatePtr *model.ChannelHealthGroupState
			if statesByGroup := groupStateMap[row.ChannelID]; statesByGroup != nil {
				if groupState, ok := statesByGroup[row.PricingGroup]; ok {
					groupStatePtr = &groupState
				}
			}
			effectiveHealthStatus := decorated.Status
			groupUnavailable := model.IsChannelHealthUnavailableForGroupState(&state, groupStatePtr, row.PricingGroup, groupThreshold)
			if !groupUnavailable && decorated.Status == model.ChannelHealthStatusUnhealthy {
				effectiveHealthStatus = model.ChannelHealthStatusSuspect
			}
			if groupStatePtr != nil {
				effectiveHealthStatus = groupStatePtr.Status
				if groupUnavailable {
					effectiveHealthStatus = model.ChannelHealthStatusUnhealthy
				}
			}
			probeModels := effectiveChannelProbeModelsFromState(&state, channelModelList(row.ChannelModels), row.TestModel)
			displayStatus := "normal"
			if groupUnavailable ||
				row.ChannelStatus == common.ChannelStatusAutoDisabled {
				displayStatus = "error"
			}
			if row.ChannelStatus == common.ChannelStatusManuallyDisabled {
				displayStatus = "disabled"
			}
			item = &UserChannelStatusItem{
				ChannelID:                 row.ChannelID,
				ChannelName:               row.ChannelName,
				ChannelStatus:             row.ChannelStatus,
				HealthStatus:              effectiveHealthStatus,
				DisplayStatus:             displayStatus,
				NextProbeAt:               decorated.NextProbeAt,
				NextProbeRemainingSeconds: decorated.NextProbeRemainingSecond,
				FailureCount:              decorated.FailureCount,
				FailureThreshold:          groupThreshold,
				ProbeIntervalSeconds:      decorated.EffectiveProbeInterval,
				LastFailureAt:             decorated.LastFailureAt,
				LastSuccessAt:             decorated.LastSuccessAt,
				AutoProbeEnabled:          decorated.AutoProbeEnabled,
				CanProbe:                  includeAll && row.ChannelStatus != common.ChannelStatusManuallyDisabled,
				Models:                    []string{},
				ProbeModels:               probeModels,
				ProbeModelResults:         probeModelResultsForDisplay(row.ChannelID, probeModels, probeResultMap),
			}
			channelMap[itemKey] = item
			modelSetMap[itemKey] = make(map[string]struct{})
		}
		if row.Model != "" {
			modelSetMap[itemKey][row.Model] = struct{}{}
		}
	}

	for _, group := range groups {
		groupStatus := groupMap[group]
		for key, item := range channelMap {
			if !strings.HasPrefix(key, group+":") {
				continue
			}
			models := make([]string, 0, len(modelSetMap[key]))
			for modelName := range modelSetMap[key] {
				models = append(models, modelName)
			}
			sort.Strings(models)
			item.Models = models
			groupStatus.Channels = append(groupStatus.Channels, *item)
		}
		sort.Slice(groupStatus.Channels, func(i, j int) bool {
			if groupStatus.Channels[i].DisplayStatus == groupStatus.Channels[j].DisplayStatus {
				return groupStatus.Channels[i].ChannelName < groupStatus.Channels[j].ChannelName
			}
			return groupStatus.Channels[i].DisplayStatus == "error"
		})
		groupStatus.Total = len(groupStatus.Channels)
		for _, item := range groupStatus.Channels {
			if item.DisplayStatus == "error" {
				groupStatus.ErrorCount++
			}
			if item.DisplayStatus == "normal" {
				groupStatus.AvailableCount++
			}
		}
		groupStatus.DisplayStatus = "normal"
		if groupStatus.AvailableCount == 0 && groupStatus.Total > 0 {
			groupStatus.DisplayStatus = "error"
		}
	}

	response := UserChannelStatusResponse{
		Groups:    make([]UserChannelStatusGroup, 0, len(groups)),
		UpdatedAt: common.GetTimestamp(),
	}
	for _, group := range groups {
		if groupStatus := groupMap[group]; groupStatus != nil {
			if groupStatus.Total > 0 {
				response.Groups = append(response.Groups, *groupStatus)
			}
		}
	}
	return response, nil
}

func channelStatusVisibleGroups(userGroup string, includeAll bool) (map[string]string, error) {
	if !includeAll {
		return GetUserUsableGroups(userGroup), nil
	}
	var groups []string
	if err := model.DB.Model(&model.Ability{}).
		Distinct(model.GroupColumn()).
		Pluck(model.GroupColumn(), &groups).Error; err != nil {
		return nil, err
	}
	defaultDescriptions := GetUserUsableGroups("")
	result := make(map[string]string, len(groups))
	for _, group := range groups {
		group = strings.TrimSpace(group)
		if group == "" {
			continue
		}
		desc := defaultDescriptions[group]
		if strings.TrimSpace(desc) == "" {
			desc = group
		}
		result[group] = desc
	}
	return result, nil
}

func recordChannelHealthFailure(channelID int, ctx ChannelHealthContext, err *types.NewAPIError) (*model.ChannelHealthState, bool, error) {
	return recordChannelHealthFailureWithEvent(channelID, ctx, err, model.ChannelHealthEventFailureRecorded, false)
}

func recordChannelHealthFailureWithEvent(channelID int, ctx ChannelHealthContext, apiErr *types.NewAPIError, eventType string, isProbe bool) (*model.ChannelHealthState, bool, error) {
	now := common.GetTimestamp()
	threshold := model.GetChannelHealthMinimumPositiveFailureThreshold(channelID)
	var result *model.ChannelHealthState
	markedUnhealthy := false
	err := model.DB.Transaction(func(tx *gorm.DB) error {
		state, err := getOrCreateChannelHealthStateForUpdate(tx, channelID, now, threshold)
		if err != nil {
			return err
		}
		fromStatus := state.Status
		if state.Status == "" {
			state.Status = model.ChannelHealthStatusHealthy
		}
		wasUnavailable := threshold > 0 && model.IsChannelHealthUnavailableForThreshold(state, threshold)
		state.FailureCount++
		state.FailureThreshold = threshold
		state.LastFailureAt = now
		state.LastStatusCode = apiErr.StatusCode
		state.LastErrorCode = trimForColumn(string(apiErr.GetErrorCode()), 128)
		state.LastError = common.LocalLogPreview(apiErr.MaskSensitiveErrorWithStatusCode())
		state.LastModel = trimForColumn(ctx.Model, 255)
		state.LastGroup = trimForColumn(ctx.Group, 64)
		state.LastTokenId = ctx.TokenID
		state.LastRequestId = trimForColumn(ctx.RequestID, 64)
		state.UpdatedAt = now
		if isProbe {
			state.LastProbeAt = now
			state.ProbeStartedAt = 0
			state.ProbeAttempts++
		}
		if threshold > 0 && state.FailureCount >= threshold {
			state.Status = model.ChannelHealthStatusUnhealthy
			state.NextProbeAt = nextProbeAtForUnhealthyState(state, now)
			markedUnhealthy = !wasUnavailable
		} else {
			state.Status = model.ChannelHealthStatusSuspect
			state.NextProbeAt = 0
		}
		if err := tx.Save(state).Error; err != nil {
			return err
		}
		if !isProbe && strings.TrimSpace(ctx.Group) != "" {
			if err := recordChannelHealthGroupFailureTx(tx, channelID, ctx, apiErr, now); err != nil {
				return err
			}
		}
		if err := createChannelHealthEvent(tx, state, ctx, eventType, fromStatus, state.Status, apiErr.StatusCode, string(apiErr.GetErrorCode()), state.LastError); err != nil {
			return err
		}
		if markedUnhealthy {
			if err := createChannelHealthEvent(tx, state, ctx, model.ChannelHealthEventMarkedUnhealthy, fromStatus, state.Status, apiErr.StatusCode, string(apiErr.GetErrorCode()), state.LastError); err != nil {
				return err
			}
		}
		result = state
		return nil
	})
	return result, markedUnhealthy, err
}

func recordChannelHealthSuccess(channelID int, ctx ChannelHealthContext, eventType string, isProbe bool) (*model.ChannelHealthState, error) {
	now := common.GetTimestamp()
	threshold := model.GetChannelHealthMinimumPositiveFailureThreshold(channelID)
	var result *model.ChannelHealthState
	err := model.DB.Transaction(func(tx *gorm.DB) error {
		state, err := getOrCreateChannelHealthStateForUpdate(tx, channelID, now, threshold)
		if err != nil {
			return err
		}
		fromStatus := state.Status
		state.Status = model.ChannelHealthStatusHealthy
		state.FailureCount = 0
		state.FailureThreshold = threshold
		state.LastSuccessAt = now
		state.NextProbeAt = 0
		state.ProbeStartedAt = 0
		state.LastModel = trimForColumn(ctx.Model, 255)
		state.LastGroup = trimForColumn(ctx.Group, 64)
		state.LastTokenId = ctx.TokenID
		state.LastRequestId = trimForColumn(ctx.RequestID, 64)
		state.UpdatedAt = now
		if isProbe {
			state.LastProbeAt = now
		}
		if err := tx.Save(state).Error; err != nil {
			return err
		}
		if isProbe {
			if err := resetChannelHealthGroupStatesTx(tx, channelID, now); err != nil {
				return err
			}
		} else if strings.TrimSpace(ctx.Group) != "" {
			if err := recordChannelHealthGroupSuccessTx(tx, channelID, ctx, now); err != nil {
				return err
			}
		}
		shouldRecordEvent := isProbe ||
			fromStatus == model.ChannelHealthStatusUnhealthy ||
			fromStatus == model.ChannelHealthStatusProbing ||
			fromStatus == model.ChannelHealthStatusSuspect
		if shouldRecordEvent {
			if err := createChannelHealthEvent(tx, state, ctx, eventType, fromStatus, state.Status, http.StatusOK, "", ""); err != nil {
				return err
			}
			if fromStatus == model.ChannelHealthStatusUnhealthy || fromStatus == model.ChannelHealthStatusProbing {
				if err := createChannelHealthEvent(tx, state, ctx, model.ChannelHealthEventRecovered, fromStatus, state.Status, http.StatusOK, "", ""); err != nil {
					return err
				}
			}
		}
		result = state
		return nil
	})
	return result, err
}

func getOrCreateChannelHealthStateForUpdate(tx *gorm.DB, channelID int, now int64, threshold int) (*model.ChannelHealthState, error) {
	var state model.ChannelHealthState
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("channel_id = ?", channelID).
		First(&state).Error
	if err == nil {
		if state.Status == "" {
			state.Status = model.ChannelHealthStatusHealthy
		}
		if state.FailureThreshold <= 0 {
			state.FailureThreshold = threshold
		}
		return &state, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	state = model.ChannelHealthState{
		ChannelId:        channelID,
		Status:           model.ChannelHealthStatusHealthy,
		FailureThreshold: threshold,
		UpdatedAt:        now,
	}
	if err := tx.Create(&state).Error; err != nil {
		return nil, err
	}
	return &state, nil
}

func getOrCreateChannelHealthGroupStateForUpdate(tx *gorm.DB, channelID int, group string, now int64) (*model.ChannelHealthGroupState, error) {
	group = strings.TrimSpace(group)
	if channelID <= 0 || group == "" {
		return nil, nil
	}
	var state model.ChannelHealthGroupState
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("channel_id = ? AND group_name = ?", channelID, group).
		First(&state).Error
	if err == nil {
		if state.Status == "" {
			state.Status = model.ChannelHealthStatusHealthy
		}
		return &state, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	state = model.ChannelHealthGroupState{
		ChannelId: channelID,
		GroupName: group,
		Status:    model.ChannelHealthStatusHealthy,
		UpdatedAt: now,
	}
	if err := tx.Create(&state).Error; err != nil {
		return nil, err
	}
	return &state, nil
}

func recordChannelHealthGroupFailureTx(tx *gorm.DB, channelID int, ctx ChannelHealthContext, apiErr *types.NewAPIError, now int64) error {
	if apiErr == nil {
		return nil
	}
	group := strings.TrimSpace(ctx.Group)
	if channelID <= 0 || group == "" {
		return nil
	}
	state, err := getOrCreateChannelHealthGroupStateForUpdate(tx, channelID, group, now)
	if err != nil || state == nil {
		return err
	}
	threshold := model.GetChannelHealthGroupFailureThreshold(group)
	state.FailureCount++
	state.LastFailureAt = now
	state.LastStatusCode = apiErr.StatusCode
	state.LastErrorCode = trimForColumn(string(apiErr.GetErrorCode()), 128)
	state.LastError = common.LocalLogPreview(apiErr.MaskSensitiveErrorWithStatusCode())
	state.LastModel = trimForColumn(ctx.Model, 255)
	state.LastTokenId = ctx.TokenID
	state.LastRequestId = trimForColumn(ctx.RequestID, 64)
	state.UpdatedAt = now
	if threshold > 0 && state.FailureCount >= threshold {
		state.Status = model.ChannelHealthStatusUnhealthy
	} else {
		state.Status = model.ChannelHealthStatusSuspect
	}
	return tx.Save(state).Error
}

func recordChannelHealthGroupSuccessTx(tx *gorm.DB, channelID int, ctx ChannelHealthContext, now int64) error {
	group := strings.TrimSpace(ctx.Group)
	if channelID <= 0 || group == "" {
		return nil
	}
	state, err := getOrCreateChannelHealthGroupStateForUpdate(tx, channelID, group, now)
	if err != nil || state == nil {
		return err
	}
	state.Status = model.ChannelHealthStatusHealthy
	state.FailureCount = 0
	state.LastSuccessAt = now
	state.LastModel = trimForColumn(ctx.Model, 255)
	state.LastTokenId = ctx.TokenID
	state.LastRequestId = trimForColumn(ctx.RequestID, 64)
	state.UpdatedAt = now
	return tx.Save(state).Error
}

func resetChannelHealthGroupStatesTx(tx *gorm.DB, channelID int, now int64) error {
	if channelID <= 0 {
		return nil
	}
	return tx.Model(&model.ChannelHealthGroupState{}).
		Where("channel_id = ?", channelID).
		Updates(map[string]any{
			"status":          model.ChannelHealthStatusHealthy,
			"failure_count":   0,
			"last_success_at": now,
			"updated_at":      now,
		}).Error
}

func UpdateChannelHealthProbeInterval(channelID int, intervalSeconds int) (*model.ChannelHealthState, error) {
	if channelID <= 0 {
		return nil, errors.New("invalid channel id")
	}
	now := common.GetTimestamp()
	threshold := model.GetChannelHealthMinimumPositiveFailureThreshold(channelID)
	var result *model.ChannelHealthState
	err := model.DB.Transaction(func(tx *gorm.DB) error {
		state, err := getOrCreateChannelHealthStateForUpdate(tx, channelID, now, threshold)
		if err != nil {
			return err
		}
		state.ProbeIntervalSeconds = &intervalSeconds
		state.FailureThreshold = threshold
		state.UpdatedAt = now
		if intervalSeconds <= 0 {
			state.NextProbeAt = 0
			state.ProbeStartedAt = 0
			if state.Status == model.ChannelHealthStatusProbing {
				switch {
				case threshold > 0 && state.FailureCount >= threshold:
					state.Status = model.ChannelHealthStatusUnhealthy
				case state.FailureCount > 0:
					state.Status = model.ChannelHealthStatusSuspect
				default:
					state.Status = model.ChannelHealthStatusHealthy
				}
			}
		} else if state.Status == model.ChannelHealthStatusUnhealthy {
			state.NextProbeAt = nextProbeAtForUnhealthyState(state, now)
		}
		if err := tx.Save(state).Error; err != nil {
			return err
		}
		result = state
		return nil
	})
	return decorateChannelHealthState(result), err
}

func UpdateChannelHealthProbeModels(channel *model.Channel, probeModels []string) (*model.ChannelHealthState, error) {
	if channel == nil || channel.Id <= 0 {
		return nil, errors.New("invalid channel")
	}
	allowedModels := channel.GetModels()
	normalized := model.NormalizeChannelHealthProbeModels(probeModels, allowedModels)
	if len(normalized) == 0 {
		return nil, errors.New("probe models must include at least one current channel model")
	}
	encoded, err := model.EncodeChannelHealthProbeModels(normalized)
	if err != nil {
		return nil, err
	}
	now := common.GetTimestamp()
	threshold := model.GetChannelHealthMinimumPositiveFailureThreshold(channel.Id)
	var result *model.ChannelHealthState
	err = model.DB.Transaction(func(tx *gorm.DB) error {
		state, err := getOrCreateChannelHealthStateForUpdate(tx, channel.Id, now, threshold)
		if err != nil {
			return err
		}
		state.ProbeModels = encoded
		state.FailureThreshold = threshold
		state.UpdatedAt = now
		if state.Status == model.ChannelHealthStatusUnhealthy {
			state.NextProbeAt = now
		}
		if err := tx.Save(state).Error; err != nil {
			return err
		}
		result = state
		return nil
	})
	if result != nil {
		result.ConfiguredProbeModels = normalized
	}
	return AttachChannelHealthProbeModelResults(decorateChannelHealthState(result), channel), err
}

func DeferChannelHealthProbe(channelID int, nextProbeAt int64, reason string) (*model.ChannelHealthState, error) {
	if channelID <= 0 {
		return nil, errors.New("invalid channel id")
	}
	now := common.GetTimestamp()
	threshold := model.GetChannelHealthMinimumPositiveFailureThreshold(channelID)
	var result *model.ChannelHealthState
	err := model.DB.Transaction(func(tx *gorm.DB) error {
		state, err := getOrCreateChannelHealthStateForUpdate(tx, channelID, now, threshold)
		if err != nil {
			return err
		}
		fromStatus := state.Status
		if nextProbeAt <= now {
			interval := effectiveProbeIntervalSeconds(state)
			if interval > 0 {
				nextProbeAt = now + int64(interval)
			} else {
				nextProbeAt = 0
			}
		}
		state.Status = model.ChannelHealthStatusUnhealthy
		if threshold > 0 && state.FailureCount < threshold {
			state.FailureCount = threshold
		}
		state.FailureThreshold = threshold
		state.LastFailureAt = now
		state.LastProbeAt = now
		state.ProbeStartedAt = 0
		state.NextProbeAt = nextProbeAt
		state.LastErrorCode = "cpa_quota_deferred"
		state.LastError = common.LocalLogPreview(reason)
		state.UpdatedAt = now
		if err := tx.Save(state).Error; err != nil {
			return err
		}
		if err := createChannelHealthEvent(tx, state, ChannelHealthContext{}, model.ChannelHealthEventProbeDeferred, fromStatus, state.Status, 0, state.LastErrorCode, state.LastError); err != nil {
			return err
		}
		result = state
		return nil
	})
	return decorateChannelHealthState(result), err
}

func UpdateChannelHealthGroupFailureThreshold(group string, threshold int) (*model.ChannelHealthGroupSetting, error) {
	group = strings.TrimSpace(group)
	if group == "" {
		group = "default"
	}
	now := common.GetTimestamp()
	setting, err := model.UpsertChannelHealthGroupSetting(group, threshold, now)
	if err != nil {
		return nil, err
	}
	if err := reconcileChannelHealthThresholdsForGroup(group, now); err != nil {
		return nil, err
	}
	return &setting, nil
}

func reconcileChannelHealthThresholdsForGroup(group string, now int64) error {
	var channelIDs []int
	if err := model.DB.Model(&model.Ability{}).
		Where(model.GroupColumn()+" = ?", group).
		Distinct("channel_id").
		Pluck("channel_id", &channelIDs).Error; err != nil {
		return err
	}
	for _, channelID := range channelIDs {
		if err := reconcileChannelHealthGroupThreshold(channelID, group, now); err != nil {
			return err
		}
		if err := reconcileChannelHealthThreshold(channelID, now); err != nil {
			return err
		}
	}
	return nil
}

func reconcileChannelHealthGroupThreshold(channelID int, group string, now int64) error {
	threshold := model.GetChannelHealthGroupFailureThreshold(group)
	return model.DB.Transaction(func(tx *gorm.DB) error {
		state, err := getOrCreateChannelHealthGroupStateForUpdate(tx, channelID, group, now)
		if err != nil || state == nil {
			return err
		}
		switch {
		case threshold > 0 && model.IsChannelHealthGroupUnavailableForThreshold(state, threshold):
			state.Status = model.ChannelHealthStatusUnhealthy
		case state.FailureCount > 0:
			state.Status = model.ChannelHealthStatusSuspect
		default:
			state.Status = model.ChannelHealthStatusHealthy
		}
		state.UpdatedAt = now
		return tx.Save(state).Error
	})
}

func reconcileChannelHealthThreshold(channelID int, now int64) error {
	threshold := model.GetChannelHealthMinimumPositiveFailureThreshold(channelID)
	return model.DB.Transaction(func(tx *gorm.DB) error {
		state, err := getOrCreateChannelHealthStateForUpdate(tx, channelID, now, threshold)
		if err != nil {
			return err
		}
		if state.Status == model.ChannelHealthStatusProbing {
			state.FailureThreshold = threshold
			state.UpdatedAt = now
			return tx.Save(state).Error
		}
		state.FailureThreshold = threshold
		switch {
		case threshold > 0 && model.IsChannelHealthUnavailableForThreshold(state, threshold):
			state.Status = model.ChannelHealthStatusUnhealthy
			state.NextProbeAt = nextProbeAtForUnhealthyState(state, now)
		case state.FailureCount > 0:
			state.Status = model.ChannelHealthStatusSuspect
			state.NextProbeAt = 0
		default:
			state.Status = model.ChannelHealthStatusHealthy
			state.NextProbeAt = 0
		}
		state.UpdatedAt = now
		return tx.Save(state).Error
	})
}

func createChannelHealthEvent(tx *gorm.DB, state *model.ChannelHealthState, ctx ChannelHealthContext, eventType, fromStatus, toStatus string, statusCode int, errorCode, message string) error {
	if state == nil {
		return nil
	}
	event := model.ChannelHealthEvent{
		ChannelId:   state.ChannelId,
		EventType:   eventType,
		FromStatus:  trimForColumn(fromStatus, 32),
		ToStatus:    trimForColumn(toStatus, 32),
		GroupName:   trimForColumn(ctx.Group, 64),
		Model:       trimForColumn(ctx.Model, 255),
		StatusCode:  statusCode,
		ErrorCode:   trimForColumn(errorCode, 128),
		Message:     common.LocalLogPreview(message),
		CreatedAt:   common.GetTimestamp(),
		RequestId:   trimForColumn(ctx.RequestID, 64),
		TokenId:     ctx.TokenID,
		ChannelName: trimForColumn(ctx.ChannelName, 255),
	}
	return tx.Create(&event).Error
}

func channelHealthContextFromGin(c *gin.Context, channelName string) ChannelHealthContext {
	if c == nil {
		return ChannelHealthContext{ChannelName: channelName}
	}
	modelName := common.GetContextKeyString(c, constant.ContextKeyOriginalModel)
	if modelName == "" {
		modelName = c.GetString("original_model")
	}
	group := common.GetContextKeyString(c, constant.ContextKeyUsingGroup)
	if group == "" {
		group = c.GetString("group")
	}
	if channelName == "" {
		channelName = c.GetString("channel_name")
	}
	return ChannelHealthContext{
		Group:       group,
		Model:       modelName,
		TokenID:     common.GetContextKeyInt(c, constant.ContextKeyTokenId),
		RequestID:   c.GetString(common.RequestIdKey),
		ChannelName: channelName,
	}
}

func channelUsingKeyFromGin(c *gin.Context) string {
	if c == nil {
		return ""
	}
	return common.GetContextKeyString(c, constant.ContextKeyChannelKey)
}

func defaultChannelHealthState(channelID int) *model.ChannelHealthState {
	effectiveInterval := operation_setting.GetChannelHealthSetting().ProbeIntervalSeconds
	return &model.ChannelHealthState{
		ChannelId:              channelID,
		Status:                 model.ChannelHealthStatusHealthy,
		FailureThreshold:       operation_setting.GetChannelHealthSetting().FailureThreshold,
		AutoProbeEnabled:       effectiveInterval > 0,
		EffectiveProbeInterval: effectiveInterval,
	}
}

func nextProbeAnchorForState(state *model.ChannelHealthState, now int64) int64 {
	if state == nil {
		return now
	}
	anchor := state.LastFailureAt
	if state.LastProbeAt > anchor {
		anchor = state.LastProbeAt
	}
	if state.UpdatedAt > anchor {
		anchor = state.UpdatedAt
	}
	if anchor <= 0 {
		anchor = now
	}
	return anchor
}

func effectiveProbeIntervalSeconds(state *model.ChannelHealthState) int {
	if state != nil && state.ProbeIntervalSeconds != nil {
		return *state.ProbeIntervalSeconds
	}
	return operation_setting.GetChannelHealthSetting().ProbeIntervalSeconds
}

func nextProbeAtForUnhealthyState(state *model.ChannelHealthState, now int64) int64 {
	interval := effectiveProbeIntervalSeconds(state)
	if state == nil || interval <= 0 {
		return 0
	}
	return nextProbeAnchorForState(state, now) + int64(interval)
}

func IsChannelHealthAutoProbeEnabled(state *model.ChannelHealthState) bool {
	return effectiveProbeIntervalSeconds(state) > 0
}

func RescheduleUnhealthyChannelHealthProbes() error {
	if !ChannelHealthEnabled() {
		return nil
	}
	now := common.GetTimestamp()
	setting := operation_setting.GetChannelHealthSetting()
	return model.DB.Model(&model.ChannelHealthState{}).
		Where("status = ?", model.ChannelHealthStatusUnhealthy).
		Updates(map[string]any{
			"next_probe_at": gorm.Expr("CASE WHEN probe_interval_seconds IS NOT NULL AND probe_interval_seconds <= 0 THEN 0 WHEN probe_interval_seconds IS NOT NULL THEN ? + probe_interval_seconds WHEN ? > 0 THEN ? + ? ELSE 0 END", now, setting.ProbeIntervalSeconds, now, setting.ProbeIntervalSeconds),
			"updated_at":    now,
		}).Error
}

func decorateChannelHealthState(state *model.ChannelHealthState) *model.ChannelHealthState {
	if state == nil {
		return nil
	}
	setting := operation_setting.GetChannelHealthSetting()
	if state.Status == "" {
		state.Status = model.ChannelHealthStatusHealthy
	}
	if state.FailureThreshold < 0 {
		state.FailureThreshold = setting.FailureThreshold
	}
	now := common.GetTimestamp()
	state.EffectiveProbeInterval = effectiveProbeIntervalSeconds(state)
	state.AutoProbeEnabled = state.EffectiveProbeInterval > 0
	state.ConfiguredProbeModels = model.DecodeChannelHealthProbeModels(state.ProbeModels)
	if !state.AutoProbeEnabled {
		state.NextProbeAt = 0
	} else if state.Status == model.ChannelHealthStatusUnhealthy {
		if state.NextProbeAt <= 0 {
			state.NextProbeAt = nextProbeAtForUnhealthyState(state, now)
		}
	} else if state.Status == model.ChannelHealthStatusProbing {
		state.NextProbeAt = 0
	} else {
		state.NextProbeAt = 0
	}
	if state.NextProbeAt > now {
		state.NextProbeRemainingSecond = state.NextProbeAt - now
	} else {
		state.NextProbeRemainingSecond = 0
	}
	state.AffectedGroups = strings.Join(GetChannelAffectedGroups(state.ChannelId), ",")
	return state
}

func GetChannelAffectedGroups(channelID int) []string {
	if channelID <= 0 {
		return nil
	}
	groupColumn := strings.TrimSpace(model.GroupColumn())
	if groupColumn == "" {
		groupColumn = "`group`"
	}
	var groups []string
	_ = model.DB.Model(&model.Ability{}).
		Where("channel_id = ?", channelID).
		Distinct(groupColumn).
		Pluck(groupColumn, &groups).Error
	sort.Strings(groups)
	return groups
}

func channelHealthUnavailableForAllAffectedGroups(state *model.ChannelHealthState) bool {
	if state == nil || state.ChannelId <= 0 {
		return false
	}
	groups := GetChannelAffectedGroups(state.ChannelId)
	if len(groups) == 0 {
		return false
	}
	thresholds := model.GetChannelHealthGroupFailureThresholds(groups)
	groupStateMap, err := GetChannelHealthGroupStateMap([]int{state.ChannelId}, groups)
	if err != nil {
		common.SysLog("failed to load channel health group states: " + err.Error())
		return false
	}
	statesByGroup := groupStateMap[state.ChannelId]
	for _, group := range groups {
		threshold, ok := thresholds[group]
		if !ok {
			threshold = operation_setting.GetChannelHealthSetting().FailureThreshold
		}
		var groupStatePtr *model.ChannelHealthGroupState
		if groupState, ok := statesByGroup[group]; ok {
			groupStatePtr = &groupState
		}
		if !model.IsChannelHealthUnavailableForGroupState(state, groupStatePtr, group, threshold) {
			return false
		}
	}
	return true
}

func trimForColumn(value string, limit int) string {
	if limit <= 0 || len(value) <= limit {
		return value
	}
	return value[:limit]
}
