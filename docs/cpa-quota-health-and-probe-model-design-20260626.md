# 2026-06-26 CPA 配额探活联动与渠道测活模型增量设计

## 背景

`base_url=http://cli-proxy-api:8317` 的渠道实际转发到本机 CPA。CPA 会按密钥识别 CPA 分组，再在该分组账号池中选择上游账号。账号可能触发 5h 或周额度限制，因此这类渠道的“下次测活时间”不能只使用 new-api 管理员配置的固定探活周期。

同时线上观察到 `lzz_plus` 渠道长期不恢复：渠道 37 的自动探活默认取 `channels.models` 第一个模型 `gpt-5.3-codex-spark`，该模型在当前 Codex 账号下返回 400/503，导致探活一直失败；而真实业务请求 `gpt-5.5` 曾经可以成功。这说明探活模型必须由管理员显式选择，并按模型记录结果。

## CPA 配额探活方案

new-api 对 `cli-proxy-api:8317` 渠道进入 unhealthy 后，下一次自动探活前先调用 CPA 内部接口判断密钥关联分组的额度刷新时间。

推荐 CPA 新增接口：

`POST /v1/internal/quota-health`

请求体：

```json
{
  "api_key": "cpa key",
  "models": ["gpt-5.5"]
}
```

响应体：

```json
{
  "success": true,
  "quota_available": false,
  "reason": "weekly_quota_exhausted",
  "next_available_at": 1780000000,
  "retry_after_seconds": 3600
}
```

规则：

- CPA 返回 401：new-api 直接按渠道错误处理，不继续探活。
- CPA 返回 500+ 或网络错误：new-api 固定 15 秒间隔重试 3 次；仍失败则保持渠道错误，并使用 new-api 当前探活周期。
- CPA 判断 5h 与周额度都充足：说明更可能是网络、上游或模型错误，new-api 使用管理员配置的探活周期继续探活。
- CPA 判断 5h 或周额度不足：new-api 将 `next_probe_at` 设置为 CPA 返回的刷新时间加 120 秒，留出刷新容错。
- CPA 接口只做额度判断，不替代 new-api 的真实模型探活。到点后仍由 new-api 使用管理员选择的测活模型调用现有 channel test 流程。

## 本次 fallback 根因

线上 `channel_health_states` 只有 channel 级 `failure_count`。在旧实现中，路由过滤按当前分组阈值读取同一个全局 `failure_count`，因此不同来源的失败会互相污染：

- 业务请求失败、自动探活失败、不同模型失败都累加到同一个计数。
- 自动探活使用 `default` 分组与错误模型失败后，也可能让 `lzz_plus` 分组被判不可用。
- 当渠道被自动禁用时，`abilities.enabled` 也会被置为 false，后续请求直接跳过第一优先级分组。

本次修复新增 `channel_health_group_states`，按 `channel_id + group_name` 保存连续失败计数。路由过滤优先使用该表；旧全局状态只在 `last_group` 正好等于当前分组时作为兼容回退。这样 `default` 探活失败不会误伤 `lzz_plus`。

## 渠道测活模型

新增 `channel_health_states.probe_models` 保存管理员选择的测活模型，格式为 JSON 字符串数组。模型只能来自该渠道当前 `models`。

新增 `channel_health_probe_model_states`，按 `channel_id + model` 保存最近探活结果：

- `status=healthy`：该模型最近一次测活成功。
- `status=unhealthy`：该模型最近一次测活失败。
- `last_probe_at`、`last_success_at`、`last_failure_at`、`last_error` 用于前端展示和排查。

自动/手动探活规则：

- 按有效测活模型逐个调用现有 `testChannel`。
- 任意一个模型成功，则渠道探活成功，渠道恢复为 healthy，并清理该渠道所有分组连续失败计数。
- 所有模型失败，则渠道探活失败，仅记录一次全局探活失败，同时保留各模型失败结果。
- 未被选为测活模型的模型不展示模型级探活结果。

## API 与页面

新增管理员接口：

`POST /api/channel/:id/health/probe_models`

请求体：

```json
{
  "probe_models": ["gpt-5.5", "gpt-5.4"]
}
```

渠道状态页返回：

- `probe_models`
- `probe_model_results`

管理员可在每个渠道行内选择测活模型；普通用户只能看到自己可见分组内渠道的状态和所选模型的最近探活结果，不能手动探活或修改模型。

## 后续 CPA 落地步骤

1. CPA 新增密钥到分组的额度状态查询接口。
2. new-api 对 `cli-proxy-api:8317` 渠道接入 CPA quota-health 预检查。
3. 将 CPA 返回的 `retry_after_seconds` 映射到 `channel_health_states.next_probe_at`。
4. 为 CPA 401、500+ 三次失败、额度不足、额度充足四类路径补测试。
