/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { Fragment, useEffect, useMemo, useState, type ReactNode } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  CalendarClock,
  ChevronDown,
  ChevronRight,
  Eye,
  EyeOff,
  Flame,
  History,
  KeyRound,
  Play,
  RefreshCw,
  Square,
  Users,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { formatLogQuota, formatNumber } from '@/lib/format'
import { cn } from '@/lib/utils'
import { useIsAdmin } from '@/hooks/use-admin'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardAction,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyTitle,
} from '@/components/ui/empty'
import { Input } from '@/components/ui/input'
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Skeleton } from '@/components/ui/skeleton'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { Dialog } from '@/components/dialog'
import {
  finishCarpool,
  finishCarnival,
  getCarpoolGroups,
  getCarpoolHistory,
  getCarpoolStatus,
  getCarpoolUsageSummary,
  getCarnivalHistory,
  getCarnivalStatus,
  getUpstreamUsage,
  startCarpool,
  startCarnival,
} from '../api'
import type {
  CarnivalSessionSummary,
  CarnivalUserUsageSummary,
  CarpoolHistorySnapshot,
  CarpoolSessionSummary,
  CarpoolUsageDailySummary,
  CarpoolUsageSummarySnapshot,
  CarpoolUsageTokenSummary,
  CarpoolUsageUserSummary,
  UpstreamUsageRateLimit,
} from '../types'
import { useUsageLogsContext } from './usage-logs-provider'

const DEFAULT_CARPOOL_GROUP = '拼车'
const ALL_MONTHS_VALUE = 'all'

function twoDigit(value: number) {
  return String(value).padStart(2, '0')
}

function formatDurationSeconds(value?: number) {
  let seconds = Math.max(0, Math.floor(value || 0))
  const days = Math.floor(seconds / 86400)
  seconds -= days * 86400
  const hours = Math.floor(seconds / 3600)
  seconds -= hours * 3600
  const minutes = Math.floor(seconds / 60)
  seconds -= minutes * 60
  const clock = `${twoDigit(hours)}:${twoDigit(minutes)}:${twoDigit(seconds)}`
  return days > 0 ? `${days}d ${clock}` : clock
}

function formatDateTime(timestamp?: number) {
  if (!timestamp) return '-'
  return new Date(timestamp * 1000).toLocaleString(undefined, {
    hour12: false,
  })
}

function formatIsoDateTime(value?: string) {
  if (!value) return '-'
  const ms = Date.parse(value)
  return Number.isFinite(ms)
    ? new Date(ms).toLocaleString(undefined, { hour12: false })
    : '-'
}

function formatHistoryMonth(timestamp?: number) {
  if (!timestamp) return ''
  const date = new Date(timestamp * 1000)
  return `${date.getFullYear()}-${twoDigit(date.getMonth() + 1)}`
}

function parseResetAt(resetAt?: string) {
  const ms = Date.parse(resetAt || '')
  return Number.isFinite(ms) ? ms : 0
}

function formatResetAt(resetAt?: string) {
  const ms = parseResetAt(resetAt)
  return ms ? new Date(ms).toLocaleString(undefined, { hour12: false }) : '-'
}

function formatResetCountdown(resetAt?: string, now = Date.now()) {
  const targetMs = parseResetAt(resetAt)
  if (!targetMs) return '-'
  let seconds = Math.max(0, Math.ceil((targetMs - now) / 1000))
  const days = Math.floor(seconds / 86400)
  seconds -= days * 86400
  const hours = Math.floor(seconds / 3600)
  seconds -= hours * 3600
  const minutes = Math.floor(seconds / 60)
  seconds -= minutes * 60
  const clock = `${twoDigit(hours)}:${twoDigit(minutes)}:${twoDigit(seconds)}`
  return days > 0 ? `${days}d ${clock}` : clock
}

function formatUpstreamCost(value?: number) {
  if (typeof value !== 'number' || !Number.isFinite(value)) return '-'
  return `$${value.toFixed(value >= 10 ? 2 : 4)}`
}

function formatCount(value?: number) {
  return formatNumber(value || 0)
}

function formatSessionRange(session?: CarpoolSessionSummary | null) {
  if (!session) return '-'
  return `${formatDateTime(session.started_at)} - ${
    session.ended_at ? formatDateTime(session.ended_at) : '进行中'
  }`
}

function formatConcurrency(concurrency?: { used?: number; limit?: number }) {
  if (!concurrency) return '-'
  const used = formatCount(concurrency.used)
  return concurrency.limit && concurrency.limit > 0
    ? `${used} / ${formatCount(concurrency.limit)}`
    : used
}

function InactiveCarpoolNotice({ group }: { group: string }) {
  return (
    <Empty className='min-h-48'>
      <EmptyHeader>
        <EmptyTitle>该分组暂未开启拼车</EmptyTitle>
        <EmptyDescription>{group || '-'} 当前没有进行中的拼车</EmptyDescription>
      </EmptyHeader>
    </Empty>
  )
}

function MetricTile({
  label,
  value,
  icon,
  sub,
}: {
  label: string
  value: ReactNode
  icon?: ReactNode
  sub?: ReactNode
}) {
  return (
    <div className='border-border/70 bg-muted/20 rounded-lg border p-3'>
      <div className='text-muted-foreground flex items-center gap-2 text-xs'>
        {icon}
        <span>{label}</span>
      </div>
      <div className='text-foreground mt-1 font-mono text-lg font-semibold tabular-nums'>
        {value}
      </div>
      {sub ? (
        <div className='text-muted-foreground mt-1 text-xs'>{sub}</div>
      ) : null}
    </div>
  )
}

function InfoRow({ label, value }: { label: string; value: ReactNode }) {
  return (
    <div className='flex items-center justify-between gap-3 py-1.5 text-sm'>
      <span className='text-muted-foreground'>{label}</span>
      <span className='text-foreground text-right font-mono tabular-nums'>
        {value}
      </span>
    </div>
  )
}

function quotaValue(value: number | undefined, masked: boolean) {
  return masked ? '••••' : formatLogQuota(value || 0)
}

function mergeCurrentCarnivalUsers(
  activeUsers: CarnivalUserUsageSummary[] | undefined,
  carpoolUsers: CarpoolUsageUserSummary[] | undefined
) {
  const activeUserMap = new Map(
    (activeUsers || []).map((user) => [user.user_id, user])
  )
  const merged = new Map<number, CarnivalUserUsageSummary>()

  ;(carpoolUsers || []).forEach((user) => {
    const activeUser = activeUserMap.get(user.user_id)
    merged.set(user.user_id, {
      user_id: user.user_id,
      username: user.username,
      quota: activeUser?.quota ?? 0,
      token_used: activeUser?.token_used ?? 0,
      request_count: activeUser?.request_count ?? 0,
    })
  })

  activeUserMap.forEach((user) => {
    if (!merged.has(user.user_id)) {
      merged.set(user.user_id, user)
    }
  })

  return Array.from(merged.values()).sort(
    (left, right) => right.quota - left.quota
  )
}

function DailySparkline({
  daily,
  masked,
}: {
  daily?: CarpoolUsageDailySummary[]
  masked: boolean
}) {
  const { t } = useTranslation()
  const rows = daily || []
  if (masked) {
    return <span className='text-muted-foreground'>••••</span>
  }
  if (rows.length === 0) {
    return <span className='text-muted-foreground'>-</span>
  }
  const max = Math.max(...rows.map((row) => row.quota || 0), 1)
  return (
    <div
      className='inline-flex h-8 w-32 items-end gap-0.5'
      aria-label={t('Daily Usage')}
    >
      {rows.map((row) => (
        <span
          key={row.date}
          title={`${row.date} ${formatLogQuota(row.quota || 0)}`}
          className='bg-primary/70 block w-1.5 rounded-t-sm'
          style={{
            height: `${Math.max(2, Math.round(((row.quota || 0) / max) * 30))}px`,
          }}
        />
      ))}
    </div>
  )
}

function TokenUsageTable({
  tokens,
  masked,
}: {
  tokens: CarpoolUsageTokenSummary[]
  masked: boolean
}) {
  const { t } = useTranslation()
  if (tokens.length === 0) {
    return (
      <div className='text-muted-foreground p-4 text-sm'>
        {t('No API key usage')}
      </div>
    )
  }

  return (
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead>{t('API Key')}</TableHead>
          <TableHead className='text-right'>{t('Period Usage')}</TableHead>
          <TableHead className='text-right'>
            {t('Total Carpool Usage')}
          </TableHead>
          <TableHead className='text-right'>{t('Requests')}</TableHead>
          <TableHead className='text-right'>{t('Status')}</TableHead>
          <TableHead className='text-right'>{t('Last Seen')}</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {tokens.map((token) => (
          <TableRow key={token.token_id}>
            <TableCell>
              <div className='flex min-w-40 flex-col'>
                <span>
                  {masked ? '••••' : token.name || `#${token.token_id}`}
                </span>
                <span className='text-muted-foreground text-xs'>
                  {t('API Key ID')}: {token.token_id}
                </span>
              </div>
            </TableCell>
            <TableCell className='text-right font-mono'>
              {quotaValue(token.period_quota, masked)}
            </TableCell>
            <TableCell className='text-right font-mono'>
              {quotaValue(token.cumulative_quota, masked)}
            </TableCell>
            <TableCell className='text-right font-mono'>
              {formatCount(token.cumulative_request_count)}
            </TableCell>
            <TableCell className='text-right'>
              <Badge variant={token.active ? 'secondary' : 'outline'}>
                {token.active ? t('Enabled') : t('Disabled')}
              </Badge>
            </TableCell>
            <TableCell className='text-right font-mono'>
              {formatIsoDateTime(token.last_seen_at)}
            </TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  )
}

function CarpoolUserUsageTable({
  data,
  masked,
}: {
  data?: CarpoolUsageSummarySnapshot | null
  masked: boolean
}) {
  const { t } = useTranslation()
  const [expanded, setExpanded] = useState<Record<number, boolean>>({})
  const users = data?.users || []

  if (users.length === 0) {
    return (
      <div className='text-muted-foreground rounded-lg border p-4 text-sm'>
        {t('No carpool API keys')}
      </div>
    )
  }

  return (
    <div className='overflow-auto rounded-lg border'>
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead className='w-10' />
            <TableHead>{t('User')}</TableHead>
            <TableHead className='text-right'>
              {t('Period Carpool Usage')}
            </TableHead>
            <TableHead className='text-right'>
              {t('Total Carpool Usage')}
            </TableHead>
            <TableHead className='text-right'>{t('API Keys')}</TableHead>
            <TableHead>{t('Daily Usage')}</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {users.map((user: CarpoolUsageUserSummary) => {
            const isExpanded = !!expanded[user.user_id]
            return (
              <Fragment key={`carpool-user-${user.user_id}`}>
                <TableRow key={`user-${user.user_id}`}>
                  <TableCell>
                    <Button
                      variant='ghost'
                      size='icon'
                      className='size-7'
                      aria-label={isExpanded ? t('Collapse') : t('Expand')}
                      onClick={() =>
                        setExpanded((value) => ({
                          ...value,
                          [user.user_id]: !isExpanded,
                        }))
                      }
                    >
                      {isExpanded ? (
                        <ChevronDown className='size-4' />
                      ) : (
                        <ChevronRight className='size-4' />
                      )}
                    </Button>
                  </TableCell>
                  <TableCell>
                    <div className='flex min-w-48 flex-col'>
                      <span>
                        {masked
                          ? '••••'
                          : user.username || user.email || `#${user.user_id}`}
                      </span>
                      <span className='text-muted-foreground text-xs'>
                        {t('User ID')}: {user.user_id}
                        {!masked && user.email ? ` · ${user.email}` : ''}
                      </span>
                    </div>
                  </TableCell>
                  <TableCell className='text-right font-mono'>
                    <div>{quotaValue(user.period_quota, masked)}</div>
                    <div className='text-muted-foreground text-xs'>
                      {formatCount(user.period_request_count)} {t('Requests')}
                    </div>
                  </TableCell>
                  <TableCell className='text-right font-mono'>
                    <div>{quotaValue(user.cumulative_quota, masked)}</div>
                    <div className='text-muted-foreground text-xs'>
                      {formatCount(user.cumulative_request_count)}{' '}
                      {t('Requests')}
                    </div>
                  </TableCell>
                  <TableCell className='text-right font-mono'>
                    {formatCount(user.active_tokens)} /{' '}
                    {formatCount(user.known_tokens)}
                  </TableCell>
                  <TableCell>
                    <DailySparkline daily={user.daily} masked={masked} />
                  </TableCell>
                </TableRow>
                {isExpanded ? (
                  <TableRow key={`tokens-${user.user_id}`}>
                    <TableCell colSpan={6} className='bg-muted/20 p-0'>
                      <TokenUsageTable
                        tokens={user.tokens || []}
                        masked={masked}
                      />
                    </TableCell>
                  </TableRow>
                ) : null}
              </Fragment>
            )
          })}
        </TableBody>
      </Table>
    </div>
  )
}

function CarpoolUsageSummaryCard({
  group,
  currentOpen,
  sessionId,
  masked,
}: {
  group: string
  currentOpen: boolean
  sessionId?: number | null
  masked: boolean
}) {
  const { t } = useTranslation()

  const query = useQuery({
    queryKey: ['carpool-usage-summary', group, 'session', sessionId || 0],
    queryFn: () =>
      getCarpoolUsageSummary({
        group,
        scope: 'session',
        session_id: sessionId || undefined,
      }),
    enabled: !!group && currentOpen,
    placeholderData: (previousData) => previousData,
    refetchInterval: 30_000,
  })

  const data = query.data
  const totals = data?.totals
  const rangeText = data?.session ? formatSessionRange(data.session) : group

  return (
    <Card className='min-h-0 shrink-0'>
      <CardHeader>
        <CardTitle>{t('Carpool quota statistics')}</CardTitle>
        <CardDescription>{rangeText}</CardDescription>
        <CardAction>
          <Tooltip>
            <TooltipTrigger
              render={
                <Button
                  variant='ghost'
                  size='icon'
                  className='text-muted-foreground hover:text-foreground size-8'
                  aria-label={t('Refresh')}
                  onClick={() => query.refetch()}
                />
              }
            >
              <RefreshCw className={cn(query.isFetching && 'animate-spin')} />
            </TooltipTrigger>
            <TooltipContent>{t('Refresh')}</TooltipContent>
          </Tooltip>
        </CardAction>
      </CardHeader>
      <CardContent className='space-y-4'>
        {!currentOpen ? (
          <InactiveCarpoolNotice group={group} />
        ) : query.isLoading && !data ? (
          <div className='space-y-3'>
            <Skeleton className='h-24 w-full rounded-lg' />
            <Skeleton className='h-80 w-full rounded-lg' />
          </div>
        ) : (
          <>
            <div className='grid gap-3 md:grid-cols-2 xl:grid-cols-4'>
              <MetricTile
                label={t('Period Carpool Usage')}
                value={quotaValue(totals?.period_quota, masked)}
                icon={<CalendarClock className='size-3.5 text-blue-500' />}
                sub={
                  <>
                    {t('Excluded Carnival')}:{' '}
                    {quotaValue(totals?.carnival_period_quota, masked)}
                  </>
                }
              />
              <MetricTile
                label={t('Total Carpool Usage')}
                value={quotaValue(totals?.cumulative_quota, masked)}
                icon={<History className='size-3.5 text-emerald-500' />}
                sub={
                  <>
                    {t('Raw Total')}:{' '}
                    {quotaValue(totals?.gross_cumulative_quota, masked)}
                  </>
                }
              />
              <MetricTile
                label={t('Users')}
                value={formatCount(totals?.users)}
                icon={<Users className='size-3.5 text-violet-500' />}
              />
              <MetricTile
                label={t('Active API Keys')}
                value={`${formatCount(totals?.active_tokens)} / ${formatCount(
                  totals?.known_tokens
                )}`}
                icon={<KeyRound className='size-3.5 text-amber-500' />}
              />
            </div>
            <div className='text-muted-foreground flex flex-wrap items-center justify-between gap-2 text-xs'>
              <span>{t('Carnival usage is excluded from carpool totals')}</span>
              <span>
                {t('Last sync')}: {formatIsoDateTime(data?.last_run_at)}
              </span>
            </div>
            <CarpoolUserUsageTable data={data} masked={masked} />
          </>
        )}
      </CardContent>
    </Card>
  )
}

function CurrentCarnivalUserTable({
  users,
  masked,
}: {
  users?: CarnivalUserUsageSummary[]
  masked: boolean
}) {
  const { t } = useTranslation()
  const rows = users || []

  if (rows.length === 0) {
    return (
      <div className='text-muted-foreground rounded-lg border p-4 text-sm'>
        {t('No user usage yet')}
      </div>
    )
  }

  return (
    <div className='overflow-auto rounded-lg border'>
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead className='w-16'>{t('Rank')}</TableHead>
            <TableHead>{t('User')}</TableHead>
            <TableHead className='text-right'>{t('Carnival Usage')}</TableHead>
            <TableHead className='text-right'>{t('Tokens')}</TableHead>
            <TableHead className='text-right'>{t('Requests')}</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {rows.map((user, index) => (
            <TableRow key={`carnival-user-${user.user_id}`}>
              <TableCell className='text-muted-foreground font-mono'>
                #{index + 1}
              </TableCell>
              <TableCell>
                <div className='flex min-w-40 flex-col'>
                  <span>
                    {masked ? '••••' : user.username || `#${user.user_id}`}
                  </span>
                  <span className='text-muted-foreground text-xs'>
                    {t('User ID')}: {user.user_id}
                  </span>
                </div>
              </TableCell>
              <TableCell className='text-right font-mono'>
                {quotaValue(user.quota, masked)}
              </TableCell>
              <TableCell className='text-right font-mono'>
                {formatCount(user.token_used)}
              </TableCell>
              <TableCell className='text-right font-mono'>
                {formatCount(user.request_count)}
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </div>
  )
}

function CarnivalHistoryPicker({
  open,
  onOpenChange,
  months,
  previewMonth,
  sessionsByMonth,
  isFetching,
  selectedLabel,
  onPreviewMonth,
  selectedSessionId,
  onSelectCurrent,
  onSelectSession,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  months: string[]
  previewMonth: string
  sessionsByMonth: Map<string, CarnivalSessionSummary[]>
  isFetching: boolean
  selectedLabel: string
  onPreviewMonth: (month: string) => void
  selectedSessionId: number | null
  onSelectCurrent: () => void
  onSelectSession: (session: CarnivalSessionSummary) => void
}) {
  const { t } = useTranslation()
  const previewSessions = previewMonth
    ? sessionsByMonth.get(previewMonth) || []
    : []

  return (
    <Popover open={open} onOpenChange={onOpenChange}>
      <PopoverTrigger
        render={
          <Button
            variant='outline'
            size='sm'
            className='h-8 max-w-[220px] gap-1.5'
            aria-label={t('Carnival History')}
          />
        }
      >
        <History className='size-3.5' />
        <span className='min-w-0 truncate'>{selectedLabel}</span>
        <ChevronDown className='size-3.5' />
      </PopoverTrigger>
      <PopoverContent
        align='end'
        className='w-[520px] max-w-[calc(100vw-2rem)] gap-0 p-0'
      >
        <div className='grid min-h-[240px] sm:grid-cols-[160px_minmax(0,1fr)]'>
          <div className='border-border/70 space-y-1 border-b p-2 sm:border-r sm:border-b-0'>
            <button
              type='button'
              className={cn(
                'hover:bg-accent hover:text-accent-foreground focus-visible:ring-ring flex w-full items-center justify-between gap-2 rounded-md px-2 py-1.5 text-left text-sm outline-none focus-visible:ring-2',
                selectedSessionId == null && 'bg-accent text-accent-foreground'
              )}
              onClick={onSelectCurrent}
            >
              <span className='font-medium'>{t('Current Carnival')}</span>
              <Flame className='size-3.5' />
            </button>
            {months.length === 0 ? (
              <div className='text-muted-foreground px-2 py-1.5 text-sm'>
                {t('No carnival session yet')}
              </div>
            ) : (
              months.map((month) => {
                const sessions = sessionsByMonth.get(month) || []
                const previewed = month === previewMonth
                return (
                  <button
                    key={month}
                    type='button'
                    className={cn(
                      'hover:bg-accent hover:text-accent-foreground focus-visible:ring-ring flex w-full items-center justify-between gap-2 rounded-md px-2 py-1.5 text-left text-sm outline-none focus-visible:ring-2',
                      previewed && 'bg-accent text-accent-foreground'
                    )}
                    onMouseEnter={() => onPreviewMonth(month)}
                    onFocus={() => onPreviewMonth(month)}
                    onClick={() => onPreviewMonth(month)}
                  >
                    <span className='font-medium'>{month}</span>
                    <span className='text-muted-foreground text-xs'>
                      {formatCount(sessions.length)}
                    </span>
                  </button>
                )
              })
            )}
          </div>
          <div className='min-w-0 space-y-2 p-2'>
            <div className='text-muted-foreground flex h-7 items-center justify-between gap-2 px-1 text-xs'>
              <span>{previewMonth || t('Carnival Sessions')}</span>
              {isFetching && <RefreshCw className='size-3.5 animate-spin' />}
            </div>
            {previewSessions.length === 0 ? (
              <div className='text-muted-foreground rounded-md border p-3 text-sm'>
                {t('No carnival session yet')}
              </div>
            ) : (
              <div className='max-h-[260px] space-y-1 overflow-auto pr-1'>
                {previewSessions.map((session) => (
                  <button
                    key={session.id}
                    type='button'
                    className={cn(
                      'hover:bg-accent hover:text-accent-foreground focus-visible:ring-ring flex w-full items-start justify-between gap-3 rounded-md px-2 py-2 text-left text-sm outline-none focus-visible:ring-2',
                      session.id === selectedSessionId &&
                        'bg-accent text-accent-foreground'
                    )}
                    onClick={() => onSelectSession(session)}
                  >
                    <span className='min-w-0'>
                      <span className='block truncate'>
                        {formatDateTime(session.started_at)}
                      </span>
                      <span className='text-muted-foreground mt-0.5 block text-xs'>
                        {formatDurationSeconds(session.duration_seconds)}
                      </span>
                    </span>
                    <span className='font-mono text-xs'>
                      {formatLogQuota(session.total_quota || 0)}
                    </span>
                  </button>
                ))}
              </div>
            )}
          </div>
        </div>
      </PopoverContent>
    </Popover>
  )
}

function CarnivalUsageCard({
  group,
  currentOpen,
  active,
  activeElapsed,
  masked,
  onRefresh,
}: {
  group: string
  currentOpen: boolean
  active?: CarnivalSessionSummary | null
  activeElapsed: number
  masked: boolean
  onRefresh: () => void
}) {
  const { t } = useTranslation()
  const isAdmin = useIsAdmin()
  const queryClient = useQueryClient()
  const { sensitiveVisible, setSensitiveVisible } = useUsageLogsContext()
  const [historyPickerOpen, setHistoryPickerOpen] = useState(false)
  const [selectedHistoryMonth, setSelectedHistoryMonth] = useState('')
  const [hoveredHistoryMonth, setHoveredHistoryMonth] = useState('')
  const [selectedHistorySessionId, setSelectedHistorySessionId] = useState<
    number | null
  >(null)

  const carpoolUsersQuery = useQuery({
    queryKey: ['carpool-usage-summary', group, 'current-carnival-users'],
    queryFn: () =>
      getCarpoolUsageSummary({
        group,
        scope: 'session',
      }),
    enabled: !!group && currentOpen,
    placeholderData: (previousData) => previousData,
    refetchInterval: 30_000,
  })

  const historyQuery = useQuery({
    queryKey: ['carnival-history', group, ALL_MONTHS_VALUE],
    queryFn: async () => {
      const result = await getCarnivalHistory({
        group,
        month: ALL_MONTHS_VALUE,
      })
      if (!result.success) return null
      return result.data || null
    },
    enabled: !!group && currentOpen,
    placeholderData: (previousData) => previousData,
  })

  const historySessions = useMemo(
    () =>
      [...(historyQuery.data?.sessions || [])].sort(
        (left, right) => right.started_at - left.started_at
      ),
    [historyQuery.data?.sessions]
  )

  const sessionsByMonth = useMemo(() => {
    const map = new Map<string, CarnivalSessionSummary[]>()
    historySessions.forEach((session) => {
      const month = formatHistoryMonth(session.started_at)
      if (!month) return
      const sessions = map.get(month) || []
      sessions.push(session)
      map.set(month, sessions)
    })
    return map
  }, [historySessions])

  const historyMonths = useMemo(() => {
    const listedMonths = historyQuery.data?.months || []
    const months = listedMonths.length
      ? listedMonths
      : Array.from(sessionsByMonth.keys())
    return months.filter((month) => sessionsByMonth.has(month))
  }, [historyQuery.data?.months, sessionsByMonth])

  const selectedHistorySession =
    selectedHistorySessionId == null
      ? null
      : historySessions.find(
          (session) => session.id === selectedHistorySessionId
        ) || null
  const selectedMonth = selectedHistorySession
    ? formatHistoryMonth(selectedHistorySession.started_at)
    : historyMonths.includes(selectedHistoryMonth)
      ? selectedHistoryMonth
      : ''
  const previewMonth = historyMonths.includes(hoveredHistoryMonth)
    ? hoveredHistoryMonth
    : historyMonths.includes(selectedMonth)
      ? selectedMonth
      : historyMonths[0] || ''
  const currentCarnivalRows = mergeCurrentCarnivalUsers(
    active?.users,
    carpoolUsersQuery.data?.users
  )
  const currentCarnivalQuota =
    active?.total_quota ??
    currentCarnivalRows.reduce((total, user) => total + user.quota, 0)
  const currentCarnivalTokens =
    active?.total_tokens ??
    currentCarnivalRows.reduce((total, user) => total + user.token_used, 0)
  const currentCarnivalRequests =
    active?.request_count ??
    currentCarnivalRows.reduce((total, user) => total + user.request_count, 0)
  const displayedCarnivalRows = selectedHistorySession
    ? [...selectedHistorySession.users].sort(
        (left, right) => right.quota - left.quota
      )
    : currentCarnivalRows
  const displayedCarnivalQuota = selectedHistorySession
    ? selectedHistorySession.total_quota
    : currentCarnivalQuota
  const displayedCarnivalTokens = selectedHistorySession
    ? selectedHistorySession.total_tokens
    : currentCarnivalTokens
  const displayedCarnivalRequests = selectedHistorySession
    ? selectedHistorySession.request_count
    : currentCarnivalRequests
  const displayedCarnivalDuration = selectedHistorySession
    ? selectedHistorySession.duration_seconds
    : activeElapsed
  const displayedCarnivalUsers = displayedCarnivalRows.length
  const historyPickerLabel = selectedHistorySession
    ? formatDateTime(selectedHistorySession.started_at)
    : t('Current Carnival')

  const invalidateCarnival = () => {
    queryClient.invalidateQueries({
      queryKey: ['carnival-status', group],
    })
    queryClient.invalidateQueries({
      queryKey: ['carnival-history', group],
    })
    queryClient.invalidateQueries({
      queryKey: ['carpool-usage-summary', group],
    })
    queryClient.invalidateQueries({ queryKey: ['usage-logs-stats'] })
  }

  const startMutation = useMutation({
    mutationFn: async () => {
      const result = await startCarnival({ group })
      if (!result.success) {
        throw new Error(result.message || t('Operation failed'))
      }
      return result.data
    },
    onSuccess: invalidateCarnival,
    onError: (error) => {
      toast.error(
        error instanceof Error ? error.message : t('Operation failed')
      )
    },
  })

  const finishMutation = useMutation({
    mutationFn: async () => {
      const result = await finishCarnival({ group })
      if (!result.success) {
        throw new Error(result.message || t('Operation failed'))
      }
      return result.data
    },
    onSuccess: invalidateCarnival,
    onError: (error) => {
      toast.error(
        error instanceof Error ? error.message : t('Operation failed')
      )
    },
  })

  const actionDisabled = startMutation.isPending || finishMutation.isPending
  const handleRefresh = () => {
    onRefresh()
    void carpoolUsersQuery.refetch()
    void historyQuery.refetch()
  }

  return (
    <Card className='min-h-0 shrink-0'>
      <CardHeader>
        <CardTitle>{t('Carnival Usage')}</CardTitle>
        <CardDescription>
          {t('Carnival usage is excluded from carpool totals')}
        </CardDescription>
        <CardAction className='flex flex-wrap items-center gap-2'>
          <CarnivalHistoryPicker
            open={historyPickerOpen}
            onOpenChange={(open) => {
              setHistoryPickerOpen(open)
              if (open) {
                setHoveredHistoryMonth(selectedMonth)
              }
            }}
            months={historyMonths}
            previewMonth={previewMonth}
            sessionsByMonth={sessionsByMonth}
            isFetching={historyQuery.isFetching}
            selectedLabel={historyPickerLabel}
            onPreviewMonth={setHoveredHistoryMonth}
            selectedSessionId={selectedHistorySessionId}
            onSelectCurrent={() => {
              setSelectedHistoryMonth('')
              setHoveredHistoryMonth(previewMonth)
              setSelectedHistorySessionId(null)
              setHistoryPickerOpen(false)
            }}
            onSelectSession={(session) => {
              const month = formatHistoryMonth(session.started_at)
              setSelectedHistoryMonth(month)
              setHoveredHistoryMonth(month)
              setSelectedHistorySessionId(session.id)
              setHistoryPickerOpen(false)
            }}
          />
          <Tooltip>
            <TooltipTrigger
              render={
                <Button
                  variant='ghost'
                  size='icon'
                  className='text-muted-foreground hover:text-foreground size-8'
                  aria-label={sensitiveVisible ? t('Hide') : t('Show')}
                  onClick={() => setSensitiveVisible(!sensitiveVisible)}
                />
              }
            >
              {sensitiveVisible ? <Eye /> : <EyeOff />}
            </TooltipTrigger>
            <TooltipContent>
              {sensitiveVisible ? t('Hide') : t('Show')}
            </TooltipContent>
          </Tooltip>
          <Button
            variant='outline'
            size='icon'
            className='size-8'
            aria-label={t('Refresh')}
            onClick={handleRefresh}
          >
            <RefreshCw className='size-4' />
          </Button>
          {isAdmin && !selectedHistorySession && (
            <Button
              variant={active ? 'outline' : 'default'}
              size='sm'
              className='h-8 gap-1.5'
              disabled={actionDisabled}
              onClick={() => {
                if (active) {
                  finishMutation.mutate()
                } else {
                  startMutation.mutate()
                }
              }}
            >
              {active ? (
                <Square className='size-3.5' />
              ) : (
                <Play className='size-3.5' />
              )}
              {active ? t('End Carnival') : t('Start Carnival')}
            </Button>
          )}
        </CardAction>
      </CardHeader>
      <CardContent className='space-y-4'>
        {!currentOpen ? (
          <InactiveCarpoolNotice group={group} />
        ) : (
          <>
            <div className='flex flex-wrap items-center gap-2'>
              <Badge
                variant='outline'
                className={cn(
                  'gap-1.5',
                  !selectedHistorySession &&
                    active &&
                    'border-orange-500/30 bg-orange-500/10 text-orange-700 dark:text-orange-300'
                )}
              >
                {selectedHistorySession ? (
                  <History className='size-3.5' />
                ) : (
                  <Flame className='size-3.5' />
                )}
                {selectedHistorySession
                  ? t('Carnival Sessions')
                  : active
                    ? t('Carnival Active')
                    : t('Carnival Idle')}
              </Badge>
              <Badge variant='secondary' className='gap-1.5'>
                <History className='size-3.5' />
                {group}
              </Badge>
            </div>

            <div className='grid gap-3 md:grid-cols-2 xl:grid-cols-4'>
              <MetricTile
                label={
                  selectedHistorySession
                    ? t('Carnival Usage')
                    : t('Current Carnival Total')
                }
                value={quotaValue(displayedCarnivalQuota, masked)}
                icon={<Flame className='size-3.5 text-orange-500' />}
                sub={
                  selectedHistorySession
                    ? `${t('Duration')}: ${formatDurationSeconds(displayedCarnivalDuration)}`
                    : active
                      ? `${t('Live Duration')}: ${formatDurationSeconds(displayedCarnivalDuration)}`
                      : t('No active carnival')
                }
              />
              <MetricTile
                label={t('Users')}
                value={formatCount(displayedCarnivalUsers)}
                icon={<History className='size-3.5 text-emerald-500' />}
                sub={
                  selectedHistorySession
                    ? t('Carnival Sessions')
                    : active
                      ? t('Current Carnival')
                      : t('No active carnival')
                }
              />
              <MetricTile
                label={t('Tokens')}
                value={formatCount(displayedCarnivalTokens)}
                icon={<KeyRound className='size-3.5 text-blue-500' />}
                sub={
                  selectedHistorySession || active
                    ? `${t('Started')}: ${formatDateTime(
                        selectedHistorySession?.started_at ?? active?.started_at
                      )}`
                    : '-'
                }
              />
              <MetricTile
                label={t('Requests')}
                value={formatCount(displayedCarnivalRequests)}
                icon={<CalendarClock className='size-3.5 text-violet-500' />}
                sub={
                  selectedHistorySession
                    ? `${t('Ended')}: ${formatDateTime(
                        selectedHistorySession.ended_at
                      )}`
                    : active
                      ? t('Current Carnival')
                      : '-'
                }
              />
            </div>

            <div className='space-y-2'>
              <div className='flex items-center justify-between gap-3'>
                <div className='text-sm font-medium'>
                  {t('Per-user Carnival Usage')}
                </div>
                <div className='text-muted-foreground text-xs'>
                  {formatCount(displayedCarnivalUsers)} {t('Users')}
                </div>
              </div>
              <CurrentCarnivalUserTable
                users={displayedCarnivalRows}
                masked={masked}
              />
            </div>
          </>
        )}
      </CardContent>
    </Card>
  )
}

function formatWindowLabel(window: string, t: (key: string) => string) {
  if (window === '7d') return t('Weekly')
  return window
}

function UpstreamLimitTable({
  limits,
  masked,
  now,
}: {
  limits: UpstreamUsageRateLimit[]
  masked: boolean
  now: number
}) {
  const { t } = useTranslation()

  if (limits.length === 0) {
    return (
      <div className='text-muted-foreground rounded-lg border p-4 text-sm'>
        {t('No data')}
      </div>
    )
  }

  return (
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead>{t('Rate Limit Window')}</TableHead>
          <TableHead className='text-right'>{t('Used')}</TableHead>
          <TableHead className='text-right'>{t('Remaining')}</TableHead>
          <TableHead className='text-right'>{t('Total')}</TableHead>
          <TableHead className='text-right'>{t('Reset Countdown')}</TableHead>
          <TableHead className='text-right'>{t('Reset at:')}</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {limits.map((limit) => (
          <TableRow key={limit.window}>
            <TableCell>{formatWindowLabel(limit.window, t)}</TableCell>
            <TableCell className='text-right font-mono'>
              {masked ? '••••' : formatUpstreamCost(limit.used)}
            </TableCell>
            <TableCell className='text-right font-mono'>
              {masked ? '••••' : formatUpstreamCost(limit.remaining)}
            </TableCell>
            <TableCell className='text-right font-mono'>
              {masked ? '••••' : formatUpstreamCost(limit.limit)}
            </TableCell>
            <TableCell className='text-right font-mono'>
              {formatResetCountdown(limit.reset_at, now)}
            </TableCell>
            <TableCell className='text-right font-mono'>
              {formatResetAt(limit.reset_at)}
            </TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  )
}

function UpstreamQuotaCard({
  group,
  currentOpen,
  masked,
}: {
  group: string
  currentOpen: boolean
  masked: boolean
}) {
  const { t } = useTranslation()
  const [refreshSeq, setRefreshSeq] = useState(0)
  const [now, setNow] = useState(() => Date.now())

  useEffect(() => {
    const timer = window.setInterval(() => setNow(Date.now()), 1000)
    return () => window.clearInterval(timer)
  }, [])

  const { data, isFetching } = useQuery({
    queryKey: ['sub2api-upstream-usage', group, refreshSeq],
    queryFn: async () => {
      const result = await getUpstreamUsage({
        group,
        refresh: refreshSeq > 0,
      })
      if (!result.success) return null
      return result.data || null
    },
    enabled: !!group && currentOpen,
    placeholderData: (previousData) => previousData,
    retry: false,
    staleTime: 30_000,
  })

  const limits = data?.rate_limits || []
  const nextRefreshMs = (data?.next_refresh_at || 0) * 1000
  const canRefresh = !isFetching && (!nextRefreshMs || now >= nextRefreshMs)
  const waitSeconds = Math.max(0, Math.ceil((nextRefreshMs - now) / 1000))
  const refreshTitle = canRefresh
    ? t('Refresh')
    : t('Refresh available in {{seconds}}s', { seconds: waitSeconds })

  return (
    <Card className='min-h-0 shrink-0'>
      <CardHeader>
        <CardTitle>{t('Upstream Quota')}</CardTitle>
        <CardDescription>{group}</CardDescription>
        <CardAction>
          <Tooltip>
            <TooltipTrigger
              render={
                <Button
                  variant='ghost'
                  size='icon'
                  className='text-muted-foreground hover:text-foreground size-8'
                  disabled={!canRefresh}
                  aria-label={refreshTitle}
                  onClick={() => {
                    if (!canRefresh) return
                    setRefreshSeq((value) => value + 1)
                  }}
                />
              }
            >
              <RefreshCw className={cn(isFetching && 'animate-spin')} />
            </TooltipTrigger>
            <TooltipContent>{refreshTitle}</TooltipContent>
          </Tooltip>
        </CardAction>
      </CardHeader>
      <CardContent className='space-y-4'>
        {!currentOpen ? (
          <InactiveCarpoolNotice group={group} />
        ) : isFetching && !data ? (
          <div className='space-y-3'>
            <Skeleton className='h-20 w-full rounded-lg' />
            <Skeleton className='h-28 w-full rounded-lg' />
          </div>
        ) : (
          <>
            <div className='grid gap-3 sm:grid-cols-2'>
              <InfoRow label={t('Group')} value={data?.upstream_group || '-'} />
              <InfoRow
                label={t('Key')}
                value={masked ? '••••' : data?.masked_key || '-'}
              />
              <InfoRow
                label='实时并发'
                value={masked ? '••••' : formatConcurrency(data?.concurrency)}
              />
              <InfoRow
                label={t('Updated')}
                value={formatDateTime(data?.updated_at)}
              />
              <InfoRow
                label={t('Status')}
                value={
                  <Badge variant='outline'>
                    {data?.cached ? t('Cached') : t('Updated')}
                  </Badge>
                }
              />
            </div>
            <div className='overflow-auto rounded-lg border'>
              <UpstreamLimitTable limits={limits} masked={masked} now={now} />
            </div>
          </>
        )}
      </CardContent>
    </Card>
  )
}

function CarpoolHistoryCard({
  history,
  selectedMonth,
  selectedSessionId,
  isFetching,
  masked,
  onMonthChange,
  onSelectSession,
}: {
  history?: CarpoolHistorySnapshot | null
  selectedMonth: string
  selectedSessionId: number | null
  isFetching: boolean
  masked: boolean
  onMonthChange: (month: string) => void
  onSelectSession: (session: CarpoolSessionSummary) => void
}) {
  const { t } = useTranslation()
  const months = history?.months || []
  const groups = history?.groups || []

  return (
    <Card className='min-h-0 shrink-0'>
      <CardHeader>
        <CardTitle className='flex items-center gap-2'>
          <History className='size-4' />
          {t('Carpool History')}
        </CardTitle>
        <CardDescription>
          {t('Carnival usage is excluded from carpool totals')}
        </CardDescription>
        <CardAction className='flex items-center gap-2'>
          {isFetching ? (
            <RefreshCw className='text-muted-foreground size-4 animate-spin' />
          ) : null}
          <Select
            items={months.map((month) => ({ value: month, label: month }))}
            value={selectedMonth}
            onValueChange={onMonthChange}
          >
            <SelectTrigger size='sm' className='min-w-32'>
              <SelectValue placeholder={t('Month')} />
            </SelectTrigger>
            <SelectContent alignItemWithTrigger={false}>
              <SelectGroup>
                {months.map((month) => (
                  <SelectItem key={month} value={month}>
                    {month}
                  </SelectItem>
                ))}
              </SelectGroup>
            </SelectContent>
          </Select>
        </CardAction>
      </CardHeader>
      <CardContent className='space-y-4'>
        {groups.length === 0 ? (
          <Empty className='min-h-40'>
            <EmptyHeader>
              <EmptyTitle>{t('No data')}</EmptyTitle>
              <EmptyDescription>{t('No carpool history yet')}</EmptyDescription>
            </EmptyHeader>
          </Empty>
        ) : (
          groups.map((group) => (
            <div key={group.group} className='space-y-2'>
              <div className='flex flex-wrap items-center gap-2'>
                <Badge variant='secondary'>{group.group}</Badge>
                <Badge variant='outline'>
                  {t('Total')}{' '}
                  {masked
                    ? '••••'
                    : formatLogQuota(group.total?.period_quota || 0)}
                </Badge>
              </div>
              <div className='overflow-auto rounded-lg border'>
                <Table>
                  <TableHeader>
                    <TableRow>
                      <TableHead>{t('Started')}</TableHead>
                      <TableHead>{t('Ended')}</TableHead>
                      <TableHead className='text-right'>{t('Total')}</TableHead>
                      <TableHead className='text-right'>
                        {t('Requests')}
                      </TableHead>
                      <TableHead className='text-right'>
                        {t('Actions')}
                      </TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {(group.sessions || []).map((session) => (
                      <TableRow key={session.id}>
                        <TableCell className='font-mono'>
                          {formatDateTime(session.started_at)}
                        </TableCell>
                        <TableCell className='font-mono'>
                          {session.ended_at
                            ? formatDateTime(session.ended_at)
                            : t('Active')}
                        </TableCell>
                        <TableCell className='text-right font-mono'>
                          {masked
                            ? '••••'
                            : formatLogQuota(session.total_quota || 0)}
                        </TableCell>
                        <TableCell className='text-right font-mono'>
                          {formatCount(session.request_count)}
                        </TableCell>
                        <TableCell className='text-right'>
                          <Button
                            variant={
                              session.id === selectedSessionId
                                ? 'secondary'
                                : 'outline'
                            }
                            size='sm'
                            onClick={() => onSelectSession(session)}
                          >
                            {t('View')}
                          </Button>
                        </TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              </div>
            </div>
          ))
        )}
      </CardContent>
    </Card>
  )
}

export function CarpoolStatsPage() {
  const { t } = useTranslation()
  const { sensitiveVisible } = useUsageLogsContext()
  const isAdmin = useIsAdmin()
  const queryClient = useQueryClient()
  const [now, setNow] = useState(() => Date.now())
  const [selectedGroup, setSelectedGroup] = useState('')
  const [selectedMonth, setSelectedMonth] = useState('')
  const [selectedSessionId, setSelectedSessionId] = useState<number | null>(
    null
  )
  const [finishDialogOpen, setFinishDialogOpen] = useState(false)
  const [finishCode, setFinishCode] = useState('')

  useEffect(() => {
    const timer = window.setInterval(() => setNow(Date.now()), 1000)
    return () => window.clearInterval(timer)
  }, [])

  const groupsQuery = useQuery({
    queryKey: ['carpool-groups'],
    queryFn: getCarpoolGroups,
    staleTime: 60_000,
  })
  const groupOptions = groupsQuery.data?.groups || []
  const group =
    selectedGroup || groupsQuery.data?.default_group || DEFAULT_CARPOOL_GROUP

  useEffect(() => {
    if (selectedGroup || groupOptions.length === 0) return
    const defaultGroup =
      groupsQuery.data?.default_group || DEFAULT_CARPOOL_GROUP
    setSelectedGroup(
      groupOptions.includes(defaultGroup) ? defaultGroup : groupOptions[0]
    )
  }, [groupOptions, groupsQuery.data?.default_group, selectedGroup])

  const carpoolStatusQuery = useQuery({
    queryKey: ['carpool-status', group],
    queryFn: () => getCarpoolStatus({ group }),
    enabled: !!group,
    placeholderData: (previousData) => previousData,
    refetchInterval: 15_000,
  })

  const historyQuery = useQuery({
    queryKey: ['carpool-history', group, selectedMonth],
    queryFn: () =>
      getCarpoolHistory({
        group,
        month: selectedMonth || undefined,
      }),
    enabled: !!group,
    placeholderData: (previousData) => previousData,
  })

  useEffect(() => {
    if (!selectedMonth && historyQuery.data?.selected_month) {
      setSelectedMonth(historyQuery.data.selected_month)
    }
  }, [historyQuery.data?.selected_month, selectedMonth])

  const carnivalStatusQuery = useQuery({
    queryKey: ['carnival-status', group],
    queryFn: async () => {
      const result = await getCarnivalStatus({ group })
      if (!result.success) return null
      return result.data || null
    },
    enabled: !!group,
    placeholderData: (previousData) => previousData,
    refetchInterval: 15_000,
  })

  const selectedSession = useMemo(() => {
    if (selectedSessionId == null) return null
    for (const historyGroup of historyQuery.data?.groups || []) {
      const session = (historyGroup.sessions || []).find(
        (item) => item.id === selectedSessionId
      )
      if (session) return session
    }
    return null
  }, [historyQuery.data?.groups, selectedSessionId])

  const carpoolActiveSession = carpoolStatusQuery.data?.active || null
  const currentOpen = Boolean(selectedSession || carpoolActiveSession)
  const active = carnivalStatusQuery.data?.active
  const activeElapsed = useMemo(
    () =>
      active ? Math.max(0, Math.floor(now / 1000) - active.started_at) : 0,
    [active, now]
  )
  const masked = !sensitiveVisible

  const invalidateCarpool = () => {
    queryClient.invalidateQueries({ queryKey: ['carpool-groups'] })
    queryClient.invalidateQueries({ queryKey: ['carpool-status', group] })
    queryClient.invalidateQueries({ queryKey: ['carpool-history', group] })
    queryClient.invalidateQueries({
      queryKey: ['carpool-usage-summary', group],
    })
    queryClient.invalidateQueries({
      queryKey: ['sub2api-upstream-usage', group],
    })
    queryClient.invalidateQueries({ queryKey: ['carnival-status', group] })
    queryClient.invalidateQueries({ queryKey: ['carnival-history', group] })
    queryClient.invalidateQueries({ queryKey: ['usage-logs-stats'] })
  }

  const startMutation = useMutation({
    mutationFn: async () => {
      const result = await startCarpool({ group })
      if (!result.success) {
        throw new Error(result.message || t('Operation failed'))
      }
      return result.data
    },
    onSuccess: () => {
      setSelectedSessionId(null)
      invalidateCarpool()
      toast.success('拼车已开启')
    },
    onError: (error) => {
      toast.error(error instanceof Error ? error.message : '开启拼车失败')
    },
  })

  const finishMutation = useMutation({
    mutationFn: async () => {
      const result = await finishCarpool({ group, code: finishCode })
      if (!result.success) {
        throw new Error(result.message || t('Operation failed'))
      }
      return result.data
    },
    onSuccess: () => {
      setFinishDialogOpen(false)
      setFinishCode('')
      setSelectedSessionId(null)
      invalidateCarpool()
      toast.success('拼车已结束')
    },
    onError: (error) => {
      toast.error(error instanceof Error ? error.message : '结束拼车失败')
    },
  })

  const refreshAll = () => {
    void groupsQuery.refetch()
    void carpoolStatusQuery.refetch()
    void historyQuery.refetch()
    void carnivalStatusQuery.refetch()
    queryClient.invalidateQueries({
      queryKey: ['carpool-usage-summary', group],
    })
    queryClient.invalidateQueries({
      queryKey: ['sub2api-upstream-usage', group],
    })
  }

  return (
    <div className='flex h-full min-h-0 flex-col gap-4 overflow-auto pb-4'>
      <div className='bg-background/80 sticky top-0 z-10 flex shrink-0 flex-col gap-3 border-b pb-3 backdrop-blur md:flex-row md:items-center md:justify-between'>
        <div className='min-w-0'>
          <div className='flex flex-wrap items-center gap-2'>
            <h2 className='text-lg font-semibold'>{t('Carpool Stats')}</h2>
            <Badge variant={currentOpen ? 'default' : 'outline'}>
              {currentOpen ? t('Active') : '未开启'}
            </Badge>
          </div>
          <div className='text-muted-foreground mt-1 truncate text-sm'>
            {selectedSession
              ? `正在查看历史拼车：${formatSessionRange(selectedSession)}`
              : carpoolActiveSession
                ? formatSessionRange(carpoolActiveSession)
                : `${group} 当前没有进行中的拼车`}
          </div>
        </div>

        <div className='flex flex-wrap items-center gap-2'>
          <Select
            items={groupOptions.map((item) => ({ value: item, label: item }))}
            value={group}
            onValueChange={(value) => {
              setSelectedGroup(value)
              setSelectedSessionId(null)
              setSelectedMonth('')
            }}
          >
            <SelectTrigger className='min-w-40'>
              <SelectValue placeholder='选择分组' />
            </SelectTrigger>
            <SelectContent alignItemWithTrigger={false}>
              <SelectGroup>
                {groupOptions.map((item) => (
                  <SelectItem key={item} value={item}>
                    {item}
                  </SelectItem>
                ))}
              </SelectGroup>
            </SelectContent>
          </Select>
          <Button
            variant='outline'
            size='sm'
            className='gap-1.5'
            onClick={refreshAll}
          >
            <RefreshCw className='size-3.5' />
            {t('Refresh')}
          </Button>
          {selectedSession ? (
            <Button
              variant='secondary'
              size='sm'
              onClick={() => setSelectedSessionId(null)}
            >
              查看当前拼车
            </Button>
          ) : null}
          {isAdmin ? (
            <>
              <Button
                size='sm'
                className='gap-1.5'
                disabled={
                  !!carpoolActiveSession || startMutation.isPending || !group
                }
                onClick={() => startMutation.mutate()}
              >
                <Play className='size-3.5' />
                开启拼车
              </Button>
              <Button
                variant='outline'
                size='sm'
                className='gap-1.5'
                disabled={!carpoolActiveSession || finishMutation.isPending}
                onClick={() => setFinishDialogOpen(true)}
              >
                <Square className='size-3.5' />
                结束拼车
              </Button>
            </>
          ) : null}
        </div>
      </div>

      <div className='grid shrink-0 gap-4 2xl:grid-cols-[minmax(360px,0.85fr)_minmax(0,1.35fr)]'>
        <UpstreamQuotaCard
          group={group}
          currentOpen={currentOpen}
          masked={masked}
        />
        <CarpoolUsageSummaryCard
          group={group}
          currentOpen={currentOpen}
          sessionId={selectedSessionId}
          masked={masked}
        />
      </div>

      {carnivalStatusQuery.isLoading && !carnivalStatusQuery.data ? (
        <Skeleton className='h-72 w-full rounded-xl' />
      ) : (
        <CarnivalUsageCard
          group={group}
          currentOpen={currentOpen}
          active={active}
          activeElapsed={activeElapsed}
          masked={masked}
          onRefresh={() => {
            void carnivalStatusQuery.refetch()
          }}
        />
      )}

      <CarpoolHistoryCard
        history={historyQuery.data}
        selectedMonth={selectedMonth}
        selectedSessionId={selectedSessionId}
        isFetching={historyQuery.isFetching}
        masked={masked}
        onMonthChange={(month) => setSelectedMonth(month)}
        onSelectSession={(session) => setSelectedSessionId(session.id)}
      />

      <Dialog
        open={finishDialogOpen}
        onOpenChange={setFinishDialogOpen}
        title='结束拼车'
        description='请输入系统设置中配置的拼车结束 2FA 密码'
        footer={
          <>
            <Button
              variant='outline'
              onClick={() => setFinishDialogOpen(false)}
            >
              {t('Cancel')}
            </Button>
            <Button
              disabled={finishMutation.isPending}
              onClick={() => finishMutation.mutate()}
            >
              结束拼车
            </Button>
          </>
        }
      >
        <Input
          type='password'
          value={finishCode}
          placeholder='输入拼车结束 2FA 密码'
          onChange={(event) => setFinishCode(event.target.value)}
        />
      </Dialog>
    </div>
  )
}
