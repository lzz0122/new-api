# 2026-06-23 fallback 与拼车统计回归分析

## 结论

本次回归的直接原因是上次部署从 `remote-sync/.../projects/new-api` 构建了二进制，而该源码树包含 token group fallback 修复，却缺失 `/home/lzz/Desktop/new-api` 中已有的拼车统计分支改动。部署时挂载的 `/new-api` 二进制覆盖了线上旧二进制，因此拼车统计表现退回旧版本。

本次修复采用选择性合入：只恢复拼车统计相关文件，保留当前源码中的 token group fallback 修复，避免再次互相覆盖。

## 影响与症状

- 拼车历史狂欢入口和历史数据展示丢失。
- 当前不在狂欢时，狂欢用量没有回退显示最近一次狂欢用量。
- 拼车额度 day/week/month 切换缺失。
- 日志清理后，用户用量统计依赖 `logs` 表导致归零或不完整。
- 用户分组定制规则的 `=:` 仅允许规则保存后，定价暴露缓存未立即失效，容易看到旧的可用分组结果。
- 部署后“日志不见了”的主要原因是查看了容器 stdout 或旧 `/app/logs` 挂载；实际业务文件日志写在 `/home/lzz/Desktop/new-api/data/logs`。

## 修复点

- 恢复拼车统计后端兼容逻辑：
  - 读取旧表 `carpool_usage_daily`、`carpool_usage_sync_runs`。
  - 用旧每日快照加最后一次同步后的实时日志尾段计算用量。
  - 查询快照前的 `quota_data`，避免日志清理后历史统计归零。
  - 支持 day/week/month 统计周期。
  - 非当前狂欢时展示最近一次狂欢统计。
- 恢复拼车统计前端：
  - 历史拼车/狂欢选择器。
  - day/week/month 切换。
  - 最近一次狂欢展示。
- 修复 `group_ratio_setting` 分层配置更新后的后处理：
  - 更新 `group_ratio_setting.group_special_usable_group` 后立即失效 pricing cache 与 ratio exposed cache。
- 补回归测试：
  - 旧每日表与实时尾段合并。
  - 快照前 quota_data 回填。
  - day 周期累计值不归零。
  - 活跃拼车分组过滤。
  - `=:` 仅允许规则。
- 更新部署 skill：
  - 明确真实业务日志路径为 `/home/lzz/Desktop/new-api/data/logs`。
  - `/home/lzz/Desktop/new-api/logs` 只是旧 `/app/logs` 挂载，可能为空。

## 部署记录

- 版本：`carpool-stats-token-fallback-20260623a`
- 本地构建产物：`deploy-artifacts/carpool-stats-token-fallback-20260623a/new-api`
- SHA256：`2e533744b7f5bf3cfabb3c4d91fdf135f061d65d4351ede60a64949b3fce24d3`
- 新容器：`new-api-carpool-stats-token-fallback-20260623a`，端口 `127.0.0.1:9103`
- 回滚容器：`new-api-token-group-routing-fallback-20260622b`，端口 `127.0.0.1:9102`
- 已删除上上次容器：`new-api-token-group-routing-fallback-20260622a`
- 备份目录：`/root/new-api-backups/carpool-stats-token-fallback-20260623a-20260623-094211`

## 后续规范

- 部署前必须确认用于构建的源码树包含所有未合并的本地定制分支，不允许只看一个目录的当前状态。
- token fallback 与拼车统计分别有独立测试，合入任一方向改动后都要运行 `go test ./model ./service ./controller -count=1`。
- 目标机只接收编译产物，不在目标机执行 `docker build`、`go build`、`npm`、`bun` 或依赖下载。
- 每次切 nginx 前保留当前生产容器作为立即回滚；部署成功后只删除上上次容器。
- 排查日志时优先看 `/home/lzz/Desktop/new-api/data/logs`，容器 stdout 只作为启动和短期运行状态参考。
