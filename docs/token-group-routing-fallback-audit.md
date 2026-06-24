# API Key 多分组、渠道转发与重试审计

本文档基于当前代码阅读，重点覆盖一次用户请求如何被 API key 分组、如何选渠道、如何 fallback，以及在“API key 对应多个分组”和“上游大量 4xx/5xx”场景下的设计风险。

## 总体结论

当前主链路是：

`router/relay-router.go` -> `middleware.TokenAuth` -> `middleware.ModelRequestRateLimit` -> `middleware.Distribute` -> `controller.Relay` -> `relay/*Handler` -> `relay/channel/*Adaptor` -> `service.RelayErrorHandler` -> `controller.Relay` 重试判断。

多分组 API key 的核心数据在 `model.Token.GroupConfig`，兼容字段 `Token.Group` 会被 `SetGroupConfig` 同步成第一个分组。真实转发时：

- `TokenAuth` 校验 token 和用户可用分组，并写入 `ContextKeyTokenGroupConfig`、`ContextKeyUsingGroup`。
- `Distribute` 初选渠道；多分组 token 会禁用 channel affinity，并从 `CacheGetRandomSatisfiedChannel` 进入多分组选择逻辑。
- `controller.Relay` 第一次使用 `Distribute` 选好的渠道；失败后进入自己的重试循环，后续每次重新调用 `CacheGetRandomSatisfiedChannel`。
- 上游错误经 `RelayErrorHandler` 包装为 `NewAPIError`，再由 `ShouldTokenGroupFailover` 和 `shouldRetry` 分别决定跨分组 fallback 与同分组/同模型重试。

当前代码已经有多分组 fallback 框架。本次已修复若干高优先级问题；仍需持续关注恢复探活不适配非 OpenAI-compatible 渠道、流式响应无法透明重试、模型发现/限流按主分组语义等后续设计问题。

## 本次修复记录

已在代码中修复：

- `specific_channel_id` 不再被普通 retry 或多分组 fallback 绕过。
- 多分组 fallback 改为复用站点自动重试状态码配置，不再默认对所有 `400-599` fallback。
- 多分组 item 级 `timeout_seconds` 会在选中该 group 时写入请求上下文，真实上游请求会使用该 timeout。
- 同一次请求内会记录已失败的 `group + model + channel`，后续选渠道会排除这些 channel；如果候选全部失败，不再退回到已失败候选，而是让选择器继续下一个优先级或分组。
- retry 切换到新 group/channel 后会重新计算价格，并通过 `BillingSession.Reserve` 补足预扣；如果第一次命中免费分组、后续 fallback 到付费分组，会创建新的预扣会话。
- token group 冷却、探活调度和探活成功后的 preferred channels 已按 `token + group + model` 隔离，避免某个模型失败拖死整个分组的其他模型。
- 恢复探活会记录原始请求路径，`/v1/responses` 请求优先用 Responses 轻量 payload 探活，不再固定使用 chat completions。
- token 新增、更新、删除后会清理该 token 的内存健康状态，避免 group config 修改后沿用旧冷却/探活缓存。
- 多分组 token 的组间 fallback 已与普通同组 retry 解耦；`RetryTimes=0` 只表示不在同一组/同一渠道继续重试，不能阻止 `lzz_plus -> 白嫖 -> 拼车` 这类按 token 配置的跨组 fallback。

仍建议后续单独处理：

- 流式响应一旦向客户端写出字节，仍不能透明 fallback。
- 恢复探活仍不是完整 adaptor-aware；非 OpenAI-compatible 渠道仍需要更系统的 channel test/probe 抽象。
- `/v1/models` 与模型限流仍主要按主分组语义处理。
- Task/Midjourney 尚未完整复用文本 relay 的多分组健康状态与跨组 fallback 编排。

## 2026-06-22 线上现象复盘：token 15 `any`

用户报告的本地时间 `2026-06-21 22:49:46` 对应服务器/数据库时间约为 `2026-06-22 13:49:46`。线上 token 配置为：

`lzz_plus(order=1) -> 白嫖(order=2) -> 拼车(order=3)`

关键日志：

- `10:50:42`：token 15 在 `白嫖` 渠道 38 上请求 `gpt-5.5` 返回 500 “当前模型负载已经达到上限”，随后白嫖被打入 10 分钟冷却，并开始探活。
- `10:50` 到 `14:41`：`白嫖` 每 10 分钟探活一次，均失败，日志只显示“随机探活未发现可用渠道”。
- `13:49:46`：token 15 在 `lzz_plus` 渠道 37 上请求 `gpt-5.5` 返回 502 `network_eof`，随后调度 lzz_plus 探活。
- 同一请求 `20260622054941402321758268d9d6E0WT5uPY` 随后在 `拼车` 渠道 35 成功；因为 `白嫖` 当时仍处于冷却，选择器按当前代码跳过了第二优先级。
- `14:19:48`：`lzz_plus` 探活恢复，available channel 为 `[37]`。
- `14:38:17` 之后 token 15 再次可见的请求已经回到 `lzz_plus`。这更像是探活已恢复后的下一次请求，而不是修改 API key 配置本身触发恢复。
- `15:19:38` 起，多次请求在 `lzz_plus` 渠道 37 返回 503：`auth_not_found: no auth available (providers=codex, model=gpt-5.5)`。每次请求都直接向用户返回 503，没有 `token group probe scheduled`，也没有 `重试：37->...` 日志。
- 线上 `options` 表没有 `RetryTimes` 记录，代码默认值为 `0`。旧逻辑先用普通 retry budget 判断，导致 `RetryTimes=0` 时还没进入 `MarkTokenGroupFailure` 就停止，因此不会冷却 `lzz_plus`、不会探活、不会切到后续分组。

结论：

- “lzz_plus 失败后直接去拼车”不是排序丢失，而是第二组 `白嫖` 已经处于冷却状态。
- 真正的问题在于 `白嫖` 过早、过久地被判不可用：默认 `failure_detection_strategy=one` 使单个渠道一次 500 就冷却整个分组；同时旧探活固定 chat completions 且失败日志不含具体渠道错误，导致恢复原因不可见。
- `lzz_plus` 探活从调度到恢复的日志间隔约 30 分钟，中间没有失败日志。代码层面已增强探活端点和错误输出；若再次出现该间隔，需要继续检查 goroutine 调度、HTTP client timeout 和上游长连接行为。
- 15:19 的 503 不 fallback 是独立 bug：普通 retry 次数被错误地作为组间 fallback 的前置条件。当前已修复为：上游 429/5xx 等可 failover 错误会先按 token 多分组配置标记当前 group 失败并切换后续 group；`RetryTimes=0` 仅禁止同组普通重试。

## 请求分组流程

### 1. API key 解析与 token 认证

入口在 `middleware/auth.go:276` 的 `TokenAuth`。

流程：

1. 兼容 OpenAI、Claude、Gemini、Midjourney 等不同 key 位置，把 key 统一放到 `Authorization`。
2. 去掉 `Bearer ` 和 `sk-` 前缀，按 `-` 切分。`parts[0]` 是 token key；`parts[1]` 在管理员 token 下可作为指定渠道 ID。
3. `model.ValidateUserToken` 校验 token 状态、过期时间、剩余额度。
4. `model.GetUserCache(token.UserId)` 写入用户上下文，包括用户原始分组 `ContextKeyUserGroup`。
5. `token.ParseGroupConfig()` 解析多分组配置。
6. 逐个校验 `GroupConfig.Groups` 中的分组是否在用户可用分组内，且非 `auto` 分组必须存在倍率配置。
7. 若 `GroupConfig.Groups` 非空，`usingGroup` 会先设为第一个 group；否则用 `token.Group` 或用户分组。
8. `SetupContextForToken` 写入 token id/key/quota/model limit/token group 等上下文。

关键约束：

- `model.Token.SetGroupConfig` 会把 `Token.Group` 设置为排序后的第一个分组，见 `model/token.go:232`。
- 因为 `relay/common.GenRelayInfo` 读取的是 `ContextKeyTokenGroup`，而 `SetupContextForToken` 写的是 `token.Group`，所以所有新增/更新 token 的路径都必须经过 `SetGroupConfig`，否则多分组 token 的主分组可能不一致。

## 渠道选择流程

### 2. 分发中间件初选渠道

入口在 `middleware/distributor.go:32` 的 `Distribute`。

核心行为：

1. 解析请求模型名。
2. 校验 token 的模型访问限制。
3. 如果是 playground，允许请求体里的 `group` 覆盖当前使用分组。
4. 多分组 token 禁用 channel affinity：
   - `GetTokenGroupConfigFromContext` 返回多个 group 时，`useChannelAffinity = false`。
5. 如果没有 affinity 命中，则调用 `service.CacheGetRandomSatisfiedChannel`。
6. `SetupContextForSelectedChannel` 把渠道 id、类型、base url、key、模型映射、状态码映射等写入 context。

### 3. 多分组选择算法

入口在 `service/channel_select.go:83` 的 `CacheGetRandomSatisfiedChannel`。

多分组 token 的条件是：

```go
cfg.Groups > 1 && param.TokenGroup != "auto"
```

行为：

- 按 `cfg.Groups` 顺序从当前 `ContextKeyTokenGroupIndex` 开始查。
- 每个 group 内按 `retry` 映射到渠道优先级。
- 如果 group 正在 cooling，则跳过，并记录失败摘要。
- 如果 group 有恢复探活得到的 preferred channels，优先从这些渠道中选。
- 找到 channel 后写入 `ContextKeyUsingGroup` 和 `ContextKeyTokenGroupIndex`。
- 如果 `priorityRetry >= common.RetryTimes`，为下一次 retry 准备切到下一个 group。

`auto` 分组走另一套逻辑：使用 `setting.GetAutoGroups()` 与用户可用分组求交集；只有 `token.CrossGroupRetry` 打开时，才会在 priority 用尽后跨 auto group。

## 转发与错误重试流程

### 4. controller.Relay 主循环

入口在 `controller/relay.go:68` 的 `Relay`。

关键阶段：

1. 解析并校验请求。
2. `GenRelayInfo` 生成 `RelayInfo`。
3. 敏感词、token 估算、价格计算。
4. `PreConsumeBilling` 预扣费。
5. 进入 retry loop：`for retry <= common.RetryTimes`。
6. 第一次请求直接复用 `Distribute` 选好的渠道；后续请求调用 `getChannel` 重新选渠道。
7. 每次请求前重置请求体。
8. 调用对应 relay handler。
9. 成功则 `MarkTokenGroupSuccess` 并返回。
10. 失败则记录渠道错误、可能自动禁用渠道，然后判断多分组 failover 或普通 retry。

`getChannel` 位于 `controller/relay.go:326`。第一次 `info.ChannelMeta == nil` 时返回 context 中已有渠道；重试时才会重新选。

### 5. 错误分类与状态码策略

普通 retry 判断在 `controller/relay.go:365` 的 `shouldRetry`：

- `skipRetry` 直接不重试。
- `specific_channel_id` 不重试。
- 2xx 不重试。
- 100 以下或 599 以上会重试。
- `AutomaticRetryStatusCodes` 命中才重试。
- 504、524 被硬编码为永不重试。
- `ErrorCodeBadResponseBody` 被硬编码为永不重试。

状态码默认配置在 `setting/operation_setting/status_code_ranges.go:21`：

- 默认重试：1xx、3xx、401-407、409-499、500-503、505-523、525-599。
- 默认不重试：400、408、504、524、2xx。

多分组 failover 判断在 `service/token_group_failover.go:443` 的 `ShouldTokenGroupFailover`：

- `channel error`、状态码为 0、非标准 HTTP 状态码可以 failover。
- `skipRetry`、`ErrorCodeBadResponseBody`、504、524 等 always-skip 错误不 failover。
- 标准 HTTP 状态码复用 `operation_setting.ShouldRetryByStatusCode`，因此默认不再对 400、408、504、524 fallback。

当前已与普通 retry 的状态码策略保持一致，但后续仍建议拆分 `retry_status_codes` 与 `failover_status_codes`，让同组 retry 和跨组 fallback 可以独立配置。

## 已发现风险与可能 bug

### P0. 已修复：多分组 fallback 会绕过 `specific_channel_id`

修复前，普通 `shouldRetry` 会阻止指定渠道请求重试，但多分组 failover 分支发生在 `shouldRetry` 之前，且没有检查 `specific_channel_id`。

影响：

- 管理员用 `sk-token-channelId` 指定渠道时，第一次请求走指定渠道。
- 如果该渠道返回 4xx/5xx，`ShouldTokenGroupFailover` 可能进入多分组 fallback。
- 下一轮 `getChannel` 会调用随机渠道选择，绕开指定渠道。

建议：

- 当前已在普通 retry 和多分组 failover 前统一检查 `specific_channel_id`。
- 如果以后要支持指定渠道内多 key 重试，应单独设计“锁定 channel，只换 key”的逻辑。

### P0. 已修复：fallback 到更贵分组时，预扣费不一定足够

修复前，预扣费发生在第一次转发前，价格来自初始 `UsingGroup`，见 `relay/helper/price.go:67` 与 `service/billing.go:19`。

重试切换 group 时，`getChannel` 会更新 `info.PriceData.GroupRatioInfo`，但不会调用 `BillingSession.Reserve` 补充预留。

影响：

- 钱包扣费允许余额变负，见 `model/user.go:909`。
- 订阅扣费会阻止超过额度，见 `model/subscription.go:1297`。
- 非流式响应通常已经在 `OpenaiHandler` 写给用户后才结算；`PostTextConsumeQuota` 只记录 `SettleBilling` 错误，不会改变用户响应，见 `service/text_quota.go:322`。

建议：

- 当前已在 retry 选出新 group/channel 后重新计算预估额度。
- 如果新预估额度大于当前预扣，会调用 `BillingSession.Reserve`。
- 如果第一次命中免费分组、后续 fallback 到付费分组，会创建新的预扣会话。
- Reserve 失败时不会请求上游，会直接返回额度不足。

### P1. 已修复：多分组 failover 与全局重试次数/状态码规则不一致

修复前，`ShouldTokenGroupFailover` 对 400-599 都返回 true，且在 `shouldRetry` 前执行。

修复结果：

- 多分组 fallback 复用站点自动重试状态码配置，但不再依赖普通 retry budget。
- `RetryTimes=0` 时，普通同组 retry 会停止；多分组 token 仍会对可 failover 错误执行组间 fallback。
- 400 默认普通 retry 不会重试，多分组也不会 fallback。
- 429、500 等仍按站点自动重试状态码配置处理。
- `specific_channel_id` 与显式 `skip_retry` 错误仍会阻止多分组 fallback。

建议：

- 拆分两个配置：`retry_status_codes` 和 `failover_status_codes`。
- 默认 failover 建议：408、409、425、429、500-599，排除 400、401、403、404、422。
- 对 400 只允许通过错误码白名单或用户显式 opt-in，例如 `invalid_request_error` 不重试，`model_not_available` 可 fallback。

### P1. 已修复：per-group `timeout_seconds` 没有作用到真实上游请求

修复前，`ContextKeyTokenGroupTimeout` 在 `TokenAuth` 中只写入 `tokenGroupConfig.TimeoutSeconds`，真实请求在 `relay/channel/api_request.go:509` 用该值设置 `ResponseHeaderTimeout`。

但 `TokenGroupItem.TimeoutSeconds` 只在 `effectiveTokenGroupItem` 和恢复探活里使用，没有在选择某个 group 后写回 context。

影响：

- 配了不同 group timeout 时，真实请求仍使用全局 group config timeout。
- 用户以为 fallback group 有短 timeout，实际可能长时间等待。

建议：

- 当前已在 `CacheGetRandomSatisfiedChannel` 选中 group 后，计算 `effectiveTokenGroupItem(cfg, group).TimeoutSeconds` 并写入 `ContextKeyTokenGroupTimeout`。
- 如果 item 未设置，会回退到 cfg-level timeout。

### P1. 部分修复：恢复探活协议过窄

恢复探活在 `service/token_group_failover.go:704` 的 `doChannelProbe`：

- 旧逻辑 URL 固定为 `/v1/chat/completions`，请求体固定为 OpenAI chat 格式。
- 当前已记录原始请求路径；`/v1/responses` 和 `/v1/responses/compact` 请求会优先使用 Responses 轻量 payload 探活。
- 鉴权仍固定为 `Authorization: Bearer <key>`，仍不是完整 adaptor-aware。

影响：

- Gemini、Claude、Vertex、AWS Bedrock、Dify、Coze 等非 OpenAI-compatible 渠道可能被误判为未恢复。
- 对 OpenAI-compatible 但 endpoint 语义不同的渠道，旧逻辑可能即便真实业务请求可用也探活失败，导致 group 长时间 cooling；当前已缓解 Responses 请求场景。

建议：

- 给 channel adaptor 增加 `Probe` 能力，或者复用现有 channel test 逻辑。
- 探活请求必须使用渠道类型对应的 URL、鉴权、模型映射和最小请求体。
- 如果不能 adaptor-aware，就只对 OpenAI-compatible 渠道启用自动探活；其他渠道 cooling 到期后试运行真实请求。

### P1. 流式响应无法透明 fallback

流式请求一旦向客户端写出 header 或 body，就不能再改成 JSON 错误，也不能透明切渠道继续同一个响应。

当前 `StreamScannerHandler` 主要记录 `StreamStatus`，中途异常通常不会回到外层 retry。相关入口在 `relay/helper/stream_scanner.go:43` 和 `relay/channel/openai/relay-openai.go:103`。

还要注意：`doRequest` 在 stream 模式下可能启动 ping keepalive。如果 ping 在上游返回错误前写给客户端，就已经提交了响应；随后再 retry 或返回 JSON 都会破坏协议。

建议：

- 透明 retry 只允许发生在没有向客户端写出任何字节之前。
- 如果要给用户“正在重试”的信息，只能选择非兼容模式：例如异步任务、SSE debug 模式、管理端事件流。
- OpenAI-compatible stream 模式下，建议只在最终选定上游并收到成功响应头后再开始向客户端写数据。

### P1. 已修复：同优先级多渠道可能抽回刚失败的渠道

修复前，普通 retry 通过 retry index 映射到下一个 priority。但如果某个 group/model 只有一个 priority、里面有多个渠道，`GetRandomSatisfiedChannel` 会继续在同一批渠道里随机，可能反复抽到刚失败的 channel。

多分组失败检测策略不是 `one` 时会维护 preferred channels，但普通 retry 和默认策略仍缺少“本请求已失败 channel 排除集”。

建议：

- 当前已在请求 context 中维护本次请求失败过的 `group + model + channel`。
- 选渠道时会排除本请求中已失败的 channel；如果候选已全部失败，不再退回到失败候选。
- 日志里记录每次尝试的 group/channel/key index。

### P2. `/v1/models` 不展开多分组 token

模型列表分组逻辑在 `controller/model.go:178`：

- `tokenGroup == auto` 时展开 auto groups。
- 其他情况只看 `ContextKeyTokenGroup`，即主分组。

影响：

- 多分组 token 实际 fallback 时可能使用后续分组的模型。
- 但 `/v1/models` 返回的模型列表只展示主分组模型，客户端会误以为其他 fallback 分组模型不可用。

建议：

- 如果 `GroupConfig.Groups > 1`，模型列表应 union 所有配置分组。
- 如果希望只展示主分组，应在文档/前端明确“fallback group 不作为模型发现能力”。

### P2. 模型请求限流按主分组，不按最终使用分组

`middleware/model-rate-limit.go:167` 读取 `ContextKeyTokenGroup` 作为分组限流依据。

影响：

- 多分组 fallback 后，实际消耗在后续 group，但限流仍按主 group。
- 如果不同 group 有不同限流策略，运营统计和限制可能不符合预期。

建议：

- 保持当前行为则定义为“token 主分组限流”。
- 若要按实际资源限流，需要在渠道选择后执行，或新增 per-token + per-final-group 限流。

### P2. Task/Midjourney 路由的多分组语义不完整

普通文本请求走 `controller.Relay` 的多分组 failover 分支。异步任务 `RelayTask` 有 retry loop，但没有调用 `MarkTokenGroupFailure` / `PrepareNextTokenGroup`。Midjourney 路由基本是 `Distribute` 初选后直接提交。

影响：

- 同一个多分组 token 在 chat/completions 和 task/MJ 上的 fallback 行为不一致。

建议：

- 明确多分组 fallback 首期只支持哪些 relay format。
- 如果要统一，抽出公共 retry/failover 编排器，Task/MJ 复用相同 group health 与状态码规则。

### P2. group health 是进程内状态，且粒度是 token+group

`service/token_group_failover.go` 中的 `tokenGroupHealth`、`tokenGroupPreferredChannels`、`tokenGroupFailureObservations` 都是内存 map。

影响：

- 多节点部署时，每个节点的 cooling 与 preferred channels 不一致。
- 进程重启后状态丢失。
- cooling key 是 token+group，不含 model。某个 model 失败会让该 token 下整个 group cooling，可能影响同 group 的其他模型。

建议：

- 单机可接受时写入文档。
- 多节点需要 Redis 存储健康状态。
- 粒度建议至少支持 token+group+model，可配置为 group 级或 model 级。

## 用户自定义重试次数设计建议

不要只新增一个“用户 retry_times”字段直接替换 `common.RetryTimes`。当前 retry 同时承担“同 group 内 priority 切换”“跨 group fallback”“上游请求超时等待”三个职责，直接放大次数会导致请求时间不可控、重复消耗上游资源、计费结算滞后。

建议拆成以下概念：

### 1. 有效重试预算

新增一个计算函数，例如：

```text
effective_retry = min(request_retry, token_retry, user_retry, system_max_retry)
```

配置层级建议：

- system max：站点最大允许值，防止用户设置过大。
- user default：用户默认重试次数。
- token override：某个 API key 的重试次数。
- request override：可选 header，例如 `X-NewAPI-Retry-Times`，但必须受 token/user/system 限制。

### 2. 分开三个预算

建议不要共用一个数字：

- `same_group_retries`：同 group 内换渠道/换 priority 的次数。
- `cross_group_failovers`：最多跨多少个后备 group。
- `total_deadline_seconds`：单次客户端请求总耗时上限。

示例：

```json
{
  "same_group_retries": 1,
  "cross_group_failovers": 2,
  "per_attempt_timeout_seconds": 20,
  "total_deadline_seconds": 70
}
```

### 3. 状态码与错误码策略

建议分为：

- `retry_status_codes`：同 group 内换渠道。
- `failover_status_codes`：跨 group。
- `never_retry_error_codes`：例如 `insufficient_user_quota`、`invalid_request`、`sensitive_words_detected`、`bad_response_body`。

默认推荐：

- retry/failover：429、500、502、503、505-599。
- 谨慎 retry：408、409、425。
- 默认不 retry：400、401、403、404、422、504、524。

如果你确定当前上游的 400 也大量是可恢复错误，应把 400 做成“按上游错误码/消息白名单重试”，而不是全局 400 重试。

### 4. 返回给用户的信息

非流式 HTTP 请求在透明 retry 时不能中途返回进度；只能最终一次性返回。

建议最终响应加 header：

- `X-NewAPI-Retry-Attempts`
- `X-NewAPI-Used-Channels`
- `X-NewAPI-Final-Group`
- `X-NewAPI-Last-Upstream-Status`
- `X-NewAPI-Request-Id`

失败响应 body 可以包含摘要，但不要泄露渠道 key/base url：

```json
{
  "error": {
    "message": "all fallback groups failed; attempts=4; final_status=503; request_id=...",
    "type": "new_api_error",
    "code": "get_channel_failed"
  }
}
```

流式请求如果要实时告知“正在重试”，只能在写出 SSE 后放弃透明 retry；建议做成 opt-in debug 模式，不作为 OpenAI-compatible 默认行为。

### 5. 计费与配额保护

重试前应做补充预留：

1. 根据候选 group 重新计算预估额度。
2. 如果比当前预扣高，调用 `BillingSession.Reserve`。
3. Reserve 失败直接返回额度不足，不请求上游。
4. 成功后最终结算按实际用量，多退少补。

这可以避免“上游成功、响应已发、订阅结算失败”或钱包扣成大额负数。

## 后续建议修复顺序

1. 拆分 `retry_status_codes` 与 `failover_status_codes`，避免同组 retry 与跨组 fallback 共用一套状态码策略。
2. 让 `/v1/models` 支持多分组 token 的模型 union，或明确只展示主分组。
3. 将恢复探活改为 adaptor-aware；非 OpenAI-compatible 渠道不要用固定 `/v1/chat/completions`。
4. 明确流式请求的 retry 边界，避免 retry 前写 ping 导致响应协议不可恢复。
5. 若部署多节点，把 token group health 从进程内 map 迁移到 Redis。
