package service

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func resetTokenGroupHealth(t *testing.T) {
	t.Helper()
	tokenGroupHealth.Lock()
	tokenGroupHealth.states = map[string]tokenGroupHealthState{}
	tokenGroupHealth.Unlock()
	tokenGroupPreferredChannels.Lock()
	tokenGroupPreferredChannels.channels = map[string][]int{}
	tokenGroupPreferredChannels.Unlock()
	tokenGroupProbeSchedules.Lock()
	tokenGroupProbeSchedules.schedules = map[string]tokenGroupProbeSchedule{}
	tokenGroupProbeSchedules.Unlock()
	tokenGroupFailureObservations.Lock()
	tokenGroupFailureObservations.observations = map[string]tokenGroupFailureObservation{}
	tokenGroupFailureObservations.Unlock()
	t.Cleanup(func() {
		tokenGroupHealth.Lock()
		tokenGroupHealth.states = map[string]tokenGroupHealthState{}
		tokenGroupHealth.Unlock()
		tokenGroupPreferredChannels.Lock()
		tokenGroupPreferredChannels.channels = map[string][]int{}
		tokenGroupPreferredChannels.Unlock()
		tokenGroupProbeSchedules.Lock()
		tokenGroupProbeSchedules.schedules = map[string]tokenGroupProbeSchedule{}
		tokenGroupProbeSchedules.Unlock()
		tokenGroupFailureObservations.Lock()
		tokenGroupFailureObservations.observations = map[string]tokenGroupFailureObservation{}
		tokenGroupFailureObservations.Unlock()
	})
}

func tokenGroupTestContext(t *testing.T, tokenID int, cfg model.TokenGroupConfig) *gin.Context {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	common.SetContextKey(ctx, constant.ContextKeyTokenId, tokenID)
	common.SetContextKey(ctx, constant.ContextKeyTokenGroupConfig, model.NormalizeTokenGroupConfig(cfg, ""))
	return ctx
}

func fallbackTokenGroupConfig(groups ...string) model.TokenGroupConfig {
	items := make([]model.TokenGroupItem, 0, len(groups))
	for idx, group := range groups {
		items = append(items, model.TokenGroupItem{Group: group, Order: idx + 1})
	}
	return model.NormalizeTokenGroupConfig(model.TokenGroupConfig{
		Groups:           items,
		FailoverStrategy: model.TokenGroupFailoverStrategyFallback,
		TimeoutSeconds:   3,
		CooldownSeconds:  60,
		RecoveryStrategy: model.TokenGroupRecoveryStrategyProbeThenSwitch,
	}, "")
}

func TestMarkTokenGroupFailureCoolsGroupAndSummarizes(t *testing.T) {
	resetTokenGroupHealth(t)
	cfg := fallbackTokenGroupConfig("primary", "secondary")
	ctx := tokenGroupTestContext(t, 101, cfg)
	apiErr := types.NewErrorWithStatusCode(errors.New("upstream timeout"), types.ErrorCodeDoRequestFailed, http.StatusGatewayTimeout)

	MarkTokenGroupFailure(ctx, "primary", cfg, apiErr)

	cooling, reason, until := IsTokenGroupCooling(101, "primary", cfg)
	require.True(t, cooling)
	require.Greater(t, until, time.Now().Unix())
	require.Contains(t, reason, "primary 分组返回 504")
	require.Contains(t, TokenGroupFailureSummary(ctx), "primary 分组返回 504")

	tokenGroupHealth.RLock()
	state := tokenGroupHealth.states[tokenGroupHealthKey(101, "primary")]
	tokenGroupHealth.RUnlock()
	require.Equal(t, http.StatusGatewayTimeout, state.LastStatus)
	require.Equal(t, 3, state.ProbeTimeout)
	require.Empty(t, state.ProbeModel, "tests omit the original model so no async probe is scheduled")
}

func TestMarkTokenGroupFailureSchedulesProbeWhenModelKnown(t *testing.T) {
	resetTokenGroupHealth(t)
	cfg := fallbackTokenGroupConfig("primary", "secondary")
	ctx := tokenGroupTestContext(t, 110, cfg)
	common.SetContextKey(ctx, constant.ContextKeyOriginalModel, "gpt-test")
	apiErr := types.NewErrorWithStatusCode(errors.New("upstream timeout"), types.ErrorCodeDoRequestFailed, http.StatusGatewayTimeout)

	require.True(t, MarkTokenGroupFailure(ctx, "primary", cfg, apiErr))

	key := tokenGroupModelHealthKey(110, "primary", "gpt-test")
	tokenGroupHealth.RLock()
	state := tokenGroupHealth.states[key]
	tokenGroupHealth.RUnlock()
	require.Equal(t, "gpt-test", state.ProbeModel)

	tokenGroupProbeSchedules.Lock()
	schedule, exists := tokenGroupProbeSchedules.schedules[key]
	tokenGroupProbeSchedules.Unlock()
	require.True(t, exists)
	require.Equal(t, state.BlockedUntil, schedule.BlockedUntil)
	require.Equal(t, "gpt-test", schedule.ProbeModel)
}

func TestTokenGroupCoolingIsScopedByModel(t *testing.T) {
	resetTokenGroupHealth(t)
	cfg := fallbackTokenGroupConfig("primary", "secondary")
	ctx := tokenGroupTestContext(t, 112, cfg)
	common.SetContextKey(ctx, constant.ContextKeyOriginalModel, "gpt-a")
	apiErr := types.NewErrorWithStatusCode(errors.New("upstream timeout"), types.ErrorCodeDoRequestFailed, http.StatusGatewayTimeout)

	require.True(t, MarkTokenGroupFailure(ctx, "primary", cfg, apiErr))

	cooling, _, _ := IsTokenGroupModelCooling(112, "primary", "gpt-a", cfg)
	require.True(t, cooling)
	cooling, _, _ = IsTokenGroupModelCooling(112, "primary", "gpt-b", cfg)
	require.False(t, cooling)
}

func TestSingleGroupDoesNotEnterTokenGroupFailover(t *testing.T) {
	resetTokenGroupHealth(t)
	cfg := fallbackTokenGroupConfig("primary")
	cfg.FailoverStrategy = model.TokenGroupFailoverStrategyFallback
	ctx := tokenGroupTestContext(t, 109, cfg)
	apiErr := types.NewErrorWithStatusCode(errors.New("upstream timeout"), types.ErrorCodeDoRequestFailed, http.StatusGatewayTimeout)

	require.False(t, MarkTokenGroupFailure(ctx, "primary", cfg, apiErr))

	cooling, reason, until := IsTokenGroupCooling(109, "primary", cfg)
	require.False(t, cooling)
	require.Empty(t, reason)
	require.Zero(t, until)
	require.Empty(t, TokenGroupFailureSummary(ctx))
}

func TestPrepareNextTokenGroupRespectsFailureStrategy(t *testing.T) {
	resetTokenGroupHealth(t)
	cfg := fallbackTokenGroupConfig("primary", "secondary")
	ctx := tokenGroupTestContext(t, 102, cfg)
	retry := 4
	retryParam := &RetryParam{Ctx: ctx, Retry: &retry}

	require.True(t, PrepareNextTokenGroup(ctx, retryParam, "primary"))
	index, exists := common.GetContextKey(ctx, constant.ContextKeyTokenGroupIndex)
	require.True(t, exists)
	require.Equal(t, 1, index)
	require.Equal(t, 0, retryParam.GetRetry())
	require.True(t, retryParam.resetNextTry)

	returnErrorCfg := cfg
	returnErrorCfg.FailoverStrategy = model.TokenGroupFailoverStrategyReturnError
	returnErrorCtx := tokenGroupTestContext(t, 103, returnErrorCfg)
	returnErrorRetry := 4
	returnErrorRetryParam := &RetryParam{Ctx: returnErrorCtx, Retry: &returnErrorRetry}
	require.False(t, PrepareNextTokenGroup(returnErrorCtx, returnErrorRetryParam, "primary"))
	_, exists = common.GetContextKey(returnErrorCtx, constant.ContextKeyTokenGroupIndex)
	require.False(t, exists)
	require.Equal(t, 4, returnErrorRetryParam.GetRetry())
}

func TestPrepareNextTokenGroupUsesPerGroupFailureStrategy(t *testing.T) {
	resetTokenGroupHealth(t)
	cfg := fallbackTokenGroupConfig("primary", "secondary")
	cfg.Groups[0].FailoverStrategy = model.TokenGroupFailoverStrategyReturnError
	ctx := tokenGroupTestContext(t, 107, cfg)
	retry := 1
	retryParam := &RetryParam{Ctx: ctx, Retry: &retry}

	require.False(t, PrepareNextTokenGroup(ctx, retryParam, "primary"))
	require.Equal(t, 1, retryParam.GetRetry())

	cfg.Groups[0].FailoverStrategy = model.TokenGroupFailoverStrategyFallback
	ctx = tokenGroupTestContext(t, 108, cfg)
	retry = 1
	retryParam = &RetryParam{Ctx: ctx, Retry: &retry}
	require.True(t, PrepareNextTokenGroup(ctx, retryParam, "primary"))
}

func TestTokenGroupDetectionThresholds(t *testing.T) {
	require.True(t, failureThresholdReached(model.TokenGroupDetectionStrategyOne, 0.5, 1, 3))
	require.False(t, failureThresholdReached(model.TokenGroupDetectionStrategyHalf, 0.5, 1, 2))
	require.True(t, failureThresholdReached(model.TokenGroupDetectionStrategyHalf, 0.5, 2, 3))
	require.False(t, failureThresholdReached(model.TokenGroupDetectionStrategyAll, 0.5, 2, 3))
	require.True(t, failureThresholdReached(model.TokenGroupDetectionStrategyAll, 0.5, 3, 3))
	require.False(t, failureThresholdReached(model.TokenGroupDetectionStrategyRatio, 0.68, 2, 3))
	require.True(t, failureThresholdReached(model.TokenGroupDetectionStrategyRatio, 0.68, 3, 4))

	require.True(t, recoveryThresholdReached(model.TokenGroupDetectionStrategyOne, 0.5, 1, 3))
	require.True(t, recoveryThresholdReached(model.TokenGroupDetectionStrategyHalf, 0.5, 1, 2))
	require.False(t, recoveryThresholdReached(model.TokenGroupDetectionStrategyAll, 0.5, 2, 3))
	require.True(t, recoveryThresholdReached(model.TokenGroupDetectionStrategyRatio, 0.68, 3, 4))
}

func TestHasAvailableLaterTokenGroupSkipsCoolingGroups(t *testing.T) {
	resetTokenGroupHealth(t)
	cfg := fallbackTokenGroupConfig("primary", "secondary", "third")
	ctx := tokenGroupTestContext(t, 104, cfg)

	tokenGroupHealth.Lock()
	tokenGroupHealth.states[tokenGroupHealthKey(104, "secondary")] = tokenGroupHealthState{
		BlockedUntil: time.Now().Add(time.Minute).Unix(),
		LastReason:   "secondary failed",
	}
	tokenGroupHealth.Unlock()
	require.True(t, HasAvailableLaterTokenGroup(ctx, "primary"))

	tokenGroupHealth.Lock()
	tokenGroupHealth.states[tokenGroupHealthKey(104, "third")] = tokenGroupHealthState{
		BlockedUntil: time.Now().Add(time.Minute).Unix(),
		LastReason:   "third failed",
	}
	tokenGroupHealth.Unlock()
	require.False(t, HasAvailableLaterTokenGroup(ctx, "primary"))
}

func TestExpiredStickyStateDoesNotRecoverWithoutProbe(t *testing.T) {
	resetTokenGroupHealth(t)
	stickyCfg := fallbackTokenGroupConfig("primary", "secondary")
	stickyCfg.RecoveryStrategy = model.TokenGroupRecoveryStrategySticky

	tokenGroupHealth.Lock()
	tokenGroupHealth.states[tokenGroupHealthKey(105, "primary")] = tokenGroupHealthState{
		BlockedUntil: time.Now().Add(-time.Second).Unix(),
		LastReason:   "primary 分组返回 429：rate limited",
		LastStatus:   http.StatusTooManyRequests,
	}
	tokenGroupHealth.Unlock()

	cooling, reason, until := IsTokenGroupCooling(105, "primary", stickyCfg)
	require.False(t, cooling)
	require.Empty(t, reason)
	require.Zero(t, until)

	tokenGroupHealth.RLock()
	_, exists := tokenGroupHealth.states[tokenGroupHealthKey(105, "primary")]
	tokenGroupHealth.RUnlock()
	require.False(t, exists)
}

func TestProbeThenSwitchRecoveryClearsExpiredCoolingState(t *testing.T) {
	resetTokenGroupHealth(t)
	cfg := fallbackTokenGroupConfig("primary", "secondary")
	tokenGroupHealth.Lock()
	tokenGroupHealth.states[tokenGroupHealthKey(106, "primary")] = tokenGroupHealthState{
		BlockedUntil: time.Now().Add(-time.Second).Unix(),
		LastReason:   "expired",
	}
	tokenGroupHealth.Unlock()

	cooling, reason, until := IsTokenGroupCooling(106, "primary", cfg)
	require.False(t, cooling)
	require.Empty(t, reason)
	require.Zero(t, until)

	tokenGroupHealth.RLock()
	_, exists := tokenGroupHealth.states[tokenGroupHealthKey(106, "primary")]
	tokenGroupHealth.RUnlock()
	require.False(t, exists)
}

func TestCoolingCheckBackfillsMissingProbeSchedule(t *testing.T) {
	resetTokenGroupHealth(t)
	cfg := fallbackTokenGroupConfig("primary", "secondary")
	blockedUntil := time.Now().Add(time.Hour).Unix()
	key := tokenGroupModelHealthKey(111, "primary", "gpt-test")
	tokenGroupHealth.Lock()
	tokenGroupHealth.states[key] = tokenGroupHealthState{
		BlockedUntil: blockedUntil,
		LastReason:   "primary 分组返回 429：rate limited",
		LastStatus:   http.StatusTooManyRequests,
		ProbeModel:   "gpt-test",
	}
	tokenGroupHealth.Unlock()

	cooling, reason, until := IsTokenGroupModelCooling(111, "primary", "gpt-test", cfg)
	require.True(t, cooling)
	require.Contains(t, reason, "429")
	require.Equal(t, blockedUntil, until)

	tokenGroupProbeSchedules.Lock()
	schedule, exists := tokenGroupProbeSchedules.schedules[key]
	tokenGroupProbeSchedules.Unlock()
	require.True(t, exists)
	require.Equal(t, blockedUntil, schedule.BlockedUntil)
	require.Equal(t, "gpt-test", schedule.ProbeModel)
}

func TestShouldTokenGroupFailoverStatusCodes(t *testing.T) {
	require.False(t, ShouldTokenGroupFailover(nil))
	require.False(t, ShouldTokenGroupFailover(types.NewErrorWithStatusCode(errors.New("bad request"), types.ErrorCodeDoRequestFailed, http.StatusBadRequest)))
	require.True(t, ShouldTokenGroupFailover(types.NewErrorWithStatusCode(errors.New("rate limited"), types.ErrorCodeDoRequestFailed, http.StatusTooManyRequests)))
	require.True(t, ShouldTokenGroupFailover(types.NewErrorWithStatusCode(errors.New("upstream failed"), types.ErrorCodeDoRequestFailed, http.StatusInternalServerError)))
	require.False(t, ShouldTokenGroupFailover(types.NewErrorWithStatusCode(errors.New("gateway timeout"), types.ErrorCodeDoRequestFailed, http.StatusGatewayTimeout)))
	require.False(t, ShouldTokenGroupFailover(types.NewErrorWithStatusCode(errors.New("do not retry"), types.ErrorCodeDoRequestFailed, http.StatusInternalServerError, types.ErrOptionWithSkipRetry())))
}

func TestFormatTokenGroupCoolingReasonIncludesFailureAndRemainingTime(t *testing.T) {
	reason := FormatTokenGroupCoolingReason("primary", "primary 分组返回 429：rate limited", time.Now().Add(30*time.Second).Unix())
	require.True(t, strings.Contains(reason, "primary 分组暂不可用"))
	require.True(t, strings.Contains(reason, "429"))

	stickyReason := FormatTokenGroupCoolingReason("primary", "探活成功", 0)
	require.Contains(t, stickyReason, "保持当前分组策略")
}

func TestTokenGroupProbeUsesOriginalEndpointPath(t *testing.T) {
	endpoint, payload := tokenGroupProbePayload("gpt-test", "/v1/responses")
	require.Equal(t, "/v1/responses", endpoint)
	require.Equal(t, "gpt-test", payload["model"])
	require.Equal(t, "hi", payload["input"])
	require.NotContains(t, payload, "messages")

	endpoint, payload = tokenGroupProbePayload("gpt-test", "/v1/chat/completions")
	require.Equal(t, "/v1/chat/completions", endpoint)
	require.Contains(t, payload, "messages")
	require.NotContains(t, payload, "input")

	require.Equal(t, "https://example.com/v1/responses", tokenGroupProbeURL("https://example.com/v1", "/v1/responses"))
	require.Equal(t, "https://example.com/root/v1/responses", tokenGroupProbeURL("https://example.com/root", "/v1/responses"))
}
