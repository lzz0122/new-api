package controller

import (
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
)

const defaultSub2APIUsageGroup = "拼车"

func GetSub2APIUsage(c *gin.Context) {
	group := strings.TrimSpace(c.DefaultQuery("group", defaultSub2APIUsageGroup))
	if group == "" {
		group = defaultSub2APIUsageGroup
	}

	role := c.GetInt("role")
	if role < common.RoleAdminUser {
		userGroup, err := model.GetUserGroup(c.GetInt("id"), false)
		if err != nil {
			common.ApiError(c, err)
			return
		}
		if !service.GroupInUserUsableGroups(userGroup, group) {
			common.ApiErrorMsg(c, "当前用户不可使用该分组")
			return
		}
	}

	credential, err := model.GetSub2APIChannelCredential(group)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	refresh := c.Query("refresh") == "1" || strings.EqualFold(c.Query("refresh"), "true")
	usage, err := service.GetSub2APIUsage(c.Request.Context(), group, credential.BaseURL, credential.APIKey, refresh)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, usage)
}
