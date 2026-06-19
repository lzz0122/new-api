package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/bytedance/gopkg/util/gopool"
	"github.com/gin-gonic/gin"
)

type tokenGroupHealthState struct {
	BlockedUntil        int64
	LastReason          string
	LastStatus          int
	ProbeModel          string
	ProbeTimeout        int
	AvailableChannelIDs []int
}

var tokenGroupHealth = struct {
	sync.RWMutex
	states map[string]tokenGroupHealthState
}{
	states: make(map[string]tokenGroupHealthState),
}

var tokenGroupPreferredChannels = struct {
	sync.RWMutex
	channels map[string][]int
}{
	channels: make(map[string][]int),
}

type tokenGroupProbeSchedule struct {
	BlockedUntil int64
	ProbeModel   string
}

var tokenGroupProbeSchedules = struct {
	sync.Mutex
	schedules map[string]tokenGroupProbeSchedule
}{
	schedules: make(map[string]tokenGroupProbeSchedule),
}

type tokenGroupFailureObservation struct {
	ExpiresAt         int64
	FailedChannelIDs  map[int]struct{}
	LastReason        string
	LastFailureStatus int
}

var tokenGroupFailureObservations = struct {
	sync.RWMutex
	observations map[string]tokenGroupFailureObservation
}{
	observations: make(map[string]tokenGroupFailureObservation),
}

func tokenGroupHealthKey(tokenID int, group string) string {
	return fmt.Sprintf("%d:%s", tokenID, group)
}

func tokenGroupFailureObservationKey(tokenID int, group string, modelName string) string {
	return fmt.Sprintf("%d:%s:%s", tokenID, group, modelName)
}

func effectiveTokenGroupItem(cfg model.TokenGroupConfig, group string) model.TokenGroupItem {
	item := model.TokenGroupItem{
		Group:                     group,
		FailoverStrategy:          cfg.FailoverStrategy,
		TimeoutSeconds:            cfg.TimeoutSeconds,
		CooldownSeconds:           cfg.CooldownSeconds,
		RecoveryStrategy:          cfg.RecoveryStrategy,
		FailureDetectionStrategy:  cfg.FailureDetectionStrategy,
		FailureDetectionRatio:     cfg.FailureDetectionRatio,
		RecoveryDetectionStrategy: cfg.RecoveryDetectionStrategy,
		RecoveryDetectionRatio:    cfg.RecoveryDetectionRatio,
	}
	for _, candidate := range cfg.Groups {
		if candidate.Group != group {
			continue
		}
		item.Order = candidate.Order
		if candidate.FailoverStrategy != "" {
			item.FailoverStrategy = candidate.FailoverStrategy
		}
		if candidate.TimeoutSeconds > 0 {
			item.TimeoutSeconds = candidate.TimeoutSeconds
		}
		if candidate.CooldownSeconds > 0 {
			item.CooldownSeconds = candidate.CooldownSeconds
		}
		if candidate.RecoveryStrategy != "" {
			item.RecoveryStrategy = candidate.RecoveryStrategy
		}
		if candidate.FailureDetectionStrategy != "" {
			item.FailureDetectionStrategy = candidate.FailureDetectionStrategy
		}
		if candidate.FailureDetectionRatio > 0 {
			item.FailureDetectionRatio = candidate.FailureDetectionRatio
		}
		if candidate.RecoveryDetectionStrategy != "" {
			item.RecoveryDetectionStrategy = candidate.RecoveryDetectionStrategy
		}
		if candidate.RecoveryDetectionRatio > 0 {
			item.RecoveryDetectionRatio = candidate.RecoveryDetectionRatio
		}
		break
	}
	if item.FailoverStrategy == "" {
		item.FailoverStrategy = model.TokenGroupFailoverStrategyReturnError
	}
	if item.CooldownSeconds <= 0 {
		item.CooldownSeconds = model.DefaultTokenGroupCooldownSeconds
	}
	if item.RecoveryStrategy == "" {
		item.RecoveryStrategy = model.TokenGroupRecoveryStrategyProbeThenSwitch
	}
	if item.FailureDetectionStrategy == "" {
		item.FailureDetectionStrategy = model.TokenGroupDetectionStrategyOne
	}
	if item.FailureDetectionRatio <= 0 {
		item.FailureDetectionRatio = model.DefaultTokenGroupDetectionRatio
	}
	if item.RecoveryDetectionStrategy == "" {
		item.RecoveryDetectionStrategy = model.TokenGroupDetectionStrategyOne
	}
	if item.RecoveryDetectionRatio <= 0 {
		item.RecoveryDetectionRatio = model.DefaultTokenGroupDetectionRatio
	}
	return item
}

func TokenGroupFailoverStrategy(cfg model.TokenGroupConfig, group string) string {
	return effectiveTokenGroupItem(cfg, group).FailoverStrategy
}

func tokenGroupRecoveryStrategy(cfg model.TokenGroupConfig, group string) string {
	return effectiveTokenGroupItem(cfg, group).RecoveryStrategy
}

func setTokenGroupPreferredChannels(tokenID int, group string, channelIDs []int) {
	if tokenID <= 0 || group == "" {
		return
	}
	key := tokenGroupHealthKey(tokenID, group)
	tokenGroupPreferredChannels.Lock()
	defer tokenGroupPreferredChannels.Unlock()
	if len(channelIDs) == 0 {
		delete(tokenGroupPreferredChannels.channels, key)
		return
	}
	copied := append([]int(nil), channelIDs...)
	tokenGroupPreferredChannels.channels[key] = copied
}

func getTokenGroupPreferredChannels(tokenID int, group string) []int {
	if tokenID <= 0 || group == "" {
		return nil
	}
	key := tokenGroupHealthKey(tokenID, group)
	tokenGroupPreferredChannels.RLock()
	defer tokenGroupPreferredChannels.RUnlock()
	return append([]int(nil), tokenGroupPreferredChannels.channels[key]...)
}

func HasTokenGroupPreferredChannels(tokenID int, group string) bool {
	return len(getTokenGroupPreferredChannels(tokenID, group)) > 0
}

func clearTokenGroupFailureObservations(tokenID int, group string) {
	if tokenID <= 0 || group == "" {
		return
	}
	prefix := tokenGroupHealthKey(tokenID, group) + ":"
	tokenGroupFailureObservations.Lock()
	defer tokenGroupFailureObservations.Unlock()
	for key := range tokenGroupFailureObservations.observations {
		if strings.HasPrefix(key, prefix) {
			delete(tokenGroupFailureObservations.observations, key)
		}
	}
}

func GetTokenGroupConfigFromContext(c *gin.Context) (model.TokenGroupConfig, bool) {
	cfg, ok := common.GetContextKeyType[model.TokenGroupConfig](c, constant.ContextKeyTokenGroupConfig)
	if !ok {
		return model.TokenGroupConfig{}, false
	}
	cfg = model.NormalizeTokenGroupConfig(cfg, "")
	return cfg, len(cfg.Groups) > 0
}

func tokenGroupNames(cfg model.TokenGroupConfig) []string {
	groups := make([]string, 0, len(cfg.Groups))
	for _, item := range cfg.Groups {
		groups = append(groups, item.Group)
	}
	return groups
}

func IsTokenGroupCooling(tokenID int, group string, cfg model.TokenGroupConfig) (bool, string, int64) {
	if tokenID <= 0 || group == "" {
		return false, "", 0
	}
	cfg = model.NormalizeTokenGroupConfig(cfg, "")
	key := tokenGroupHealthKey(tokenID, group)
	now := time.Now().Unix()
	tokenGroupHealth.RLock()
	state, ok := tokenGroupHealth.states[key]
	tokenGroupHealth.RUnlock()
	if !ok {
		return false, "", 0
	}
	if len(cfg.Groups) <= 1 {
		tokenGroupHealth.Lock()
		delete(tokenGroupHealth.states, key)
		tokenGroupHealth.Unlock()
		return false, "", 0
	}
	if state.BlockedUntil <= 0 {
		if state.LastStatus == 0 && tokenGroupRecoveryStrategy(cfg, group) == model.TokenGroupRecoveryStrategySticky {
			return true, state.LastReason, 0
		}
		tokenGroupHealth.Lock()
		delete(tokenGroupHealth.states, key)
		tokenGroupHealth.Unlock()
		return false, "", 0
	}
	if state.BlockedUntil <= now {
		if state.ProbeModel == "" {
			tokenGroupHealth.Lock()
			delete(tokenGroupHealth.states, key)
			tokenGroupHealth.Unlock()
			return false, "", 0
		}
		scheduleTokenGroupProbe(tokenID, group, cfg, state)
		return true, state.LastReason, state.BlockedUntil
	}
	if state.ProbeModel != "" {
		scheduleTokenGroupProbe(tokenID, group, cfg, state)
	}
	return true, state.LastReason, state.BlockedUntil
}

func MarkTokenGroupSuccess(c *gin.Context, group string) {
	tokenID := common.GetContextKeyInt(c, constant.ContextKeyTokenId)
	if tokenID <= 0 || group == "" {
		return
	}
	tokenGroupHealth.Lock()
	delete(tokenGroupHealth.states, tokenGroupHealthKey(tokenID, group))
	tokenGroupHealth.Unlock()
	clearTokenGroupFailureObservations(tokenID, group)
	setTokenGroupPreferredChannels(tokenID, group, nil)
}

func MarkTokenGroupFailure(c *gin.Context, group string, cfg model.TokenGroupConfig, err *types.NewAPIError) bool {
	tokenID := common.GetContextKeyInt(c, constant.ContextKeyTokenId)
	if tokenID <= 0 || group == "" || err == nil {
		return false
	}
	cfg = model.NormalizeTokenGroupConfig(cfg, "")
	if len(cfg.Groups) <= 1 {
		return false
	}
	settings := effectiveTokenGroupItem(cfg, group)
	groupFailed, reason, availableChannelIDs := shouldMarkTokenGroupFailed(c, group, settings, err)
	if len(availableChannelIDs) > 0 {
		setTokenGroupPreferredChannels(tokenID, group, availableChannelIDs)
	}
	if !groupFailed {
		AppendTokenGroupFailure(c, reason)
		return false
	}
	cooldown := settings.CooldownSeconds
	if cooldown <= 0 {
		cooldown = model.DefaultTokenGroupCooldownSeconds
	}
	state := tokenGroupHealthState{
		BlockedUntil:        time.Now().Add(time.Duration(cooldown) * time.Second).Unix(),
		LastReason:          reason,
		LastStatus:          err.StatusCode,
		ProbeModel:          common.GetContextKeyString(c, constant.ContextKeyOriginalModel),
		ProbeTimeout:        settings.TimeoutSeconds,
		AvailableChannelIDs: availableChannelIDs,
	}
	tokenGroupHealth.Lock()
	tokenGroupHealth.states[tokenGroupHealthKey(tokenID, group)] = state
	tokenGroupHealth.Unlock()
	AppendTokenGroupFailure(c, reason)
	scheduleTokenGroupProbe(tokenID, group, cfg, state)
	return true
}

func shouldMarkTokenGroupFailed(c *gin.Context, group string, settings model.TokenGroupItem, apiErr *types.NewAPIError) (bool, string, []int) {
	baseReason := formatTokenGroupFailureReason(group, apiErr)
	if settings.FailureDetectionStrategy == model.TokenGroupDetectionStrategyOne {
		return true, baseReason, nil
	}
	tokenID := common.GetContextKeyInt(c, constant.ContextKeyTokenId)
	modelName := common.GetContextKeyString(c, constant.ContextKeyOriginalModel)
	if modelName == "" {
		return true, baseReason + "（无法获取原始模型，按单渠道失败处理）", nil
	}
	candidates, err := model.GetSatisfiedChannels(group, modelName)
	if err != nil || len(candidates) == 0 {
		if err != nil {
			return true, baseReason + "；失败判定获取候选渠道失败：" + common.LocalLogPreview(err.Error()), nil
		}
		return true, baseReason + "；失败判定无候选渠道", nil
	}
	failedChannelID := common.GetContextKeyInt(c, constant.ContextKeyChannelId)
	now := time.Now().Unix()
	cooldown := settings.CooldownSeconds
	if cooldown <= 0 {
		cooldown = model.DefaultTokenGroupCooldownSeconds
	}
	observationKey := tokenGroupFailureObservationKey(tokenID, group, modelName)
	tokenGroupFailureObservations.Lock()
	observation := tokenGroupFailureObservations.observations[observationKey]
	if observation.ExpiresAt <= now {
		observation = tokenGroupFailureObservation{
			ExpiresAt:        now + int64(cooldown),
			FailedChannelIDs: make(map[int]struct{}),
		}
	}
	if observation.FailedChannelIDs == nil {
		observation.FailedChannelIDs = make(map[int]struct{})
	}
	if failedChannelID > 0 {
		observation.FailedChannelIDs[failedChannelID] = struct{}{}
	}
	observation.LastReason = baseReason
	observation.LastFailureStatus = apiErr.StatusCode
	tokenGroupFailureObservations.observations[observationKey] = observation
	tokenGroupFailureObservations.Unlock()

	knownCandidateIDs := make(map[int]struct{}, len(candidates))
	available := make([]int, 0, len(candidates))
	failed := 0
	for _, channel := range candidates {
		knownCandidateIDs[channel.Id] = struct{}{}
		if _, failedKnown := observation.FailedChannelIDs[channel.Id]; failedKnown {
			failed++
			continue
		}
		available = append(available, channel.Id)
	}
	total := len(candidates)
	if failedChannelID > 0 {
		if _, ok := knownCandidateIDs[failedChannelID]; !ok {
			total++
			failed++
		}
	}
	groupFailed := failureThresholdReached(settings.FailureDetectionStrategy, settings.FailureDetectionRatio, failed, total)
	detail := fmt.Sprintf("失败判定：%d/%d 个渠道已真实失败，策略=%s", failed, total, detectionStrategyLabel(settings.FailureDetectionStrategy, settings.FailureDetectionRatio))
	if groupFailed {
		tokenGroupFailureObservations.Lock()
		delete(tokenGroupFailureObservations.observations, observationKey)
		tokenGroupFailureObservations.Unlock()
		return true, baseReason + "；" + detail, available
	}
	return false, baseReason + "；" + detail + "，未达到分组失败阈值", available
}

func failureThresholdReached(strategy string, ratio float64, failed int, total int) bool {
	if total <= 0 {
		return true
	}
	switch strategy {
	case model.TokenGroupDetectionStrategyHalf:
		return float64(failed)/float64(total) > 0.5
	case model.TokenGroupDetectionStrategyAll:
		return failed >= total
	case model.TokenGroupDetectionStrategyRatio:
		return float64(failed)/float64(total) >= ratio
	default:
		return failed > 0
	}
}

func recoveryThresholdReached(strategy string, ratio float64, success int, total int) bool {
	if total <= 0 {
		return false
	}
	switch strategy {
	case model.TokenGroupDetectionStrategyHalf:
		return float64(success)/float64(total) >= 0.5
	case model.TokenGroupDetectionStrategyAll:
		return success >= total
	case model.TokenGroupDetectionStrategyRatio:
		return float64(success)/float64(total) >= ratio
	default:
		return success > 0
	}
}

func detectionStrategyLabel(strategy string, ratio float64) string {
	switch strategy {
	case model.TokenGroupDetectionStrategyHalf:
		return "超过一半"
	case model.TokenGroupDetectionStrategyAll:
		return "全部"
	case model.TokenGroupDetectionStrategyRatio:
		return fmt.Sprintf("%.2f", ratio)
	default:
		return "单个"
	}
}

func AppendTokenGroupFailure(c *gin.Context, reason string) {
	if strings.TrimSpace(reason) == "" {
		return
	}
	failures := c.GetStringSlice("token_group_failures")
	failures = append(failures, reason)
	c.Set("token_group_failures", failures)
}

func TokenGroupFailureSummary(c *gin.Context) string {
	failures := c.GetStringSlice("token_group_failures")
	if len(failures) == 0 {
		return ""
	}
	return strings.Join(failures, "；")
}

func ShouldTokenGroupFailover(err *types.NewAPIError) bool {
	if err == nil {
		return false
	}
	if types.IsSkipRetryError(err) {
		return false
	}
	if operation_setting.IsAlwaysSkipRetryCode(err.GetErrorCode()) {
		return false
	}
	code := err.StatusCode
	if code == 0 {
		return true
	}
	return code >= http.StatusBadRequest && code <= 599
}

func PrepareNextTokenGroup(c *gin.Context, retryParam *RetryParam, currentGroup string) bool {
	cfg, ok := GetTokenGroupConfigFromContext(c)
	if !ok || len(cfg.Groups) <= 1 || retryParam == nil {
		return false
	}
	if TokenGroupFailoverStrategy(cfg, currentGroup) != model.TokenGroupFailoverStrategyFallback {
		return false
	}
	nextIndex := -1
	for idx, item := range cfg.Groups {
		if item.Group == currentGroup {
			nextIndex = idx + 1
			break
		}
	}
	if nextIndex < 0 || nextIndex >= len(cfg.Groups) {
		return false
	}
	common.SetContextKey(c, constant.ContextKeyTokenGroupIndex, nextIndex)
	retryParam.SetRetry(0)
	retryParam.ResetRetryNextTry()
	return true
}

func PrepareRecoveredTokenGroup(c *gin.Context, retryParam *RetryParam) bool {
	cfg, ok := GetTokenGroupConfigFromContext(c)
	if !ok || len(cfg.Groups) <= 1 || retryParam == nil {
		return false
	}
	if cfg.FailoverStrategy != model.TokenGroupFailoverStrategyFallback {
		return false
	}
	tokenID := common.GetContextKeyInt(c, constant.ContextKeyTokenId)
	for idx, item := range cfg.Groups {
		cooling, _, until := IsTokenGroupCooling(tokenID, item.Group, cfg)
		if cooling && until == 0 {
			common.SetContextKey(c, constant.ContextKeyTokenGroupIndex, idx)
			retryParam.SetRetry(0)
			retryParam.ResetRetryNextTry()
			return true
		}
	}
	return false
}

func HasAvailableLaterTokenGroup(c *gin.Context, currentGroup string) bool {
	cfg, ok := GetTokenGroupConfigFromContext(c)
	if !ok || len(cfg.Groups) <= 1 {
		return false
	}
	tokenID := common.GetContextKeyInt(c, constant.ContextKeyTokenId)
	seenCurrent := false
	for _, item := range cfg.Groups {
		if item.Group == currentGroup {
			seenCurrent = true
			continue
		}
		if !seenCurrent {
			continue
		}
		cooling, _, _ := IsTokenGroupCooling(tokenID, item.Group, cfg)
		if !cooling {
			return true
		}
	}
	return false
}

func formatTokenGroupFailureReason(group string, err *types.NewAPIError) string {
	reason := strings.TrimSpace(err.Error())
	if err.StatusCode > 0 {
		return fmt.Sprintf("%s 分组返回 %d：%s", group, err.StatusCode, common.LocalLogPreview(reason))
	}
	return fmt.Sprintf("%s 分组请求失败：%s", group, common.LocalLogPreview(reason))
}

func FormatTokenGroupCoolingReason(group string, reason string, blockedUntil int64) string {
	if blockedUntil <= 0 {
		if reason == "" {
			reason = "此前请求失败"
		}
		return fmt.Sprintf("%s 分组已过停顿期但按保持当前分组策略暂不切回（%s）", group, reason)
	}
	remaining := blockedUntil - time.Now().Unix()
	if remaining < 0 {
		remaining = 0
	}
	if reason == "" {
		reason = "此前请求失败"
	}
	return fmt.Sprintf("%s 分组暂不可用（%s，约 %d 秒后重试）", group, reason, remaining)
}

func TokenGroupListForLog(c *gin.Context) string {
	cfg, ok := GetTokenGroupConfigFromContext(c)
	if !ok {
		return ""
	}
	return strings.Join(tokenGroupNames(cfg), " -> ")
}

func scheduleTokenGroupProbe(tokenID int, group string, cfg model.TokenGroupConfig, state tokenGroupHealthState) {
	cfg = model.NormalizeTokenGroupConfig(cfg, "")
	if len(cfg.Groups) <= 1 || state.ProbeModel == "" || group == "" || tokenID <= 0 {
		return
	}
	key := tokenGroupHealthKey(tokenID, group)
	schedule := tokenGroupProbeSchedule{
		BlockedUntil: state.BlockedUntil,
		ProbeModel:   state.ProbeModel,
	}
	tokenGroupProbeSchedules.Lock()
	if current, ok := tokenGroupProbeSchedules.schedules[key]; ok && current == schedule {
		tokenGroupProbeSchedules.Unlock()
		return
	}
	tokenGroupProbeSchedules.schedules[key] = schedule
	tokenGroupProbeSchedules.Unlock()

	delay := time.Until(time.Unix(state.BlockedUntil, 0))
	if delay < 0 {
		delay = 0
	}
	common.SysLog(fmt.Sprintf("token group probe scheduled: token=%d group=%s model=%s delay=%s", tokenID, group, state.ProbeModel, delay.Round(time.Second)))
	gopool.Go(func() {
		timer := time.NewTimer(delay)
		defer timer.Stop()
		<-timer.C
		defer clearTokenGroupProbeSchedule(key, schedule)
		probeTokenGroup(tokenID, group, cfg, state)
	})
}

func clearTokenGroupProbeSchedule(key string, schedule tokenGroupProbeSchedule) {
	tokenGroupProbeSchedules.Lock()
	defer tokenGroupProbeSchedules.Unlock()
	if current, ok := tokenGroupProbeSchedules.schedules[key]; ok && current == schedule {
		delete(tokenGroupProbeSchedules.schedules, key)
	}
}

func probeTokenGroup(tokenID int, group string, cfg model.TokenGroupConfig, expected tokenGroupHealthState) {
	cfg = model.NormalizeTokenGroupConfig(cfg, "")
	if len(cfg.Groups) <= 1 {
		return
	}
	key := tokenGroupHealthKey(tokenID, group)
	tokenGroupHealth.RLock()
	current, ok := tokenGroupHealth.states[key]
	tokenGroupHealth.RUnlock()
	if !ok || current.BlockedUntil != expected.BlockedUntil || current.ProbeModel != expected.ProbeModel {
		return
	}

	settings := effectiveTokenGroupItem(cfg, group)
	recovered, availableChannelIDs, probeErr := probeTokenGroupRecovery(group, current.ProbeModel, settings)
	if recovered {
		common.SysLog(fmt.Sprintf("token group probe recovered: token=%d group=%s model=%s available_channels=%v", tokenID, group, current.ProbeModel, availableChannelIDs))
		setTokenGroupPreferredChannels(tokenID, group, availableChannelIDs)
		tokenGroupHealth.Lock()
		if settings.RecoveryStrategy == model.TokenGroupRecoveryStrategySticky {
			current.BlockedUntil = 0
			current.LastReason = "探活成功"
			current.LastStatus = 0
			current.AvailableChannelIDs = availableChannelIDs
			tokenGroupHealth.states[key] = current
		} else {
			delete(tokenGroupHealth.states, key)
		}
		tokenGroupHealth.Unlock()
		return
	}

	cooldown := settings.CooldownSeconds
	if cooldown <= 0 {
		cooldown = model.DefaultTokenGroupCooldownSeconds
	}
	reason := "探活失败"
	if probeErr != nil {
		reason = "探活失败：" + common.LocalLogPreview(probeErr.Error())
	}
	common.SysLog(fmt.Sprintf("token group probe failed: token=%d group=%s model=%s reason=%s", tokenID, group, current.ProbeModel, reason))
	next := tokenGroupHealthState{
		BlockedUntil:        time.Now().Add(time.Duration(cooldown) * time.Second).Unix(),
		LastReason:          reason,
		LastStatus:          http.StatusServiceUnavailable,
		ProbeModel:          current.ProbeModel,
		ProbeTimeout:        current.ProbeTimeout,
		AvailableChannelIDs: availableChannelIDs,
	}
	tokenGroupHealth.Lock()
	tokenGroupHealth.states[key] = next
	tokenGroupHealth.Unlock()
	scheduleTokenGroupProbe(tokenID, group, cfg, next)
}

func probeTokenGroupRecovery(group string, modelName string, settings model.TokenGroupItem) (bool, []int, error) {
	candidates, err := model.GetSatisfiedChannels(group, modelName)
	if err != nil {
		return false, nil, err
	}
	if len(candidates) == 0 {
		return false, nil, fmt.Errorf("分组 %s 下模型 %s 无可用渠道", group, modelName)
	}
	if settings.RecoveryDetectionStrategy == model.TokenGroupDetectionStrategyOne {
		shuffled := append([]*model.Channel(nil), candidates...)
		rand.Shuffle(len(shuffled), func(i, j int) {
			shuffled[i], shuffled[j] = shuffled[j], shuffled[i]
		})
		for _, channel := range shuffled {
			if err := doChannelProbe(channel, modelName, settings.TimeoutSeconds); err == nil {
				return true, []int{channel.Id}, nil
			}
		}
		return false, nil, fmt.Errorf("随机探活未发现可用渠道")
	}
	success := 0
	var lastErr error
	available := make([]int, 0, len(candidates))
	for idx, channel := range candidates {
		if err := doChannelProbe(channel, modelName, settings.TimeoutSeconds); err != nil {
			lastErr = err
		} else {
			success++
			available = append(available, channel.Id)
		}
		remaining := len(candidates) - idx - 1
		if recoveryThresholdReached(settings.RecoveryDetectionStrategy, settings.RecoveryDetectionRatio, success, len(candidates)) {
			return true, available, nil
		}
		if !recoveryThresholdCanStillPass(settings.RecoveryDetectionStrategy, settings.RecoveryDetectionRatio, success, remaining, len(candidates)) {
			break
		}
	}
	if lastErr != nil {
		return false, available, fmt.Errorf("探活成功 %d/%d，未达到恢复阈值：%w", success, len(candidates), lastErr)
	}
	return false, available, fmt.Errorf("探活成功 %d/%d，未达到恢复阈值", success, len(candidates))
}

func recoveryThresholdCanStillPass(strategy string, ratio float64, success int, remaining int, total int) bool {
	return recoveryThresholdReached(strategy, ratio, success+remaining, total)
}

func doChannelProbe(channel *model.Channel, modelName string, timeoutSeconds int) error {
	if channel == nil {
		return fmt.Errorf("渠道为空")
	}
	apiKey, _, apiErr := channel.GetNextEnabledKey()
	if apiErr != nil {
		return apiErr
	}
	if mappedModel := mapProbeModelName(channel.GetModelMapping(), modelName); mappedModel != "" {
		modelName = mappedModel
	}
	baseURL := strings.TrimRight(channel.GetBaseURL(), "/")
	if baseURL == "" {
		return fmt.Errorf("channel #%d base url is empty", channel.Id)
	}
	url := baseURL + "/v1/chat/completions"
	if strings.HasSuffix(baseURL, "/v1") {
		url = baseURL + "/chat/completions"
	}
	body, err := json.Marshal(map[string]any{
		"model":      modelName,
		"messages":   []map[string]string{{"role": "user", "content": "hi"}},
		"max_tokens": 1,
		"stream":     false,
	})
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)
	if channel.OpenAIOrganization != nil && *channel.OpenAIOrganization != "" {
		req.Header.Set("OpenAI-Organization", *channel.OpenAIOrganization)
	}
	client := GetHttpClient()
	channelSetting := channel.GetSetting()
	if channelSetting.Proxy != "" {
		proxyClient, proxyErr := NewProxyHttpClient(channelSetting.Proxy)
		if proxyErr != nil {
			return proxyErr
		}
		client = proxyClient
	}
	if client == nil {
		client = http.DefaultClient
	}
	if timeoutSeconds <= 0 {
		timeoutSeconds = 10
	}
	cloned := *client
	cloned.Timeout = time.Duration(timeoutSeconds) * time.Second
	resp, err := cloned.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		preview, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("探活返回 %d: %s", resp.StatusCode, strings.TrimSpace(string(preview)))
	}
	return nil
}

func mapProbeModelName(mapping string, modelName string) string {
	if mapping == "" || mapping == "{}" || modelName == "" {
		return modelName
	}
	modelMap := make(map[string]string)
	if err := json.Unmarshal([]byte(mapping), &modelMap); err != nil {
		return modelName
	}
	current := modelName
	visited := map[string]bool{current: true}
	for {
		next, ok := modelMap[current]
		if !ok || next == "" {
			return current
		}
		if visited[next] {
			return current
		}
		visited[next] = true
		current = next
	}
}
