package controller

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func tokenGroupFailoverTestContext(t *testing.T) *gin.Context {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	cfg := model.NormalizeTokenGroupConfig(model.TokenGroupConfig{
		Groups: []model.TokenGroupItem{
			{Group: "primary", Order: 1},
			{Group: "secondary", Order: 2},
		},
		FailoverStrategy: model.TokenGroupFailoverStrategyFallback,
	}, "")
	common.SetContextKey(ctx, constant.ContextKeyTokenGroupConfig, cfg)
	return ctx
}

func TestTokenGroupFailoverDoesNotDependOnRetryBudget(t *testing.T) {
	ctx := tokenGroupFailoverTestContext(t)
	apiErr := types.NewErrorWithStatusCode(errors.New("upstream unavailable"), types.ErrorCodeDoRequestFailed, http.StatusServiceUnavailable)

	require.False(t, shouldRetry(ctx, apiErr, 0))

	cfg, ok := shouldUseTokenGroupFailover(ctx, apiErr)
	require.True(t, ok)
	require.Len(t, cfg.Groups, 2)
}

func TestTokenGroupFailoverStillBlocksSpecificChannelAndSkipRetryErrors(t *testing.T) {
	ctx := tokenGroupFailoverTestContext(t)
	ctx.Set("specific_channel_id", 37)
	apiErr := types.NewErrorWithStatusCode(errors.New("upstream unavailable"), types.ErrorCodeDoRequestFailed, http.StatusServiceUnavailable)

	_, ok := shouldUseTokenGroupFailover(ctx, apiErr)
	require.False(t, ok)

	ctx = tokenGroupFailoverTestContext(t)
	skipRetryErr := types.NewErrorWithStatusCode(errors.New("bad request"), types.ErrorCodeDoRequestFailed, http.StatusServiceUnavailable, types.ErrOptionWithSkipRetry())
	_, ok = shouldUseTokenGroupFailover(ctx, skipRetryErr)
	require.False(t, ok)
}
