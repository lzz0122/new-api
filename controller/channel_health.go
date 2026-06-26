package controller

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
)

var channelHealthProbeOnce sync.Once

type channelHealthProbeIntervalRequest struct {
	ProbeIntervalSeconds int `json:"probe_interval_seconds"`
}

type channelHealthProbeModelsRequest struct {
	ProbeModels []string `json:"probe_models"`
}

type channelHealthGroupThresholdRequest struct {
	Group            string `json:"group"`
	FailureThreshold int    `json:"failure_threshold"`
}

func ProbeChannelHealth(c *gin.Context) {
	channelID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	channel, err := model.CacheGetChannel(channelID)
	if err != nil {
		channel, err = model.GetChannelById(channelID, true)
		if err != nil {
			common.ApiError(c, err)
			return
		}
	}
	if channel.Status == common.ChannelStatusManuallyDisabled {
		common.ApiErrorMsg(c, "手动禁用的渠道不能执行健康探活")
		return
	}
	healthMap, _ := service.GetChannelHealthMap([]int{channel.Id})
	if state, ok := healthMap[channel.Id]; ok && state.ManualProbeRequestedAt > 0 {
		cooldown := int64(operation_setting.GetChannelHealthSetting().ManualProbeCooldownSeconds)
		remaining := state.ManualProbeRequestedAt + cooldown - common.GetTimestamp()
		if remaining > 0 {
			common.ApiErrorMsg(c, fmt.Sprintf("请等待 %d 秒后再手动探活", remaining))
			return
		}
	}

	testUserID, err := resolveChannelTestUserID(c)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	health, result, err := runChannelHealthProbe(channel, testUserID, true)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if result != nil && result.newAPIError != nil {
		c.JSON(http.StatusOK, gin.H{
			"success":    false,
			"message":    result.newAPIError.Error(),
			"error_code": result.newAPIError.GetErrorCode(),
			"data":       health,
		})
		return
	}
	common.ApiSuccess(c, health)
}

func MarkChannelHealthHealthy(c *gin.Context) {
	channelID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	channel, err := model.GetChannelById(channelID, true)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if channel.Status == common.ChannelStatusManuallyDisabled {
		common.ApiErrorMsg(c, "手动禁用的渠道不能手动置活")
		return
	}
	health, err := service.MarkChannelHealthHealthy(channel)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, health)
}

func GetUserChannelStatus(c *gin.Context) {
	userGroup := common.GetContextKeyString(c, constant.ContextKeyUserGroup)
	if userGroup == "" {
		userGroup = c.GetString("group")
	}
	includeAll := c.GetInt("role") >= common.RoleAdminUser
	result, err := service.GetUserChannelStatus(userGroup, includeAll)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, result)
}

func UpdateChannelHealthProbeInterval(c *gin.Context) {
	channelID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if _, err := model.GetChannelById(channelID, true); err != nil {
		common.ApiError(c, err)
		return
	}
	var req channelHealthProbeIntervalRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}
	state, err := service.UpdateChannelHealthProbeInterval(channelID, req.ProbeIntervalSeconds)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, state)
}

func UpdateChannelHealthProbeModels(c *gin.Context) {
	channelID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	channel, err := model.GetChannelById(channelID, true)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	var req channelHealthProbeModelsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}
	state, err := service.UpdateChannelHealthProbeModels(channel, req.ProbeModels)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, state)
}

func UpdateChannelHealthGroupThreshold(c *gin.Context) {
	var req channelHealthGroupThresholdRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}
	setting, err := service.UpdateChannelHealthGroupFailureThreshold(req.Group, req.FailureThreshold)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, setting)
}

func StartChannelHealthProbeScheduler() {
	if !common.IsMasterNode {
		return
	}
	channelHealthProbeOnce.Do(func() {
		go func() {
			for {
				if !operation_setting.GetChannelHealthSetting().Enabled {
					time.Sleep(30 * time.Second)
					continue
				}
				if err := runDueChannelHealthProbes(); err != nil {
					common.SysLog("channel health probe scheduler error: " + err.Error())
				}
				time.Sleep(10 * time.Second)
			}
		}()
	})
}

func runDueChannelHealthProbes() error {
	states, err := service.ListDueChannelHealthStates(operation_setting.GetChannelHealthSetting().ProbeBatchSize)
	if err != nil {
		return err
	}
	if len(states) == 0 {
		return nil
	}
	testUserID, err := resolveChannelTestUserID(nil)
	if err != nil {
		return err
	}
	for _, state := range states {
		if err := probeDueChannelHealth(state.ChannelId, testUserID); err != nil {
			common.SysLog(fmt.Sprintf("channel health probe failed: channel_id=%d, error=%v", state.ChannelId, err))
		}
		time.Sleep(common.RequestInterval)
	}
	return nil
}

func probeDueChannelHealth(channelID int, testUserID int) error {
	channel, err := model.CacheGetChannel(channelID)
	if err != nil {
		channel, err = model.GetChannelById(channelID, true)
		if err != nil {
			return err
		}
	}
	if channel.Status == common.ChannelStatusManuallyDisabled {
		return nil
	}
	_, _, err = runChannelHealthProbe(channel, testUserID, false)
	return err
}

func runChannelHealthProbe(channel *model.Channel, testUserID int, manual bool) (*model.ChannelHealthState, *testResult, error) {
	if channel == nil || channel.Id <= 0 {
		return nil, nil, fmt.Errorf("invalid channel")
	}
	healthMap, _ := service.GetChannelHealthMap([]int{channel.Id})
	var existingState *model.ChannelHealthState
	if state, ok := healthMap[channel.Id]; ok {
		existingState = &state
	}
	probeModels := service.EffectiveChannelProbeModels(channel, existingState)
	if len(probeModels) == 0 {
		return nil, nil, fmt.Errorf("channel #%d has no probe models", channel.Id)
	}
	if !manual {
		health, apiErr, handled, err := service.HandleCPAQuotaHealthBeforeProbe(context.Background(), channel, probeModels)
		if handled {
			var result *testResult
			if apiErr != nil {
				result = &testResult{newAPIError: apiErr}
			}
			return health, result, err
		}
	}
	_, _ = service.MarkChannelHealthProbing(channel.Id, manual)
	var firstSuccess *testResult
	var lastFailure *testResult
	var lastLocalErr error
	for _, probeModel := range probeModels {
		endpointType := channelHealthProbeEndpointType(channel, probeModel)
		isStream := shouldUseStreamForChannelHealthProbe(channel, endpointType)
		result := testChannel(channel, testUserID, probeModel, endpointType, isStream)
		if result.localErr != nil && result.newAPIError == nil {
			lastLocalErr = result.localErr
			continue
		}
		if result.newAPIError != nil {
			lastFailure = &result
			if err := service.RecordChannelProbeModelFailure(channel.Id, probeModel, result.newAPIError); err != nil {
				return nil, &result, err
			}
			continue
		}
		if err := service.RecordChannelProbeModelSuccess(channel.Id, probeModel); err != nil {
			return nil, &result, err
		}
		if firstSuccess == nil {
			firstSuccess = &result
		}
	}
	if firstSuccess != nil {
		health, err := service.RecordChannelProbeSuccess(firstSuccess.context, channel)
		return service.AttachChannelHealthProbeModelResults(health, channel), nil, err
	}
	if lastFailure != nil && lastFailure.newAPIError != nil {
		health, err := service.RecordChannelProbeFailure(lastFailure.context, channel, lastFailure.newAPIError)
		return service.AttachChannelHealthProbeModelResults(health, channel), lastFailure, err
	}
	service.CancelChannelHealthProbe(channel.Id)
	if lastLocalErr != nil {
		return nil, nil, lastLocalErr
	}
	return nil, nil, fmt.Errorf("channel probe failed")
}

func channelHealthProbeEndpointType(channel *model.Channel, probeModel string) string {
	endpointType := normalizeChannelTestEndpoint(channel, probeModel, "")
	if endpointType != "" {
		return endpointType
	}
	if service.IsCPAQuotaHealthChannel(channel) {
		return string(constant.EndpointTypeOpenAIResponse)
	}
	return ""
}

func shouldUseStreamForChannelHealthProbe(channel *model.Channel, endpointType string) bool {
	if shouldUseStreamForAutomaticChannelTest(channel) {
		return true
	}
	return service.IsCPAQuotaHealthChannel(channel) &&
		constant.EndpointType(endpointType) == constant.EndpointTypeOpenAIResponse
}
