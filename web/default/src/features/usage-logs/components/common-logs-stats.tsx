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
import { useEffect, useMemo, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { getRouteApi } from '@tanstack/react-router'
import { Flame, History, Play, RefreshCw, Square } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { formatLogQuota } from '@/lib/format'
import { cn } from '@/lib/utils'
import { useIsAdmin } from '@/hooks/use-admin'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
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
  finishCarnival,
  getCarnivalHistory,
  getCarnivalStatus,
  getLogStats,
  getUpstreamUsage,
  getUserLogStats,
  startCarnival,
} from '../api'
import { DEFAULT_LOG_STATS } from '../constants'
import { buildApiParams } from '../lib/utils'
import type { CarnivalAggregateSummary, CarnivalSessionSummary } from '../types'
import { useUsageLogsContext } from './usage-logs-provider'

const route = getRouteApi('/_authenticated/usage-logs/$section')

function StatBadge(props: {
  label: string
  value: string | number
  accent: string
  title?: string
}) {
  return (
    <span
      className='border-border/60 bg-muted/25 inline-flex h-7 items-center gap-2 rounded-md border px-2.5 text-xs shadow-xs'
      title={props.title}
    >
      <span className={cn('h-3.5 w-0.5 rounded-full', props.accent)} />
      <span className='text-muted-foreground'>{props.label}</span>
      <span className='text-foreground/85 font-mono font-semibold tabular-nums'>
        {props.value}
      </span>
    </span>
  )
}

function formatUpstreamCost(value?: number) {
  if (typeof value !== 'number' || !Number.isFinite(value)) return '-'
  return `$${value.toFixed(value >= 10 ? 2 : 4)}`
}

function formatRefreshTime(timestamp?: number) {
  if (!timestamp) return '-'
  return new Date(timestamp * 1000).toLocaleTimeString(undefined, {
    hour12: false,
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
  })
}

function twoDigit(value: number) {
  return String(value).padStart(2, '0')
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

function CarnivalAggregateBlock({
  title,
  aggregate,
  compact = false,
}: {
  title: string
  aggregate?: CarnivalAggregateSummary
  compact?: boolean
}) {
  const { t } = useTranslation()
  return (
    <div
      className={cn(
        compact
          ? 'border-border/60 mt-3 border-t pt-3'
          : 'border-border/60 bg-muted/20 rounded-md border p-3'
      )}
    >
      <div className='text-foreground mb-2 text-sm font-medium'>{title}</div>
      <div className='grid gap-2 text-xs sm:grid-cols-3'>
        <div>
          <div className='text-muted-foreground'>{t('Carnival Usage')}</div>
          <div className='font-mono font-semibold'>
            {formatLogQuota(aggregate?.total_quota || 0)}
          </div>
        </div>
        <div>
          <div className='text-muted-foreground'>{t('Requests')}</div>
          <div className='font-mono font-semibold'>
            {aggregate?.request_count || 0}
          </div>
        </div>
        <div>
          <div className='text-muted-foreground'>{t('Tokens')}</div>
          <div className='font-mono font-semibold'>
            {aggregate?.total_tokens || 0}
          </div>
        </div>
      </div>
      {(aggregate?.users?.length || 0) > 0 && (
        <div className='mt-3 max-h-40 overflow-auto'>
          <table className='w-full text-xs'>
            <thead className='text-muted-foreground'>
              <tr className='border-border/50 border-b'>
                <th className='py-1 pr-2 text-left font-medium'>{t('User')}</th>
                <th className='px-2 py-1 text-right font-medium'>
                  {t('Usage')}
                </th>
                <th className='py-1 pl-2 text-right font-medium'>
                  {t('Requests')}
                </th>
              </tr>
            </thead>
            <tbody>
              {aggregate?.users.map((user) => (
                <tr key={`${user.user_id}-${user.username}`}>
                  <td className='text-foreground/85 py-1 pr-2'>
                    {user.username || `#${user.user_id}`}
                  </td>
                  <td className='px-2 py-1 text-right font-mono'>
                    {formatLogQuota(user.quota)}
                  </td>
                  <td className='py-1 pl-2 text-right font-mono'>
                    {user.request_count}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  )
}

function CarnivalSessionBlock({
  session,
}: {
  session: CarnivalSessionSummary
}) {
  const { t } = useTranslation()
  return (
    <div className='border-border/60 rounded-md border p-3'>
      <div className='flex flex-wrap items-center justify-between gap-2'>
        <div>
          <div className='text-sm font-medium'>
            {formatDateTime(session.started_at)}
          </div>
          <div className='text-muted-foreground text-xs'>
            {t('Duration')} {formatDurationSeconds(session.duration_seconds)}
            {session.ended_at > 0 && (
              <>
                {' · '}
                {t('Ended')} {formatDateTime(session.ended_at)}
                {' · '}
                {formatDurationSeconds(session.since_end_seconds)} {t('ago')}
              </>
            )}
          </div>
        </div>
        <div className='text-right'>
          <div className='font-mono text-sm font-semibold'>
            {formatLogQuota(session.total_quota)}
          </div>
          <div className='text-muted-foreground text-xs'>
            {session.request_count} {t('Requests')}
          </div>
        </div>
      </div>
      <CarnivalAggregateBlock
        title={t('Per-user Carnival Usage')}
        aggregate={{
          total_quota: session.total_quota,
          total_tokens: session.total_tokens,
          request_count: session.request_count,
          users: session.users,
        }}
        compact
      />
    </div>
  )
}

function CarnivalHistoryDialog({
  open,
  onOpenChange,
  group,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  group: string
}) {
  const { t } = useTranslation()
  const [selectedMonth, setSelectedMonth] = useState('')

  const { data, isFetching } = useQuery({
    queryKey: ['carnival-history', group, selectedMonth],
    enabled: open,
    queryFn: async () => {
      const result = await getCarnivalHistory({
        group,
        month: selectedMonth || undefined,
      })
      if (!result.success) return null
      return result.data || null
    },
    placeholderData: (previousData) => previousData,
  })

  const selectValue = selectedMonth || data?.selected_month || 'all'
  const months = data?.months || []

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className='max-h-[82vh] overflow-hidden sm:max-w-3xl'>
        <DialogHeader>
          <DialogTitle>{t('Carnival History')}</DialogTitle>
        </DialogHeader>
        <div className='flex min-h-0 flex-col gap-3'>
          <div className='flex flex-wrap items-center gap-2'>
            <Select
              value={selectValue}
              onValueChange={(value) => setSelectedMonth(value || '')}
            >
              <SelectTrigger className='h-8 w-[180px]'>
                <SelectValue placeholder={t('Select Month')} />
              </SelectTrigger>
              <SelectContent alignItemWithTrigger={false}>
                <SelectGroup>
                  <SelectItem value='all'>{t('All Months')}</SelectItem>
                  {months.map((month) => (
                    <SelectItem key={month} value={month}>
                      {month}
                    </SelectItem>
                  ))}
                </SelectGroup>
              </SelectContent>
            </Select>
            {isFetching && (
              <span className='text-muted-foreground text-xs'>
                {t('Loading')}...
              </span>
            )}
          </div>

          <div className='min-h-0 space-y-3 overflow-auto pr-1'>
            <CarnivalAggregateBlock
              title={t('Selected Month Carnival Total')}
              aggregate={data?.month_total}
            />
            <CarnivalAggregateBlock
              title={t('All Carnival Total')}
              aggregate={data?.all_total}
            />
            <div className='space-y-3'>
              <div className='text-sm font-medium'>
                {t('Carnival Sessions')}
              </div>
              {(data?.sessions?.length || 0) === 0 ? (
                <div className='text-muted-foreground rounded-md border p-4 text-sm'>
                  {t('No data')}
                </div>
              ) : (
                data?.sessions.map((session) => (
                  <CarnivalSessionBlock key={session.id} session={session} />
                ))
              )}
            </div>
          </div>
        </div>
      </DialogContent>
    </Dialog>
  )
}

function CarnivalStats() {
  const { t } = useTranslation()
  const isAdmin = useIsAdmin()
  const queryClient = useQueryClient()
  const searchParams = route.useSearch()
  const { sensitiveVisible } = useUsageLogsContext()
  const [now, setNow] = useState(() => Date.now())
  const [historyOpen, setHistoryOpen] = useState(false)
  const selectedGroup = String(searchParams.group || '')
  const targetGroup = selectedGroup || '拼车'
  const enabled = targetGroup === '拼车'

  useEffect(() => {
    const timer = window.setInterval(() => setNow(Date.now()), 1000)
    return () => window.clearInterval(timer)
  }, [])

  const statusQuery = useQuery({
    queryKey: ['carnival-status', targetGroup],
    enabled,
    queryFn: async () => {
      const result = await getCarnivalStatus({ group: targetGroup })
      if (!result.success) return null
      return result.data || null
    },
    placeholderData: (previousData) => previousData,
    refetchInterval: 30_000,
  })

  const startMutation = useMutation({
    mutationFn: async () => {
      const result = await startCarnival({ group: targetGroup })
      if (!result.success) {
        throw new Error(result.message || t('Operation failed'))
      }
      return result.data
    },
    onSuccess: () => {
      queryClient.invalidateQueries({
        queryKey: ['carnival-status', targetGroup],
      })
      queryClient.invalidateQueries({
        queryKey: ['carnival-history', targetGroup],
      })
      queryClient.invalidateQueries({ queryKey: ['usage-logs-stats'] })
    },
    onError: (error) => {
      toast.error(
        error instanceof Error ? error.message : t('Operation failed')
      )
    },
  })

  const finishMutation = useMutation({
    mutationFn: async () => {
      const result = await finishCarnival({ group: targetGroup })
      if (!result.success) {
        throw new Error(result.message || t('Operation failed'))
      }
      return result.data
    },
    onSuccess: () => {
      queryClient.invalidateQueries({
        queryKey: ['carnival-status', targetGroup],
      })
      queryClient.invalidateQueries({
        queryKey: ['carnival-history', targetGroup],
      })
      queryClient.invalidateQueries({ queryKey: ['usage-logs-stats'] })
    },
    onError: (error) => {
      toast.error(
        error instanceof Error ? error.message : t('Operation failed')
      )
    },
  })

  if (!enabled) return null

  const active = statusQuery.data?.active
  const last = statusQuery.data?.last
  const activeElapsed = active
    ? Math.max(0, Math.floor(now / 1000) - active.started_at)
    : 0
  const actionDisabled = startMutation.isPending || finishMutation.isPending

  return (
    <>
      <span
        className={cn(
          'inline-flex h-7 items-center gap-2 rounded-md border px-2.5 text-xs shadow-xs',
          active
            ? 'border-orange-500/25 bg-orange-500/10 text-orange-700 dark:text-orange-300'
            : 'border-border/60 bg-muted/25 text-muted-foreground'
        )}
      >
        <Flame className='size-3.5' />
        <span>{active ? t('Carnival Active') : t('Carnival Idle')}</span>
        {active && (
          <span className='font-mono font-semibold tabular-nums'>
            {formatDurationSeconds(activeElapsed)}
          </span>
        )}
      </span>
      {active && (
        <StatBadge
          label={t('Carnival Usage')}
          value={sensitiveVisible ? formatLogQuota(active.total_quota) : '••••'}
          accent='bg-orange-500/70'
          title={t('Carnival usage is excluded from normal total usage')}
        />
      )}
      {!active && last && (
        <>
          <StatBadge
            label={t('Last Carnival')}
            value={sensitiveVisible ? formatLogQuota(last.total_quota) : '••••'}
            accent='bg-orange-400/70'
          />
          <span className='text-muted-foreground inline-flex h-7 items-center text-xs'>
            {t('Started')}: {formatDateTime(last.started_at)}
            {' · '}
            {t('Duration')} {formatDurationSeconds(last.duration_seconds)}
            {' · '}
            {formatDurationSeconds(last.since_end_seconds)} {t('ago')}
          </span>
        </>
      )}
      {isAdmin && (
        <Button
          variant={active ? 'outline' : 'default'}
          size='sm'
          className='h-7 gap-1.5 px-2.5 text-xs'
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
      <Button
        variant='outline'
        size='sm'
        className='h-7 gap-1.5 px-2.5 text-xs'
        onClick={() => setHistoryOpen(true)}
      >
        <History className='size-3.5' />
        {t('Carnival History')}
      </Button>
      <CarnivalHistoryDialog
        open={historyOpen}
        onOpenChange={setHistoryOpen}
        group={targetGroup}
      />
    </>
  )
}

function UpstreamUsageStats() {
  const { t } = useTranslation()
  const searchParams = route.useSearch()
  const { sensitiveVisible } = useUsageLogsContext()
  const [refreshSeq, setRefreshSeq] = useState(0)
  const [now, setNow] = useState(() => Date.now())
  const selectedGroup = String(searchParams.group || '')
  const targetGroup = selectedGroup || '拼车'
  const enabled = targetGroup === '拼车'

  useEffect(() => {
    const timer = window.setInterval(() => setNow(Date.now()), 1000)
    return () => window.clearInterval(timer)
  }, [])

  const { data, isFetching } = useQuery({
    queryKey: ['sub2api-upstream-usage', targetGroup, refreshSeq],
    enabled,
    queryFn: async () => {
      const result = await getUpstreamUsage({
        group: targetGroup,
        refresh: refreshSeq > 0,
      })
      if (!result.success) return null
      return result.data || null
    },
    placeholderData: (previousData) => previousData,
    retry: false,
    staleTime: 30_000,
  })

  const limits = useMemo(() => {
    const items = data?.rate_limits || []
    return {
      fiveHour: items.find((item) => item.window === '5h'),
      weekly: items.find((item) => item.window === '7d'),
    }
  }, [data?.rate_limits])

  if (!enabled || !data) return null

  const nextRefreshMs = (data.next_refresh_at || 0) * 1000
  const canRefresh = !isFetching && (!nextRefreshMs || now >= nextRefreshMs)
  const waitSeconds = Math.max(0, Math.ceil((nextRefreshMs - now) / 1000))
  const refreshTitle = canRefresh
    ? t('Refresh')
    : t('Refresh available in {{seconds}}s', { seconds: waitSeconds })

  return (
    <div className='flex flex-wrap items-center gap-2'>
      <StatBadge
        label='5h'
        value={
          sensitiveVisible
            ? `${formatUpstreamCost(limits.fiveHour?.used)} / ${formatUpstreamCost(limits.fiveHour?.limit)}`
            : '••••'
        }
        accent='bg-amber-500/70'
        title={`${t('Remaining')}: ${formatUpstreamCost(limits.fiveHour?.remaining)}`}
      />
      <StatBadge
        label={t('Weekly')}
        value={
          sensitiveVisible
            ? `${formatUpstreamCost(limits.weekly?.used)} / ${formatUpstreamCost(limits.weekly?.limit)}`
            : '••••'
        }
        accent='bg-emerald-500/70'
        title={`${t('Remaining')}: ${formatUpstreamCost(limits.weekly?.remaining)}`}
      />
      <span
        className='text-muted-foreground inline-flex h-7 items-center text-xs tabular-nums'
        title={`${t('Reset at:')} ${formatResetAt(limits.fiveHour?.reset_at)}`}
      >
        5h {t('Quota Reset')}{' '}
        {formatResetCountdown(limits.fiveHour?.reset_at, now)}
      </span>
      <span
        className='text-muted-foreground inline-flex h-7 items-center text-xs tabular-nums'
        title={`${t('Reset at:')} ${formatResetAt(limits.weekly?.reset_at)}`}
      >
        {t('Weekly')} {t('Quota Reset')}{' '}
        {formatResetCountdown(limits.weekly?.reset_at, now)}
      </span>
      <span className='text-muted-foreground inline-flex h-7 items-center text-xs'>
        {t('Updated')} {formatRefreshTime(data.updated_at)}
      </span>
      <Button
        variant='ghost'
        size='icon'
        className='text-muted-foreground hover:text-foreground size-7'
        disabled={!canRefresh}
        title={refreshTitle}
        aria-label={refreshTitle}
        onClick={() => {
          if (!canRefresh) return
          setRefreshSeq((value) => value + 1)
        }}
      >
        <RefreshCw className={cn(isFetching && 'animate-spin')} />
      </Button>
    </div>
  )
}

export function CommonLogsStats() {
  const { t } = useTranslation()
  const isAdmin = useIsAdmin()
  const searchParams = route.useSearch()
  const { sensitiveVisible } = useUsageLogsContext()

  const { data: stats, isLoading } = useQuery({
    queryKey: ['usage-logs-stats', isAdmin, searchParams],
    queryFn: async () => {
      const params = buildApiParams({
        page: 1,
        pageSize: 1,
        searchParams,
        columnFilters: [],
        isAdmin,
      })

      const result = isAdmin
        ? await getLogStats(params)
        : await getUserLogStats(params)

      return result.success
        ? result.data || DEFAULT_LOG_STATS
        : DEFAULT_LOG_STATS
    },
    placeholderData: (previousData) => previousData,
  })

  if (isLoading) {
    return (
      <div className='flex items-center gap-2'>
        <Skeleton className='h-7 w-[150px] rounded-md' />
        <Skeleton className='h-7 w-[100px] rounded-md' />
        <Skeleton className='h-7 w-[120px] rounded-md' />
      </div>
    )
  }

  return (
    <div className='flex flex-wrap items-center gap-2'>
      <StatBadge
        label={t('Usage')}
        value={sensitiveVisible ? formatLogQuota(stats?.quota || 0) : '••••'}
        accent='bg-sky-500/70'
      />
      <StatBadge
        label={t('RPM')}
        value={stats?.rpm || 0}
        accent='bg-rose-500/65'
      />
      <StatBadge
        label={t('TPM')}
        value={stats?.tpm || 0}
        accent='bg-slate-400/70'
      />
      <CarnivalStats />
      <UpstreamUsageStats />
    </div>
  )
}
