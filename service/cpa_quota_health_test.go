package service

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/stretchr/testify/require"
)

func TestIsCPAQuotaHealthChannel(t *testing.T) {
	baseURL := "http://cli-proxy-api:8317"
	channel := &model.Channel{BaseURL: &baseURL}

	if !IsCPAQuotaHealthChannel(channel) {
		t.Fatal("cli-proxy-api base URL should be recognized as CPA quota health channel")
	}

	otherURL := "https://example.com"
	channel.BaseURL = &otherURL
	if IsCPAQuotaHealthChannel(channel) {
		t.Fatal("non-CPA base URL should not use CPA quota health")
	}
}

func TestCPAQuotaHealthNextProbeAtAddsGrace(t *testing.T) {
	now := common.GetTimestamp()
	result := &CPAQuotaHealthResponse{
		QuotaAvailable:    false,
		RetryAfterSeconds: 300,
	}

	next := cpaQuotaHealthNextProbeAt(result)
	wantMin := now + 300 + cpaQuotaHealthGraceSeconds
	if next < wantMin {
		t.Fatalf("next probe = %d, want at least %d", next, wantMin)
	}
}

func TestCheckCPAQuotaHealthIntegration(t *testing.T) {
	baseURL := os.Getenv("CPA_QUOTA_HEALTH_BASE_URL")
	apiKey := os.Getenv("CPA_QUOTA_HEALTH_API_KEY")
	if baseURL == "" || apiKey == "" {
		t.Skip("set CPA_QUOTA_HEALTH_BASE_URL and CPA_QUOTA_HEALTH_API_KEY to run CPA integration test")
	}
	modelName := os.Getenv("CPA_QUOTA_HEALTH_MODEL")
	if modelName == "" {
		modelName = "gpt-5.5"
	}
	channel := &model.Channel{
		Key:     apiKey,
		BaseURL: &baseURL,
		Models:  modelName,
	}

	result, apiErr := CheckCPAQuotaHealth(context.Background(), channel, []string{modelName})
	if apiErr != nil {
		t.Fatalf("CheckCPAQuotaHealth returned error: %v", apiErr)
	}
	if result == nil || !result.Success {
		t.Fatalf("CheckCPAQuotaHealth result = %+v, want success", result)
	}
	if result.Reason == "" {
		t.Fatalf("CheckCPAQuotaHealth result missing reason: %+v", result)
	}
}

type rewriteCPAQuotaHealthTransport struct {
	target *url.URL
}

func (t rewriteCPAQuotaHealthTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	cloned := req.Clone(req.Context())
	cloned.URL.Scheme = t.target.Scheme
	cloned.URL.Host = t.target.Host
	cloned.Host = t.target.Host
	return http.DefaultTransport.RoundTrip(cloned)
}

func withMockCPAQuotaHealthServer(t *testing.T, handler http.HandlerFunc) {
	t.Helper()
	server := httptest.NewServer(handler)
	target, err := url.Parse(server.URL)
	require.NoError(t, err)

	originalClient := newCPAQuotaHealthClient
	originalDelay := cpaQuotaHealthRetryDelay
	newCPAQuotaHealthClient = func() *http.Client {
		return &http.Client{
			Timeout:   cpaQuotaHealthHTTPTimeout,
			Transport: rewriteCPAQuotaHealthTransport{target: target},
		}
	}
	cpaQuotaHealthRetryDelay = time.Millisecond
	t.Cleanup(func() {
		newCPAQuotaHealthClient = originalClient
		cpaQuotaHealthRetryDelay = originalDelay
		server.Close()
	})
}

func cpaQuotaHealthTestChannel() *model.Channel {
	baseURL := "http://cli-proxy-api:8317"
	return &model.Channel{
		Id:      701,
		Key:     "team-key",
		BaseURL: &baseURL,
		Models:  "gpt-5.4,gpt-5.5",
	}
}

func withCPAChannelHealthSetting(t *testing.T) {
	t.Helper()
	setting := operation_setting.GetChannelHealthSetting()
	original := *setting
	setting.Enabled = true
	setting.FailureThreshold = 3
	setting.ProbeIntervalSeconds = 600
	t.Cleanup(func() {
		*setting = original
	})
}

func TestHandleCPAQuotaHealthDefersProbeUntilNextAvailableAtPlusGrace(t *testing.T) {
	db := setupChannelHealthTestDB(t)
	withCPAChannelHealthSetting(t)
	next := common.GetTimestamp() + 300
	attempts := 0
	withMockCPAQuotaHealthServer(t, func(w http.ResponseWriter, r *http.Request) {
		attempts++
		require.Equal(t, "/v1/internal/quota-health", r.URL.Path)
		require.Equal(t, "Bearer team-key", r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"success":true,"quota_available":false,"reason":"quota_exhausted","next_available_at":%d,"retry_after_seconds":300,"total_candidates":2,"quota_blocked_candidates":2}`, next)
	})

	health, apiErr, handled, err := HandleCPAQuotaHealthBeforeProbe(context.Background(), cpaQuotaHealthTestChannel(), []string{"gpt-5.4"})
	require.NoError(t, err)
	require.Nil(t, apiErr)
	require.True(t, handled)
	require.Equal(t, 1, attempts)
	require.NotNil(t, health)
	require.Equal(t, model.ChannelHealthStatusUnhealthy, health.Status)
	require.GreaterOrEqual(t, health.NextProbeAt, next+cpaQuotaHealthGraceSeconds)
	require.Contains(t, health.LastError, "quota_exhausted")

	var stored model.ChannelHealthState
	require.NoError(t, db.First(&stored, "channel_id = ?", 701).Error)
	require.Equal(t, int64(next+cpaQuotaHealthGraceSeconds), stored.NextProbeAt)
	require.Equal(t, "cpa_quota_deferred", stored.LastErrorCode)
}

func TestHandleCPAQuotaHealthUsesRetryAfterWhenNextAvailableAtMissing(t *testing.T) {
	db := setupChannelHealthTestDB(t)
	withCPAChannelHealthSetting(t)
	before := common.GetTimestamp()
	attempts := 0
	withMockCPAQuotaHealthServer(t, func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"quota_available":false,"reason":"quota_exhausted","retry_after_seconds":240,"total_candidates":1,"quota_blocked_candidates":1}`))
	})

	health, apiErr, handled, err := HandleCPAQuotaHealthBeforeProbe(context.Background(), cpaQuotaHealthTestChannel(), []string{"gpt-5.5"})
	require.NoError(t, err)
	require.Nil(t, apiErr)
	require.True(t, handled)
	require.Equal(t, 1, attempts)
	require.GreaterOrEqual(t, health.NextProbeAt, before+240+cpaQuotaHealthGraceSeconds)

	var stored model.ChannelHealthState
	require.NoError(t, db.First(&stored, "channel_id = ?", 701).Error)
	require.Equal(t, health.NextProbeAt, stored.NextProbeAt)
}

func TestHandleCPAQuotaHealthAvailableLetsNormalProbeContinue(t *testing.T) {
	setupChannelHealthTestDB(t)
	withCPAChannelHealthSetting(t)
	attempts := 0
	withMockCPAQuotaHealthServer(t, func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"quota_available":true,"reason":"quota_available","total_candidates":1,"available_candidates":1}`))
	})

	health, apiErr, handled, err := HandleCPAQuotaHealthBeforeProbe(context.Background(), cpaQuotaHealthTestChannel(), []string{"gpt-5.4"})
	require.NoError(t, err)
	require.Nil(t, apiErr)
	require.False(t, handled)
	require.Nil(t, health)
	require.Equal(t, 1, attempts)
}

func TestHandleCPAQuotaHealthNoMatchingAuthDefersInsteadOfContinuingProbe(t *testing.T) {
	db := setupChannelHealthTestDB(t)
	withCPAChannelHealthSetting(t)
	before := common.GetTimestamp()
	attempts := 0
	withMockCPAQuotaHealthServer(t, func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"quota_available":true,"reason":"no_matching_auth","models":["gpt-5.5"],"total_candidates":0,"available_candidates":0}`))
	})

	health, apiErr, handled, err := HandleCPAQuotaHealthBeforeProbe(context.Background(), cpaQuotaHealthTestChannel(), []string{"gpt-5.5"})
	require.NoError(t, err)
	require.Nil(t, apiErr)
	require.True(t, handled)
	require.Equal(t, 1, attempts)
	require.NotNil(t, health)
	require.Equal(t, model.ChannelHealthStatusUnhealthy, health.Status)
	require.GreaterOrEqual(t, health.NextProbeAt, before+600)
	require.Contains(t, health.LastError, "no_matching_auth")
	require.Contains(t, health.LastError, "total_candidates=0")

	var stored model.ChannelHealthState
	require.NoError(t, db.First(&stored, "channel_id = ?", 701).Error)
	require.Equal(t, "cpa_quota_deferred", stored.LastErrorCode)
	require.Contains(t, stored.LastError, "gpt-5.5")
}

func TestHandleCPAQuotaHealthUnauthorizedDoesNotRetryAndRecordsFailure(t *testing.T) {
	setupChannelHealthTestDB(t)
	withCPAChannelHealthSetting(t)
	attempts := 0
	withMockCPAQuotaHealthServer(t, func(w http.ResponseWriter, r *http.Request) {
		attempts++
		http.Error(w, `{"success":false,"reason":"unauthorized"}`, http.StatusUnauthorized)
	})

	health, apiErr, handled, err := HandleCPAQuotaHealthBeforeProbe(context.Background(), cpaQuotaHealthTestChannel(), []string{"gpt-5.4"})
	require.NoError(t, err)
	require.True(t, handled)
	require.NotNil(t, apiErr)
	require.Equal(t, http.StatusUnauthorized, apiErr.StatusCode)
	require.Equal(t, 1, attempts)
	require.NotNil(t, health)
	require.Equal(t, model.ChannelHealthStatusSuspect, health.Status)
	require.Equal(t, 1, health.FailureCount)
}

func TestHandleCPAQuotaHealthRetriesServerErrorsThenRecordsFailure(t *testing.T) {
	setupChannelHealthTestDB(t)
	withCPAChannelHealthSetting(t)
	attempts := 0
	withMockCPAQuotaHealthServer(t, func(w http.ResponseWriter, r *http.Request) {
		attempts++
		http.Error(w, `{"success":false,"reason":"temporary"}`, http.StatusServiceUnavailable)
	})

	health, apiErr, handled, err := HandleCPAQuotaHealthBeforeProbe(context.Background(), cpaQuotaHealthTestChannel(), []string{"gpt-5.4"})
	require.NoError(t, err)
	require.True(t, handled)
	require.NotNil(t, apiErr)
	require.Equal(t, http.StatusServiceUnavailable, apiErr.StatusCode)
	require.Equal(t, cpaQuotaHealthAttempts, attempts)
	require.NotNil(t, health)
	require.Equal(t, model.ChannelHealthStatusSuspect, health.Status)
	require.Equal(t, 1, health.FailureCount)
	require.Zero(t, health.NextProbeAt)
}

func TestHandleCPAQuotaHealthRetriesServerErrorsThenDefersWhenQuotaResponseSucceeds(t *testing.T) {
	setupChannelHealthTestDB(t)
	withCPAChannelHealthSetting(t)
	next := common.GetTimestamp() + 420
	attempts := 0
	withMockCPAQuotaHealthServer(t, func(w http.ResponseWriter, r *http.Request) {
		attempts++
		current := attempts
		if current < cpaQuotaHealthAttempts {
			http.Error(w, `{"success":false,"reason":"temporary"}`, http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"success":true,"quota_available":false,"reason":"quota_exhausted","next_available_at":%d,"retry_after_seconds":420,"total_candidates":1,"quota_blocked_candidates":1}`, next)
	})

	health, apiErr, handled, err := HandleCPAQuotaHealthBeforeProbe(context.Background(), cpaQuotaHealthTestChannel(), []string{"gpt-5.4"})
	require.NoError(t, err)
	require.Nil(t, apiErr)
	require.True(t, handled)
	require.Equal(t, cpaQuotaHealthAttempts, attempts)
	require.NotNil(t, health)
	require.Equal(t, model.ChannelHealthStatusUnhealthy, health.Status)
	require.Equal(t, next+cpaQuotaHealthGraceSeconds, health.NextProbeAt)
}

func TestCPAQuotaHealthDeferredStateAppearsUnavailableInChannelStatus(t *testing.T) {
	db := setupChannelHealthTestDB(t)
	withCPAChannelHealthSetting(t)
	baseURL := "http://cli-proxy-api:8317"
	channel := &model.Channel{
		Id:      702,
		Key:     "team-key",
		Name:    "cpa-lzz-plus",
		Status:  common.ChannelStatusEnabled,
		BaseURL: &baseURL,
		Models:  "gpt-5.4,gpt-5.5",
		Group:   "lzz_plus",
	}
	require.NoError(t, db.Create(channel).Error)
	require.NoError(t, db.Create(&model.Ability{
		Group:     "lzz_plus",
		Model:     "gpt-5.4",
		ChannelId: channel.Id,
		Enabled:   true,
	}).Error)

	next := common.GetTimestamp() + 300
	attempts := 0
	withMockCPAQuotaHealthServer(t, func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"success":true,"quota_available":false,"reason":"quota_exhausted","next_available_at":%d,"retry_after_seconds":300,"total_candidates":1,"quota_blocked_candidates":1}`, next)
	})

	health, apiErr, handled, err := HandleCPAQuotaHealthBeforeProbe(context.Background(), channel, []string{"gpt-5.4"})
	require.NoError(t, err)
	require.Nil(t, apiErr)
	require.True(t, handled)
	require.Equal(t, 1, attempts)
	require.Equal(t, model.ChannelHealthStatusUnhealthy, health.Status)
	require.Equal(t, 3, health.FailureCount)

	status, err := GetUserChannelStatus("", true)
	require.NoError(t, err)
	require.Len(t, status.Groups, 1)
	require.Equal(t, "lzz_plus", status.Groups[0].Group)
	require.Equal(t, "error", status.Groups[0].DisplayStatus)
	require.Equal(t, 1, status.Groups[0].ErrorCount)
	require.Len(t, status.Groups[0].Channels, 1)
	item := status.Groups[0].Channels[0]
	require.Equal(t, channel.Id, item.ChannelID)
	require.Equal(t, model.ChannelHealthStatusUnhealthy, item.HealthStatus)
	require.Equal(t, "error", item.DisplayStatus)
	require.Equal(t, next+cpaQuotaHealthGraceSeconds, item.NextProbeAt)
	require.Greater(t, item.NextProbeRemainingSeconds, int64(0))
	require.Greater(t, item.LastFailureAt, int64(0))
	require.Equal(t, 600, item.ProbeIntervalSeconds)
	require.True(t, item.AutoProbeEnabled)
}
