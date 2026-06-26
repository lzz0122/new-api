# 2026-06-25 渠道全局健康、探活与 API key 多分组 fallback 重构设计

## 背景

当前 new-api 已支持一个 API key 配置多个分组，并支持 `fallback`、`return_error`、冷却、探活和恢复策略。但现有实现把“分组是否可用”的核心状态放在 token 级内存结构中，key 形如 `token_id + group + model`。这和线上真实故障模型不匹配：

- 渠道是否可用应是全局事实，不应因为不同 API key 各自维护一份状态而分裂。
- 单个用户请求失败不应立即禁用渠道，应按管理员配置的连续失败阈值判断，例如默认连续 3 次。
- 探活和恢复应围绕渠道本身运行，然后由分组聚合和 API key 策略消费结果。
- 前端渠道列表应能看到不可用倒计时，并支持管理员手动探活。
- 上游大量 4xx/5xx 时，需要区分“用户请求错误”“可重试瞬时错误”“渠道健康错误”，避免错误地 poison 全局渠道。

本设计目标是把“渠道健康”从 token fallback 中拆出来，建立一个可持久化、可观测、可手动干预的全局健康状态源。

## 当前代码事实

### 渠道状态与 abilities

- 渠道状态常量在 `common/constants.go`：
  - `0`: unknown
  - `1`: enabled
  - `2`: manually disabled
  - `3`: auto disabled
- 路由实际查询 `abilities`。`channels.group + channels.models + channels.status` 会派生出 `abilities` 行。
- `model.UpdateChannelStatus` 会更新 `channels.status`，并在状态变化时更新 `abilities.enabled`。
- `model.CacheUpdateChannelStatus` 在内存缓存开启时，禁用渠道会从 `group2model2channels` 删除该渠道；重新启用只改 `channelsIDM.status`，不会立刻把渠道加回 `group2model2channels`，需要后续 `InitChannelCache` 全量同步才能恢复选择。这是当前恢复延迟或看似未恢复的风险点。

### 自动禁用与自动测试

- `service.ShouldDisableChannel` 由 `AutomaticDisableChannelEnabled`、自动禁用状态码、关键字和 `types.IsChannelError` 决定。
- `controller.processChannelError` 在 relay 失败后立即调用 `service.DisableChannel`，没有连续失败计数。
- `controller.channel-test.go` 的 `testAllChannels` 自动测试也是单次失败即可禁用，单次成功即可恢复。
- `testAllChannels` 内部启动 goroutine 后立即返回；`AutomaticallyTestChannels` 紧接着打印 `automatically channel test finished`，日志会误导运维以为测试已完成。
- 当前探活没有全局倒计时状态，也没有持久化任务。进程重启后探活上下文会丢失。

### token 分组 fallback

- `service/token_group_failover.go` 维护了 `tokenGroupHealth`、`tokenGroupProbeSchedules`、`tokenGroupFailureObservations` 等内存 map。
- key 维度是 `token_id + group + model`，不是渠道全局维度。
- token 更新、删除会调用 `ClearTokenGroupHealth`，这会清掉冷却和恢复上下文。因此用户修改 API key 配置后，可能看起来“重新从第一优先级开始”。
- token group 探活会直接按 group/model 找候选渠道，并执行自定义 OpenAI-compatible probe。它不是完整 adaptor-aware。
- 当前 token 策略既在判断 fallback，又在记录健康事实，职责混在一起。

### 前端

- 新版前端状态列只展示 `channels.status` 和 `other_info.status_reason/status_time`。
- 新版前端已有普通“Test Connection”按钮，调用 `GET /api/channel/test/:id`。
- classic 前端同样只有状态、响应时间、即时测试。
- 两套前端都没有 `health` 字段、倒计时、探活中状态或手动探活接口。

### 线上抽样状态

本次尝试 SSH 到 `root@23.151.220.138` 时，远端在 SSH key 认证前关闭连接：

`kex_exchange_identification: Connection closed by remote host`

因此当前没有确认线上实时 `channels / abilities / tokens` 数据。旧 dump 只含 1 个 token 和 0 个 channel，不能代表当前线上。待 SSH 恢复后应执行本文末尾 SQL 验证真实关系。

## 已确认的问题

### P0. 渠道健康状态源错误

当前“不可用、冷却、探活”主要是 token 级状态，导致：

- 同一个渠道对 token A 已失败，对 token B 仍被当成健康。
- token 配置变更会清掉状态，导致重新从第一优先级开始尝试。
- 多进程或重启后状态丢失。
- 前端无法展示全局倒计时。

### P0. 单次错误可直接禁用渠道

relay 中只要 `ShouldDisableChannel(err)` 为 true 且 `auto_ban=1`，就会异步禁用渠道。没有连续失败阈值，也没有跨用户累计。

这会导致一个用户的一次 401/429/500 就把全局渠道打下线，尤其在上游不稳定时影响面过大。

### P1. 探活不是真正的渠道探活

token group 探活是 group/model 维度的内存调度，不是 channel 维度。探活结果只影响某个 token 的 group cooling，不直接更新渠道全局状态，也不能通知所有相关分组和 token。

### P1. 自动测试日志和状态不可观测

自动测试没有持久任务状态。`automatically channel test finished` 可能在实际测试 goroutine 完成前出现。用户观察日志时容易误判“没有探活”或“探活完成但没恢复”。

### P1. 恢复路径依赖缓存全量同步

内存缓存开启时，自动恢复更新 DB abilities，但内存 `group2model2channels` 不一定立即重新包含该渠道。新的健康服务应显式刷新该渠道的路由索引，或改为选择时读取健康状态过滤，而不是靠删除/重建切换。

### P2. 多 key 渠道需要独立处理

多 key 渠道中某个 key 401 不等价于整个渠道不可用。当前 `UpdateChannelStatus` 支持 key 级禁用，但健康设计需要明确：

- key 级认证失败优先标记 key 不健康。
- 只有所有可用 key 都失败，或错误是 base_url/provider 级别时，才标记 channel 不健康。

## 设计目标

1. 渠道健康是全局状态，以 `channel_id` 为主键持久化。
2. 连续失败阈值由管理员配置，默认 3 次。
3. 失败可来自不同用户、不同 token、不同请求。成功请求会重置连续失败计数。
4. 自动禁用后进入倒计时，到点由后台探活；管理员可手动提前探活。
5. 渠道探活成功后恢复渠道状态，刷新相关分组健康聚合，并通知受影响 token/group。
6. API key fallback 策略只消费渠道/分组健康结果，不再自行制造健康事实。
7. 人工禁用优先级最高，自动探活不得恢复人工禁用渠道。
8. 兼容现有 `channels.status`、`abilities`、`auto_ban`、状态码配置和 token group 配置。

## 推荐总体架构

```
relay error/success
  -> ChannelHealthService.RecordFailure / RecordSuccess
      -> channel_health_states
      -> on state transition update channels.status + abilities
      -> invalidate group health cache
      -> emit health event/notification

background probe scheduler / manual probe
  -> ChannelHealthService.ProbeChannel
      -> adapter-aware channel probe
      -> RecordProbeSuccess / RecordProbeFailure

channel selection
  -> abilities candidates
  -> filter channels.status enabled
  -> filter channel_health_states routable
  -> priority/weight selection

token group fallback
  -> token config decides fallback order and return_error/fallback behavior
  -> group health aggregate decides whether a group has available channels
```

## 数据模型

### `channel_health_states`

新增表，推荐模型名 `ChannelHealthState`。

字段建议：

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `channel_id` | int primary key | 对应 `channels.id` |
| `status` | string | `healthy`、`suspect`、`unhealthy`、`probing` |
| `failure_count` | int | 当前连续失败次数 |
| `failure_threshold` | int | 状态变化时使用的阈值快照，默认来自配置 |
| `last_success_at` | int64 | 最近成功请求或探活时间 |
| `last_failure_at` | int64 | 最近健康失败时间 |
| `last_probe_at` | int64 | 最近探活时间 |
| `next_probe_at` | int64 | 下一次自动探活时间，前端倒计时来源 |
| `probe_started_at` | int64 | 探活中开始时间 |
| `probe_attempts` | int | 当前 unhealthy 周期内探活次数 |
| `last_status_code` | int | 最近错误 HTTP 状态码 |
| `last_error_code` | string | 最近错误码 |
| `last_error` | text | 脱敏后的最近错误 |
| `last_model` | string | 最近触发健康失败的原始模型 |
| `last_group` | string | 最近触发健康失败时使用的分组 |
| `last_token_id` | int | 只存 id 用于审计，不作为健康 key |
| `last_request_id` | string | 方便查日志 |
| `manual_probe_requested_at` | int64 | 最近手动探活时间 |
| `updated_at` | int64 | 更新时间 |

索引：

- primary key: `channel_id`
- `idx_channel_health_status_next_probe(status, next_probe_at)`
- `idx_channel_health_last_failure(last_failure_at)`

### `channel_health_events`

可选但推荐，用于审计和通知。保留最近 N 天即可。

字段：

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `id` | int primary key | 自增 |
| `channel_id` | int | 渠道 |
| `event_type` | string | `failure_recorded`、`marked_unhealthy`、`probe_success`、`probe_failure`、`recovered`、`manual_probe` |
| `from_status` | string | 旧状态 |
| `to_status` | string | 新状态 |
| `group` | string | 相关 group |
| `model` | string | 相关 model |
| `status_code` | int | HTTP 状态码 |
| `error_code` | string | 错误码 |
| `message` | text | 脱敏摘要 |
| `created_at` | int64 | 时间 |

### 多 key 后续表

第一阶段可继续复用 `ChannelInfo.MultiKeyStatusList`。后续建议新增 `channel_key_health_states`：

- `channel_id`
- `key_index`
- `status`
- `failure_count`
- `last_error`
- `next_probe_at`

这样可以避免把 key 级状态藏在 JSON 里，便于查询和前端展示。

## 管理员配置

新增配置模块建议为 `channel_health_setting`，不要继续把所有字段塞进散落的 `common` 变量。当前实现保留全局配置作为默认值，同时把两类高频运维配置放到渠道状态页：

- 每个渠道的自动测活周期在渠道状态页该渠道行内配置，字段为 `channel_health_states.probe_interval_seconds`。`null` 表示继承全局默认；当前 UI 保存具体秒数。`<0` 表示该渠道不自动测活，只允许管理员手动探活。
- 每个定价分组的连续错误阈值在渠道状态页分组标题区配置，字段为 `channel_health_group_settings.failure_threshold`。默认继承全局 `channel_health_setting.failure_threshold`。`<=0` 表示该分组下渠道即使连续错误也不因健康系统判为不可用，可作为兜底分组。
- 同一渠道挂在多个分组时，路由过滤按“当前分组阈值”判断；状态记录按该渠道相关分组里的最小正数阈值进入 `unhealthy/probing` 队列。存在 `<=0` 兜底分组时，不会因为其它分组不可用而在该兜底分组里被过滤。
- 自动物理禁用 `channels.status=3` 只应在该渠道对所有相关分组都不可用时触发，避免把兜底分组也打穿。

默认值建议：

| 配置 | 默认值 | 说明 |
| --- | --- | --- |
| `enabled` | true | 是否启用全局健康记录 |
| `auto_disable_enabled` | 复用 `AutomaticDisableChannelEnabled` | 是否达到阈值后自动下线 |
| `failure_threshold` | 3 | 连续健康失败阈值 |
| `failure_window_seconds` | 600 | 可选，超过窗口后失败计数重置 |
| `probe_cooldown_seconds` | 600 | 标记 unhealthy 后首次探活倒计时 |
| `probe_timeout_seconds` | 10 | 单次探活 timeout |
| `probe_success_threshold` | 1 | 连续成功几次后恢复 |
| `max_probe_concurrency` | 3 | 后台探活并发 |
| `manual_probe_cooldown_seconds` | 10 | 手动探活防抖 |
| `record_success_resets_failures` | true | 正常成功请求是否重置失败计数 |

复用现有配置：

- `AutomaticDisableStatusCodes`: 哪些状态码可计入健康失败。
- `AutomaticDisableKeywords`: 哪些错误关键字可计入健康失败。
- `AutomaticRetryStatusCodes`: 请求级 retry/fallback 状态码。
- `ChannelDisableThreshold`: 可保留为响应时间健康失败条件，但应纳入连续失败计数，不再单次禁用。

## 错误分类规则

必须把三类决策拆开：

1. `ShouldRetryRequest`: 当前请求是否可以重试。
2. `ShouldFailoverTokenGroup`: 当前 API key 是否应该尝试下一个 group。
3. `ShouldRecordChannelHealthFailure`: 是否计入全局渠道健康失败。

建议规则：

- 用户请求错误不计入渠道健康，例如 malformed JSON、模型被 token 限制、余额不足、敏感词、明确 `skip_retry`。
- 400 默认不计入健康失败，除非错误码或关键字明确是上游渠道故障。
- 401/403 可计入 key/channel 健康，但通过阈值控制，默认也需要连续 3 次。
- 429 可计入健康失败，也可只计入容量降级，是否自动下线由管理员状态码配置决定。
- 5xx、网络错误、EOF、连接超时、DNS/TLS 错误可计入健康失败。
- 流式响应已经写出字节后的错误可记录健康失败，但不能透明 fallback。

## 状态机

### 正常请求成功

条件：上游请求完整成功，HTTP 小于 400，且没有 relay 层错误。

动作：

1. `RecordChannelSuccess(channel_id, group, model, token_id, request_id)`
2. 如果 `status=healthy/suspect`，清零 `failure_count`。
3. 如果 `status=unhealthy`，默认不因真实请求成功直接恢复，因为 unhealthy 渠道不应被普通路由选中；除非管理员手动指定渠道或探活成功。

### 正常请求失败

条件：错误命中 `ShouldRecordChannelHealthFailure`。

动作：

1. 对 `channel_health_states` 使用 DB 事务或行锁。
2. 若当前 `channels.status=2` 人工禁用，记录事件但不改变健康状态。
3. 若超过 `failure_window_seconds`，先重置计数。
4. `failure_count += 1`，状态为 `suspect`。
5. 如果 `failure_count < threshold`，保持 `channels.status=1`，路由仍可选，但可在未来做降权。
6. 如果 `failure_count >= threshold` 且 `auto_ban=1` 且自动禁用开启：
   - `status=unhealthy`
   - `next_probe_at=now + probe_cooldown_seconds`
   - `channels.status=3`
   - `abilities.enabled=false`
   - 刷新/失效路由缓存
   - 通知 root/admin
   - 通知相关 group 聚合缓存失效

### 自动探活到点

后台 scheduler 只处理：

- `channel_health_states.status in ('unhealthy')`
- `next_probe_at <= now`
- `channels.status=3`
- `channels.status != 2`

动作：

1. 用 Redis/DB 锁抢占 `channel_id`，避免多节点重复探活。
2. 设置 `status=probing`、`probe_started_at=now`。
3. 调用 adaptor-aware probe。
4. 成功：
   - `status=healthy`
   - `failure_count=0`
   - `next_probe_at=0`
   - `channels.status=1`
   - `abilities.enabled=true`
   - 立即刷新路由缓存
   - 通知相关 group/token。
5. 失败：
   - `status=unhealthy`
   - `probe_attempts += 1`
   - `next_probe_at=now + probe_cooldown_seconds`，后续可做指数退避但要有上限
   - 写 `last_error`
   - 不恢复 `channels.status`。

### 手动探活

管理员点击前端倒计时或健康按钮后：

1. `POST /api/channel/:id/health/probe`
2. 如果人工禁用，返回 `success=false`，提示需要先手动启用或使用明确的 force 参数。
3. 如果已有探活中，返回当前状态。
4. 启动一次同步探活或短后台任务。
5. 前端刷新该行健康状态。

第一阶段推荐同步执行，受 `probe_timeout_seconds` 限制，便于实现和反馈。后续可改成 async job。

## 探活实现

推荐新建 `service/channel_health.go`，暴露：

- `RecordChannelSuccess(ctx, event)`
- `RecordChannelFailure(ctx, event, err)`
- `ProbeChannel(ctx, channelID, options)`
- `GetChannelHealth(channelID)`
- `ListChannelHealth(channelIDs)`
- `GetGroupHealth(group, model)`

探活请求来源：

1. 优先用 `channel.TestModel`。
2. 如果为空，用最近失败模型 `last_model`。
3. 如果仍为空，用渠道 models 的第一个模型。
4. endpoint 优先沿用最近失败请求类型，否则用现有 `testChannel` 的自动 endpoint 选择。

关键要求：

- 不能继续写死 `/v1/chat/completions` 和 `Authorization: Bearer`。
- 第一阶段可复用 `controller/channel-test.go` 的 `testChannel` 内部逻辑，但建议把它下沉到 service，避免 controller 互相调用。
- probe 不应计费，也不应写普通用户消费日志。
- probe 错误也要走同一套状态码和关键字脱敏。

## 分组健康聚合

分组健康不是独立事实源，而是从 `abilities + channels + channel_health_states` 聚合。

推荐返回结构：

```json
{
  "group": "lzz_plus",
  "model": "gpt-5.5",
  "total_channels": 3,
  "routable_channels": 2,
  "suspect_channels": 1,
  "unhealthy_channels": 1,
  "manual_disabled_channels": 0,
  "next_probe_at": 1780000000,
  "channels": [37, 49]
}
```

路由判断：

- `channels.status=1`
- `abilities.enabled=true`
- `channel_health_states.status` 为空、`healthy` 或 `suspect`
- `unhealthy/probing` 不可路由
- `manual disabled` 永不自动路由

缓存策略：

- `GetGroupHealth(group, model)` 可使用内存 TTL，例如 5 秒。
- `RecordChannelFailure` 达到状态迁移、`RecordChannelSuccess`、`ProbeChannel`、渠道编辑、渠道启停、abilities 修复后，主动失效相关 group/model cache。
- 相关 group 可从 `channels.group` 字符串和 `abilities` 表反查。

## token/API key fallback 改造

现有 token 配置继续保留：

- group 顺序
- failover strategy
- timeout
- cooldown
- recovery strategy
- detection strategy

但职责调整：

- token 配置只决定“遇到当前 group 不可用时是否切换、按什么顺序切换、是否 sticky”。
- `tokenGroupHealth` 不再作为健康事实源。
- `IsTokenGroupModelCooling` 应改为读取 group health 和 token 策略，或者只保留 token 的 sticky/preferred 偏好。
- `MarkTokenGroupFailure` 不再单独启动 group probe，而是调用 `RecordChannelFailure`，然后根据 group 聚合结果判断当前 group 是否还有可用渠道。
- `HasAvailableLaterTokenGroup` 不再只看 token 内存 cooling，而是看后续 group 的 `GetGroupHealth(group, model).routable_channels > 0`。

请求失败后的新流程：

1. relay 返回错误。
2. `RecordChannelFailure` 记录全局渠道失败。
3. 当前请求内继续排除已失败 channel，避免同一次请求重复打同一渠道。
4. 如果当前 group 仍有其他 routable channel，按 token 策略决定继续本组 retry 还是切组。
5. 如果当前 group 无 routable channel：
   - `fallback` 策略：尝试下一优先级 group。
   - `return_error` 策略：返回错误摘要。
6. 所有 group 均不可用时，返回 503，并带结构化摘要。

用户返回信息建议：

- 不为了等待探活而长时间挂住用户请求。
- 当前请求只做有限 retry/fallback。
- 若最终失败，返回最后错误加调度摘要，例如：
  - `lzz_plus 分组暂无可用渠道，已切换白嫖失败；白嫖下一次探活约 08:32 后；拼车返回 503 auth_not_found`
- 前端/管理端通过渠道健康倒计时观察恢复，不把普通用户请求变成长轮询。

## API 设计

### 渠道列表返回

在 `GetAllChannels`、`SearchChannels`、`GetChannel` 的 channel JSON 中增加可选字段：

```json
{
  "health": {
    "status": "unhealthy",
    "failure_count": 3,
    "failure_threshold": 3,
    "last_status_code": 503,
    "last_error_code": "bad_response_status_code",
    "last_error": "upstream 503 auth_not_found",
    "last_failure_at": 1780000000,
    "last_success_at": 1779999000,
    "next_probe_at": 1780000600,
    "probe_started_at": 0,
    "affected_groups": ["lzz_plus", "jhl"],
    "last_model": "gpt-5.5"
  }
}
```

为避免列表过重，可先在列表中只返回摘要，详情页再取完整事件。

### 手动探活

`POST /api/channel/:id/health/probe`

参数：

```json
{
  "model": "optional",
  "force": false
}
```

返回当前 health。

### 批量健康查询

`GET /api/channel/health?ids=1,2,3`

用于前端倒计时轻量刷新，不必每秒拉完整渠道列表。倒计时本身在浏览器本地计算，每 15 到 30 秒刷新一次服务端状态即可。

### 分组健康

`GET /api/channel/health/groups?group=lzz_plus&model=gpt-5.5`

管理员调试 token fallback 时使用。

### 事件

`GET /api/channel/:id/health/events`

管理员详情页使用，默认最近 50 条。

## 前端设计

新版前端：

- 在渠道状态列或新增健康列展示：
  - `Healthy`
  - `Suspect 1/3`
  - `Unhealthy 08:32`
  - `Probing`
  - `Manual disabled`
- `Unhealthy` 的倒计时是按钮，点击触发手动探活。
- tooltip 展示最近错误、状态码、最近失败时间、影响分组。
- 探活中按钮禁用并显示 spinner。
- 操作菜单增加 `Probe now`。
- 普通 `Test Connection` 保持原语义，不自动改变全局健康，或仅通过明确参数 `record_health=true` 才写健康。

classic 前端：

- 状态列 tooltip 扩展健康信息。
- 操作栏增加手动探活按钮。
- 保持已有即时测试弹窗。

系统设置：

- 在 Monitoring & Alerts 增加全局渠道健康设置：
  - 连续失败阈值
  - 探活冷却秒数
  - 探活 timeout
  - 恢复成功阈值
  - 最大探活并发

## 通知设计

状态迁移时通知：

- `suspect` 不通知，避免噪音。
- `marked_unhealthy` 通知 root/admin。
- `recovered` 通知 root/admin。
- 手动探活成功/失败可只在前端 toast，不一定发系统通知。

相关 group 计算：

- 从 `abilities` 查 `channel_id` 对应 group/model。
- 从 `channels.group` 取完整 group 字符串作为兜底。

相关 token 计算：

- `tokens.group = group`
- 或 `tokens.group_config LIKE '%group%'` 作为初版近似。
- 后续可维护 token group 反向索引，避免 LIKE。

通知 token 的含义不是逐个推送给终端用户，而是：

- 清理 token 级 sticky/preferred cache。
- 让下次请求重新读取 group health。
- 管理端可以看到受影响 token 数。

## 迁移与兼容

数据库变更：

- 新增 `channel_health_states`。
- 可选新增 `channel_health_events`。
- 不删除、不重写 `channels`、`abilities`、`tokens` 现有数据。
- `channels.status=3` 的现有自动禁用渠道在迁移后 seed 为 `unhealthy`，`failure_count=failure_threshold`，`next_probe_at=now + probe_cooldown_seconds`。
- `channels.status=1` 且无 health 记录视为 `healthy`。
- `channels.status=2` 视为人工禁用，health 不自动恢复。

代码迁移点：

- `model/main.go` 普通 `migrateDB` 和 `migrateDBFast` 都要注册新模型。
- 若使用独立 LOG_DB，不建议把 health 状态放 LOG_DB；它参与路由，应放主 DB。
- 增加 `InitChannelHealthState` 或 lazy create，避免上线后无记录时误判。

配置兼容：

- 保留 `AutomaticDisableChannelEnabled`，映射为是否达到阈值后自动下线。
- 保留 `AutomaticEnableChannelEnabled`，但恢复应由新 probe 成功驱动；旧自动测试单次成功恢复应废弃或改为调用 `RecordProbeSuccess`。
- 保留 `auto_ban` 字段，作为单渠道是否允许自动下线。
- token group `cooldown_seconds` 可继续作为 token 策略冷却，但全局渠道 `next_probe_at` 才是恢复倒计时事实。

部署风险：

- 新增表是非破坏性变更，但仍需按部署 skill 先备份 PostgreSQL。
- 上线初期建议先 observer mode：只记录 health，不改变路由，确认分类准确后再启用自动下线。

## 分阶段落地计划

### 阶段 1：观察模式

- 新增 health 表和 service。
- relay 成功/失败写 health 记录。
- 不改变 `channels.status`，不改变路由。
- 前端只展示健康状态和失败计数。
- 测试：连续失败计数、成功重置、错误分类。

### 阶段 2：阈值自动下线

- 达到阈值后调用统一 `MarkChannelUnhealthy`。
- 更新 `channels.status=3` 和 `abilities.enabled=false`。
- 修复内存缓存重新启用不回填的问题，或改为路由时动态过滤 health。
- 测试：第 1、2 次失败不下线，第 3 次下线；人工禁用不被自动恢复。

### 阶段 3：后台探活和手动探活

- 新增 scheduler。
- 新增 `POST /api/channel/:id/health/probe`。
- 前端显示倒计时和 Probe now。
- 测试：到点探活、手动探活、并发锁、探活失败延后。

### 阶段 4：group health 驱动 token fallback

- `CacheGetRandomSatisfiedChannel` 过滤 unhealthy/probing 渠道。
- `HasAvailableLaterTokenGroup` 改读 group health。
- `MarkTokenGroupFailure` 降级为策略和摘要记录。
- 删除或废弃 token group probe scheduler。
- 测试：`lzz_plus -> 白嫖 -> 拼车` 顺序稳定，第二组可用时不会跳过；token 配置修改不清空全局渠道健康。

### 阶段 5：多 key 精细化

- key 级 health 表或结构化存储。
- 401/403 先禁用 key，所有 key 不可用才禁用 channel。
- 前端 key 管理弹窗展示 key 健康和倒计时。

## 测试计划

后端单元测试：

- `RecordChannelFailure` 连续 3 次才 unhealthy。
- 成功请求清零 suspect failure count。
- 失败窗口过期后重置计数。
- 人工禁用渠道不被 `ProbeChannel` 自动启用。
- unhealthy 渠道到点探活成功后恢复 `channels.status` 和 `abilities.enabled`。
- 探活失败更新 `next_probe_at`。
- 并发 10 个失败事件不会丢计数或重复通知。
- `GetGroupHealth` 正确聚合 healthy/suspect/unhealthy/manual disabled。

relay 集成测试：

- 上游连续两次 503 不禁用渠道，第三次禁用。
- token 多分组中，第一组无可用渠道时进入第二组，不跳第三组。
- `RetryTimes=0` 不阻止跨 group fallback。
- 400 invalid request 不计入渠道健康。
- 401 按配置和阈值计入。
- 流式写出后错误只记录健康，不尝试透明 fallback。

前端测试：

- 渠道列表显示倒计时。
- 点击倒计时触发手动探活。
- 探活中按钮禁用。
- 健康状态刷新不导致整表布局跳动。

## 待线上验证 SQL

SSH 恢复后执行，注意不要查询或打印 key 明文。

```sql
select id, name, status, "group", models, auto_ban, priority, weight, test_time, response_time
from channels
order by id;

select "group", model, count(*) as ability_count, count(distinct channel_id) as channel_count
from abilities
where enabled = true
group by "group", model
order by "group", model;

select u.username, t.id, t.name, t.status, t."group", t.cross_group_retry, t.group_config
from tokens t
join users u on u.id = t.user_id
where u.username in ('lzz')
order by t.id;

select key, value
from options
where key in (
  'AutomaticDisableChannelEnabled',
  'AutomaticEnableChannelEnabled',
  'AutomaticDisableStatusCodes',
  'AutomaticRetryStatusCodes',
  'RetryTimes',
  'ChannelDisableThreshold',
  'monitor_setting.auto_test_channel_enabled',
  'monitor_setting.auto_test_channel_minutes'
)
order by key;

select id, name, status, other_info
from channels
where status <> 1
order by id;
```

## 推荐结论

推荐不要继续扩大 token 级 fallback map 的职责。应新增全局 `channel_health_states`，让 relay、自动探活、手动探活都写同一个渠道健康状态；group health 从渠道健康聚合；API key 只保留策略决策。

这样能解决当前三类核心问题：

- fallback 不再因为 token 级冷却和配置修改产生跳组。
- 探活倒计时和恢复结果可见、可手动触发、可持久化。
- 渠道禁用从“单次错误”变为“管理员配置的连续失败阈值”，更适合上游频繁 4xx/5xx 的生产环境。
