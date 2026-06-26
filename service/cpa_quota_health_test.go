package service

import (
	"context"
	"os"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
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
