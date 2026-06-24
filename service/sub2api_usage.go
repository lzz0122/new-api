package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"
)

const Sub2APIUsageRefreshInterval = time.Minute

type Sub2APIUsageResponse struct {
	Group         string             `json:"group"`
	BaseURL       string             `json:"base_url"`
	KeyName       string             `json:"key_name,omitempty"`
	MaskedKey     string             `json:"masked_key,omitempty"`
	KeyStatus     string             `json:"key_status,omitempty"`
	UpstreamGroup string             `json:"upstream_group,omitempty"`
	RateLimits    []Sub2APIRateLimit `json:"rate_limits"`
	Concurrency   *Sub2APIConcurrency `json:"concurrency,omitempty"`
	UpdatedAt     int64              `json:"updated_at"`
	NextRefreshAt int64              `json:"next_refresh_at"`
	Cached        bool               `json:"cached"`
}

type Sub2APIRateLimit struct {
	Window    string  `json:"window"`
	Limit     float64 `json:"limit"`
	Used      float64 `json:"used"`
	Remaining float64 `json:"remaining"`
	ResetAt   string  `json:"reset_at,omitempty"`
}

type Sub2APIConcurrency struct {
	Used      float64 `json:"used"`
	Limit     float64 `json:"limit,omitempty"`
	Remaining float64 `json:"remaining,omitempty"`
	UpdatedAt string  `json:"updated_at,omitempty"`
}

type sub2APIUsageCacheEntry struct {
	data      Sub2APIUsageResponse
	updatedAt time.Time
}

var sub2APIUsageCache = struct {
	sync.Mutex
	items map[string]sub2APIUsageCacheEntry
	group singleflight.Group
}{
	items: make(map[string]sub2APIUsageCacheEntry),
}

func GetSub2APIUsage(ctx context.Context, group string, baseURL string, apiKey string, refresh bool) (Sub2APIUsageResponse, error) {
	group = strings.TrimSpace(group)
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	apiKey = strings.TrimSpace(apiKey)
	if baseURL == "" || apiKey == "" {
		return Sub2APIUsageResponse{}, errors.New("拼车渠道配置不完整")
	}
	cacheKey := sub2APIUsageCacheKey(baseURL, apiKey)
	now := time.Now()

	sub2APIUsageCache.Lock()
	if cached, ok := sub2APIUsageCache.items[cacheKey]; ok {
		age := now.Sub(cached.updatedAt)
		if age < Sub2APIUsageRefreshInterval {
			data := cached.data
			data.Group = group
			data.Cached = true
			data.NextRefreshAt = cached.updatedAt.Add(Sub2APIUsageRefreshInterval).Unix()
			sub2APIUsageCache.Unlock()
			return data, nil
		}
		if !refresh {
			data := cached.data
			data.Group = group
			data.Cached = true
			data.NextRefreshAt = now.Unix()
			sub2APIUsageCache.Unlock()
			return data, nil
		}
	}
	sub2APIUsageCache.Unlock()

	value, err, _ := sub2APIUsageCache.group.Do(cacheKey, func() (any, error) {
		data, errFetch := fetchSub2APIUsage(ctx, group, baseURL, apiKey)
		if errFetch != nil {
			return Sub2APIUsageResponse{}, errFetch
		}
		updatedAt := time.Now()
		data.UpdatedAt = updatedAt.Unix()
		data.NextRefreshAt = updatedAt.Add(Sub2APIUsageRefreshInterval).Unix()

		sub2APIUsageCache.Lock()
		sub2APIUsageCache.items[cacheKey] = sub2APIUsageCacheEntry{
			data:      data,
			updatedAt: updatedAt,
		}
		sub2APIUsageCache.Unlock()
		return data, nil
	})
	if err != nil {
		sub2APIUsageCache.Lock()
		cached, ok := sub2APIUsageCache.items[cacheKey]
		sub2APIUsageCache.Unlock()
		if ok {
			data := cached.data
			data.Group = group
			data.Cached = true
			data.NextRefreshAt = now.Unix()
			return data, nil
		}
		return Sub2APIUsageResponse{}, err
	}
	data := value.(Sub2APIUsageResponse)
	data.Group = group
	return data, nil
}

func sub2APIUsageCacheKey(baseURL string, apiKey string) string {
	sum := sha256.Sum256([]byte(baseURL + "\x00" + apiKey))
	return hex.EncodeToString(sum[:])
}

func fetchSub2APIUsage(ctx context.Context, group string, baseURL string, apiKey string) (Sub2APIUsageResponse, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return Sub2APIUsageResponse{}, err
	}
	client := &http.Client{
		Timeout: 15 * time.Second,
		Jar:     jar,
	}
	panelAPI := sub2APIPanelAPIURL(baseURL)

	loginBody, _ := json.Marshal(map[string]string{"apiKey": apiKey})
	var loginResult struct {
		Session any    `json:"session"`
		Error   string `json:"error"`
	}
	if err := sub2APIRequest(ctx, client, http.MethodPost, panelAPI+"/session", loginBody, &loginResult); err != nil {
		return Sub2APIUsageResponse{}, err
	}

	var current map[string]any
	if err := sub2APIRequest(ctx, client, http.MethodGet, panelAPI+"/key/current", nil, &current); err != nil {
		return Sub2APIUsageResponse{}, err
	}
	currentKey := sub2APIMapValue(current, "currentKey", "current_key")

	rateLimitItems := sub2APIArrayValue(currentKey, "rateLimits", "rate_limits")
	if len(rateLimitItems) == 0 {
		rateLimitItems = sub2APIArrayValue(current, "rateLimits", "rate_limits")
	}
	rateLimits := make([]Sub2APIRateLimit, 0, len(rateLimitItems))
	for _, rawItem := range rateLimitItems {
		item, ok := rawItem.(map[string]any)
		if !ok {
			continue
		}
		window := strings.TrimSpace(sub2APIStringValue(item, "window"))
		if window != "5h" && window != "7d" {
			continue
		}
		limit, _ := sub2APIFloatValue(item, "limit")
		used, _ := sub2APIFloatValue(item, "used")
		remaining, _ := sub2APIFloatValue(item, "remaining")
		rateLimits = append(rateLimits, Sub2APIRateLimit{
			Window:    window,
			Limit:     limit,
			Used:      used,
			Remaining: remaining,
			ResetAt:   sub2APIStringValue(item, "resetAt", "reset_at"),
		})
	}
	if len(rateLimits) == 0 {
		return Sub2APIUsageResponse{}, errors.New("sub2api 未返回 5h/周用量")
	}

	return Sub2APIUsageResponse{
		Group:         group,
		BaseURL:       baseURL,
		KeyName:       sub2APIStringValue(currentKey, "keyName", "key_name"),
		MaskedKey:     sub2APIStringValue(currentKey, "maskedKey", "masked_key"),
		KeyStatus:     sub2APIStringValue(currentKey, "keyStatus", "key_status"),
		UpstreamGroup: sub2APIStringValue(currentKey, "groupName", "group_name"),
		RateLimits:    rateLimits,
		Concurrency:   sub2APIExtractConcurrency(current, currentKey),
		Cached:        false,
	}, nil
}

func sub2APIPanelAPIURL(baseURL string) string {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	switch {
	case strings.HasSuffix(baseURL, "/panel/api"):
		return baseURL
	case strings.HasSuffix(baseURL, "/panel"):
		return baseURL + "/api"
	default:
		return baseURL + "/panel/api"
	}
}

func sub2APIExtractConcurrency(payload map[string]any, currentKey map[string]any) *Sub2APIConcurrency {
	if concurrency := sub2APIConcurrencyFromMap(currentKey); concurrency != nil {
		return concurrency
	}
	if concurrency := sub2APIConcurrencyFromMap(payload); concurrency != nil {
		return concurrency
	}
	return sub2APIFindConcurrency(payload, 0)
}

func sub2APIConcurrencyFromMap(data map[string]any) *Sub2APIConcurrency {
	if len(data) == 0 {
		return nil
	}
	for _, key := range []string{"concurrency", "currentConcurrency", "current_concurrency", "activeConcurrency", "active_concurrency", "realtimeConcurrency", "realtime_concurrency", "concurrencyUsage", "concurrency_usage"} {
		value, ok := sub2APIAnyValue(data, key)
		if !ok {
			continue
		}
		if nested, ok := value.(map[string]any); ok {
			if concurrency := sub2APIConcurrencyMetric(nested); concurrency != nil {
				return concurrency
			}
			continue
		}
		if used, ok := sub2APINumber(value); ok {
			if concurrency := sub2APIConcurrencyMetric(data); concurrency != nil {
				concurrency.Used = used
				return concurrency
			}
			return &Sub2APIConcurrency{Used: used}
		}
	}
	return sub2APIConcurrencyMetric(data)
}

func sub2APIConcurrencyMetric(data map[string]any) *Sub2APIConcurrency {
	used, hasUsed := sub2APIFloatValue(data, "used", "current", "active", "inflight", "inFlight", "running", "activeRequests", "active_requests", "currentRequests", "current_requests", "currentConcurrency", "current_concurrency", "usedConcurrency", "used_concurrency", "concurrencyUsed", "concurrency_used")
	limit, hasLimit := sub2APIFloatValue(data, "limit", "max", "total", "maxConcurrency", "max_concurrency", "concurrencyLimit", "concurrency_limit", "maxConcurrent", "max_concurrent")
	remaining, hasRemaining := sub2APIFloatValue(data, "remaining", "available", "availableConcurrency", "available_concurrency", "remainingConcurrency", "remaining_concurrency", "concurrencyRemaining", "concurrency_remaining")
	if !hasUsed && hasLimit && hasRemaining {
		used = limit - remaining
		hasUsed = true
	}
	if !hasRemaining && hasUsed && hasLimit {
		remaining = limit - used
		hasRemaining = true
	}
	if !hasUsed && !hasLimit && !hasRemaining {
		return nil
	}
	concurrency := &Sub2APIConcurrency{
		Used:      used,
		UpdatedAt: sub2APIStringValue(data, "updatedAt", "updated_at", "time", "timestamp"),
	}
	if hasLimit {
		concurrency.Limit = limit
	}
	if hasRemaining {
		concurrency.Remaining = remaining
	}
	return concurrency
}

func sub2APIFindConcurrency(value any, depth int) *Sub2APIConcurrency {
	if depth > 4 || value == nil {
		return nil
	}
	switch typed := value.(type) {
	case map[string]any:
		for key, nested := range typed {
			if strings.Contains(sub2APINormalizeKey(key), "concurrency") || strings.Contains(sub2APINormalizeKey(key), "concurrent") {
				if nestedMap, ok := nested.(map[string]any); ok {
					if concurrency := sub2APIConcurrencyMetric(nestedMap); concurrency != nil {
						return concurrency
					}
				}
				if used, ok := sub2APINumber(nested); ok {
					return &Sub2APIConcurrency{Used: used}
				}
			}
		}
		for _, nested := range typed {
			if concurrency := sub2APIFindConcurrency(nested, depth+1); concurrency != nil {
				return concurrency
			}
		}
	case []any:
		for _, nested := range typed {
			if concurrency := sub2APIFindConcurrency(nested, depth+1); concurrency != nil {
				return concurrency
			}
		}
	}
	return nil
}

func sub2APIMapValue(data map[string]any, keys ...string) map[string]any {
	value, ok := sub2APIAnyValue(data, keys...)
	if !ok {
		return map[string]any{}
	}
	if item, ok := value.(map[string]any); ok {
		return item
	}
	return map[string]any{}
}

func sub2APIArrayValue(data map[string]any, keys ...string) []any {
	value, ok := sub2APIAnyValue(data, keys...)
	if !ok {
		return nil
	}
	if items, ok := value.([]any); ok {
		return items
	}
	return nil
}

func sub2APIStringValue(data map[string]any, keys ...string) string {
	value, ok := sub2APIAnyValue(data, keys...)
	if !ok {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return typed
	case fmt.Stringer:
		return typed.String()
	default:
		return fmt.Sprint(typed)
	}
}

func sub2APIFloatValue(data map[string]any, keys ...string) (float64, bool) {
	value, ok := sub2APIAnyValue(data, keys...)
	if !ok {
		return 0, false
	}
	return sub2APINumber(value)
}

func sub2APIAnyValue(data map[string]any, keys ...string) (any, bool) {
	if len(data) == 0 {
		return nil, false
	}
	for _, key := range keys {
		if value, ok := data[key]; ok {
			return value, true
		}
	}
	normalizedKeys := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		normalizedKeys[sub2APINormalizeKey(key)] = struct{}{}
	}
	for key, value := range data {
		if _, ok := normalizedKeys[sub2APINormalizeKey(key)]; ok {
			return value, true
		}
	}
	return nil, false
}

func sub2APINormalizeKey(key string) string {
	key = strings.ToLower(key)
	key = strings.ReplaceAll(key, "_", "")
	key = strings.ReplaceAll(key, "-", "")
	return key
}

func sub2APINumber(value any) (float64, bool) {
	switch typed := value.(type) {
	case float64:
		return typed, true
	case float32:
		return float64(typed), true
	case int:
		return float64(typed), true
	case int64:
		return float64(typed), true
	case json.Number:
		parsed, err := typed.Float64()
		return parsed, err == nil
	case string:
		parsed, err := strconv.ParseFloat(strings.TrimSpace(typed), 64)
		return parsed, err == nil
	default:
		return 0, false
	}
}

func sub2APIRequest(ctx context.Context, client *http.Client, method string, rawURL string, body []byte, out any) error {
	if _, err := url.ParseRequestURI(rawURL); err != nil {
		return err
	}
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, rawURL, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var payload struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(data, &payload)
		if payload.Error != "" {
			return errors.New(payload.Error)
		}
		return fmt.Errorf("sub2api 返回 HTTP %d", resp.StatusCode)
	}
	if out == nil || len(data) == 0 {
		return nil
	}
	if err := json.Unmarshal(data, out); err != nil {
		return err
	}
	return nil
}
