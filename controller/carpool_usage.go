package controller

import (
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
)

type carpoolFinishRequest struct {
	Group string `json:"group"`
	Code  string `json:"code"`
}

func normalizeCarpoolGroupQuery(c *gin.Context) string {
	group := strings.TrimSpace(c.DefaultQuery("group", model.DefaultCarnivalGroup))
	if group == "" {
		return model.DefaultCarnivalGroup
	}
	return group
}

func ensureCarpoolGroupVisible(c *gin.Context, group string) bool {
	role := c.GetInt("role")
	if role >= common.RoleAdminUser {
		return true
	}
	ok, err := model.UserParticipatesInCarpoolGroup(c.GetInt("id"), group)
	if err != nil {
		common.ApiError(c, err)
		return false
	}
	if !ok {
		common.ApiErrorMsg(c, "当前用户未参加该分组拼车")
		return false
	}
	return true
}

func GetCarpoolGroups(c *gin.Context) {
	role := c.GetInt("role")
	groupSet := map[string]struct{}{}
	if role >= common.RoleAdminUser {
		for group := range ratio_setting.GetGroupRatioCopy() {
			group = strings.TrimSpace(group)
			if group != "" {
				groupSet[group] = struct{}{}
			}
		}
		sessionGroups, err := model.ListCarpoolSessionGroups()
		if err != nil {
			common.ApiError(c, err)
			return
		}
		for _, group := range sessionGroups {
			groupSet[group] = struct{}{}
		}
		groupSet[model.DefaultCarnivalGroup] = struct{}{}
	} else {
		groups, err := model.ListUserCarpoolGroups(c.GetInt("id"))
		if err != nil {
			common.ApiError(c, err)
			return
		}
		for _, group := range groups {
			groupSet[group] = struct{}{}
		}
	}
	groups := make([]string, 0, len(groupSet))
	for group := range groupSet {
		groups = append(groups, group)
	}
	sort.Strings(groups)
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"groups":        groups,
			"default_group": model.DefaultCarnivalGroup,
		},
	})
}

func GetCarpoolStatus(c *gin.Context) {
	group := normalizeCarpoolGroupQuery(c)
	if !ensureCarpoolGroupVisible(c, group) {
		return
	}
	status, err := model.GetCarpoolStatus(group)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, status)
}

func GetCarpoolUsageSummary(c *gin.Context) {
	group := normalizeCarpoolGroupQuery(c)
	if !ensureCarpoolGroupVisible(c, group) {
		return
	}
	sessionID := 0
	if rawSessionID := strings.TrimSpace(c.Query("session_id")); rawSessionID != "" {
		sessionID, _ = strconv.Atoi(rawSessionID)
	}
	var summary *model.CarpoolUsageSummarySnapshot
	var err error
	if strings.EqualFold(strings.TrimSpace(c.Query("scope")), "session") || sessionID > 0 {
		summary, err = model.GetCarpoolUsageSessionSummary(group, sessionID)
	} else {
		period := strings.TrimSpace(c.DefaultQuery("period", "week"))
		summary, err = model.GetCarpoolUsageSummary(group, period)
	}
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(200, summary)
}

func GetCarpoolHistory(c *gin.Context) {
	role := c.GetInt("role")
	groups := make([]string, 0)
	if role >= common.RoleAdminUser {
		for group := range ratio_setting.GetGroupRatioCopy() {
			group = strings.TrimSpace(group)
			if group != "" {
				groups = append(groups, group)
			}
		}
		sessionGroups, err := model.ListCarpoolSessionGroups()
		if err != nil {
			common.ApiError(c, err)
			return
		}
		groups = append(groups, sessionGroups...)
		groups = append(groups, model.DefaultCarnivalGroup)
	} else {
		userGroups, err := model.ListUserCarpoolGroups(c.GetInt("id"))
		if err != nil {
			common.ApiError(c, err)
			return
		}
		groups = userGroups
		if len(groups) == 0 {
			common.ApiSuccess(c, model.CarpoolHistorySnapshot{
				Months: make([]string, 0),
				Groups: make([]model.CarpoolHistoryGroup, 0),
			})
			return
		}
	}
	group := strings.TrimSpace(c.Query("group"))
	if group != "" && !ensureCarpoolGroupVisible(c, group) {
		return
	}
	history, err := model.GetCarpoolHistory(groups, strings.TrimSpace(c.Query("month")), group)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, history)
}

func StartCarpool(c *gin.Context) {
	group := normalizeCarpoolGroupQuery(c)
	session, err := model.StartCarpoolSession(group, c.GetInt("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	status, err := model.GetCarpoolStatus(session.GroupName)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, status)
}

func FinishCarpool(c *gin.Context) {
	var req carpoolFinishRequest
	_ = common.DecodeJson(c.Request.Body, &req)
	group := strings.TrimSpace(req.Group)
	if group == "" {
		group = normalizeCarpoolGroupQuery(c)
	}
	expectedCode := model.GetCarpoolFinish2FACode()
	if expectedCode == "" {
		common.ApiErrorMsg(c, "请先在系统设置的安全与限制中设置拼车结束 2FA 密码")
		return
	}
	if strings.TrimSpace(req.Code) != expectedCode {
		common.ApiErrorMsg(c, "2FA 密码不正确")
		return
	}
	summary, err := model.FinishCarpoolSession(group, c.GetInt("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	status, err := model.GetCarpoolStatus(summary.Group)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, status)
}
