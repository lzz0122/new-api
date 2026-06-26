package service

import (
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupChannelHealthTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	originalDB := model.DB
	model.DB = db
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.ChannelHealthState{}, &model.ChannelHealthGroupState{}, &model.ChannelHealthGroupSetting{}, &model.ChannelHealthEvent{}, &model.Ability{}))
	t.Cleanup(func() {
		model.DB = originalDB
	})
	return db
}

func withChannelHealthSetting(t *testing.T, fn func(setting *operation_setting.ChannelHealthSetting)) {
	t.Helper()
	setting := operation_setting.GetChannelHealthSetting()
	original := *setting
	fn(setting)
	t.Cleanup(func() {
		*setting = original
	})
}

func TestListDueChannelHealthStatesDisabledWhenProbeIntervalNegative(t *testing.T) {
	db := setupChannelHealthTestDB(t)
	withChannelHealthSetting(t, func(setting *operation_setting.ChannelHealthSetting) {
		setting.Enabled = true
		setting.FailureThreshold = 3
		setting.ProbeIntervalSeconds = -1
		setting.ProbeBatchSize = 10
	})
	require.NoError(t, db.Create(&model.ChannelHealthState{
		ChannelId:        1,
		Status:           model.ChannelHealthStatusUnhealthy,
		FailureCount:     3,
		LastFailureAt:    100,
		NextProbeAt:      101,
		FailureThreshold: 3,
		UpdatedAt:        100,
	}).Error)

	states, err := ListDueChannelHealthStates(10)
	require.NoError(t, err)
	require.Empty(t, states)

	state := decorateChannelHealthState(&model.ChannelHealthState{
		ChannelId:     1,
		Status:        model.ChannelHealthStatusUnhealthy,
		LastFailureAt: 100,
		NextProbeAt:   101,
		UpdatedAt:     100,
	})
	require.False(t, state.AutoProbeEnabled)
	require.Zero(t, state.NextProbeAt)
	require.Zero(t, state.NextProbeRemainingSecond)
}

func TestRescheduleUnhealthyChannelHealthProbesPreservesGroupThreshold(t *testing.T) {
	db := setupChannelHealthTestDB(t)
	withChannelHealthSetting(t, func(setting *operation_setting.ChannelHealthSetting) {
		setting.Enabled = true
		setting.FailureThreshold = 4
		setting.ProbeIntervalSeconds = 120
		setting.ProbeBatchSize = 10
	})
	require.NoError(t, db.Create(&model.ChannelHealthState{
		ChannelId:        2,
		Status:           model.ChannelHealthStatusUnhealthy,
		FailureCount:     3,
		LastFailureAt:    100,
		NextProbeAt:      101,
		FailureThreshold: 3,
		UpdatedAt:        100,
	}).Error)

	require.NoError(t, RescheduleUnhealthyChannelHealthProbes())

	var state model.ChannelHealthState
	require.NoError(t, db.First(&state, "channel_id = ?", 2).Error)
	require.Equal(t, 3, state.FailureThreshold)
	require.Greater(t, state.UpdatedAt, int64(100))
	require.Equal(t, state.UpdatedAt+120, state.NextProbeAt)
}

func TestListDueChannelHealthStatesUsesPerChannelProbeInterval(t *testing.T) {
	db := setupChannelHealthTestDB(t)
	withChannelHealthSetting(t, func(setting *operation_setting.ChannelHealthSetting) {
		setting.Enabled = true
		setting.FailureThreshold = 3
		setting.ProbeIntervalSeconds = -1
		setting.ProbeBatchSize = 10
	})
	interval := 1
	require.NoError(t, db.Create(&model.ChannelHealthState{
		ChannelId:            3,
		Status:               model.ChannelHealthStatusUnhealthy,
		FailureCount:         3,
		FailureThreshold:     3,
		ProbeIntervalSeconds: &interval,
		LastFailureAt:        100,
		UpdatedAt:            100,
	}).Error)

	states, err := ListDueChannelHealthStates(10)
	require.NoError(t, err)
	require.Len(t, states, 1)
	require.Equal(t, 3, states[0].ChannelId)
	require.True(t, states[0].AutoProbeEnabled)
	require.Equal(t, 1, states[0].EffectiveProbeInterval)
}
