package model

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalizeTokenGroupConfigSortsDeduplicatesAndDefaults(t *testing.T) {
	cfg := NormalizeTokenGroupConfig(TokenGroupConfig{
		Groups: []TokenGroupItem{
			{Group: "b", Order: 2},
			{Group: " a ", Order: 1},
			{Group: "b", Order: 3},
			{Group: " ", Order: 4},
			{Group: "c", Order: 0},
		},
		FailoverStrategy: "bad",
		TimeoutSeconds:   -3,
		CooldownSeconds:  -1,
		RecoveryStrategy: "bad",
	}, "fallback")

	require.Equal(t, TokenGroupFailoverStrategyFallback, cfg.FailoverStrategy)
	require.Equal(t, 0, cfg.TimeoutSeconds)
	require.Equal(t, DefaultTokenGroupCooldownSeconds, cfg.CooldownSeconds)
	require.Equal(t, TokenGroupRecoveryStrategyProbeThenSwitch, cfg.RecoveryStrategy)
	require.Equal(t, TokenGroupDetectionStrategyOne, cfg.FailureDetectionStrategy)
	require.Equal(t, DefaultTokenGroupDetectionRatio, cfg.FailureDetectionRatio)
	require.Equal(t, TokenGroupDetectionStrategyOne, cfg.RecoveryDetectionStrategy)
	require.Equal(t, DefaultTokenGroupDetectionRatio, cfg.RecoveryDetectionRatio)
	require.Equal(t, []TokenGroupItem{
		{Group: "a", Order: 1},
		{Group: "b", Order: 2},
		{Group: "c", Order: 5},
	}, cfg.Groups)
}

func TestDefaultSingleGroupPreservesLegacyBehavior(t *testing.T) {
	token := &Token{Group: "default"}

	cfg := token.ParseGroupConfig()
	require.Equal(t, TokenGroupFailoverStrategyReturnError, cfg.FailoverStrategy)
	require.Equal(t, 0, cfg.TimeoutSeconds)
	require.Equal(t, DefaultTokenGroupCooldownSeconds, cfg.CooldownSeconds)
	require.Equal(t, TokenGroupRecoveryStrategyProbeThenSwitch, cfg.RecoveryStrategy)
	require.Equal(t, TokenGroupDetectionStrategyOne, cfg.FailureDetectionStrategy)
	require.Equal(t, TokenGroupDetectionStrategyOne, cfg.RecoveryDetectionStrategy)
	require.Equal(t, []string{"default"}, token.OrderedGroups())

	err := token.SetGroupConfig(TokenGroupConfig{
		Groups:                    []TokenGroupItem{{Group: "default", Order: 1}},
		FailoverStrategy:          TokenGroupFailoverStrategyReturnError,
		TimeoutSeconds:            0,
		CooldownSeconds:           DefaultTokenGroupCooldownSeconds,
		RecoveryStrategy:          TokenGroupRecoveryStrategyProbeThenSwitch,
		FailureDetectionStrategy:  TokenGroupDetectionStrategyOne,
		FailureDetectionRatio:     DefaultTokenGroupDetectionRatio,
		RecoveryDetectionStrategy: TokenGroupDetectionStrategyOne,
		RecoveryDetectionRatio:    DefaultTokenGroupDetectionRatio,
	})
	require.NoError(t, err)
	require.Empty(t, token.GroupConfig)
	require.Equal(t, "default", token.Group)
}

func TestSetGroupConfigStoresMultiGroupAndPrimaryGroup(t *testing.T) {
	token := &Token{Group: "legacy"}

	err := token.SetGroupConfig(TokenGroupConfig{
		Groups: []TokenGroupItem{
			{Group: "second", Order: 2},
			{Group: "first", Order: 1},
		},
		FailoverStrategy:          TokenGroupFailoverStrategyFallback,
		TimeoutSeconds:            8,
		CooldownSeconds:           30,
		RecoveryStrategy:          TokenGroupRecoveryStrategySticky,
		FailureDetectionStrategy:  TokenGroupDetectionStrategyHalf,
		FailureDetectionRatio:     0.5,
		RecoveryDetectionStrategy: TokenGroupDetectionStrategyAll,
		RecoveryDetectionRatio:    1,
	})
	require.NoError(t, err)

	require.Equal(t, "first", token.Group)
	require.NotEmpty(t, token.GroupConfig)

	var stored TokenGroupConfig
	require.NoError(t, json.Unmarshal([]byte(token.GroupConfig), &stored))
	stored = NormalizeTokenGroupConfig(stored, "")
	require.Equal(t, []TokenGroupItem{
		{Group: "first", Order: 1},
		{Group: "second", Order: 2},
	}, stored.Groups)
	require.Equal(t, TokenGroupFailoverStrategyFallback, stored.FailoverStrategy)
	require.Equal(t, 8, stored.TimeoutSeconds)
	require.Equal(t, 30, stored.CooldownSeconds)
	require.Equal(t, TokenGroupRecoveryStrategySticky, stored.RecoveryStrategy)
	require.Equal(t, TokenGroupDetectionStrategyHalf, stored.FailureDetectionStrategy)
	require.Equal(t, 0.5, stored.FailureDetectionRatio)
	require.Equal(t, TokenGroupDetectionStrategyAll, stored.RecoveryDetectionStrategy)
	require.Equal(t, 1.0, stored.RecoveryDetectionRatio)
}
