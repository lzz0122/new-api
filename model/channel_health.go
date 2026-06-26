package model

import (
	"errors"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"gorm.io/gorm"
)

const (
	ChannelHealthStatusHealthy   = "healthy"
	ChannelHealthStatusSuspect   = "suspect"
	ChannelHealthStatusUnhealthy = "unhealthy"
	ChannelHealthStatusProbing   = "probing"
)

const (
	ChannelHealthEventFailureRecorded = "failure_recorded"
	ChannelHealthEventMarkedUnhealthy = "marked_unhealthy"
	ChannelHealthEventProbeSuccess    = "probe_success"
	ChannelHealthEventProbeFailure    = "probe_failure"
	ChannelHealthEventRecovered       = "recovered"
	ChannelHealthEventManualProbe     = "manual_probe"
)

type ChannelHealthState struct {
	ChannelId                int                            `json:"channel_id" gorm:"primaryKey;autoIncrement:false"`
	Status                   string                         `json:"status" gorm:"type:varchar(32);index:idx_channel_health_status_next_probe,priority:1;default:'healthy'"`
	FailureCount             int                            `json:"failure_count" gorm:"default:0"`
	FailureThreshold         int                            `json:"failure_threshold" gorm:"default:3"`
	ProbeIntervalSeconds     *int                           `json:"probe_interval_seconds,omitempty" gorm:"default:null"`
	ProbeModels              string                         `json:"-" gorm:"type:text"`
	LastSuccessAt            int64                          `json:"last_success_at" gorm:"bigint;default:0"`
	LastFailureAt            int64                          `json:"last_failure_at" gorm:"bigint;index;default:0"`
	LastProbeAt              int64                          `json:"last_probe_at" gorm:"bigint;default:0"`
	NextProbeAt              int64                          `json:"next_probe_at" gorm:"bigint;index:idx_channel_health_status_next_probe,priority:2;default:0"`
	ProbeStartedAt           int64                          `json:"probe_started_at" gorm:"bigint;default:0"`
	ProbeAttempts            int                            `json:"probe_attempts" gorm:"default:0"`
	LastStatusCode           int                            `json:"last_status_code" gorm:"default:0"`
	LastErrorCode            string                         `json:"last_error_code" gorm:"type:varchar(128);default:''"`
	LastError                string                         `json:"last_error" gorm:"type:text"`
	LastModel                string                         `json:"last_model" gorm:"type:varchar(255);default:''"`
	LastGroup                string                         `json:"last_group" gorm:"type:varchar(64);default:''"`
	LastTokenId              int                            `json:"last_token_id" gorm:"default:0"`
	LastRequestId            string                         `json:"last_request_id" gorm:"type:varchar(64);default:''"`
	ManualProbeRequestedAt   int64                          `json:"manual_probe_requested_at" gorm:"bigint;default:0"`
	UpdatedAt                int64                          `json:"updated_at" gorm:"bigint;default:0"`
	AffectedGroups           string                         `json:"affected_groups,omitempty" gorm:"-"`
	NextProbeRemainingSecond int64                          `json:"next_probe_remaining_seconds,omitempty" gorm:"-"`
	AutoProbeEnabled         bool                           `json:"auto_probe_enabled" gorm:"-"`
	EffectiveProbeInterval   int                            `json:"effective_probe_interval_seconds" gorm:"-"`
	ConfiguredProbeModels    []string                       `json:"probe_models,omitempty" gorm:"-"`
	ProbeModelResults        []ChannelHealthProbeModelState `json:"probe_model_results,omitempty" gorm:"-"`
}

type ChannelHealthGroupSetting struct {
	GroupName        string `json:"group" gorm:"column:group_name;type:varchar(64);primaryKey;autoIncrement:false"`
	FailureThreshold int    `json:"failure_threshold" gorm:"default:3"`
	UpdatedAt        int64  `json:"updated_at" gorm:"bigint;default:0"`
}

type ChannelHealthGroupState struct {
	ChannelId      int    `json:"channel_id" gorm:"primaryKey;autoIncrement:false"`
	GroupName      string `json:"group" gorm:"column:group_name;type:varchar(64);primaryKey;autoIncrement:false"`
	Status         string `json:"status" gorm:"type:varchar(32);index;default:'healthy'"`
	FailureCount   int    `json:"failure_count" gorm:"default:0"`
	LastSuccessAt  int64  `json:"last_success_at" gorm:"bigint;default:0"`
	LastFailureAt  int64  `json:"last_failure_at" gorm:"bigint;index;default:0"`
	LastStatusCode int    `json:"last_status_code" gorm:"default:0"`
	LastErrorCode  string `json:"last_error_code" gorm:"type:varchar(128);default:''"`
	LastError      string `json:"last_error" gorm:"type:text"`
	LastModel      string `json:"last_model" gorm:"type:varchar(255);default:''"`
	LastTokenId    int    `json:"last_token_id" gorm:"default:0"`
	LastRequestId  string `json:"last_request_id" gorm:"type:varchar(64);default:''"`
	UpdatedAt      int64  `json:"updated_at" gorm:"bigint;default:0"`
}

type ChannelHealthProbeModelState struct {
	ChannelId      int    `json:"channel_id" gorm:"primaryKey;autoIncrement:false"`
	Model          string `json:"model" gorm:"type:varchar(255);primaryKey;autoIncrement:false"`
	Status         string `json:"status" gorm:"type:varchar(32);index;default:''"`
	LastProbeAt    int64  `json:"last_probe_at" gorm:"bigint;default:0"`
	LastSuccessAt  int64  `json:"last_success_at" gorm:"bigint;default:0"`
	LastFailureAt  int64  `json:"last_failure_at" gorm:"bigint;default:0"`
	LastStatusCode int    `json:"last_status_code" gorm:"default:0"`
	LastErrorCode  string `json:"last_error_code" gorm:"type:varchar(128);default:''"`
	LastError      string `json:"last_error" gorm:"type:text"`
	UpdatedAt      int64  `json:"updated_at" gorm:"bigint;default:0"`
}

func GetChannelHealthGroupFailureThreshold(group string) int {
	thresholds := GetChannelHealthGroupFailureThresholds([]string{group})
	group = strings.TrimSpace(group)
	if group == "" {
		group = "default"
	}
	return thresholds[group]
}

func GetChannelHealthGroupFailureThresholds(groups []string) map[string]int {
	defaultThreshold := operation_setting.GetChannelHealthSetting().FailureThreshold
	result := make(map[string]int, len(groups))
	normalizedSet := make(map[string]struct{}, len(groups))
	normalizedGroups := make([]string, 0, len(groups))
	for _, group := range groups {
		group = strings.TrimSpace(group)
		if group == "" {
			group = "default"
		}
		if _, ok := normalizedSet[group]; ok {
			continue
		}
		normalizedSet[group] = struct{}{}
		normalizedGroups = append(normalizedGroups, group)
		result[group] = defaultThreshold
	}
	if len(normalizedGroups) == 0 {
		return result
	}
	var settings []ChannelHealthGroupSetting
	if err := DB.Where("group_name IN ?", normalizedGroups).Find(&settings).Error; err != nil {
		return result
	}
	for _, setting := range settings {
		group := strings.TrimSpace(setting.GroupName)
		if group == "" {
			group = "default"
		}
		result[group] = setting.FailureThreshold
	}
	return result
}

func GetChannelHealthMinimumPositiveFailureThreshold(channelID int) int {
	if channelID <= 0 {
		return operation_setting.GetChannelHealthSetting().FailureThreshold
	}
	var groups []string
	if err := DB.Model(&Ability{}).
		Where("channel_id = ?", channelID).
		Distinct(GroupColumn()).
		Pluck(GroupColumn(), &groups).Error; err != nil || len(groups) == 0 {
		return operation_setting.GetChannelHealthSetting().FailureThreshold
	}
	thresholds := GetChannelHealthGroupFailureThresholds(groups)
	minThreshold := 0
	for _, group := range groups {
		group = strings.TrimSpace(group)
		if group == "" {
			group = "default"
		}
		threshold, ok := thresholds[group]
		if !ok {
			threshold = operation_setting.GetChannelHealthSetting().FailureThreshold
		}
		if threshold <= 0 {
			continue
		}
		if minThreshold == 0 || threshold < minThreshold {
			minThreshold = threshold
		}
	}
	return minThreshold
}

func IsChannelHealthUnavailableForGroup(state *ChannelHealthState, group string) bool {
	if !operation_setting.GetChannelHealthSetting().Enabled || state == nil {
		return false
	}
	threshold := GetChannelHealthGroupFailureThreshold(group)
	return IsChannelHealthUnavailableForThreshold(state, threshold)
}

func IsChannelHealthUnavailableForThreshold(state *ChannelHealthState, threshold int) bool {
	if state == nil || threshold <= 0 {
		return false
	}
	if state.Status == ChannelHealthStatusProbing {
		return true
	}
	if state.LastSuccessAt > 0 && state.LastSuccessAt > state.LastFailureAt {
		return false
	}
	return state.FailureCount >= threshold
}

func IsChannelHealthGroupUnavailableForThreshold(state *ChannelHealthGroupState, threshold int) bool {
	if state == nil || threshold <= 0 {
		return false
	}
	if state.LastSuccessAt > 0 && state.LastSuccessAt > state.LastFailureAt {
		return false
	}
	return state.FailureCount >= threshold
}

func IsChannelHealthUnavailableForGroupState(globalState *ChannelHealthState, groupState *ChannelHealthGroupState, group string, threshold int) bool {
	if threshold <= 0 {
		return false
	}
	if groupState != nil {
		return IsChannelHealthGroupUnavailableForThreshold(groupState, threshold)
	}
	if globalState == nil {
		return false
	}
	lastGroup := strings.TrimSpace(globalState.LastGroup)
	group = strings.TrimSpace(group)
	if lastGroup == "" || lastGroup != group {
		return false
	}
	return IsChannelHealthUnavailableForThreshold(globalState, threshold)
}

func DecodeChannelHealthProbeModels(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var models []string
	if err := common.UnmarshalJsonStr(raw, &models); err != nil {
		return nil
	}
	return NormalizeChannelHealthProbeModels(models, nil)
}

func EncodeChannelHealthProbeModels(models []string) (string, error) {
	models = NormalizeChannelHealthProbeModels(models, nil)
	if len(models) == 0 {
		return "", nil
	}
	data, err := common.Marshal(models)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func NormalizeChannelHealthProbeModels(models []string, allowed []string) []string {
	allowedSet := make(map[string]struct{}, len(allowed))
	for _, modelName := range allowed {
		modelName = strings.TrimSpace(modelName)
		if modelName != "" {
			allowedSet[modelName] = struct{}{}
		}
	}
	result := make([]string, 0, len(models))
	seen := make(map[string]struct{}, len(models))
	for _, modelName := range models {
		modelName = strings.TrimSpace(modelName)
		if modelName == "" {
			continue
		}
		if len(allowedSet) > 0 {
			if _, ok := allowedSet[modelName]; !ok {
				continue
			}
		}
		if _, ok := seen[modelName]; ok {
			continue
		}
		seen[modelName] = struct{}{}
		result = append(result, modelName)
	}
	return result
}

func UpsertChannelHealthGroupSetting(group string, failureThreshold int, updatedAt int64) (ChannelHealthGroupSetting, error) {
	group = strings.TrimSpace(group)
	if group == "" {
		group = "default"
	}
	setting := ChannelHealthGroupSetting{
		GroupName:        group,
		FailureThreshold: failureThreshold,
		UpdatedAt:        updatedAt,
	}
	var existing ChannelHealthGroupSetting
	err := DB.Where("group_name = ?", group).First(&existing).Error
	if err == nil {
		existing.FailureThreshold = failureThreshold
		existing.UpdatedAt = updatedAt
		return existing, DB.Save(&existing).Error
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return setting, err
	}
	return setting, DB.Create(&setting).Error
}

type ChannelHealthEvent struct {
	Id          int    `json:"id"`
	ChannelId   int    `json:"channel_id" gorm:"index"`
	EventType   string `json:"event_type" gorm:"type:varchar(64);index"`
	FromStatus  string `json:"from_status" gorm:"type:varchar(32);default:''"`
	ToStatus    string `json:"to_status" gorm:"type:varchar(32);default:''"`
	GroupName   string `json:"group" gorm:"column:group_name;type:varchar(64);index;default:''"`
	Model       string `json:"model" gorm:"type:varchar(255);index;default:''"`
	StatusCode  int    `json:"status_code" gorm:"default:0"`
	ErrorCode   string `json:"error_code" gorm:"type:varchar(128);default:''"`
	Message     string `json:"message" gorm:"type:text"`
	CreatedAt   int64  `json:"created_at" gorm:"bigint;index"`
	RequestId   string `json:"request_id" gorm:"type:varchar(64);default:''"`
	TokenId     int    `json:"token_id" gorm:"default:0"`
	ChannelName string `json:"channel_name" gorm:"type:varchar(255);default:''"`
}
