package model

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/bytedance/gopkg/util/gopool"
	"gorm.io/gorm"
)

type Token struct {
	Id                 int            `json:"id"`
	UserId             int            `json:"user_id" gorm:"index"`
	Key                string         `json:"key" gorm:"type:varchar(128);uniqueIndex"`
	Status             int            `json:"status" gorm:"default:1"`
	Name               string         `json:"name" gorm:"index" `
	CreatedTime        int64          `json:"created_time" gorm:"bigint"`
	AccessedTime       int64          `json:"accessed_time" gorm:"bigint"`
	ExpiredTime        int64          `json:"expired_time" gorm:"bigint;default:-1"` // -1 means never expired
	RemainQuota        int            `json:"remain_quota" gorm:"default:0"`
	UnlimitedQuota     bool           `json:"unlimited_quota"`
	ModelLimitsEnabled bool           `json:"model_limits_enabled"`
	ModelLimits        string         `json:"model_limits" gorm:"type:text"`
	AllowIps           *string        `json:"allow_ips" gorm:"default:''"`
	UsedQuota          int            `json:"used_quota" gorm:"default:0"` // used quota
	Group              string         `json:"group" gorm:"default:''"`
	CrossGroupRetry    bool           `json:"cross_group_retry"` // 跨分组重试，仅auto分组有效
	GroupConfig        string         `json:"group_config" gorm:"type:text"`
	DeletedAt          gorm.DeletedAt `gorm:"index"`
}

const (
	TokenGroupFailoverStrategyFallback    = "fallback"
	TokenGroupFailoverStrategyReturnError = "return_error"

	TokenGroupRecoveryStrategyProbeThenSwitch = "probe_then_switch"
	TokenGroupRecoveryStrategySticky          = "sticky"

	TokenGroupDetectionStrategyOne   = "one"
	TokenGroupDetectionStrategyHalf  = "half"
	TokenGroupDetectionStrategyAll   = "all"
	TokenGroupDetectionStrategyRatio = "ratio"

	DefaultTokenGroupCooldownSeconds = 600
	DefaultTokenGroupDetectionRatio  = 0.5
)

type TokenGroupItem struct {
	Group                     string  `json:"group"`
	Order                     int     `json:"order"`
	FailoverStrategy          string  `json:"failover_strategy,omitempty"`
	TimeoutSeconds            int     `json:"timeout_seconds,omitempty"`
	CooldownSeconds           int     `json:"cooldown_seconds,omitempty"`
	RecoveryStrategy          string  `json:"recovery_strategy,omitempty"`
	FailureDetectionStrategy  string  `json:"failure_detection_strategy,omitempty"`
	FailureDetectionRatio     float64 `json:"failure_detection_ratio,omitempty"`
	RecoveryDetectionStrategy string  `json:"recovery_detection_strategy,omitempty"`
	RecoveryDetectionRatio    float64 `json:"recovery_detection_ratio,omitempty"`
}

type TokenGroupConfig struct {
	Groups                    []TokenGroupItem `json:"groups"`
	FailoverStrategy          string           `json:"failover_strategy"`
	TimeoutSeconds            int              `json:"timeout_seconds"`
	CooldownSeconds           int              `json:"cooldown_seconds"`
	RecoveryStrategy          string           `json:"recovery_strategy"`
	FailureDetectionStrategy  string           `json:"failure_detection_strategy"`
	FailureDetectionRatio     float64          `json:"failure_detection_ratio"`
	RecoveryDetectionStrategy string           `json:"recovery_detection_strategy"`
	RecoveryDetectionRatio    float64          `json:"recovery_detection_ratio"`
}

func DefaultTokenGroupConfig(group string) TokenGroupConfig {
	group = strings.TrimSpace(group)
	cfg := TokenGroupConfig{
		FailoverStrategy:          TokenGroupFailoverStrategyReturnError,
		CooldownSeconds:           DefaultTokenGroupCooldownSeconds,
		RecoveryStrategy:          TokenGroupRecoveryStrategyProbeThenSwitch,
		FailureDetectionStrategy:  TokenGroupDetectionStrategyOne,
		FailureDetectionRatio:     DefaultTokenGroupDetectionRatio,
		RecoveryDetectionStrategy: TokenGroupDetectionStrategyOne,
		RecoveryDetectionRatio:    DefaultTokenGroupDetectionRatio,
	}
	if group != "" {
		cfg.Groups = []TokenGroupItem{{Group: group, Order: 1}}
	}
	return cfg
}

func NormalizeTokenGroupConfig(cfg TokenGroupConfig, fallbackGroup string) TokenGroupConfig {
	if cfg.FailoverStrategy != TokenGroupFailoverStrategyFallback &&
		cfg.FailoverStrategy != TokenGroupFailoverStrategyReturnError {
		if len(cfg.Groups) > 1 {
			cfg.FailoverStrategy = TokenGroupFailoverStrategyFallback
		} else {
			cfg.FailoverStrategy = TokenGroupFailoverStrategyReturnError
		}
	}
	if cfg.CooldownSeconds <= 0 {
		cfg.CooldownSeconds = DefaultTokenGroupCooldownSeconds
	}
	if cfg.TimeoutSeconds < 0 {
		cfg.TimeoutSeconds = 0
	}
	if cfg.RecoveryStrategy != TokenGroupRecoveryStrategyProbeThenSwitch &&
		cfg.RecoveryStrategy != TokenGroupRecoveryStrategySticky {
		cfg.RecoveryStrategy = TokenGroupRecoveryStrategyProbeThenSwitch
	}
	cfg.FailureDetectionStrategy = normalizeTokenGroupDetectionStrategy(cfg.FailureDetectionStrategy)
	cfg.FailureDetectionRatio = normalizeTokenGroupDetectionRatio(cfg.FailureDetectionRatio)
	cfg.RecoveryDetectionStrategy = normalizeTokenGroupDetectionStrategy(cfg.RecoveryDetectionStrategy)
	cfg.RecoveryDetectionRatio = normalizeTokenGroupDetectionRatio(cfg.RecoveryDetectionRatio)

	seen := make(map[string]struct{}, len(cfg.Groups))
	groups := make([]TokenGroupItem, 0, len(cfg.Groups))
	for idx, item := range cfg.Groups {
		item.Group = strings.TrimSpace(item.Group)
		if item.Group == "" {
			continue
		}
		if _, ok := seen[item.Group]; ok {
			continue
		}
		seen[item.Group] = struct{}{}
		if item.Order <= 0 {
			item.Order = idx + 1
		}
		item = normalizeTokenGroupItemSettings(item)
		groups = append(groups, item)
	}
	if len(groups) == 0 {
		fallbackGroup = strings.TrimSpace(fallbackGroup)
		if fallbackGroup != "" {
			groups = append(groups, TokenGroupItem{Group: fallbackGroup, Order: 1})
		}
	}
	sort.SliceStable(groups, func(i, j int) bool {
		if groups[i].Order == groups[j].Order {
			return i < j
		}
		return groups[i].Order < groups[j].Order
	})
	for idx := range groups {
		if groups[idx].Order <= 0 {
			groups[idx].Order = idx + 1
		}
	}
	cfg.Groups = groups
	return cfg
}

func normalizeTokenGroupDetectionStrategy(strategy string) string {
	switch strategy {
	case TokenGroupDetectionStrategyHalf,
		TokenGroupDetectionStrategyAll,
		TokenGroupDetectionStrategyRatio:
		return strategy
	default:
		return TokenGroupDetectionStrategyOne
	}
}

func normalizeTokenGroupDetectionRatio(ratio float64) float64 {
	if ratio <= 0 || ratio > 1 {
		return DefaultTokenGroupDetectionRatio
	}
	return ratio
}

func normalizeTokenGroupItemSettings(item TokenGroupItem) TokenGroupItem {
	if item.FailoverStrategy != "" &&
		item.FailoverStrategy != TokenGroupFailoverStrategyFallback &&
		item.FailoverStrategy != TokenGroupFailoverStrategyReturnError {
		item.FailoverStrategy = ""
	}
	if item.TimeoutSeconds < 0 {
		item.TimeoutSeconds = 0
	}
	if item.CooldownSeconds < 0 {
		item.CooldownSeconds = 0
	}
	if item.RecoveryStrategy != "" &&
		item.RecoveryStrategy != TokenGroupRecoveryStrategyProbeThenSwitch &&
		item.RecoveryStrategy != TokenGroupRecoveryStrategySticky {
		item.RecoveryStrategy = ""
	}
	if item.FailureDetectionStrategy != "" {
		item.FailureDetectionStrategy = normalizeTokenGroupDetectionStrategy(item.FailureDetectionStrategy)
	}
	if item.FailureDetectionRatio < 0 || item.FailureDetectionRatio > 1 {
		item.FailureDetectionRatio = 0
	}
	if item.RecoveryDetectionStrategy != "" {
		item.RecoveryDetectionStrategy = normalizeTokenGroupDetectionStrategy(item.RecoveryDetectionStrategy)
	}
	if item.RecoveryDetectionRatio < 0 || item.RecoveryDetectionRatio > 1 {
		item.RecoveryDetectionRatio = 0
	}
	return item
}

func tokenGroupItemHasCustomSettings(item TokenGroupItem) bool {
	return item.FailoverStrategy != "" ||
		item.TimeoutSeconds != 0 ||
		item.CooldownSeconds != 0 ||
		item.RecoveryStrategy != "" ||
		item.FailureDetectionStrategy != "" ||
		item.FailureDetectionRatio != 0 ||
		item.RecoveryDetectionStrategy != "" ||
		item.RecoveryDetectionRatio != 0
}

func (token *Token) ParseGroupConfig() TokenGroupConfig {
	if token == nil {
		return NormalizeTokenGroupConfig(TokenGroupConfig{}, "")
	}
	if strings.TrimSpace(token.GroupConfig) == "" {
		return DefaultTokenGroupConfig(token.Group)
	}
	var cfg TokenGroupConfig
	if err := json.Unmarshal([]byte(token.GroupConfig), &cfg); err != nil {
		return DefaultTokenGroupConfig(token.Group)
	}
	return NormalizeTokenGroupConfig(cfg, token.Group)
}

func (token *Token) SetGroupConfig(cfg TokenGroupConfig) error {
	cfg = NormalizeTokenGroupConfig(cfg, token.Group)
	if len(cfg.Groups) > 0 {
		token.Group = cfg.Groups[0].Group
	}
	if len(cfg.Groups) <= 1 &&
		cfg.TimeoutSeconds == 0 &&
		cfg.CooldownSeconds == DefaultTokenGroupCooldownSeconds &&
		cfg.RecoveryStrategy == TokenGroupRecoveryStrategyProbeThenSwitch &&
		cfg.FailureDetectionStrategy == TokenGroupDetectionStrategyOne &&
		cfg.FailureDetectionRatio == DefaultTokenGroupDetectionRatio &&
		cfg.RecoveryDetectionStrategy == TokenGroupDetectionStrategyOne &&
		cfg.RecoveryDetectionRatio == DefaultTokenGroupDetectionRatio &&
		(len(cfg.Groups) == 0 || !tokenGroupItemHasCustomSettings(cfg.Groups[0])) {
		token.GroupConfig = ""
		return nil
	}
	data, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	token.GroupConfig = string(data)
	return nil
}

func (token *Token) OrderedGroups() []string {
	cfg := token.ParseGroupConfig()
	groups := make([]string, 0, len(cfg.Groups))
	for _, item := range cfg.Groups {
		groups = append(groups, item.Group)
	}
	return groups
}

func (token *Token) Clean() {
	token.Key = ""
}

func MaskTokenKey(key string) string {
	if key == "" {
		return ""
	}
	if len(key) <= 4 {
		return strings.Repeat("*", len(key))
	}
	if len(key) <= 8 {
		return key[:2] + "****" + key[len(key)-2:]
	}
	return key[:4] + "**********" + key[len(key)-4:]
}

func (token *Token) GetFullKey() string {
	return token.Key
}

func (token *Token) GetMaskedKey() string {
	return MaskTokenKey(token.Key)
}

func (token *Token) GetIpLimits() []string {
	// delete empty spaces
	//split with \n
	ipLimits := make([]string, 0)
	if token.AllowIps == nil {
		return ipLimits
	}
	cleanIps := strings.ReplaceAll(*token.AllowIps, " ", "")
	if cleanIps == "" {
		return ipLimits
	}
	ips := strings.Split(cleanIps, "\n")
	for _, ip := range ips {
		ip = strings.TrimSpace(ip)
		ip = strings.ReplaceAll(ip, ",", "")
		if ip != "" {
			ipLimits = append(ipLimits, ip)
		}
	}
	return ipLimits
}

func GetAllUserTokens(userId int, startIdx int, num int) ([]*Token, error) {
	var tokens []*Token
	var err error
	err = DB.Where("user_id = ?", userId).Order("id desc").Limit(num).Offset(startIdx).Find(&tokens).Error
	return tokens, err
}

// sanitizeLikePattern 校验并清洗用户输入的 LIKE 搜索模式。
// 规则：
//  1. 转义 ! 和 _（使用 ! 作为 ESCAPE 字符，兼容 MySQL/PostgreSQL/SQLite）
//  2. 连续的 % 合并为单个 %
//  3. 最多允许 2 个 %
//  4. 含 % 时（模糊搜索），去掉 % 后关键词长度必须 >= 2
//  5. 不含 % 时按精确匹配
func sanitizeLikePattern(input string) (string, error) {
	// 1. 先转义 ESCAPE 字符 ! 自身，再转义 _
	//    使用 ! 而非 \ 作为 ESCAPE 字符，避免 MySQL 中反斜杠的字符串转义问题
	input = strings.ReplaceAll(input, "!", "!!")
	input = strings.ReplaceAll(input, `_`, `!_`)

	// 2. 连续的 % 直接拒绝
	if strings.Contains(input, "%%") {
		return "", errors.New("搜索模式中不允许包含连续的 % 通配符")
	}

	// 3. 统计 % 数量，不得超过 2
	count := strings.Count(input, "%")
	if count > 2 {
		return "", errors.New("搜索模式中最多允许包含 2 个 % 通配符")
	}

	// 4. 含 % 时，去掉 % 后关键词长度必须 >= 2
	if count > 0 {
		stripped := strings.ReplaceAll(input, "%", "")
		if len(stripped) < 2 {
			return "", errors.New("使用模糊搜索时，关键词长度至少为 2 个字符")
		}
		return input, nil
	}

	// 5. 无 % 时，精确全匹配
	return input, nil
}

const searchHardLimit = 100

func SearchUserTokens(userId int, keyword string, token string, offset int, limit int) (tokens []*Token, total int64, err error) {
	// model 层强制截断
	if limit <= 0 || limit > searchHardLimit {
		limit = searchHardLimit
	}
	if offset < 0 {
		offset = 0
	}

	if token != "" {
		token = strings.TrimPrefix(token, "sk-")
	}

	// 超量用户（令牌数超过上限）只允许精确搜索，禁止模糊搜索
	maxTokens := operation_setting.GetMaxUserTokens()
	hasFuzzy := strings.Contains(keyword, "%") || strings.Contains(token, "%")
	if hasFuzzy {
		count, err := CountUserTokens(userId)
		if err != nil {
			common.SysLog("failed to count user tokens: " + err.Error())
			return nil, 0, errors.New("获取令牌数量失败")
		}
		if int(count) > maxTokens {
			return nil, 0, errors.New("令牌数量超过上限，仅允许精确搜索，请勿使用 % 通配符")
		}
	}

	baseQuery := DB.Model(&Token{}).Where("user_id = ?", userId)

	// 非空才加 LIKE 条件，空则跳过（不过滤该字段）
	if keyword != "" {
		keywordPattern, err := sanitizeLikePattern(keyword)
		if err != nil {
			return nil, 0, err
		}
		baseQuery = baseQuery.Where("name LIKE ? ESCAPE '!'", keywordPattern)
	}
	if token != "" {
		tokenPattern, err := sanitizeLikePattern(token)
		if err != nil {
			return nil, 0, err
		}
		baseQuery = baseQuery.Where(commonKeyCol+" LIKE ? ESCAPE '!'", tokenPattern)
	}

	// 先查匹配总数（用于分页，受 maxTokens 上限保护，避免全表 COUNT）
	err = baseQuery.Limit(maxTokens).Count(&total).Error
	if err != nil {
		common.SysError("failed to count search tokens: " + err.Error())
		return nil, 0, errors.New("搜索令牌失败")
	}

	// 再分页查数据
	err = baseQuery.Order("id desc").Offset(offset).Limit(limit).Find(&tokens).Error
	if err != nil {
		common.SysError("failed to search tokens: " + err.Error())
		return nil, 0, errors.New("搜索令牌失败")
	}
	return tokens, total, nil
}

func ValidateUserToken(key string) (token *Token, err error) {
	if key == "" {
		return nil, ErrTokenNotProvided
	}
	token, err = GetTokenByKey(key, false)
	if err == nil {
		if token.Status == common.TokenStatusExhausted ||
			token.Status == common.TokenStatusExpired ||
			token.Status != common.TokenStatusEnabled {
			return token, ErrTokenInvalid
		}
		if token.ExpiredTime != -1 && token.ExpiredTime < common.GetTimestamp() {
			if !common.RedisEnabled {
				token.Status = common.TokenStatusExpired
				err := token.SelectUpdate()
				if err != nil {
					common.SysLog("failed to update token status" + err.Error())
				}
			}
			return token, ErrTokenInvalid
		}
		if !token.UnlimitedQuota && token.RemainQuota <= 0 {
			if !common.RedisEnabled {
				token.Status = common.TokenStatusExhausted
				err := token.SelectUpdate()
				if err != nil {
					common.SysLog("failed to update token status" + err.Error())
				}
			}
			return token, ErrTokenInvalid
		}
		return token, nil
	}
	common.SysLog("ValidateUserToken: failed to get token: " + err.Error())
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrTokenInvalid
	}
	return nil, fmt.Errorf("%w: %v", ErrDatabase, err)
}

func GetTokenByIds(id int, userId int) (*Token, error) {
	if id == 0 || userId == 0 {
		return nil, errors.New("id 或 userId 为空！")
	}
	token := Token{Id: id, UserId: userId}
	var err error = nil
	err = DB.First(&token, "id = ? and user_id = ?", id, userId).Error
	return &token, err
}

func GetTokenById(id int) (*Token, error) {
	if id == 0 {
		return nil, errors.New("id 为空！")
	}
	token := Token{Id: id}
	var err error = nil
	err = DB.First(&token, "id = ?", id).Error
	if shouldUpdateRedis(true, err) {
		gopool.Go(func() {
			if err := cacheSetToken(token); err != nil {
				common.SysLog("failed to update user status cache: " + err.Error())
			}
		})
	}
	return &token, err
}

func GetTokenByKey(key string, fromDB bool) (token *Token, err error) {
	defer func() {
		// Update Redis cache asynchronously on successful DB read
		if shouldUpdateRedis(fromDB, err) && token != nil {
			gopool.Go(func() {
				if err := cacheSetToken(*token); err != nil {
					common.SysLog("failed to update user status cache: " + err.Error())
				}
			})
		}
	}()
	if !fromDB && common.RedisEnabled {
		// Try Redis first
		token, err := cacheGetTokenByKey(key)
		if err == nil {
			return token, nil
		}
		// Don't return error - fall through to DB
	}
	fromDB = true
	err = DB.Where(commonKeyCol+" = ?", key).First(&token).Error
	return token, err
}

func (token *Token) Insert() error {
	var err error
	err = DB.Create(token).Error
	return err
}

// Update Make sure your token's fields is completed, because this will update non-zero values
func (token *Token) Update() (err error) {
	defer func() {
		if shouldUpdateRedis(true, err) {
			gopool.Go(func() {
				err := cacheSetToken(*token)
				if err != nil {
					common.SysLog("failed to update token cache: " + err.Error())
				}
			})
		}
	}()
	err = DB.Model(token).Select("name", "status", "expired_time", "remain_quota", "unlimited_quota",
		"model_limits_enabled", "model_limits", "allow_ips", "group", "cross_group_retry", "group_config").Updates(token).Error
	return err
}

func (token *Token) SelectUpdate() (err error) {
	defer func() {
		if shouldUpdateRedis(true, err) {
			gopool.Go(func() {
				err := cacheSetToken(*token)
				if err != nil {
					common.SysLog("failed to update token cache: " + err.Error())
				}
			})
		}
	}()
	// This can update zero values
	return DB.Model(token).Select("accessed_time", "status").Updates(token).Error
}

func (token *Token) Delete() (err error) {
	defer func() {
		if shouldUpdateRedis(true, err) {
			gopool.Go(func() {
				err := cacheDeleteToken(token.Key)
				if err != nil {
					common.SysLog("failed to delete token cache: " + err.Error())
				}
			})
		}
	}()
	err = DB.Delete(token).Error
	return err
}

func (token *Token) IsModelLimitsEnabled() bool {
	return token.ModelLimitsEnabled
}

func (token *Token) GetModelLimits() []string {
	if token.ModelLimits == "" {
		return []string{}
	}
	return strings.Split(token.ModelLimits, ",")
}

func (token *Token) GetModelLimitsMap() map[string]bool {
	limits := token.GetModelLimits()
	limitsMap := make(map[string]bool)
	for _, limit := range limits {
		limitsMap[limit] = true
	}
	return limitsMap
}

func DisableModelLimits(tokenId int) error {
	token, err := GetTokenById(tokenId)
	if err != nil {
		return err
	}
	token.ModelLimitsEnabled = false
	token.ModelLimits = ""
	return token.Update()
}

func DeleteTokenById(id int, userId int) (err error) {
	// Why we need userId here? In case user want to delete other's token.
	if id == 0 || userId == 0 {
		return errors.New("id 或 userId 为空！")
	}
	token := Token{Id: id, UserId: userId}
	err = DB.Where(token).First(&token).Error
	if err != nil {
		return err
	}
	return token.Delete()
}

func IncreaseTokenQuota(tokenId int, key string, quota int) (err error) {
	if quota < 0 {
		return errors.New("quota 不能为负数！")
	}
	if common.RedisEnabled {
		gopool.Go(func() {
			err := cacheIncrTokenQuota(key, int64(quota))
			if err != nil {
				common.SysLog("failed to increase token quota: " + err.Error())
			}
		})
	}
	if common.BatchUpdateEnabled {
		addNewRecord(BatchUpdateTypeTokenQuota, tokenId, quota)
		return nil
	}
	return increaseTokenQuota(tokenId, quota)
}

func increaseTokenQuota(id int, quota int) (err error) {
	err = DB.Model(&Token{}).Where("id = ?", id).Updates(
		map[string]interface{}{
			"remain_quota":  gorm.Expr("remain_quota + ?", quota),
			"used_quota":    gorm.Expr("used_quota - ?", quota),
			"accessed_time": common.GetTimestamp(),
		},
	).Error
	return err
}

func DecreaseTokenQuota(id int, key string, quota int) (err error) {
	if quota < 0 {
		return errors.New("quota 不能为负数！")
	}
	if common.RedisEnabled {
		gopool.Go(func() {
			err := cacheDecrTokenQuota(key, int64(quota))
			if err != nil {
				common.SysLog("failed to decrease token quota: " + err.Error())
			}
		})
	}
	if common.BatchUpdateEnabled {
		addNewRecord(BatchUpdateTypeTokenQuota, id, -quota)
		return nil
	}
	return decreaseTokenQuota(id, quota)
}

func decreaseTokenQuota(id int, quota int) (err error) {
	err = DB.Model(&Token{}).Where("id = ?", id).Updates(
		map[string]interface{}{
			"remain_quota":  gorm.Expr("remain_quota - ?", quota),
			"used_quota":    gorm.Expr("used_quota + ?", quota),
			"accessed_time": common.GetTimestamp(),
		},
	).Error
	return err
}

// CountUserTokens returns total number of tokens for the given user, used for pagination
func CountUserTokens(userId int) (int64, error) {
	var total int64
	err := DB.Model(&Token{}).Where("user_id = ?", userId).Count(&total).Error
	return total, err
}

// BatchDeleteTokens 删除指定用户的一组令牌，返回成功删除数量
func BatchDeleteTokens(ids []int, userId int) (int, error) {
	if len(ids) == 0 {
		return 0, errors.New("ids 不能为空！")
	}

	tx := DB.Begin()

	var tokens []Token
	if err := tx.Where("user_id = ? AND id IN (?)", userId, ids).Find(&tokens).Error; err != nil {
		tx.Rollback()
		return 0, err
	}

	if err := tx.Where("user_id = ? AND id IN (?)", userId, ids).Delete(&Token{}).Error; err != nil {
		tx.Rollback()
		return 0, err
	}

	if err := tx.Commit().Error; err != nil {
		return 0, err
	}

	if common.RedisEnabled {
		gopool.Go(func() {
			for _, t := range tokens {
				_ = cacheDeleteToken(t.Key)
			}
		})
	}

	return len(tokens), nil
}

func GetTokenKeysByIds(ids []int, userId int) ([]Token, error) {
	var tokens []Token
	err := DB.Select("id", commonKeyCol).
		Where("user_id = ? AND id IN (?)", userId, ids).
		Find(&tokens).Error
	return tokens, err
}

// InvalidateUserTokensCache 清理指定用户所有令牌在 Redis 中的缓存，
// 配合 InvalidateUserCache 使用，可在用户被禁用/删除时立即阻断其令牌的请求。
// 下一次请求将从数据库重新加载令牌及用户状态，从而立即识别出被禁用的用户。
func InvalidateUserTokensCache(userId int) error {
	if !common.RedisEnabled {
		return nil
	}
	if userId <= 0 {
		return errors.New("userId 无效")
	}
	var tokens []Token
	if err := DB.Unscoped().
		Select("id", commonKeyCol).
		Where("user_id = ?", userId).
		Find(&tokens).Error; err != nil {
		return err
	}
	var firstErr error
	for _, t := range tokens {
		if t.Key == "" {
			continue
		}
		if err := cacheDeleteToken(t.Key); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}
