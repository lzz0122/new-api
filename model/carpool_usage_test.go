package model

import (
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm/clause"
)

func TestCarpoolUsageSummarySubtractsPersistentCarnivalUsage(t *testing.T) {
	truncateTables(t)

	now := common.GetTimestamp()
	require.NoError(t, DB.Create(&User{Id: 1, Username: "alice", DisplayName: "Alice"}).Error)
	require.NoError(t, DB.Create(&Token{
		Id:        11,
		UserId:    1,
		Name:      "alice-key",
		Status:    common.TokenStatusEnabled,
		Group:     DefaultCarnivalGroup,
		UsedQuota: 1000,
	}).Error)
	require.NoError(t, LOG_DB.Create(&CarnivalSession{
		Id:        7,
		GroupName: DefaultCarnivalGroup,
		StartedAt: now - 3600,
		CreatedAt: now - 3600,
		UpdatedAt: now,
	}).Error)
	require.NoError(t, LOG_DB.Create(&CarnivalUsage{
		SessionID:    7,
		GroupName:    DefaultCarnivalGroup,
		UserID:       1,
		Username:     "alice",
		TokenID:      11,
		TokenName:    "alice-key",
		Quota:        200,
		TokenUsed:    20,
		CreatedAt:    now,
		PromptTokens: 10,
	}).Error)
	require.NoError(t, RecordCarpoolUsageDailySnapshot(CarpoolUsageDailySnapshotParams{
		Group:        DefaultCarnivalGroup,
		UserID:       1,
		Username:     "alice",
		TokenID:      11,
		TokenName:    "alice-key",
		Quota:        300,
		TokenUsed:    30,
		RequestCount: 1,
		CreatedAt:    now,
	}))

	summary, err := GetCarpoolUsageSummary(DefaultCarnivalGroup, "week")
	require.NoError(t, err)
	require.EqualValues(t, 300, summary.Totals.PeriodQuota)
	require.EqualValues(t, 200, summary.Totals.CarnivalPeriodQuota)
	require.EqualValues(t, 200, summary.Totals.CurrentCarnivalQuota)
	require.EqualValues(t, 800, summary.Totals.CumulativeQuota)
	require.EqualValues(t, 1000, summary.Totals.GrossCumulativeQuota)
	require.Len(t, summary.Users, 1)
	require.EqualValues(t, 800, summary.Users[0].CumulativeQuota)
	require.Len(t, summary.Users[0].Tokens, 1)
	require.EqualValues(t, 800, summary.Users[0].Tokens[0].CumulativeQuota)
}

func TestCarpoolSessionSummaryUsesLegacyDailyAndRealtimeTail(t *testing.T) {
	truncateTables(t)

	day := carpoolStartOfDay(time.Now().In(time.Local).AddDate(0, 0, -1))
	sessionStart := day.Add(9 * time.Hour)
	require.NoError(t, DB.Create(&User{Id: 1, Username: "alice", DisplayName: "Alice"}).Error)
	require.NoError(t, DB.Create(&Token{
		Id:     11,
		UserId: 1,
		Name:   "alice-key",
		Status: common.TokenStatusEnabled,
		Group:  DefaultCarnivalGroup,
	}).Error)
	require.NoError(t, LOG_DB.Create(&CarpoolSession{
		GroupName: DefaultCarnivalGroup,
		StartedAt: sessionStart.Unix(),
		CreatedAt: sessionStart.Unix(),
		UpdatedAt: sessionStart.Unix(),
	}).Error)
	require.NoError(t, LOG_DB.Create(&CarpoolUsageLegacyDaily{
		UsageDate:     day.Format("2006-01-02"),
		UserID:        1,
		TokenID:       11,
		Username:      "alice",
		TokenName:     "alice-key",
		QuotaDelta:    1000,
		FirstSyncedAt: day.Add(time.Hour),
		LastSyncedAt:  day.Add(time.Hour),
	}).Error)
	require.NoError(t, LOG_DB.Create(&CarnivalUsage{
		GroupName: DefaultCarnivalGroup,
		UserID:    1,
		Username:  "alice",
		TokenID:   0,
		TokenName: "alice-key",
		Quota:     300,
		CreatedAt: day.Add(time.Hour).Unix(),
	}).Error)
	require.NoError(t, LOG_DB.Create(&CarpoolUsageLegacySyncRun{
		SyncedAt:  sessionStart.Add(time.Hour),
		UsageDate: day.Format("2006-01-02"),
	}).Error)
	require.NoError(t, LOG_DB.Create(&Log{
		UserId:           1,
		Username:         "alice",
		CreatedAt:        sessionStart.Add(2 * time.Hour).Unix(),
		Type:             LogTypeConsume,
		TokenId:          11,
		TokenName:        "alice-key",
		Group:            DefaultCarnivalGroup,
		Quota:            200,
		PromptTokens:     20,
		CompletionTokens: 10,
	}).Error)

	summary, err := GetCarpoolUsageSessionSummary(DefaultCarnivalGroup, 0)
	require.NoError(t, err)
	require.EqualValues(t, 900, summary.Totals.PeriodQuota)
	require.EqualValues(t, 900, summary.Totals.CumulativeQuota)
	require.EqualValues(t, 300, summary.Totals.CarnivalPeriodQuota)
	require.EqualValues(t, 1, summary.Totals.PeriodRequestCount)
	require.Len(t, summary.Users, 1)
	require.EqualValues(t, 900, summary.Users[0].PeriodQuota)
	require.Len(t, summary.Users[0].Tokens, 1)
	require.EqualValues(t, 900, summary.Users[0].Tokens[0].PeriodQuota)
	require.EqualValues(t, 900, summary.Users[0].Daily[0].Quota)
}

func TestCarpoolSnapshotUsageFallsBackToQuotaDataBeforeSnapshots(t *testing.T) {
	truncateTables(t)

	now := time.Now().In(time.Local)
	yesterday := carpoolStartOfDay(now.AddDate(0, 0, -1))
	require.NoError(t, DB.Create(&Token{
		Id:     22,
		UserId: 2,
		Name:   "bob-key",
		Status: common.TokenStatusEnabled,
		Group:  DefaultCarnivalGroup,
	}).Error)
	require.NoError(t, DB.Create(&QuotaData{
		UserID:    2,
		Username:  "bob",
		ModelName: "gpt-test",
		CreatedAt: yesterday.Add(10 * time.Hour).Unix(),
		Quota:     700,
		TokenUsed: 70,
		Count:     7,
	}).Error)

	rows, err := queryCarpoolSnapshotUserUsage(DefaultCarnivalGroup, yesterday, yesterday.Add(23*time.Hour))
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.EqualValues(t, 2, rows[0].UserID)
	require.EqualValues(t, 700, rows[0].Quota)
	require.EqualValues(t, 70, rows[0].TokenUsed)
	require.EqualValues(t, 7, rows[0].RequestCount)
}

func TestCarpoolSnapshotUsageFallsBackUntilFirstSnapshotTimestamp(t *testing.T) {
	truncateTables(t)

	day := carpoolStartOfDay(time.Now().In(time.Local).AddDate(0, 0, -1))
	snapshotAt := day.Add(15 * time.Hour)
	require.NoError(t, DB.Create(&Token{
		Id:     33,
		UserId: 3,
		Name:   "carol-key",
		Status: common.TokenStatusEnabled,
		Group:  DefaultCarnivalGroup,
	}).Error)
	require.NoError(t, DB.Create(&QuotaData{
		UserID:    3,
		Username:  "carol",
		ModelName: "gpt-test",
		CreatedAt: day.Add(10 * time.Hour).Unix(),
		Quota:     100,
		TokenUsed: 10,
		Count:     1,
	}).Error)
	require.NoError(t, DB.Create(&QuotaData{
		UserID:    3,
		Username:  "carol",
		ModelName: "gpt-test",
		CreatedAt: day.Add(16 * time.Hour).Unix(),
		Quota:     900,
		TokenUsed: 90,
		Count:     9,
	}).Error)
	require.NoError(t, LOG_DB.Create(&CarpoolUsageDailyRecord{
		GroupName:    DefaultCarnivalGroup,
		UsageDate:    day.Format("2006-01-02"),
		UserID:       3,
		Username:     "carol",
		TokenID:      33,
		TokenName:    "carol-key",
		Quota:        200,
		TokenUsed:    20,
		RequestCount: 2,
		CreatedAt:    snapshotAt.Unix(),
		UpdatedAt:    snapshotAt.Unix(),
	}).Error)

	rows, err := queryCarpoolSnapshotUserUsage(DefaultCarnivalGroup, day, day.Add(23*time.Hour))
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.EqualValues(t, 3, rows[0].UserID)
	require.EqualValues(t, 300, rows[0].Quota)
	require.EqualValues(t, 30, rows[0].TokenUsed)
	require.EqualValues(t, 3, rows[0].RequestCount)
}

func TestCarpoolDayPeriodUsesRealtimeLogsForCurrentDay(t *testing.T) {
	truncateTables(t)

	now := time.Now().In(time.Local)
	today := carpoolStartOfDay(now)
	require.NoError(t, DB.Create(&User{Id: 5, Username: "eve", DisplayName: "Eve"}).Error)
	require.NoError(t, DB.Create(&Token{
		Id:     55,
		UserId: 5,
		Name:   "eve-key",
		Status: common.TokenStatusEnabled,
		Group:  DefaultCarnivalGroup,
	}).Error)
	require.NoError(t, LOG_DB.Create(&CarpoolUsageDailyRecord{
		GroupName:    DefaultCarnivalGroup,
		UsageDate:    today.Format("2006-01-02"),
		UserID:       5,
		Username:     "eve",
		TokenID:      55,
		TokenName:    "eve-key",
		Quota:        100,
		TokenUsed:    10,
		RequestCount: 1,
		CreatedAt:    now.Unix(),
		UpdatedAt:    now.Unix(),
	}).Error)
	require.NoError(t, LOG_DB.Create(&Log{
		UserId:           5,
		Username:         "eve",
		CreatedAt:        now.Unix(),
		Type:             LogTypeConsume,
		TokenId:          55,
		TokenName:        "eve-key",
		Group:            DefaultCarnivalGroup,
		Quota:            700,
		PromptTokens:     70,
		CompletionTokens: 7,
	}).Error)

	summary, err := buildCarpoolUsageSummary(DefaultCarnivalGroup, "day", today, now, false, nil)
	require.NoError(t, err)
	require.EqualValues(t, 700, summary.Totals.PeriodQuota)
	require.EqualValues(t, 700, summary.Totals.CumulativeQuota)
	require.EqualValues(t, 77, summary.Totals.PeriodTokenUsed)
	require.EqualValues(t, 1, summary.Totals.PeriodRequestCount)
	require.Len(t, summary.Users, 1)
	require.EqualValues(t, 700, summary.Users[0].PeriodQuota)
	require.EqualValues(t, 700, summary.Users[0].CumulativeQuota)
	require.Len(t, summary.Users[0].Daily, 1)
	require.EqualValues(t, 700, summary.Users[0].Daily[0].Quota)
	require.Len(t, summary.Users[0].Tokens, 1)
	require.EqualValues(t, 700, summary.Users[0].Tokens[0].PeriodQuota)
	require.EqualValues(t, 700, summary.Users[0].Tokens[0].CumulativeQuota)
}

func TestCarpoolDayPeriodKeepsLegacyCumulativeUsage(t *testing.T) {
	truncateTables(t)

	now := time.Now().In(time.Local)
	today := carpoolStartOfDay(now)
	yesterday := today.AddDate(0, 0, -1)
	require.NoError(t, DB.Create(&Token{
		Id:     44,
		UserId: 4,
		Name:   "dave-key",
		Status: common.TokenStatusEnabled,
		Group:  DefaultCarnivalGroup,
	}).Error)
	require.NoError(t, LOG_DB.Create(&CarpoolUsageLegacyDaily{
		UsageDate:     yesterday.Format("2006-01-02"),
		UserID:        4,
		TokenID:       44,
		Username:      "dave",
		TokenName:     "dave-key",
		QuotaDelta:    1000,
		FirstSyncedAt: yesterday.Add(time.Hour),
		LastSyncedAt:  yesterday.Add(time.Hour),
	}).Error)
	require.NoError(t, LOG_DB.Create(&CarpoolUsageLegacyDaily{
		UsageDate:     today.Format("2006-01-02"),
		UserID:        4,
		TokenID:       44,
		Username:      "dave",
		TokenName:     "dave-key",
		QuotaDelta:    200,
		FirstSyncedAt: today.Add(time.Hour),
		LastSyncedAt:  today.Add(time.Hour),
	}).Error)
	require.NoError(t, LOG_DB.Create(&CarpoolUsageLegacySyncRun{
		SyncedAt:  today.Add(2 * time.Hour),
		UsageDate: today.Format("2006-01-02"),
	}).Error)
	require.NoError(t, LOG_DB.Create(&CarpoolUsageDailyRecord{
		GroupName:    DefaultCarnivalGroup,
		UsageDate:    today.Format("2006-01-02"),
		UserID:       4,
		Username:     "dave",
		TokenID:      44,
		TokenName:    "dave-key",
		Quota:        200,
		TokenUsed:    20,
		RequestCount: 2,
		CreatedAt:    yesterday.Add(12 * time.Hour).Unix(),
		UpdatedAt:    today.Add(time.Hour).Unix(),
	}).Error)

	period, start, end := carpoolPeriodRange("day", today.Add(12*time.Hour))
	require.Equal(t, "day", period)
	require.Equal(t, today, start)

	summary, err := buildCarpoolUsageSummary(DefaultCarnivalGroup, period, start, end, false, nil)
	require.NoError(t, err)
	require.EqualValues(t, 200, summary.Totals.PeriodQuota)
	require.EqualValues(t, 1200, summary.Totals.CumulativeQuota)
	require.Len(t, summary.Users, 1)
	require.EqualValues(t, 200, summary.Users[0].PeriodQuota)
	require.EqualValues(t, 1200, summary.Users[0].CumulativeQuota)
}

func TestCarpoolUsageDailySnapshotUpsertQualifiesConflictColumns(t *testing.T) {
	conflict := carpoolUsageDailySnapshotOnConflict(CarpoolUsageDailySnapshotParams{
		Username:     "alice",
		TokenName:    "alice-key",
		Quota:        100,
		TokenUsed:    10,
		RequestCount: 1,
	}, 2)

	expressions := map[string]string{}
	for _, assignment := range conflict.DoUpdates {
		expr, ok := assignment.Value.(clause.Expr)
		if !ok {
			continue
		}
		expressions[assignment.Column.Name] = expr.SQL
	}

	require.Contains(t, expressions["username"], "carpool_usage_daily_records.username")
	require.Contains(t, expressions["token_name"], "carpool_usage_daily_records.token_name")
	require.NotContains(t, strings.ToLower(expressions["username"]), "else username end")
	require.NotContains(t, strings.ToLower(expressions["token_name"]), "else token_name end")
}

func TestEnsureDefaultCarpoolSessionCreatesActiveWhenOnlyHistoryExists(t *testing.T) {
	truncateTables(t)

	require.NoError(t, LOG_DB.Create(&CarpoolSession{
		GroupName: DefaultCarnivalGroup,
		StartedAt: 1,
		EndedAt:   2,
		CreatedAt: 1,
		UpdatedAt: 2,
	}).Error)

	require.NoError(t, EnsureDefaultCarpoolSession())
	require.NoError(t, EnsureDefaultCarpoolSession())

	var activeSessions []CarpoolSession
	require.NoError(t, LOG_DB.Where("group_name = ? AND ended_at = 0", DefaultCarnivalGroup).Find(&activeSessions).Error)
	require.Len(t, activeSessions, 1)

	expectedStart, err := time.ParseInLocation("2006-01-02 15:04:05", defaultCarpoolSessionStartedAt, time.Local)
	require.NoError(t, err)
	require.EqualValues(t, expectedStart.Unix(), activeSessions[0].StartedAt)

	var total int64
	require.NoError(t, LOG_DB.Model(&CarpoolSession{}).Where("group_name = ?", DefaultCarnivalGroup).Count(&total).Error)
	require.EqualValues(t, 2, total)
}

func TestListActiveCarpoolSessionGroupsFiltersInactiveAndAllowedGroups(t *testing.T) {
	truncateTables(t)

	now := common.GetTimestamp()
	require.NoError(t, LOG_DB.Create(&CarpoolSession{
		GroupName: DefaultCarnivalGroup,
		StartedAt: now - 3600,
		EndedAt:   0,
		CreatedAt: now - 3600,
		UpdatedAt: now,
	}).Error)
	require.NoError(t, LOG_DB.Create(&CarpoolSession{
		GroupName: "beta",
		StartedAt: now - 7200,
		EndedAt:   now - 3600,
		CreatedAt: now - 7200,
		UpdatedAt: now - 3600,
	}).Error)
	require.NoError(t, LOG_DB.Create(&CarpoolSession{
		GroupName: "vip",
		StartedAt: now - 1800,
		EndedAt:   0,
		CreatedAt: now - 1800,
		UpdatedAt: now,
	}).Error)

	groups, err := ListActiveCarpoolSessionGroups([]string{DefaultCarnivalGroup, "beta"})
	require.NoError(t, err)
	require.Equal(t, []string{DefaultCarnivalGroup}, groups)

	allGroups, err := ListActiveCarpoolSessionGroups(nil)
	require.NoError(t, err)
	require.ElementsMatch(t, []string{DefaultCarnivalGroup, "vip"}, allGroups)
}
