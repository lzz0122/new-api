package controller

import (
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
)

func normalizeCarnivalGroupQuery(c *gin.Context) string {
	group := strings.TrimSpace(c.DefaultQuery("group", model.DefaultCarnivalGroup))
	if group == "" {
		return model.DefaultCarnivalGroup
	}
	return group
}

func ensureCarnivalGroupVisible(c *gin.Context, group string) bool {
	role := c.GetInt("role")
	if role >= common.RoleAdminUser {
		return true
	}
	userGroup, err := model.GetUserGroup(c.GetInt("id"), false)
	if err != nil {
		common.ApiError(c, err)
		return false
	}
	if !service.GroupInUserUsableGroups(userGroup, group) {
		common.ApiErrorMsg(c, "当前用户不可查看该分组狂欢")
		return false
	}
	return true
}

func GetCarnivalStatus(c *gin.Context) {
	group := normalizeCarnivalGroupQuery(c)
	if !ensureCarnivalGroupVisible(c, group) {
		return
	}
	status, err := model.GetCarnivalStatus(group)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, status)
}

func GetCarnivalHistory(c *gin.Context) {
	group := normalizeCarnivalGroupQuery(c)
	if !ensureCarnivalGroupVisible(c, group) {
		return
	}
	month := strings.TrimSpace(c.Query("month"))
	history, err := model.GetCarnivalHistory(group, month)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, history)
}

func StartCarnival(c *gin.Context) {
	group := normalizeCarnivalGroupQuery(c)
	session, err := model.StartCarnivalSession(group, c.GetInt("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	status, err := model.GetCarnivalStatus(session.GroupName)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, status)
}

func FinishCarnival(c *gin.Context) {
	group := normalizeCarnivalGroupQuery(c)
	summary, err := model.FinishCarnivalSession(group, c.GetInt("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	status, err := model.GetCarnivalStatus(summary.Group)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, status)
}
