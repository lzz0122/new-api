package model

import (
	"errors"
	"strings"

	"github.com/QuantumNous/new-api/common"
)

type Sub2APIChannelCredential struct {
	ChannelID int
	Name      string
	BaseURL   string
	APIKey    string
}

func GetSub2APIChannelCredential(group string) (*Sub2APIChannelCredential, error) {
	group = strings.TrimSpace(group)
	if group == "" {
		return nil, errors.New("分组不能为空")
	}

	var channel Channel
	query := DB.Model(&Channel{}).Where("status = ?", common.ChannelStatusEnabled)
	query = ApplyChannelGroupFilter(query, group)
	query = query.Where("LOWER(COALESCE(base_url, '')) LIKE ?", "%sub2api.980118.xyz%")
	if err := query.Order("priority desc, id desc").First(&channel).Error; err != nil {
		return nil, err
	}

	baseURL := strings.TrimRight(strings.TrimSpace(channel.GetBaseURL()), "/")
	keys := channel.GetKeys()
	apiKey := ""
	if len(keys) > 0 {
		apiKey = strings.TrimSpace(keys[0])
	}
	if apiKey == "" {
		apiKey = strings.TrimSpace(channel.Key)
	}
	if apiKey == "" {
		return nil, errors.New("sub2api 拼车渠道未配置 apikey")
	}

	return &Sub2APIChannelCredential{
		ChannelID: channel.Id,
		Name:      channel.Name,
		BaseURL:   baseURL,
		APIKey:    apiKey,
	}, nil
}
