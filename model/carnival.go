package model

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

const DefaultCarnivalGroup = "拼车"

type CarnivalSession struct {
	Id           int    `json:"id"`
	GroupName    string `json:"group" gorm:"column:group_name;size:64;index:idx_carnival_sessions_group_active,priority:1;index"`
	StartedAt    int64  `json:"started_at" gorm:"bigint;index"`
	EndedAt      int64  `json:"ended_at" gorm:"bigint;default:0;index:idx_carnival_sessions_group_active,priority:2;index"`
	StartedBy    int    `json:"started_by" gorm:"default:0"`
	EndedBy      int    `json:"ended_by" gorm:"default:0"`
	TotalQuota   int64  `json:"total_quota" gorm:"bigint;default:0"`
	TotalTokens  int64  `json:"total_tokens" gorm:"bigint;default:0"`
	RequestCount int    `json:"request_count" gorm:"default:0"`
	CreatedAt    int64  `json:"created_at" gorm:"bigint"`
	UpdatedAt    int64  `json:"updated_at" gorm:"bigint"`
}

type CarnivalUsage struct {
	Id               int    `json:"id"`
	SessionID        int    `json:"session_id" gorm:"index;index:idx_carnival_usage_session_user,priority:1"`
	GroupName        string `json:"group" gorm:"column:group_name;size:64;index"`
	LogID            int    `json:"log_id" gorm:"index"`
	UserID           int    `json:"user_id" gorm:"index;index:idx_carnival_usage_session_user,priority:2"`
	Username         string `json:"username" gorm:"size:64;index"`
	ModelName        string `json:"model_name" gorm:"size:64;index"`
	TokenID          int    `json:"token_id" gorm:"default:0;index"`
	TokenName        string `json:"token_name" gorm:"size:64;default:''"`
	ChannelID        int    `json:"channel_id" gorm:"index"`
	Quota            int    `json:"quota" gorm:"default:0"`
	PromptTokens     int    `json:"prompt_tokens" gorm:"default:0"`
	CompletionTokens int    `json:"completion_tokens" gorm:"default:0"`
	TokenUsed        int    `json:"token_used" gorm:"default:0"`
	CreatedAt        int64  `json:"created_at" gorm:"bigint;index"`
}

type CarnivalUsageParams struct {
	SessionID        int
	Group            string
	LogID            int
	UserID           int
	Username         string
	ModelName        string
	TokenID          int
	TokenName        string
	ChannelID        int
	Quota            int
	PromptTokens     int
	CompletionTokens int
}

type CarnivalUserUsageSummary struct {
	UserID       int    `json:"user_id"`
	Username     string `json:"username"`
	Quota        int64  `json:"quota"`
	TokenUsed    int64  `json:"token_used"`
	RequestCount int64  `json:"request_count"`
}

type CarnivalAggregateSummary struct {
	TotalQuota   int64                      `json:"total_quota"`
	TotalTokens  int64                      `json:"total_tokens"`
	RequestCount int64                      `json:"request_count"`
	Users        []CarnivalUserUsageSummary `json:"users"`
}

type CarnivalSessionSummary struct {
	ID              int                        `json:"id"`
	Group           string                     `json:"group"`
	StartedAt       int64                      `json:"started_at"`
	EndedAt         int64                      `json:"ended_at"`
	DurationSeconds int64                      `json:"duration_seconds"`
	SinceEndSeconds int64                      `json:"since_end_seconds"`
	TotalQuota      int64                      `json:"total_quota"`
	TotalTokens     int64                      `json:"total_tokens"`
	RequestCount    int64                      `json:"request_count"`
	Users           []CarnivalUserUsageSummary `json:"users"`
}

type CarnivalStatusSnapshot struct {
	Group      string                  `json:"group"`
	Active     *CarnivalSessionSummary `json:"active"`
	Last       *CarnivalSessionSummary `json:"last"`
	ServerTime int64                   `json:"server_time"`
}

type CarnivalHistorySnapshot struct {
	Group         string                   `json:"group"`
	Months        []string                 `json:"months"`
	SelectedMonth string                   `json:"selected_month"`
	Sessions      []CarnivalSessionSummary `json:"sessions"`
	MonthTotal    CarnivalAggregateSummary `json:"month_total"`
	AllTotal      CarnivalAggregateSummary `json:"all_total"`
}

func normalizeCarnivalGroup(group string) string {
	group = strings.TrimSpace(group)
	if group == "" {
		return DefaultCarnivalGroup
	}
	return group
}

func StartCarnivalSession(group string, adminID int) (*CarnivalSession, error) {
	group = normalizeCarnivalGroup(group)
	now := common.GetTimestamp()
	var session CarnivalSession
	err := LOG_DB.Transaction(func(tx *gorm.DB) error {
		var sessions []CarnivalSession
		if err := tx.Where("group_name = ? AND ended_at = 0", group).
			Order("id desc").
			Limit(1).
			Find(&sessions).Error; err != nil {
			return err
		}
		if len(sessions) > 0 {
			session = sessions[0]
			return nil
		}
		session = CarnivalSession{
			GroupName: group,
			StartedAt: now,
			StartedBy: adminID,
			CreatedAt: now,
			UpdatedAt: now,
		}
		return tx.Create(&session).Error
	})
	if err != nil {
		return nil, err
	}
	return &session, nil
}

func FinishCarnivalSession(group string, adminID int) (*CarnivalSessionSummary, error) {
	group = normalizeCarnivalGroup(group)
	now := common.GetTimestamp()
	var session CarnivalSession
	if err := LOG_DB.Transaction(func(tx *gorm.DB) error {
		var sessions []CarnivalSession
		if err := tx.Where("group_name = ? AND ended_at = 0", group).
			Order("id desc").
			Limit(1).
			Find(&sessions).Error; err != nil {
			return err
		}
		if len(sessions) == 0 {
			return gorm.ErrRecordNotFound
		}
		session = sessions[0]
		session.EndedAt = now
		session.EndedBy = adminID
		session.UpdatedAt = now
		return tx.Save(&session).Error
	}); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("当前没有进行中的狂欢")
		}
		return nil, err
	}
	return buildCarnivalSessionSummary(&session, now)
}

func GetActiveCarnivalSession(group string) (*CarnivalSession, error) {
	group = normalizeCarnivalGroup(group)
	var sessions []CarnivalSession
	err := LOG_DB.Where("group_name = ? AND ended_at = 0", group).
		Order("id desc").
		Limit(1).
		Find(&sessions).Error
	if err != nil {
		return nil, err
	}
	if len(sessions) == 0 {
		return nil, nil
	}
	return &sessions[0], nil
}

func RecordCarnivalUsage(params CarnivalUsageParams) error {
	if params.SessionID <= 0 || params.Quota <= 0 {
		return nil
	}
	group := normalizeCarnivalGroup(params.Group)
	now := common.GetTimestamp()
	tokenUsed := params.PromptTokens + params.CompletionTokens
	usage := CarnivalUsage{
		SessionID:        params.SessionID,
		GroupName:        group,
		LogID:            params.LogID,
		UserID:           params.UserID,
		Username:         params.Username,
		ModelName:        params.ModelName,
		TokenID:          params.TokenID,
		TokenName:        params.TokenName,
		ChannelID:        params.ChannelID,
		Quota:            params.Quota,
		PromptTokens:     params.PromptTokens,
		CompletionTokens: params.CompletionTokens,
		TokenUsed:        tokenUsed,
		CreatedAt:        now,
	}
	return LOG_DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&usage).Error; err != nil {
			return err
		}
		return tx.Model(&CarnivalSession{}).
			Where("id = ? AND ended_at = 0", params.SessionID).
			Updates(map[string]interface{}{
				"total_quota":   gorm.Expr("total_quota + ?", params.Quota),
				"total_tokens":  gorm.Expr("total_tokens + ?", tokenUsed),
				"request_count": gorm.Expr("request_count + ?", 1),
				"updated_at":    now,
			}).Error
	})
}

func GetCarnivalStatus(group string) (CarnivalStatusSnapshot, error) {
	group = normalizeCarnivalGroup(group)
	now := common.GetTimestamp()
	status := CarnivalStatusSnapshot{
		Group:      group,
		ServerTime: now,
	}

	active, err := GetActiveCarnivalSession(group)
	if err != nil {
		return status, err
	}
	if active != nil {
		status.Active, err = buildCarnivalSessionSummary(active, now)
		if err != nil {
			return status, err
		}
	}

	var lastSessions []CarnivalSession
	err = LOG_DB.Where("group_name = ? AND ended_at > 0", group).
		Order("ended_at desc, id desc").
		Limit(1).
		Find(&lastSessions).Error
	if err != nil {
		return status, err
	}
	if len(lastSessions) > 0 {
		status.Last, err = buildCarnivalSessionSummary(&lastSessions[0], now)
		if err != nil {
			return status, err
		}
	}

	return status, nil
}

func GetCarnivalHistory(group string, month string) (CarnivalHistorySnapshot, error) {
	group = normalizeCarnivalGroup(group)
	months, err := ListCarnivalMonths(group)
	if err != nil {
		return CarnivalHistorySnapshot{}, err
	}
	month = strings.TrimSpace(month)
	if month == "" && len(months) > 0 {
		month = months[0]
	}

	history := CarnivalHistorySnapshot{
		Group:         group,
		Months:        months,
		SelectedMonth: month,
	}

	sessionQuery := LOG_DB.Where("group_name = ?", group)
	usageQuery := LOG_DB.Where("group_name = ?", group)
	if month != "" && !strings.EqualFold(month, "all") {
		start, end, errRange := carnivalMonthRange(month)
		if errRange != nil {
			return history, errRange
		}
		sessionQuery = sessionQuery.Where("started_at >= ? AND started_at < ?", start, end)
		usageQuery = usageQuery.Where("created_at >= ? AND created_at < ?", start, end)
	}

	var sessions []CarnivalSession
	if err := sessionQuery.Order("started_at desc, id desc").Find(&sessions).Error; err != nil {
		return history, err
	}
	history.Sessions = make([]CarnivalSessionSummary, 0, len(sessions))
	now := common.GetTimestamp()
	for i := range sessions {
		summary, errSummary := buildCarnivalSessionSummary(&sessions[i], now)
		if errSummary != nil {
			return history, errSummary
		}
		history.Sessions = append(history.Sessions, *summary)
	}

	monthTotal, err := aggregateCarnivalUsage(usageQuery)
	if err != nil {
		return history, err
	}
	history.MonthTotal = monthTotal

	allTotal, err := aggregateCarnivalUsage(LOG_DB.Where("group_name = ?", group))
	if err != nil {
		return history, err
	}
	history.AllTotal = allTotal

	return history, nil
}

func ListCarnivalMonths(group string) ([]string, error) {
	group = normalizeCarnivalGroup(group)
	var sessions []CarnivalSession
	if err := LOG_DB.Select("started_at").
		Where("group_name = ?", group).
		Order("started_at desc").
		Find(&sessions).Error; err != nil {
		return nil, err
	}
	seen := make(map[string]struct{}, len(sessions))
	months := make([]string, 0)
	for _, session := range sessions {
		if session.StartedAt <= 0 {
			continue
		}
		month := time.Unix(session.StartedAt, 0).In(time.Local).Format("2006-01")
		if _, ok := seen[month]; ok {
			continue
		}
		seen[month] = struct{}{}
		months = append(months, month)
	}
	return months, nil
}

func buildCarnivalSessionSummary(session *CarnivalSession, now int64) (*CarnivalSessionSummary, error) {
	if session == nil {
		return nil, nil
	}
	aggregate, err := aggregateCarnivalUsage(LOG_DB.Where("session_id = ?", session.Id))
	if err != nil {
		return nil, err
	}
	totalQuota := aggregate.TotalQuota
	totalTokens := aggregate.TotalTokens
	requestCount := aggregate.RequestCount
	if totalQuota == 0 && session.TotalQuota > 0 {
		totalQuota = session.TotalQuota
	}
	if totalTokens == 0 && session.TotalTokens > 0 {
		totalTokens = session.TotalTokens
	}
	if requestCount == 0 && session.RequestCount > 0 {
		requestCount = int64(session.RequestCount)
	}
	end := session.EndedAt
	if end <= 0 {
		end = now
	}
	duration := end - session.StartedAt
	if duration < 0 {
		duration = 0
	}
	sinceEnd := int64(0)
	if session.EndedAt > 0 {
		sinceEnd = now - session.EndedAt
		if sinceEnd < 0 {
			sinceEnd = 0
		}
	}
	return &CarnivalSessionSummary{
		ID:              session.Id,
		Group:           session.GroupName,
		StartedAt:       session.StartedAt,
		EndedAt:         session.EndedAt,
		DurationSeconds: duration,
		SinceEndSeconds: sinceEnd,
		TotalQuota:      totalQuota,
		TotalTokens:     totalTokens,
		RequestCount:    requestCount,
		Users:           aggregate.Users,
	}, nil
}

func aggregateCarnivalUsage(tx *gorm.DB) (CarnivalAggregateSummary, error) {
	var users []CarnivalUserUsageSummary
	if err := tx.Model(&CarnivalUsage{}).
		Select("user_id, username, sum(quota) as quota, sum(token_used) as token_used, count(*) as request_count").
		Group("user_id, username").
		Order("quota desc, request_count desc, username asc").
		Find(&users).Error; err != nil {
		return CarnivalAggregateSummary{}, err
	}
	aggregate := CarnivalAggregateSummary{Users: users}
	for _, user := range users {
		aggregate.TotalQuota += user.Quota
		aggregate.TotalTokens += user.TokenUsed
		aggregate.RequestCount += user.RequestCount
	}
	return aggregate, nil
}

func carnivalMonthRange(month string) (int64, int64, error) {
	parsed, err := time.ParseInLocation("2006-01", strings.TrimSpace(month), time.Local)
	if err != nil {
		return 0, 0, fmt.Errorf("月份格式应为 YYYY-MM")
	}
	start := time.Date(parsed.Year(), parsed.Month(), 1, 0, 0, 0, 0, time.Local)
	end := start.AddDate(0, 1, 0)
	return start.Unix(), end.Unix(), nil
}
