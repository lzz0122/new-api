package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/types"
)

const (
	cpaQuotaHealthPath         = "/v1/internal/quota-health"
	cpaQuotaHealthGraceSeconds = int64(120)
	cpaQuotaHealthAttempts     = 3
	cpaQuotaHealthRetryDelay   = 15 * time.Second
	cpaQuotaHealthHTTPTimeout  = 10 * time.Second
)

type cpaQuotaHealthRequest struct {
	Models []string `json:"models"`
}

type CPAQuotaHealthResponse struct {
	Success                bool     `json:"success"`
	QuotaAvailable         bool     `json:"quota_available"`
	Reason                 string   `json:"reason"`
	NextAvailableAt        int64    `json:"next_available_at"`
	RetryAfterSeconds      int64    `json:"retry_after_seconds"`
	Groups                 []string `json:"groups"`
	Models                 []string `json:"models"`
	TotalCandidates        int      `json:"total_candidates"`
	AvailableCandidates    int      `json:"available_candidates"`
	QuotaBlockedCandidates int      `json:"quota_blocked_candidates"`
	OtherBlockedCandidates int      `json:"other_blocked_candidates"`
}

func HandleCPAQuotaHealthBeforeProbe(ctx context.Context, channel *model.Channel, probeModels []string) (*model.ChannelHealthState, *types.NewAPIError, bool, error) {
	if channel == nil || !IsCPAQuotaHealthChannel(channel) {
		return nil, nil, false, nil
	}
	result, apiErr := CheckCPAQuotaHealth(ctx, channel, probeModels)
	if apiErr != nil {
		health, err := RecordChannelProbeFailure(nil, channel, apiErr)
		return AttachChannelHealthProbeModelResults(health, channel), apiErr, true, err
	}
	if result == nil || result.QuotaAvailable {
		return nil, nil, false, nil
	}
	nextProbeAt := cpaQuotaHealthNextProbeAt(result)
	reason := fmt.Sprintf("CPA quota unavailable: reason=%s retry_after_seconds=%d next_available_at=%d", result.Reason, result.RetryAfterSeconds, result.NextAvailableAt)
	health, err := DeferChannelHealthProbe(channel.Id, nextProbeAt, reason)
	return AttachChannelHealthProbeModelResults(health, channel), nil, true, err
}

func CheckCPAQuotaHealth(ctx context.Context, channel *model.Channel, probeModels []string) (*CPAQuotaHealthResponse, *types.NewAPIError) {
	if channel == nil || !IsCPAQuotaHealthChannel(channel) {
		return nil, nil
	}
	apiKey := cpaQuotaHealthAPIKey(channel)
	if apiKey == "" {
		return nil, types.NewOpenAIError(errors.New("CPA quota health check missing channel key"), types.ErrorCodeChannelInvalidKey, http.StatusUnauthorized)
	}
	endpoint := strings.TrimRight(channel.GetBaseURL(), "/") + cpaQuotaHealthPath
	payload, err := json.Marshal(cpaQuotaHealthRequest{Models: model.NormalizeChannelHealthProbeModels(probeModels, channel.GetModels())})
	if err != nil {
		return nil, types.NewOpenAIError(fmt.Errorf("marshal CPA quota health request: %w", err), types.ErrorCodeJsonMarshalFailed, http.StatusInternalServerError)
	}
	if ctx == nil {
		ctx = context.Background()
	}

	var lastErr *types.NewAPIError
	for attempt := 1; attempt <= cpaQuotaHealthAttempts; attempt++ {
		result, apiErr := doCPAQuotaHealthRequest(ctx, endpoint, apiKey, payload)
		if apiErr == nil {
			return result, nil
		}
		lastErr = apiErr
		if apiErr.StatusCode == http.StatusUnauthorized || (apiErr.StatusCode > 0 && apiErr.StatusCode < http.StatusInternalServerError) {
			return nil, apiErr
		}
		if attempt < cpaQuotaHealthAttempts {
			if !sleepWithContext(ctx, cpaQuotaHealthRetryDelay) {
				return nil, types.NewOpenAIError(ctx.Err(), types.ErrorCodeDoRequestFailed, http.StatusRequestTimeout)
			}
		}
	}
	return nil, lastErr
}

func doCPAQuotaHealthRequest(ctx context.Context, endpoint string, apiKey string, payload []byte) (*CPAQuotaHealthResponse, *types.NewAPIError) {
	reqCtx, cancel := context.WithTimeout(ctx, cpaQuotaHealthHTTPTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeDoRequestFailed, http.StatusInternalServerError)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	client := &http.Client{Timeout: cpaQuotaHealthHTTPTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return nil, types.NewOpenAIError(fmt.Errorf("CPA quota health request failed: %w", err), types.ErrorCodeDoRequestFailed, http.StatusBadGateway)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			common.SysLog("failed to close CPA quota health response body: " + closeErr.Error())
		}
	}()

	body, readErr := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if readErr != nil {
		return nil, types.NewOpenAIError(fmt.Errorf("read CPA quota health response: %w", readErr), types.ErrorCodeReadResponseBodyFailed, http.StatusBadGateway)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, types.NewOpenAIError(fmt.Errorf("CPA quota health status=%d body=%s", resp.StatusCode, common.LocalLogPreview(string(body))), types.ErrorCodeBadResponseStatusCode, resp.StatusCode)
	}
	var result CPAQuotaHealthResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, types.NewOpenAIError(fmt.Errorf("decode CPA quota health response: %w", err), types.ErrorCodeBadResponseBody, http.StatusBadGateway)
	}
	if !result.Success {
		return nil, types.NewOpenAIError(fmt.Errorf("CPA quota health returned success=false reason=%s", result.Reason), types.ErrorCodeBadResponse, http.StatusBadGateway)
	}
	return &result, nil
}

func IsCPAQuotaHealthChannel(channel *model.Channel) bool {
	if channel == nil {
		return false
	}
	baseURL := strings.TrimSpace(channel.GetBaseURL())
	if baseURL == "" {
		return false
	}
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return strings.Contains(strings.ToLower(baseURL), "cli-proxy-api:8317")
	}
	return strings.EqualFold(parsed.Host, "cli-proxy-api:8317")
}

func cpaQuotaHealthAPIKey(channel *model.Channel) string {
	if channel == nil {
		return ""
	}
	keys := channel.GetKeys()
	if len(keys) == 0 {
		return ""
	}
	key := strings.TrimSpace(keys[0])
	if strings.HasPrefix(key, `"`) {
		var decoded string
		if err := json.Unmarshal([]byte(key), &decoded); err == nil {
			key = strings.TrimSpace(decoded)
		}
	}
	return key
}

func cpaQuotaHealthNextProbeAt(result *CPAQuotaHealthResponse) int64 {
	now := common.GetTimestamp()
	if result == nil {
		return now
	}
	next := result.NextAvailableAt
	if next <= now && result.RetryAfterSeconds > 0 {
		next = now + result.RetryAfterSeconds
	}
	if next > now {
		return next + cpaQuotaHealthGraceSeconds
	}
	return now
}

func sleepWithContext(ctx context.Context, delay time.Duration) bool {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
