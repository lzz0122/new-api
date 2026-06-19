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
