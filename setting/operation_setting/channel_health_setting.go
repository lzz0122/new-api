package operation_setting

import "github.com/QuantumNous/new-api/setting/config"

type ChannelHealthSetting struct {
	Enabled                    bool `json:"enabled"`
	FailureThreshold           int  `json:"failure_threshold"`
	ProbeIntervalSeconds       int  `json:"probe_interval_seconds"`
	ProbeBatchSize             int  `json:"probe_batch_size"`
	ManualProbeCooldownSeconds int  `json:"manual_probe_cooldown_seconds"`
}

var channelHealthSetting = ChannelHealthSetting{
	Enabled:                    true,
	FailureThreshold:           3,
	ProbeIntervalSeconds:       600,
	ProbeBatchSize:             10,
	ManualProbeCooldownSeconds: 10,
}

func init() {
	config.GlobalConfig.Register("channel_health_setting", &channelHealthSetting)
}

func GetChannelHealthSetting() *ChannelHealthSetting {
	if channelHealthSetting.FailureThreshold <= 0 {
		channelHealthSetting.FailureThreshold = 3
	}
	if channelHealthSetting.ProbeBatchSize <= 0 {
		channelHealthSetting.ProbeBatchSize = 10
	}
	if channelHealthSetting.ManualProbeCooldownSeconds <= 0 {
		channelHealthSetting.ManualProbeCooldownSeconds = 10
	}
	return &channelHealthSetting
}
