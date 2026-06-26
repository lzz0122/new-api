package service

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
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
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.ChannelHealthState{}, &model.ChannelHealthGroupState{}, &model.ChannelHealthGroupSetting{}, &model.ChannelHealthProbeModelState{}, &model.ChannelHealthEvent{}, &model.Ability{}))
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

func TestMarkChannelHealthHealthyClearsFailuresAndAutoDisabledStatus(t *testing.T) {
	db := setupChannelHealthTestDB(t)
	withChannelHealthSetting(t, func(setting *operation_setting.ChannelHealthSetting) {
		setting.Enabled = true
		setting.FailureThreshold = 3
		setting.ProbeIntervalSeconds = 600
	})
	channel := &model.Channel{
		Id:     11,
		Name:   "manual-healthy-channel",
		Status: common.ChannelStatusAutoDisabled,
		Models: "gpt-5.5",
		Group:  "default",
	}
	require.NoError(t, db.Create(channel).Error)
	require.NoError(t, db.Create(&model.Ability{
		Group:     "default",
		Model:     "gpt-5.5",
		ChannelId: channel.Id,
		Enabled:   false,
	}).Error)
	require.NoError(t, db.Create(&model.ChannelHealthState{
		ChannelId:        channel.Id,
		Status:           model.ChannelHealthStatusUnhealthy,
		FailureCount:     5,
		FailureThreshold: 3,
		LastFailureAt:    100,
		NextProbeAt:      700,
		LastStatusCode:   503,
		LastErrorCode:    "bad_response_status_code",
		LastError:        "auth_unavailable",
		UpdatedAt:        100,
	}).Error)
	require.NoError(t, db.Create(&model.ChannelHealthGroupState{
		ChannelId:     channel.Id,
		GroupName:     "default",
		Status:        model.ChannelHealthStatusUnhealthy,
		FailureCount:  3,
		LastFailureAt: 100,
		UpdatedAt:     100,
	}).Error)
	require.NoError(t, db.Create(&model.ChannelHealthProbeModelState{
		ChannelId:      channel.Id,
		Model:          "gpt-5.5",
		Status:         model.ChannelHealthStatusUnhealthy,
		LastFailureAt:  100,
		LastStatusCode: 503,
		LastError:      "auth_unavailable",
		UpdatedAt:      100,
	}).Error)

	health, err := MarkChannelHealthHealthy(channel)
	require.NoError(t, err)
	require.NotNil(t, health)
	require.Equal(t, model.ChannelHealthStatusHealthy, health.Status)
	require.Zero(t, health.FailureCount)
	require.Zero(t, health.NextProbeAt)
	require.Empty(t, health.LastError)
	require.Len(t, health.ProbeModelResults, 1)
	require.Empty(t, health.ProbeModelResults[0].Status)

	var stored model.ChannelHealthState
	require.NoError(t, db.First(&stored, "channel_id = ?", channel.Id).Error)
	require.Equal(t, model.ChannelHealthStatusHealthy, stored.Status)
	require.Zero(t, stored.FailureCount)

	var groupState model.ChannelHealthGroupState
	require.NoError(t, db.First(&groupState, "channel_id = ? AND group_name = ?", channel.Id, "default").Error)
	require.Equal(t, model.ChannelHealthStatusHealthy, groupState.Status)
	require.Zero(t, groupState.FailureCount)

	var modelStateCount int64
	require.NoError(t, db.Model(&model.ChannelHealthProbeModelState{}).Where("channel_id = ?", channel.Id).Count(&modelStateCount).Error)
	require.Zero(t, modelStateCount)

	var refreshed model.Channel
	require.NoError(t, db.First(&refreshed, "id = ?", channel.Id).Error)
	require.Equal(t, common.ChannelStatusEnabled, refreshed.Status)
}
