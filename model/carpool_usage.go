package model

import (
	"errors"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const DefaultCarpoolFinish2FAOption = "carpool.finish_2fa_code"
const defaultCarpoolSessionStartedAt = "2026-06-14 09:00:00"

type CarpoolUsageDailySummary struct {
	Date  string `json:"date"`
	Quota int64  `json:"quota"`
}

type CarpoolSession struct {
	Id           int    `json:"id"`
	GroupName    string `json:"group" gorm:"column:group_name;size:64;index:idx_carpool_sessions_group_active,priority:1;index"`
	StartedAt    int64  `json:"started_at" gorm:"bigint;index"`
	EndedAt      int64  `json:"ended_at" gorm:"bigint;default:0;index:idx_carpool_sessions_group_active,priority:2;index"`
	StartedBy    int    `json:"started_by" gorm:"default:0"`
	EndedBy      int    `json:"ended_by" gorm:"default:0"`
	TotalQuota   int64  `json:"total_quota" gorm:"bigint;default:0"`
	TotalTokens  int64  `json:"total_tokens" gorm:"bigint;default:0"`
	RequestCount int64  `json:"request_count" gorm:"bigint;default:0"`
	CreatedAt    int64  `json:"created_at" gorm:"bigint"`
	UpdatedAt    int64  `json:"updated_at" gorm:"bigint"`
}

type CarpoolSessionSummary struct {
	ID              int    `json:"id"`
	Group           string `json:"group"`
	StartedAt       int64  `json:"started_at"`
	EndedAt         int64  `json:"ended_at"`
	DurationSeconds int64  `json:"duration_seconds"`
	TotalQuota      int64  `json:"total_quota"`
	TotalTokens     int64  `json:"total_tokens"`
	RequestCount    int64  `json:"request_count"`
}

type CarpoolStatusSnapshot struct {
	Group      string                 `json:"group"`
	Active     *CarpoolSessionSummary `json:"active"`
	Last       *CarpoolSessionSummary `json:"last"`
	ServerTime int64                  `json:"server_time"`
}

type CarpoolHistoryGroup struct {
	Group    string                  `json:"group"`
	Total    CarpoolUsageTotals      `json:"total"`
	Sessions []CarpoolSessionSummary `json:"sessions"`
}

type CarpoolHistorySnapshot struct {
	Months        []string              `json:"months"`
	SelectedMonth string                `json:"selected_month"`
	Groups        []CarpoolHistoryGroup `json:"groups"`
}

type CarpoolUsageTokenSummary struct {
	TokenID                 int    `json:"token_id"`
	UserID                  int    `json:"user_id"`
	Name                    string `json:"name"`
	PeriodQuota             int64  `json:"period_quota"`
	CumulativeQuota         int64  `json:"cumulative_quota"`
	GrossPeriodQuota        int64  `json:"gross_period_quota"`
	GrossCumulativeQuota    int64  `json:"gross_cumulative_quota"`
	CarnivalPeriodQuota     int64  `json:"carnival_period_quota"`
	CarnivalCumulativeQuota int64  `json:"carnival_cumulative_quota"`
	CurrentCarnivalQuota    int64  `json:"current_carnival_quota"`
	PeriodTokenUsed         int64  `json:"period_token_used"`
	CumulativeTokenUsed     int64  `json:"cumulative_token_used"`
	PeriodRequestCount      int64  `json:"period_request_count"`
	CumulativeRequestCount  int64  `json:"cumulative_request_count"`
	Active                  bool   `json:"active"`
	LastSeenAt              string `json:"last_seen_at"`
	TokenStoreKnown         bool   `json:"-"`
	TokenStoreUsedQuota     int64  `json:"-"`
}

type CarpoolUsageUserSummary struct {
	UserID                  int                        `json:"user_id"`
	Username                string                     `json:"username"`
	Email                   string                     `json:"email"`
	PeriodQuota             int64                      `json:"period_quota"`
	CumulativeQuota         int64                      `json:"cumulative_quota"`
	GrossPeriodQuota        int64                      `json:"gross_period_quota"`
	GrossCumulativeQuota    int64                      `json:"gross_cumulative_quota"`
	CarnivalPeriodQuota     int64                      `json:"carnival_period_quota"`
	CarnivalCumulativeQuota int64                      `json:"carnival_cumulative_quota"`
	CurrentCarnivalQuota    int64                      `json:"current_carnival_quota"`
	PeriodTokenUsed         int64                      `json:"period_token_used"`
	CumulativeTokenUsed     int64                      `json:"cumulative_token_used"`
	PeriodRequestCount      int64                      `json:"period_request_count"`
	CumulativeRequestCount  int64                      `json:"cumulative_request_count"`
	ActiveTokens            int                        `json:"active_tokens"`
	KnownTokens             int                        `json:"known_tokens"`
	Daily                   []CarpoolUsageDailySummary `json:"daily"`
	Tokens                  []CarpoolUsageTokenSummary `json:"tokens"`
}

type CarpoolUsageTotals struct {
	PeriodQuota             int64 `json:"period_quota"`
	CumulativeQuota         int64 `json:"cumulative_quota"`
	GrossPeriodQuota        int64 `json:"gross_period_quota"`
	GrossCumulativeQuota    int64 `json:"gross_cumulative_quota"`
	CarnivalPeriodQuota     int64 `json:"carnival_period_quota"`
	CarnivalCumulativeQuota int64 `json:"carnival_cumulative_quota"`
	CurrentCarnivalQuota    int64 `json:"current_carnival_quota"`
	PeriodTokenUsed         int64 `json:"period_token_used"`
	CumulativeTokenUsed     int64 `json:"cumulative_token_used"`
	PeriodRequestCount      int64 `json:"period_request_count"`
	CumulativeRequestCount  int64 `json:"cumulative_request_count"`
	Users                   int   `json:"users"`
	ActiveTokens            int   `json:"active_tokens"`
	KnownTokens             int   `json:"known_tokens"`
}

type CarpoolUsageSyncSummary struct {
	DeltaQuota int64 `json:"delta_quota"`
}

type CarpoolUsageDailyRecord struct {
	Id           int    `json:"id" gorm:"primaryKey"`
	GroupName    string `json:"group" gorm:"column:group_name;size:64;uniqueIndex:idx_carpool_usage_daily_record,priority:1;index"`
	UsageDate    string `json:"date" gorm:"column:usage_date;size:10;uniqueIndex:idx_carpool_usage_daily_record,priority:2;index"`
	UserID       int    `json:"user_id" gorm:"uniqueIndex:idx_carpool_usage_daily_record,priority:3;index"`
	Username     string `json:"username" gorm:"size:64;default:''"`
	TokenID      int    `json:"token_id" gorm:"uniqueIndex:idx_carpool_usage_daily_record,priority:4;index"`
	TokenName    string `json:"token_name" gorm:"size:64;default:''"`
	Quota        int64  `json:"quota" gorm:"bigint;default:0"`
	TokenUsed    int64  `json:"token_used" gorm:"bigint;default:0"`
	RequestCount int64  `json:"request_count" gorm:"bigint;default:0"`
	CreatedAt    int64  `json:"created_at" gorm:"bigint"`
	UpdatedAt    int64  `json:"updated_at" gorm:"bigint"`
}

type CarpoolUsageLegacyDaily struct {
	UsageDate     string    `gorm:"column:usage_date;primaryKey;type:date"`
	UserID        int       `gorm:"column:user_id;primaryKey"`
	TokenID       int       `gorm:"column:token_id;primaryKey"`
	Username      string    `gorm:"column:username"`
	Email         string    `gorm:"column:email"`
	TokenName     string    `gorm:"column:token_name"`
	QuotaDelta    int64     `gorm:"column:quota_delta"`
	FirstSyncedAt time.Time `gorm:"column:first_synced_at"`
	LastSyncedAt  time.Time `gorm:"column:last_synced_at"`
}

func (CarpoolUsageLegacyDaily) TableName() string {
	return "carpool_usage_daily"
}

type CarpoolUsageLegacySyncRun struct {
	Id           int       `gorm:"column:id;primaryKey"`
	SyncedAt     time.Time `gorm:"column:synced_at"`
	UsageDate    string    `gorm:"column:usage_date;type:date"`
	ActiveTokens int       `gorm:"column:active_tokens"`
	ActiveUsers  int       `gorm:"column:active_users"`
	DeltaQuota   int64     `gorm:"column:delta_quota"`
}

func (CarpoolUsageLegacySyncRun) TableName() string {
	return "carpool_usage_sync_runs"
}

type CarpoolUsageDailySnapshotParams struct {
	Group        string
	UserID       int
	Username     string
	TokenID      int
	TokenName    string
	Quota        int
	TokenUsed    int
	RequestCount int64
	CreatedAt    int64
}

type CarpoolUsageSummarySnapshot struct {
	Group        string                    `json:"group"`
	Period       string                    `json:"period"`
	StartDate    string                    `json:"start_date"`
	EndDate      string                    `json:"end_date"`
	LastRunAt    string                    `json:"last_run_at"`
	QuotaPerUnit float64                   `json:"quota_per_unit"`
	Active       bool                      `json:"active"`
	Session      *CarpoolSessionSummary    `json:"session,omitempty"`
	Totals       CarpoolUsageTotals        `json:"totals"`
	LastSync     CarpoolUsageSyncSummary   `json:"last_sync"`
	Users        []CarpoolUsageUserSummary `json:"users"`
}

type carpoolUsageRow struct {
	UserID       int    `gorm:"column:user_id"`
	Username     string `gorm:"column:username"`
	Quota        int64  `gorm:"column:quota"`
	TokenUsed    int64  `gorm:"column:token_used"`
	RequestCount int64  `gorm:"column:request_count"`
}

type carpoolTokenUsageRow struct {
	TokenID      int    `gorm:"column:token_id"`
	UserID       int    `gorm:"column:user_id"`
	TokenName    string `gorm:"column:token_name"`
	Quota        int64  `gorm:"column:quota"`
	TokenUsed    int64  `gorm:"column:token_used"`
	RequestCount int64  `gorm:"column:request_count"`
	LastSeen     int64  `gorm:"column:last_seen"`
}

type carpoolDailyUsageRow struct {
	UserID int    `gorm:"column:user_id"`
	Date   string `gorm:"column:date"`
	Quota  int64  `gorm:"column:quota"`
}

type carpoolLegacyUsageSegment struct {
	Date         string
	UserID       int
	Username     string
	TokenID      int
	TokenName    string
	Quota        int64
	TokenUsed    int64
	RequestCount int64
	LastSeen     int64
}

type carpoolLegacyDailyRow struct {
	UsageDate string `gorm:"column:usage_date"`
	UserID    int    `gorm:"column:user_id"`
	Username  string `gorm:"column:username"`
	TokenID   int    `gorm:"column:token_id"`
	TokenName string `gorm:"column:token_name"`
	Quota     int64  `gorm:"column:quota"`
}

type carpoolCarnivalDailyTokenRow struct {
	CreatedAt int64  `gorm:"column:created_at"`
	UserID    int    `gorm:"column:user_id"`
	TokenName string `gorm:"column:token_name"`
	Quota     int64  `gorm:"column:quota"`
}

type carpoolTailLogRow struct {
	CreatedAt    int64  `gorm:"column:created_at"`
	UserID       int    `gorm:"column:user_id"`
	Username     string `gorm:"column:username"`
	TokenID      int    `gorm:"column:token_id"`
	TokenName    string `gorm:"column:token_name"`
	Quota        int64  `gorm:"column:quota"`
	TokenUsed    int64  `gorm:"column:token_used"`
	RequestCount int64  `gorm:"column:request_count"`
}

type carpoolQuotaDataDailyRow struct {
	UserID       int    `gorm:"column:user_id"`
	Username     string `gorm:"column:username"`
	CreatedAt    int64  `gorm:"column:created_at"`
	Quota        int64  `gorm:"column:quota"`
	TokenUsed    int64  `gorm:"column:token_used"`
	RequestCount int64  `gorm:"column:request_count"`
}

const (
	carpoolUsageFilterAll = iota
	carpoolUsageFilterNormal
	carpoolUsageFilterCarnival
	carpoolUsageFilterSession
)

func RecordCarpoolUsageDailySnapshot(params CarpoolUsageDailySnapshotParams) error {
	if LOG_DB == nil {
		return nil
	}
	group := strings.TrimSpace(params.Group)
	if group == "" || params.UserID <= 0 || params.Quota <= 0 {
		return nil
	}
	if params.CreatedAt <= 0 {
		params.CreatedAt = common.GetTimestamp()
	}
	if params.RequestCount <= 0 {
		params.RequestCount = 1
	}
	createdAt := time.Unix(params.CreatedAt, 0).In(time.Local)
	now := common.GetTimestamp()
	record := &CarpoolUsageDailyRecord{
		GroupName:    group,
		UsageDate:    createdAt.Format("2006-01-02"),
		UserID:       params.UserID,
		Username:     params.Username,
		TokenID:      params.TokenID,
		TokenName:    params.TokenName,
		Quota:        int64(params.Quota),
		TokenUsed:    int64(params.TokenUsed),
		RequestCount: params.RequestCount,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	return LOG_DB.Clauses(carpoolUsageDailySnapshotOnConflict(params, now)).Create(record).Error
}

func recordCarpoolUsageDailySnapshotFromLog(group string, userID int, username string, tokenID int, tokenName string, quota int, tokenUsed int, createdAt int64, carnivalSessionID int) {
	group = strings.TrimSpace(group)
	if group == "" || userID <= 0 || quota <= 0 || carnivalSessionID > 0 {
		return
	}
	if err := RecordCarpoolUsageDailySnapshot(CarpoolUsageDailySnapshotParams{
		Group:        group,
		UserID:       userID,
		Username:     username,
		TokenID:      tokenID,
		TokenName:    tokenName,
		Quota:        quota,
		TokenUsed:    tokenUsed,
		RequestCount: 1,
		CreatedAt:    createdAt,
	}); err != nil {
		common.SysLog("failed to record carpool usage daily snapshot: " + err.Error())
	}
}

func EnsureDefaultCarpoolSession() error {
	if LOG_DB == nil {
		return nil
	}
	group := DefaultCarnivalGroup
	var count int64
	if err := LOG_DB.Model(&CarpoolSession{}).Where("group_name = ? AND ended_at = 0", group).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	start, err := time.ParseInLocation("2006-01-02 15:04:05", defaultCarpoolSessionStartedAt, time.Local)
	if err != nil {
		return err
	}
	now := common.GetTimestamp()
	return LOG_DB.Create(&CarpoolSession{
		GroupName: group,
		StartedAt: start.Unix(),
		CreatedAt: now,
		UpdatedAt: now,
	}).Error
}

func GetCarpoolFinish2FACode() string {
	common.OptionMapRWMutex.RLock()
	defer common.OptionMapRWMutex.RUnlock()
	return strings.TrimSpace(common.OptionMap[DefaultCarpoolFinish2FAOption])
}

func StartCarpoolSession(group string, adminID int) (*CarpoolSession, error) {
	group = normalizeCarnivalGroup(group)
	now := common.GetTimestamp()
	var session CarpoolSession
	err := LOG_DB.Transaction(func(tx *gorm.DB) error {
		var sessions []CarpoolSession
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
		session = CarpoolSession{
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

func FinishCarpoolSession(group string, adminID int) (*CarpoolSessionSummary, error) {
	group = normalizeCarnivalGroup(group)
	now := common.GetTimestamp()
	var session CarpoolSession
	if err := LOG_DB.Transaction(func(tx *gorm.DB) error {
		var sessions []CarpoolSession
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
		summary, err := aggregateCarpoolSessionUsage(&session)
		if err != nil {
			return err
		}
		session.TotalQuota = summary.TotalQuota
		session.TotalTokens = summary.TotalTokens
		session.RequestCount = summary.RequestCount
		return tx.Save(&session).Error
	}); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("当前分组没有进行中的拼车")
		}
		return nil, err
	}
	return buildCarpoolSessionSummary(&session, now)
}

func GetActiveCarpoolSession(group string) (*CarpoolSession, error) {
	group = normalizeCarnivalGroup(group)
	var sessions []CarpoolSession
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

func GetCarpoolSessionByID(id int) (*CarpoolSession, error) {
	if id <= 0 {
		return nil, errors.New("拼车记录不存在")
	}
	var session CarpoolSession
	if err := LOG_DB.First(&session, id).Error; err != nil {
		return nil, err
	}
	return &session, nil
}

func GetCarpoolStatus(group string) (CarpoolStatusSnapshot, error) {
	group = normalizeCarnivalGroup(group)
	now := common.GetTimestamp()
	status := CarpoolStatusSnapshot{
		Group:      group,
		ServerTime: now,
	}
	active, err := GetActiveCarpoolSession(group)
	if err != nil {
		return status, err
	}
	if active != nil {
		status.Active, err = buildCarpoolSessionSummary(active, now)
		if err != nil {
			return status, err
		}
	}
	var sessions []CarpoolSession
	if err := LOG_DB.Where("group_name = ? AND ended_at > 0", group).
		Order("ended_at desc, id desc").
		Limit(1).
		Find(&sessions).Error; err != nil {
		return status, err
	}
	if len(sessions) > 0 {
		status.Last, err = buildCarpoolSessionSummary(&sessions[0], now)
		if err != nil {
			return status, err
		}
	}
	return status, nil
}

func ListCarpoolMonths(groups []string) ([]string, error) {
	query := LOG_DB.Model(&CarpoolSession{}).Select("started_at")
	if len(groups) > 0 {
		query = query.Where("group_name IN ?", groups)
	}
	var sessions []CarpoolSession
	if err := query.Order("started_at desc").Find(&sessions).Error; err != nil {
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

func ListCarpoolSessionGroups() ([]string, error) {
	var groups []string
	if err := LOG_DB.Model(&CarpoolSession{}).
		Distinct("group_name").
		Pluck("group_name", &groups).Error; err != nil {
		return nil, err
	}
	result := make([]string, 0, len(groups))
	for _, group := range groups {
		group = strings.TrimSpace(group)
		if group != "" {
			result = append(result, group)
		}
	}
	sort.Strings(result)
	return result, nil
}

func ListActiveCarpoolSessionGroups(groups []string) ([]string, error) {
	query := LOG_DB.Model(&CarpoolSession{}).
		Where("ended_at = 0")
	if len(groups) > 0 {
		query = query.Where("group_name IN ?", groups)
	}
	var activeGroups []string
	if err := query.Distinct("group_name").Pluck("group_name", &activeGroups).Error; err != nil {
		return nil, err
	}
	result := make([]string, 0, len(activeGroups))
	for _, group := range activeGroups {
		group = strings.TrimSpace(group)
		if group != "" {
			result = append(result, group)
		}
	}
	sort.Strings(result)
	return result, nil
}

func GetCarpoolHistory(groups []string, month string, selectedGroup string) (CarpoolHistorySnapshot, error) {
	months, err := ListCarpoolMonths(groups)
	if err != nil {
		return CarpoolHistorySnapshot{}, err
	}
	month = strings.TrimSpace(month)
	if month == "" && len(months) > 0 {
		month = months[0]
	}
	history := CarpoolHistorySnapshot{
		Months:        months,
		SelectedMonth: month,
		Groups:        make([]CarpoolHistoryGroup, 0),
	}
	query := LOG_DB.Model(&CarpoolSession{})
	if len(groups) > 0 {
		query = query.Where("group_name IN ?", groups)
	}
	selectedGroup = strings.TrimSpace(selectedGroup)
	if selectedGroup != "" {
		query = query.Where("group_name = ?", selectedGroup)
	}
	if month != "" && !strings.EqualFold(month, "all") {
		start, end, errRange := carnivalMonthRange(month)
		if errRange != nil {
			return history, errRange
		}
		query = query.Where("started_at >= ? AND started_at < ?", start, end)
	}
	var sessions []CarpoolSession
	if err := query.Order("group_name asc, started_at desc, id desc").Find(&sessions).Error; err != nil {
		return history, err
	}
	now := common.GetTimestamp()
	groupMap := map[string]*CarpoolHistoryGroup{}
	for i := range sessions {
		summary, errSummary := buildCarpoolSessionSummary(&sessions[i], now)
		if errSummary != nil {
			return history, errSummary
		}
		item := groupMap[summary.Group]
		if item == nil {
			item = &CarpoolHistoryGroup{Group: summary.Group, Sessions: make([]CarpoolSessionSummary, 0)}
			groupMap[summary.Group] = item
		}
		item.Sessions = append(item.Sessions, *summary)
		item.Total.PeriodQuota += summary.TotalQuota
		item.Total.CumulativeQuota += summary.TotalQuota
		item.Total.PeriodTokenUsed += summary.TotalTokens
		item.Total.CumulativeTokenUsed += summary.TotalTokens
		item.Total.PeriodRequestCount += summary.RequestCount
		item.Total.CumulativeRequestCount += summary.RequestCount
	}
	for _, group := range groupMap {
		history.Groups = append(history.Groups, *group)
	}
	sort.Slice(history.Groups, func(i, j int) bool {
		return history.Groups[i].Group < history.Groups[j].Group
	})
	return history, nil
}

func ListUserCarpoolGroups(userID int) ([]string, error) {
	groups := map[string]struct{}{}
	var tokens []Token
	if err := DB.Unscoped().Where("user_id = ?", userID).Find(&tokens).Error; err != nil {
		return nil, err
	}
	for i := range tokens {
		for _, group := range tokens[i].OrderedGroups() {
			group = strings.TrimSpace(group)
			if group != "" && group != "auto" {
				groups[group] = struct{}{}
			}
		}
	}
	var snapshotGroups []string
	if err := LOG_DB.Model(&CarpoolUsageDailyRecord{}).
		Where("user_id = ?", userID).
		Distinct("group_name").
		Pluck("group_name", &snapshotGroups).Error; err == nil {
		for _, group := range snapshotGroups {
			group = strings.TrimSpace(group)
			if group != "" {
				groups[group] = struct{}{}
			}
		}
	}
	var logGroups []string
	if err := LOG_DB.Model(&Log{}).
		Where("user_id = ? AND type = ? AND quota > 0", userID, LogTypeConsume).
		Distinct(logGroupCol).
		Pluck(logGroupCol, &logGroups).Error; err == nil {
		for _, group := range logGroups {
			group = strings.TrimSpace(group)
			if group != "" {
				groups[group] = struct{}{}
			}
		}
	}
	result := make([]string, 0, len(groups))
	for group := range groups {
		result = append(result, group)
	}
	sort.Strings(result)
	return result, nil
}

func UserParticipatesInCarpoolGroup(userID int, group string) (bool, error) {
	group = strings.TrimSpace(group)
	if group == "" {
		return false, nil
	}
	groups, err := ListUserCarpoolGroups(userID)
	if err != nil {
		return false, err
	}
	for _, item := range groups {
		if item == group {
			return true, nil
		}
	}
	return false, nil
}

func carpoolUsageDailySnapshotOnConflict(params CarpoolUsageDailySnapshotParams, now int64) clause.OnConflict {
	return clause.OnConflict{
		Columns: []clause.Column{
			{Name: "group_name"},
			{Name: "usage_date"},
			{Name: "user_id"},
			{Name: "token_id"},
		},
		DoUpdates: clause.Assignments(map[string]interface{}{
			"username":      gorm.Expr("CASE WHEN ? <> '' THEN ? ELSE carpool_usage_daily_records.username END", params.Username, params.Username),
			"token_name":    gorm.Expr("CASE WHEN ? <> '' THEN ? ELSE carpool_usage_daily_records.token_name END", params.TokenName, params.TokenName),
			"quota":         gorm.Expr("carpool_usage_daily_records.quota + ?", params.Quota),
			"token_used":    gorm.Expr("carpool_usage_daily_records.token_used + ?", params.TokenUsed),
			"request_count": gorm.Expr("carpool_usage_daily_records.request_count + ?", params.RequestCount),
			"updated_at":    now,
		}),
	}
}

func GetCarpoolUsageSummary(group string, period string) (*CarpoolUsageSummarySnapshot, error) {
	group = normalizeCarnivalGroup(group)
	period, start, end := carpoolPeriodRange(period, time.Now())
	return buildCarpoolUsageSummary(group, period, start, end, false, nil)
}

func GetCarpoolUsageSessionSummary(group string, sessionID int) (*CarpoolUsageSummarySnapshot, error) {
	group = normalizeCarnivalGroup(group)
	var session *CarpoolSession
	var err error
	if sessionID > 0 {
		session, err = GetCarpoolSessionByID(sessionID)
		if err != nil {
			return nil, err
		}
		if session.GroupName != group {
			return nil, errors.New("拼车记录不属于当前分组")
		}
	} else {
		session, err = GetActiveCarpoolSession(group)
		if err != nil {
			return nil, err
		}
	}
	if session == nil {
		now := time.Now().In(time.Local)
		return &CarpoolUsageSummarySnapshot{
			Group:        group,
			Period:       "session",
			StartDate:    now.Format("2006-01-02"),
			EndDate:      now.Format("2006-01-02"),
			LastRunAt:    now.Format(time.RFC3339),
			QuotaPerUnit: common.QuotaPerUnit,
			Users:        make([]CarpoolUsageUserSummary, 0),
		}, nil
	}
	now := common.GetTimestamp()
	endTs := session.EndedAt
	if endTs <= 0 {
		endTs = now
	}
	start := time.Unix(session.StartedAt, 0).In(time.Local)
	end := time.Unix(endTs, 0).In(time.Local)
	sessionSummary, err := buildCarpoolSessionSummary(session, now)
	if err != nil {
		return nil, err
	}
	return buildCarpoolUsageSummary(group, "session", start, end, true, sessionSummary)
}

func buildCarpoolUsageSummary(group string, period string, start time.Time, end time.Time, rangeOnly bool, session *CarpoolSessionSummary) (*CarpoolUsageSummarySnapshot, error) {
	startTs := start.Unix()
	endTs := end.Unix()

	summary := &CarpoolUsageSummarySnapshot{
		Group:        group,
		Period:       period,
		StartDate:    start.Format("2006-01-02"),
		EndDate:      end.Format("2006-01-02"),
		LastRunAt:    end.Format(time.RFC3339),
		QuotaPerUnit: common.QuotaPerUnit,
		Active:       session != nil && session.EndedAt == 0,
		Session:      session,
		Users:        make([]CarpoolUsageUserSummary, 0),
	}

	userMap := map[int]*CarpoolUsageUserSummary{}
	tokenMap := map[int]*CarpoolUsageTokenSummary{}
	userIDs := map[int]struct{}{}

	ensureUser := func(userID int) *CarpoolUsageUserSummary {
		user, ok := userMap[userID]
		if ok {
			return user
		}
		user = &CarpoolUsageUserSummary{
			UserID: userID,
			Daily:  make([]CarpoolUsageDailySummary, 0),
			Tokens: make([]CarpoolUsageTokenSummary, 0),
		}
		userMap[userID] = user
		if userID > 0 {
			userIDs[userID] = struct{}{}
		}
		return user
	}
	ensureToken := func(tokenID int, userID int, tokenName string) *CarpoolUsageTokenSummary {
		token, ok := tokenMap[tokenID]
		if ok {
			if token.UserID == 0 && userID > 0 {
				token.UserID = userID
			}
			if token.Name == "" && tokenName != "" {
				token.Name = tokenName
			}
			return token
		}
		token = &CarpoolUsageTokenSummary{
			TokenID: tokenID,
			UserID:  userID,
			Name:    tokenName,
		}
		tokenMap[tokenID] = token
		if userID > 0 {
			ensureUser(userID)
		}
		return token
	}

	if err := seedCarpoolTokens(group, ensureUser, ensureToken); err != nil {
		return nil, err
	}

	useLegacyPeriodUsage := shouldUseLegacyCarpoolUsage(group, start, end)
	useLegacyCumulativeUsage := useLegacyPeriodUsage || shouldUseLegacyCarpoolUsage(group, time.Time{}, time.Time{})
	carnivalPeriodStartTs, carnivalPeriodEndTs := startTs, endTs
	if useLegacyPeriodUsage {
		carnivalPeriodStartTs, carnivalPeriodEndTs = carpoolLegacyDailyUnixBounds(start, end)
	}

	var normalPeriodUsers []carpoolUsageRow
	var err error
	if useLegacyPeriodUsage {
		normalPeriodUsers, err = queryCarpoolLegacyUserUsage(group, start, end)
	} else if rangeOnly {
		normalPeriodUsers, err = queryCarpoolUserUsage(group, startTs, endTs, carpoolUsageFilterNormal, 0)
	} else {
		normalPeriodUsers, err = queryCarpoolSnapshotUserUsage(group, start, end)
	}
	if err != nil {
		return nil, err
	}
	var normalCumulativeUsers []carpoolUsageRow
	if rangeOnly {
		normalCumulativeUsers = normalPeriodUsers
	} else if useLegacyCumulativeUsage {
		normalCumulativeUsers, err = queryCarpoolLegacyUserUsage(group, time.Time{}, time.Time{})
	} else {
		normalCumulativeUsers, err = queryCarpoolSnapshotUserUsage(group, time.Time{}, time.Time{})
	}
	if err != nil {
		return nil, err
	}
	carnivalPeriodUsers, err := queryCarpoolCarnivalUserUsage(group, carnivalPeriodStartTs, carnivalPeriodEndTs, 0)
	if err != nil {
		return nil, err
	}
	var carnivalCumulativeUsers []carpoolUsageRow
	if rangeOnly {
		carnivalCumulativeUsers = carnivalPeriodUsers
	} else {
		carnivalCumulativeUsers, err = queryCarpoolCarnivalUserUsage(group, 0, 0, 0)
	}
	if err != nil {
		return nil, err
	}

	activeSession, err := GetActiveCarnivalSession(group)
	if err != nil {
		return nil, err
	}
	var currentCarnivalUsers []carpoolUsageRow
	if activeSession != nil {
		currentCarnivalUsers, err = queryCarpoolCarnivalUserUsage(group, 0, 0, activeSession.Id)
		if err != nil {
			return nil, err
		}
	}

	applyUserRows := func(rows []carpoolUsageRow, apply func(*CarpoolUsageUserSummary, carpoolUsageRow)) {
		for _, row := range rows {
			user := ensureUser(row.UserID)
			if user.Username == "" && row.Username != "" {
				user.Username = row.Username
			}
			apply(user, row)
		}
	}
	applyUserRows(normalPeriodUsers, func(user *CarpoolUsageUserSummary, row carpoolUsageRow) {
		user.PeriodQuota = row.Quota
		user.PeriodTokenUsed = row.TokenUsed
		user.PeriodRequestCount = row.RequestCount
		summary.Totals.PeriodQuota += row.Quota
		summary.Totals.PeriodTokenUsed += row.TokenUsed
		summary.Totals.PeriodRequestCount += row.RequestCount
	})
	applyUserRows(normalCumulativeUsers, func(user *CarpoolUsageUserSummary, row carpoolUsageRow) {
		user.CumulativeQuota = row.Quota
		user.CumulativeTokenUsed = row.TokenUsed
		user.CumulativeRequestCount = row.RequestCount
		summary.Totals.CumulativeQuota += row.Quota
		summary.Totals.CumulativeTokenUsed += row.TokenUsed
		summary.Totals.CumulativeRequestCount += row.RequestCount
	})
	applyUserRows(carnivalPeriodUsers, func(user *CarpoolUsageUserSummary, row carpoolUsageRow) {
		user.CarnivalPeriodQuota = row.Quota
		summary.Totals.CarnivalPeriodQuota += row.Quota
	})
	applyUserRows(carnivalCumulativeUsers, func(user *CarpoolUsageUserSummary, row carpoolUsageRow) {
		user.CarnivalCumulativeQuota = row.Quota
		summary.Totals.CarnivalCumulativeQuota += row.Quota
	})
	applyUserRows(currentCarnivalUsers, func(user *CarpoolUsageUserSummary, row carpoolUsageRow) {
		user.CurrentCarnivalQuota = row.Quota
		summary.Totals.CurrentCarnivalQuota += row.Quota
	})

	var normalPeriodTokens []carpoolTokenUsageRow
	if useLegacyPeriodUsage {
		normalPeriodTokens, err = queryCarpoolLegacyTokenUsage(group, start, end)
	} else if rangeOnly {
		normalPeriodTokens, err = queryCarpoolTokenUsage(group, startTs, endTs, carpoolUsageFilterNormal, 0)
	} else {
		normalPeriodTokens, err = queryCarpoolSnapshotTokenUsage(group, start, end)
	}
	if err != nil {
		return nil, err
	}
	var normalCumulativeTokens []carpoolTokenUsageRow
	if rangeOnly {
		normalCumulativeTokens = normalPeriodTokens
	} else if useLegacyCumulativeUsage {
		normalCumulativeTokens, err = queryCarpoolLegacyTokenUsage(group, time.Time{}, time.Time{})
	} else {
		normalCumulativeTokens, err = queryCarpoolSnapshotTokenUsage(group, time.Time{}, time.Time{})
	}
	if err != nil {
		return nil, err
	}
	carnivalPeriodTokens, err := queryCarpoolCarnivalTokenUsage(group, carnivalPeriodStartTs, carnivalPeriodEndTs, 0)
	if err != nil {
		return nil, err
	}
	var carnivalCumulativeTokens []carpoolTokenUsageRow
	if rangeOnly {
		carnivalCumulativeTokens = carnivalPeriodTokens
	} else {
		carnivalCumulativeTokens, err = queryCarpoolCarnivalTokenUsage(group, 0, 0, 0)
	}
	if err != nil {
		return nil, err
	}
	var currentCarnivalTokens []carpoolTokenUsageRow
	if activeSession != nil {
		currentCarnivalTokens, err = queryCarpoolCarnivalTokenUsage(group, 0, 0, activeSession.Id)
		if err != nil {
			return nil, err
		}
	}

	resolveTokenID := func(row carpoolTokenUsageRow) int {
		if row.TokenID > 0 {
			return row.TokenID
		}
		if row.UserID <= 0 {
			return 0
		}
		if row.TokenName != "" {
			for _, token := range tokenMap {
				if token.UserID == row.UserID && token.Name == row.TokenName {
					return token.TokenID
				}
			}
		}
		matchedTokenID := 0
		for _, token := range tokenMap {
			if token.UserID != row.UserID {
				continue
			}
			if matchedTokenID != 0 {
				return 0
			}
			matchedTokenID = token.TokenID
		}
		return matchedTokenID
	}
	applyTokenRows := func(rows []carpoolTokenUsageRow, apply func(*CarpoolUsageTokenSummary, carpoolTokenUsageRow)) {
		for _, row := range rows {
			tokenID := resolveTokenID(row)
			if tokenID <= 0 {
				continue
			}
			token := ensureToken(tokenID, row.UserID, row.TokenName)
			if row.LastSeen > 0 {
				token.LastSeenAt = carpoolFormatTimestamp(row.LastSeen)
			}
			apply(token, row)
		}
	}
	applyTokenRows(normalPeriodTokens, func(token *CarpoolUsageTokenSummary, row carpoolTokenUsageRow) {
		token.PeriodQuota = row.Quota
		token.PeriodTokenUsed = row.TokenUsed
		token.PeriodRequestCount = row.RequestCount
	})
	applyTokenRows(normalCumulativeTokens, func(token *CarpoolUsageTokenSummary, row carpoolTokenUsageRow) {
		token.CumulativeQuota = row.Quota
		token.CumulativeTokenUsed = row.TokenUsed
		token.CumulativeRequestCount = row.RequestCount
	})
	applyTokenRows(carnivalPeriodTokens, func(token *CarpoolUsageTokenSummary, row carpoolTokenUsageRow) {
		token.CarnivalPeriodQuota = row.Quota
	})
	applyTokenRows(carnivalCumulativeTokens, func(token *CarpoolUsageTokenSummary, row carpoolTokenUsageRow) {
		token.CarnivalCumulativeQuota = row.Quota
	})
	applyTokenRows(currentCarnivalTokens, func(token *CarpoolUsageTokenSummary, row carpoolTokenUsageRow) {
		token.CurrentCarnivalQuota = row.Quota
	})

	var dailyRows []carpoolDailyUsageRow
	if useLegacyPeriodUsage {
		dailyRows, err = queryCarpoolLegacyDailyUsage(group, start, end)
	} else {
		dailyRows, err = queryCarpoolDailyUsage(group, startTs, endTs)
	}
	if err != nil {
		return nil, err
	}
	applyCarpoolDailyRows(userMap, dailyRows, start, end)

	if err := hydrateCarpoolUsers(userMap, userIDs); err != nil {
		return nil, err
	}

	tokensByUser := map[int][]CarpoolUsageTokenSummary{}
	tokenGrossCumulativeByUser := map[int]int64{}
	for _, token := range tokenMap {
		logGrossCumulativeQuota := token.CumulativeQuota + token.CarnivalCumulativeQuota
		if !rangeOnly && token.TokenStoreKnown && token.TokenStoreUsedQuota > logGrossCumulativeQuota {
			token.GrossCumulativeQuota = token.TokenStoreUsedQuota
			token.CumulativeQuota = token.TokenStoreUsedQuota - token.CarnivalCumulativeQuota
			if token.CumulativeQuota < 0 {
				token.CumulativeQuota = 0
			}
		} else {
			token.GrossCumulativeQuota = logGrossCumulativeQuota
		}
		token.GrossPeriodQuota = token.PeriodQuota + token.CarnivalPeriodQuota
		tokensByUser[token.UserID] = append(tokensByUser[token.UserID], *token)
		tokenGrossCumulativeByUser[token.UserID] += token.GrossCumulativeQuota
		summary.Totals.KnownTokens++
		if token.Active {
			summary.Totals.ActiveTokens++
		}
	}

	summary.Totals.CumulativeQuota = 0
	summary.Totals.GrossCumulativeQuota = 0
	users := make([]CarpoolUsageUserSummary, 0, len(userMap))
	for userID, user := range userMap {
		user.GrossPeriodQuota = user.PeriodQuota + user.CarnivalPeriodQuota
		logGrossCumulativeQuota := user.CumulativeQuota + user.CarnivalCumulativeQuota
		if !rangeOnly && tokenGrossCumulativeByUser[userID] > logGrossCumulativeQuota {
			user.GrossCumulativeQuota = tokenGrossCumulativeByUser[userID]
			user.CumulativeQuota = user.GrossCumulativeQuota - user.CarnivalCumulativeQuota
			if user.CumulativeQuota < 0 {
				user.CumulativeQuota = 0
			}
		} else {
			user.GrossCumulativeQuota = logGrossCumulativeQuota
		}
		tokens := tokensByUser[userID]
		sort.Slice(tokens, func(i, j int) bool {
			if tokens[i].CumulativeQuota != tokens[j].CumulativeQuota {
				return tokens[i].CumulativeQuota > tokens[j].CumulativeQuota
			}
			if tokens[i].PeriodQuota != tokens[j].PeriodQuota {
				return tokens[i].PeriodQuota > tokens[j].PeriodQuota
			}
			return tokens[i].TokenID < tokens[j].TokenID
		})
		user.Tokens = tokens
		user.KnownTokens = len(tokens)
		for _, token := range tokens {
			if token.Active {
				user.ActiveTokens++
			}
		}
		summary.Totals.CumulativeQuota += user.CumulativeQuota
		summary.Totals.GrossCumulativeQuota += user.GrossCumulativeQuota
		users = append(users, *user)
	}

	sort.Slice(users, func(i, j int) bool {
		if users[i].CumulativeQuota != users[j].CumulativeQuota {
			return users[i].CumulativeQuota > users[j].CumulativeQuota
		}
		if users[i].PeriodQuota != users[j].PeriodQuota {
			return users[i].PeriodQuota > users[j].PeriodQuota
		}
		return users[i].UserID < users[j].UserID
	})

	summary.Totals.Users = len(users)
	summary.Totals.GrossPeriodQuota = summary.Totals.PeriodQuota + summary.Totals.CarnivalPeriodQuota
	summary.Users = users
	return summary, nil
}

func seedCarpoolTokens(group string, ensureUser func(int) *CarpoolUsageUserSummary, ensureToken func(int, int, string) *CarpoolUsageTokenSummary) error {
	var tokens []Token
	if err := DB.Unscoped().Model(&Token{}).
		Where(commonGroupCol+" = ? OR group_config LIKE ?", group, "%"+group+"%").
		Find(&tokens).Error; err != nil {
		return err
	}
	for _, token := range tokens {
		if !tokenContainsGroup(&token, group) {
			continue
		}
		ensureUser(token.UserId)
		summary := ensureToken(token.Id, token.UserId, token.Name)
		summary.Active = token.Status == common.TokenStatusEnabled && !token.DeletedAt.Valid
		summary.TokenStoreKnown = true
		summary.TokenStoreUsedQuota = int64(token.UsedQuota)
		if summary.LastSeenAt == "" && token.AccessedTime > 0 {
			summary.LastSeenAt = carpoolFormatTimestamp(token.AccessedTime)
		}
	}
	return nil
}

func tokenContainsGroup(token *Token, group string) bool {
	if token == nil {
		return false
	}
	for _, item := range token.OrderedGroups() {
		if item == group {
			return true
		}
	}
	return false
}

func buildCarpoolSessionSummary(session *CarpoolSession, now int64) (*CarpoolSessionSummary, error) {
	if session == nil {
		return nil, nil
	}
	aggregate, err := aggregateCarpoolSessionUsage(session)
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
		requestCount = session.RequestCount
	}
	end := session.EndedAt
	if end <= 0 {
		end = now
	}
	duration := end - session.StartedAt
	if duration < 0 {
		duration = 0
	}
	return &CarpoolSessionSummary{
		ID:              session.Id,
		Group:           session.GroupName,
		StartedAt:       session.StartedAt,
		EndedAt:         session.EndedAt,
		DurationSeconds: duration,
		TotalQuota:      totalQuota,
		TotalTokens:     totalTokens,
		RequestCount:    requestCount,
	}, nil
}

type carpoolSessionAggregate struct {
	TotalQuota   int64
	TotalTokens  int64
	RequestCount int64
}

func aggregateCarpoolSessionUsage(session *CarpoolSession) (carpoolSessionAggregate, error) {
	if session == nil {
		return carpoolSessionAggregate{}, nil
	}
	end := session.EndedAt
	if end <= 0 {
		end = common.GetTimestamp()
	}
	start := time.Unix(session.StartedAt, 0).In(time.Local)
	finish := time.Unix(end, 0).In(time.Local)
	if shouldUseLegacyCarpoolUsage(session.GroupName, start, finish) {
		rows, err := queryCarpoolLegacyUserUsage(session.GroupName, start, finish)
		if err != nil {
			return carpoolSessionAggregate{}, err
		}
		var aggregate carpoolSessionAggregate
		for _, row := range rows {
			aggregate.TotalQuota += row.Quota
			aggregate.TotalTokens += row.TokenUsed
			aggregate.RequestCount += row.RequestCount
		}
		return aggregate, nil
	}
	var row carpoolSessionAggregate
	err := baseCarpoolLogQuery(session.GroupName, session.StartedAt, end, carpoolUsageFilterNormal, 0).
		Select("COALESCE(SUM(quota), 0) AS total_quota, COALESCE(SUM(prompt_tokens + completion_tokens), 0) AS total_tokens, COUNT(*) AS request_count").
		Scan(&row).Error
	return row, err
}

func baseCarpoolLogQuery(group string, startTimestamp int64, endTimestamp int64, filter int, sessionID int) *gorm.DB {
	tx := LOG_DB.Model(&Log{}).
		Where("type = ?", LogTypeConsume).
		Where(logGroupCol+" = ?", group).
		Where("quota > 0")
	if startTimestamp > 0 {
		tx = tx.Where("created_at >= ?", startTimestamp)
	}
	if endTimestamp > 0 {
		tx = tx.Where("created_at <= ?", endTimestamp)
	}
	switch filter {
	case carpoolUsageFilterNormal:
		tx = tx.Where("(carnival_session_id IS NULL OR carnival_session_id = 0)")
	case carpoolUsageFilterCarnival:
		tx = tx.Where("carnival_session_id > 0")
	case carpoolUsageFilterSession:
		tx = tx.Where("carnival_session_id = ?", sessionID)
	}
	return tx
}

func queryCarpoolUserUsage(group string, startTimestamp int64, endTimestamp int64, filter int, sessionID int) ([]carpoolUsageRow, error) {
	var rows []carpoolUsageRow
	err := baseCarpoolLogQuery(group, startTimestamp, endTimestamp, filter, sessionID).
		Select("user_id, MAX(username) AS username, COALESCE(SUM(quota), 0) AS quota, COALESCE(SUM(prompt_tokens + completion_tokens), 0) AS token_used, COUNT(*) AS request_count").
		Group("user_id").
		Find(&rows).Error
	return rows, err
}

func queryCarpoolTokenUsage(group string, startTimestamp int64, endTimestamp int64, filter int, sessionID int) ([]carpoolTokenUsageRow, error) {
	var rows []carpoolTokenUsageRow
	err := baseCarpoolLogQuery(group, startTimestamp, endTimestamp, filter, sessionID).
		Where("token_id > 0").
		Select("token_id, MAX(user_id) AS user_id, MAX(token_name) AS token_name, COALESCE(SUM(quota), 0) AS quota, COALESCE(SUM(prompt_tokens + completion_tokens), 0) AS token_used, COUNT(*) AS request_count, MAX(created_at) AS last_seen").
		Group("token_id").
		Find(&rows).Error
	return rows, err
}

func queryCarpoolSnapshotUserUsage(group string, start time.Time, end time.Time) ([]carpoolUsageRow, error) {
	var rows []carpoolUsageRow
	currentDayRows, err := queryCarpoolCurrentDayUserUsage(group, start, end)
	if err != nil {
		return nil, err
	}
	snapshotStart, snapshotEnd, querySnapshots := carpoolSnapshotRangeForCurrentDay(start, end, len(currentDayRows) > 0)
	if querySnapshots {
		query := LOG_DB.Model(&CarpoolUsageDailyRecord{}).
			Where("group_name = ?", group)
		if startDate, endDate, ok := carpoolDateBounds(snapshotStart, snapshotEnd); ok {
			query = query.Where("usage_date >= ? AND usage_date <= ?", startDate, endDate)
		}
		if err := query.
			Select("user_id, MAX(username) AS username, COALESCE(SUM(quota), 0) AS quota, COALESCE(SUM(token_used), 0) AS token_used, COALESCE(SUM(request_count), 0) AS request_count").
			Group("user_id").
			Find(&rows).Error; err != nil {
			return nil, err
		}
		fallbackRows, err := queryCarpoolQuotaDataFallbackUserUsage(group, snapshotStart, snapshotEnd)
		if err != nil {
			return nil, err
		}
		rows = mergeCarpoolUsageRows(rows, fallbackRows)
	}
	return mergeCarpoolUsageRows(rows, currentDayRows), nil
}

func queryCarpoolSnapshotTokenUsage(group string, start time.Time, end time.Time) ([]carpoolTokenUsageRow, error) {
	var rows []carpoolTokenUsageRow
	currentDayRows, err := queryCarpoolCurrentDayTokenUsage(group, start, end)
	if err != nil {
		return nil, err
	}
	snapshotStart, snapshotEnd, querySnapshots := carpoolSnapshotRangeForCurrentDay(start, end, len(currentDayRows) > 0)
	if querySnapshots {
		query := LOG_DB.Model(&CarpoolUsageDailyRecord{}).
			Where("group_name = ?", group).
			Where("token_id > 0")
		if startDate, endDate, ok := carpoolDateBounds(snapshotStart, snapshotEnd); ok {
			query = query.Where("usage_date >= ? AND usage_date <= ?", startDate, endDate)
		}
		if err := query.
			Select("token_id, MAX(user_id) AS user_id, MAX(token_name) AS token_name, COALESCE(SUM(quota), 0) AS quota, COALESCE(SUM(token_used), 0) AS token_used, COALESCE(SUM(request_count), 0) AS request_count, MAX(updated_at) AS last_seen").
			Group("token_id").
			Find(&rows).Error; err != nil {
			return nil, err
		}
	}
	return mergeCarpoolTokenUsageRows(rows, currentDayRows), nil
}

func queryCarpoolCarnivalUserUsage(group string, startTimestamp int64, endTimestamp int64, sessionID int) ([]carpoolUsageRow, error) {
	var rows []carpoolUsageRow
	err := baseCarpoolCarnivalUsageQuery(group, startTimestamp, endTimestamp, sessionID).
		Select("user_id, MAX(username) AS username, COALESCE(SUM(quota), 0) AS quota, COALESCE(SUM(token_used), 0) AS token_used, COUNT(*) AS request_count").
		Group("user_id").
		Find(&rows).Error
	return rows, err
}

func queryCarpoolCarnivalTokenUsage(group string, startTimestamp int64, endTimestamp int64, sessionID int) ([]carpoolTokenUsageRow, error) {
	var rows []carpoolTokenUsageRow
	err := baseCarpoolCarnivalUsageQuery(group, startTimestamp, endTimestamp, sessionID).
		Select("MAX(token_id) AS token_id, MAX(user_id) AS user_id, MAX(token_name) AS token_name, COALESCE(SUM(quota), 0) AS quota, COALESCE(SUM(token_used), 0) AS token_used, COUNT(*) AS request_count, MAX(created_at) AS last_seen").
		Group("user_id, token_name").
		Find(&rows).Error
	return rows, err
}

func baseCarpoolCarnivalUsageQuery(group string, startTimestamp int64, endTimestamp int64, sessionID int) *gorm.DB {
	tx := LOG_DB.Model(&CarnivalUsage{}).
		Where("group_name = ?", group).
		Where("quota > 0")
	if sessionID > 0 {
		tx = tx.Where("session_id = ?", sessionID)
	}
	if startTimestamp > 0 {
		tx = tx.Where("created_at >= ?", startTimestamp)
	}
	if endTimestamp > 0 {
		tx = tx.Where("created_at <= ?", endTimestamp)
	}
	return tx
}

func shouldUseLegacyCarpoolUsage(group string, start time.Time, end time.Time) bool {
	if strings.TrimSpace(group) != DefaultCarnivalGroup || LOG_DB == nil {
		return false
	}
	firstSnapshotAt, err := getFirstCarpoolSnapshotTimestamp(group)
	if err == nil && firstSnapshotAt > 0 && !start.IsZero() && start.Unix() > firstSnapshotAt {
		return false
	}
	ok, err := hasLegacyCarpoolUsage(start, end)
	return err == nil && ok
}

func hasLegacyCarpoolUsage(start time.Time, end time.Time) (bool, error) {
	query := LOG_DB.Table("carpool_usage_daily")
	if startDate, endDate, ok := carpoolDateBounds(start, end); ok {
		query = query.Where("usage_date >= ? AND usage_date <= ?", startDate, endDate)
	}
	var count int64
	if err := query.Count(&count).Error; err != nil {
		if isMissingCarpoolLegacyTableError(err) {
			return false, nil
		}
		return false, err
	}
	return count > 0, nil
}

func queryCarpoolLegacyUserUsage(group string, start time.Time, end time.Time) ([]carpoolUsageRow, error) {
	segments, err := queryCarpoolLegacyUsageSegments(group, start, end)
	if err != nil {
		return nil, err
	}
	merged := map[int]carpoolUsageRow{}
	for _, segment := range segments {
		row := merged[segment.UserID]
		row.UserID = segment.UserID
		if row.Username == "" && segment.Username != "" {
			row.Username = segment.Username
		}
		row.Quota += segment.Quota
		row.TokenUsed += segment.TokenUsed
		row.RequestCount += segment.RequestCount
		merged[segment.UserID] = row
	}
	rows := make([]carpoolUsageRow, 0, len(merged))
	for _, row := range merged {
		rows = append(rows, row)
	}
	return rows, nil
}

func queryCarpoolLegacyTokenUsage(group string, start time.Time, end time.Time) ([]carpoolTokenUsageRow, error) {
	segments, err := queryCarpoolLegacyUsageSegments(group, start, end)
	if err != nil {
		return nil, err
	}
	merged := map[int]carpoolTokenUsageRow{}
	for _, segment := range segments {
		if segment.TokenID <= 0 {
			continue
		}
		row := merged[segment.TokenID]
		row.TokenID = segment.TokenID
		row.UserID = segment.UserID
		if row.TokenName == "" && segment.TokenName != "" {
			row.TokenName = segment.TokenName
		}
		row.Quota += segment.Quota
		row.TokenUsed += segment.TokenUsed
		row.RequestCount += segment.RequestCount
		if segment.LastSeen > row.LastSeen {
			row.LastSeen = segment.LastSeen
		}
		merged[segment.TokenID] = row
	}
	rows := make([]carpoolTokenUsageRow, 0, len(merged))
	for _, row := range merged {
		rows = append(rows, row)
	}
	return rows, nil
}

func queryCarpoolLegacyDailyUsage(group string, start time.Time, end time.Time) ([]carpoolDailyUsageRow, error) {
	segments, err := queryCarpoolLegacyUsageSegments(group, start, end)
	if err != nil {
		return nil, err
	}
	merged := map[string]carpoolDailyUsageRow{}
	for _, segment := range segments {
		key := carpoolDailyRowKey(segment.UserID, segment.Date)
		row := merged[key]
		row.UserID = segment.UserID
		row.Date = segment.Date
		row.Quota += segment.Quota
		merged[key] = row
	}
	rows := make([]carpoolDailyUsageRow, 0, len(merged))
	for _, row := range merged {
		rows = append(rows, row)
	}
	return rows, nil
}

func queryCarpoolLegacyUsageSegments(group string, start time.Time, end time.Time) ([]carpoolLegacyUsageSegment, error) {
	oldRows, err := queryCarpoolLegacyDailyRows(start, end)
	if err != nil {
		return nil, err
	}
	startTs, endTs := carpoolUnixBounds(start, end)
	carnivalStartTs, carnivalEndTs := carpoolLegacyDailyUnixBounds(start, end)
	carnivalRows, err := queryCarpoolCarnivalDailyTokenRows(group, carnivalStartTs, carnivalEndTs)
	if err != nil {
		return nil, err
	}
	carnivalByKey := map[string]int64{}
	for _, row := range carnivalRows {
		date := time.Unix(row.CreatedAt, 0).In(time.Local).Format("2006-01-02")
		carnivalByKey[carpoolLegacyDailyTokenKey(date, row.UserID, row.TokenName)] += row.Quota
	}

	segments := make([]carpoolLegacyUsageSegment, 0, len(oldRows))
	for _, row := range oldRows {
		usageDate := normalizeCarpoolLegacyUsageDate(row.UsageDate)
		quota := row.Quota - carnivalByKey[carpoolLegacyDailyTokenKey(usageDate, row.UserID, row.TokenName)]
		if quota <= 0 {
			continue
		}
		segments = append(segments, carpoolLegacyUsageSegment{
			Date:      usageDate,
			UserID:    row.UserID,
			Username:  row.Username,
			TokenID:   row.TokenID,
			TokenName: row.TokenName,
			Quota:     quota,
		})
	}

	tailRows, err := queryCarpoolLegacyTailLogSegments(group, startTs, endTs)
	if err != nil {
		return nil, err
	}
	segments = append(segments, tailRows...)
	return segments, nil
}

func queryCarpoolLegacyDailyRows(start time.Time, end time.Time) ([]carpoolLegacyDailyRow, error) {
	dateSelect := "usage_date"
	if common.UsingLogDatabase(common.DatabaseTypePostgreSQL) {
		dateSelect = "usage_date::text"
	}
	query := LOG_DB.Table("carpool_usage_daily")
	if startDate, endDate, ok := carpoolDateBounds(start, end); ok {
		query = query.Where("usage_date >= ? AND usage_date <= ?", startDate, endDate)
	}
	var rows []carpoolLegacyDailyRow
	err := query.
		Select(dateSelect + " AS usage_date, user_id, MAX(username) AS username, token_id, MAX(token_name) AS token_name, COALESCE(SUM(quota_delta), 0) AS quota").
		Group("usage_date, user_id, token_id").
		Order("usage_date asc, user_id asc, token_id asc").
		Find(&rows).Error
	if err != nil && isMissingCarpoolLegacyTableError(err) {
		return nil, nil
	}
	return rows, err
}

func queryCarpoolCarnivalDailyTokenRows(group string, startTimestamp int64, endTimestamp int64) ([]carpoolCarnivalDailyTokenRow, error) {
	var rows []carpoolCarnivalDailyTokenRow
	err := baseCarpoolCarnivalUsageQuery(group, startTimestamp, endTimestamp, 0).
		Select("created_at, user_id, token_name, quota").
		Find(&rows).Error
	return rows, err
}

func queryCarpoolLegacyTailLogSegments(group string, startTimestamp int64, endTimestamp int64) ([]carpoolLegacyUsageSegment, error) {
	lastSyncAt, err := getCarpoolLegacyLastSyncTimestamp()
	if err != nil {
		return nil, err
	}
	if lastSyncAt <= 0 {
		return nil, nil
	}
	tx := baseCarpoolLogQuery(group, 0, 0, carpoolUsageFilterNormal, 0).
		Where("created_at > ?", lastSyncAt)
	if startTimestamp > 0 {
		tx = tx.Where("created_at >= ?", startTimestamp)
	}
	if endTimestamp > 0 {
		tx = tx.Where("created_at <= ?", endTimestamp)
	}
	var rows []carpoolTailLogRow
	if err := tx.
		Select("created_at, user_id, username, token_id, token_name, quota, prompt_tokens + completion_tokens AS token_used, 1 AS request_count").
		Find(&rows).Error; err != nil {
		return nil, err
	}
	segments := make([]carpoolLegacyUsageSegment, 0, len(rows))
	for _, row := range rows {
		segments = append(segments, carpoolLegacyUsageSegment{
			Date:         time.Unix(row.CreatedAt, 0).In(time.Local).Format("2006-01-02"),
			UserID:       row.UserID,
			Username:     row.Username,
			TokenID:      row.TokenID,
			TokenName:    row.TokenName,
			Quota:        row.Quota,
			TokenUsed:    row.TokenUsed,
			RequestCount: row.RequestCount,
			LastSeen:     row.CreatedAt,
		})
	}
	return segments, nil
}

func getCarpoolLegacyLastSyncTimestamp() (int64, error) {
	if common.UsingLogDatabase(common.DatabaseTypePostgreSQL) {
		var timestamp int64
		err := LOG_DB.Raw("SELECT COALESCE(EXTRACT(EPOCH FROM MAX(synced_at))::bigint, 0) FROM carpool_usage_sync_runs").Scan(&timestamp).Error
		if err != nil && isMissingCarpoolLegacyTableError(err) {
			return 0, nil
		}
		return timestamp, err
	}
	if common.UsingLogDatabase(common.DatabaseTypeMySQL) {
		var timestamp int64
		err := LOG_DB.Raw("SELECT COALESCE(UNIX_TIMESTAMP(MAX(synced_at)), 0) FROM carpool_usage_sync_runs").Scan(&timestamp).Error
		if err != nil && isMissingCarpoolLegacyTableError(err) {
			return 0, nil
		}
		return timestamp, err
	}
	var run CarpoolUsageLegacySyncRun
	err := LOG_DB.Model(&CarpoolUsageLegacySyncRun{}).
		Order("synced_at desc").
		Limit(1).
		Find(&run).Error
	if err != nil && isMissingCarpoolLegacyTableError(err) {
		return 0, nil
	}
	if err != nil || run.SyncedAt.IsZero() {
		return 0, err
	}
	return run.SyncedAt.Unix(), nil
}

func carpoolUnixBounds(start time.Time, end time.Time) (int64, int64) {
	var startTimestamp int64
	var endTimestamp int64
	if !start.IsZero() {
		startTimestamp = start.Unix()
	}
	if !end.IsZero() {
		endTimestamp = end.Unix()
	}
	return startTimestamp, endTimestamp
}

func carpoolLegacyDailyUnixBounds(start time.Time, end time.Time) (int64, int64) {
	var startTimestamp int64
	var endTimestamp int64
	if !start.IsZero() {
		startTimestamp = carpoolStartOfDay(start.In(time.Local)).Unix()
	}
	if !end.IsZero() {
		endTimestamp = carpoolStartOfDay(end.In(time.Local)).Add(24*time.Hour - time.Second).Unix()
	}
	return startTimestamp, endTimestamp
}

func carpoolLegacyDailyTokenKey(date string, userID int, tokenName string) string {
	return date + "|" + strconv.Itoa(userID) + "|" + tokenName
}

func normalizeCarpoolLegacyUsageDate(value string) string {
	value = strings.TrimSpace(value)
	if len(value) >= len("2006-01-02") {
		return value[:len("2006-01-02")]
	}
	return value
}

func isMissingCarpoolLegacyTableError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "no such table") || strings.Contains(message, "does not exist")
}

func queryCarpoolDailyUsage(group string, startTimestamp int64, endTimestamp int64) ([]carpoolDailyUsageRow, error) {
	var rows []carpoolDailyUsageRow
	start := time.Unix(startTimestamp, 0).In(time.Local)
	end := time.Unix(endTimestamp, 0).In(time.Local)
	currentDayRows, err := queryCarpoolCurrentDayDailyUsage(group, start, end)
	if err != nil {
		return nil, err
	}
	snapshotStart, snapshotEnd, querySnapshots := carpoolSnapshotRangeForCurrentDay(start, end, len(currentDayRows) > 0)
	if querySnapshots {
		query := LOG_DB.Model(&CarpoolUsageDailyRecord{}).
			Where("group_name = ?", group)
		if startDate, endDate, ok := carpoolDateBounds(snapshotStart, snapshotEnd); ok {
			query = query.Where("usage_date >= ? AND usage_date <= ?", startDate, endDate)
		}
		if err := query.
			Select("user_id, usage_date AS date, COALESCE(SUM(quota), 0) AS quota").
			Group("user_id, usage_date").
			Find(&rows).Error; err != nil {
			return nil, err
		}
		fallbackRows, err := queryCarpoolQuotaDataFallbackDailyUsage(group, snapshotStart, snapshotEnd)
		if err != nil {
			return nil, err
		}
		rows = mergeCarpoolDailyRows(rows, fallbackRows)
	}
	return mergeCarpoolDailyRows(rows, currentDayRows), nil
}

func applyCarpoolDailyRows(userMap map[int]*CarpoolUsageUserSummary, rows []carpoolDailyUsageRow, start time.Time, end time.Time) {
	if len(userMap) == 0 {
		return
	}
	quotaByUserDate := map[int]map[string]int64{}
	for _, row := range rows {
		userDateQuota := quotaByUserDate[row.UserID]
		if userDateQuota == nil {
			userDateQuota = map[string]int64{}
			quotaByUserDate[row.UserID] = userDateQuota
		}
		userDateQuota[row.Date] += row.Quota
	}
	for userID, user := range userMap {
		daily := make([]CarpoolUsageDailySummary, 0)
		dateQuota := quotaByUserDate[userID]
		for day := carpoolStartOfDay(start); !day.After(end); day = day.AddDate(0, 0, 1) {
			date := day.Format("2006-01-02")
			daily = append(daily, CarpoolUsageDailySummary{
				Date:  date,
				Quota: dateQuota[date],
			})
		}
		user.Daily = daily
	}
}

func queryCarpoolQuotaDataFallbackUserUsage(group string, start time.Time, end time.Time) ([]carpoolUsageRow, error) {
	if strings.TrimSpace(group) != DefaultCarnivalGroup || DB == nil {
		return nil, nil
	}
	startTs, endTs, ok := carpoolTimestampBoundsBeforeFirstSnapshot(group, start, end)
	if !ok {
		return nil, nil
	}
	query := DB.Table("quota_data").
		Where("user_id IN (?)", DB.Unscoped().Model(&Token{}).Select("user_id").Where(commonGroupCol+" = ? OR group_config LIKE ?", group, "%"+group+"%"))
	if startTs > 0 {
		query = query.Where("created_at >= ?", startTs)
	}
	if endTs > 0 {
		query = query.Where("created_at <= ?", endTs)
	}
	var rows []carpoolUsageRow
	err := query.
		Select("user_id, MAX(username) AS username, COALESCE(SUM(quota), 0) AS quota, COALESCE(SUM(token_used), 0) AS token_used, COALESCE(SUM(count), 0) AS request_count").
		Group("user_id").
		Find(&rows).Error
	return rows, err
}

func queryCarpoolQuotaDataFallbackDailyUsage(group string, start time.Time, end time.Time) ([]carpoolDailyUsageRow, error) {
	if strings.TrimSpace(group) != DefaultCarnivalGroup || DB == nil {
		return nil, nil
	}
	startTs, endTs, ok := carpoolTimestampBoundsBeforeFirstSnapshot(group, start, end)
	if !ok {
		return nil, nil
	}
	query := DB.Table("quota_data").
		Where("user_id IN (?)", DB.Unscoped().Model(&Token{}).Select("user_id").Where(commonGroupCol+" = ? OR group_config LIKE ?", group, "%"+group+"%"))
	if startTs > 0 {
		query = query.Where("created_at >= ?", startTs)
	}
	if endTs > 0 {
		query = query.Where("created_at <= ?", endTs)
	}
	var quotaRows []carpoolQuotaDataDailyRow
	if err := query.
		Select("user_id, created_at, COALESCE(SUM(quota), 0) AS quota, COALESCE(SUM(token_used), 0) AS token_used, COALESCE(SUM(count), 0) AS request_count").
		Group("user_id, created_at").
		Find(&quotaRows).Error; err != nil {
		return nil, err
	}
	merged := map[string]carpoolDailyUsageRow{}
	for _, row := range quotaRows {
		date := time.Unix(row.CreatedAt, 0).In(time.Local).Format("2006-01-02")
		key := carpoolDailyRowKey(row.UserID, date)
		daily := merged[key]
		daily.UserID = row.UserID
		daily.Date = date
		daily.Quota += row.Quota
		merged[key] = daily
	}
	rows := make([]carpoolDailyUsageRow, 0, len(merged))
	for _, row := range merged {
		rows = append(rows, row)
	}
	return rows, nil
}

func carpoolTimestampBoundsBeforeFirstSnapshot(group string, start time.Time, end time.Time) (int64, int64, bool) {
	startTs := int64(0)
	endTs := int64(0)
	if !start.IsZero() {
		startTs = start.Unix()
	}
	if !end.IsZero() {
		endTs = end.Unix()
	}
	firstSnapshotAt, err := getFirstCarpoolSnapshotTimestamp(group)
	if err != nil {
		return 0, 0, false
	}
	if firstSnapshotAt > 0 {
		beforeSnapshotEnd := firstSnapshotAt - 1
		if endTs == 0 || beforeSnapshotEnd < endTs {
			endTs = beforeSnapshotEnd
		}
	}
	if endTs > 0 && startTs > 0 && endTs < startTs {
		return 0, 0, false
	}
	return startTs, endTs, true
}

func getFirstCarpoolSnapshotTimestamp(group string) (int64, error) {
	var record CarpoolUsageDailyRecord
	err := LOG_DB.Model(&CarpoolUsageDailyRecord{}).
		Select("created_at").
		Where("group_name = ?", group).
		Order("created_at asc").
		Limit(1).
		Find(&record).Error
	if err != nil {
		return 0, err
	}
	return record.CreatedAt, nil
}

func queryCarpoolCurrentDayUserUsage(group string, start time.Time, end time.Time) ([]carpoolUsageRow, error) {
	startTs, endTs, ok := carpoolCurrentDayLogBounds(start, end)
	if !ok {
		return nil, nil
	}
	return queryCarpoolUserUsage(group, startTs, endTs, carpoolUsageFilterNormal, 0)
}

func queryCarpoolCurrentDayTokenUsage(group string, start time.Time, end time.Time) ([]carpoolTokenUsageRow, error) {
	startTs, endTs, ok := carpoolCurrentDayLogBounds(start, end)
	if !ok {
		return nil, nil
	}
	return queryCarpoolTokenUsage(group, startTs, endTs, carpoolUsageFilterNormal, 0)
}

func queryCarpoolCurrentDayDailyUsage(group string, start time.Time, end time.Time) ([]carpoolDailyUsageRow, error) {
	startTs, endTs, ok := carpoolCurrentDayLogBounds(start, end)
	if !ok {
		return nil, nil
	}
	return queryCarpoolLogDailyUsage(group, startTs, endTs)
}

func queryCarpoolLogDailyUsage(group string, startTimestamp int64, endTimestamp int64) ([]carpoolDailyUsageRow, error) {
	var logRows []carpoolTailLogRow
	if err := baseCarpoolLogQuery(group, startTimestamp, endTimestamp, carpoolUsageFilterNormal, 0).
		Select("created_at, user_id, quota").
		Find(&logRows).Error; err != nil {
		return nil, err
	}
	merged := map[string]carpoolDailyUsageRow{}
	for _, row := range logRows {
		date := time.Unix(row.CreatedAt, 0).In(time.Local).Format("2006-01-02")
		key := carpoolDailyRowKey(row.UserID, date)
		daily := merged[key]
		daily.UserID = row.UserID
		daily.Date = date
		daily.Quota += row.Quota
		merged[key] = daily
	}
	rows := make([]carpoolDailyUsageRow, 0, len(merged))
	for _, row := range merged {
		rows = append(rows, row)
	}
	return rows, nil
}

func carpoolSnapshotRangeForCurrentDay(start time.Time, end time.Time, excludeCurrentDay bool) (time.Time, time.Time, bool) {
	if !excludeCurrentDay {
		return start, end, true
	}
	if _, _, ok := carpoolCurrentDayLogBounds(start, end); !ok {
		return start, end, true
	}
	today := carpoolStartOfDay(time.Now().In(time.Local))
	historicalEnd := today.Add(-time.Nanosecond)
	if !end.IsZero() && end.Before(historicalEnd) {
		historicalEnd = end
	}
	if !start.IsZero() && start.After(historicalEnd) {
		return time.Time{}, time.Time{}, false
	}
	return start, historicalEnd, true
}

func carpoolCurrentDayLogBounds(start time.Time, end time.Time) (int64, int64, bool) {
	now := time.Now().In(time.Local)
	today := carpoolStartOfDay(now)
	tomorrow := today.AddDate(0, 0, 1)
	rangeStart := today
	rangeEnd := now
	if !start.IsZero() {
		start = start.In(time.Local)
		if !start.Before(tomorrow) {
			return 0, 0, false
		}
		if start.After(rangeStart) {
			rangeStart = start
		}
	}
	if !end.IsZero() {
		end = end.In(time.Local)
		if end.Before(today) {
			return 0, 0, false
		}
		if end.Before(rangeEnd) {
			rangeEnd = end
		}
	}
	if rangeEnd.Before(rangeStart) {
		return 0, 0, false
	}
	return rangeStart.Unix(), rangeEnd.Unix(), true
}

func carpoolDateBounds(start time.Time, end time.Time) (string, string, bool) {
	if start.IsZero() && end.IsZero() {
		return "", "", false
	}
	if start.IsZero() {
		start = end
	}
	if end.IsZero() {
		end = start
	}
	startDate := carpoolStartOfDay(start).Format("2006-01-02")
	endDate := carpoolStartOfDay(end).Format("2006-01-02")
	return startDate, endDate, true
}

func mergeCarpoolUsageRows(primary []carpoolUsageRow, fallback []carpoolUsageRow) []carpoolUsageRow {
	if len(fallback) == 0 {
		return primary
	}
	merged := make(map[int]carpoolUsageRow, len(primary)+len(fallback))
	for _, row := range primary {
		merged[row.UserID] = row
	}
	for _, row := range fallback {
		existing := merged[row.UserID]
		if existing.Username == "" {
			existing.Username = row.Username
		}
		existing.UserID = row.UserID
		existing.Quota += row.Quota
		existing.TokenUsed += row.TokenUsed
		existing.RequestCount += row.RequestCount
		merged[row.UserID] = existing
	}
	rows := make([]carpoolUsageRow, 0, len(merged))
	for _, row := range merged {
		rows = append(rows, row)
	}
	return rows
}

func mergeCarpoolTokenUsageRows(primary []carpoolTokenUsageRow, fallback []carpoolTokenUsageRow) []carpoolTokenUsageRow {
	if len(fallback) == 0 {
		return primary
	}
	merged := make(map[int]carpoolTokenUsageRow, len(primary)+len(fallback))
	for _, row := range primary {
		if row.TokenID > 0 {
			merged[row.TokenID] = row
		}
	}
	for _, row := range fallback {
		if row.TokenID <= 0 {
			continue
		}
		existing := merged[row.TokenID]
		existing.TokenID = row.TokenID
		if existing.UserID == 0 {
			existing.UserID = row.UserID
		}
		if existing.TokenName == "" {
			existing.TokenName = row.TokenName
		}
		existing.Quota += row.Quota
		existing.TokenUsed += row.TokenUsed
		existing.RequestCount += row.RequestCount
		if row.LastSeen > existing.LastSeen {
			existing.LastSeen = row.LastSeen
		}
		merged[row.TokenID] = existing
	}
	rows := make([]carpoolTokenUsageRow, 0, len(merged))
	for _, row := range merged {
		rows = append(rows, row)
	}
	return rows
}

func mergeCarpoolDailyRows(primary []carpoolDailyUsageRow, fallback []carpoolDailyUsageRow) []carpoolDailyUsageRow {
	if len(fallback) == 0 {
		return primary
	}
	merged := make(map[string]carpoolDailyUsageRow, len(primary)+len(fallback))
	for _, row := range primary {
		merged[carpoolDailyRowKey(row.UserID, row.Date)] = row
	}
	for _, row := range fallback {
		key := carpoolDailyRowKey(row.UserID, row.Date)
		existing := merged[key]
		existing.UserID = row.UserID
		existing.Date = row.Date
		existing.Quota += row.Quota
		merged[key] = existing
	}
	rows := make([]carpoolDailyUsageRow, 0, len(merged))
	for _, row := range merged {
		rows = append(rows, row)
	}
	return rows
}

func carpoolDailyRowKey(userID int, date string) string {
	return strconv.Itoa(userID) + "|" + date
}

func hydrateCarpoolUsers(userMap map[int]*CarpoolUsageUserSummary, userIDs map[int]struct{}) error {
	if len(userIDs) == 0 {
		return nil
	}
	ids := make([]int, 0, len(userIDs))
	for userID := range userIDs {
		ids = append(ids, userID)
	}
	var users []User
	if err := DB.Unscoped().
		Model(&User{}).
		Select("id, username, display_name, email").
		Where("id IN ?", ids).
		Find(&users).Error; err != nil {
		return err
	}
	for _, user := range users {
		summary := userMap[user.Id]
		if summary == nil {
			continue
		}
		displayName := strings.TrimSpace(user.DisplayName)
		if displayName == "" {
			displayName = user.Username
		}
		if displayName != "" {
			summary.Username = displayName
		}
		summary.Email = user.Email
	}
	return nil
}

func carpoolPeriodRange(period string, now time.Time) (string, time.Time, time.Time) {
	period = strings.ToLower(strings.TrimSpace(period))
	now = now.In(time.Local)
	today := carpoolStartOfDay(now)
	if period == "day" {
		return period, today, now
	}
	if period == "month" {
		return period, time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location()), now
	}
	weekday := int(today.Weekday())
	if weekday == 0 {
		weekday = 7
	}
	start := today.AddDate(0, 0, -(weekday - 1))
	return "week", start, now
}

func carpoolStartOfDay(value time.Time) time.Time {
	value = value.In(time.Local)
	return time.Date(value.Year(), value.Month(), value.Day(), 0, 0, 0, 0, value.Location())
}

func carpoolFormatTimestamp(timestamp int64) string {
	if timestamp <= 0 {
		return ""
	}
	return time.Unix(timestamp, 0).In(time.Local).Format(time.RFC3339)
}
