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
import {
  Activity,
  AlertTriangle,
  CheckCircle2,
  ChevronDown,
  Loader2,
  Radio,
  RefreshCw,
  XCircle,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { formatTimestampToDate } from '@/lib/format'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover'
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
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { SectionPageLayout } from '@/components/layout'
import { StatusBadge } from '@/components/status-badge'
import {
  getUserChannelStatus,
  probeChannelHealth,
  updateChannelHealthGroupThreshold,
  updateChannelHealthProbeInterval,
  updateChannelHealthProbeModels,
} from '@/features/channels/api'
import type {
  UserChannelStatusGroup,
  UserChannelStatusItem,
} from '@/features/channels/types'

function formatCountdown(seconds: number) {
  const value = Math.max(0, Math.floor(seconds))
  const minutes = Math.floor(value / 60)
  const remainingSeconds = value % 60
  if (minutes <= 0) return `${remainingSeconds}s`
  return `${minutes}m ${remainingSeconds.toString().padStart(2, '0')}s`
}

function remainingSeconds(item: UserChannelStatusItem, nowSeconds: number) {
  if (item.next_probe_at > 0) {
    return Math.max(0, item.next_probe_at - nowSeconds)
  }
  return Math.max(0, item.next_probe_remaining_seconds)
}

function ChannelStatusBadge({
  item,
  nowSeconds,
}: {
  item: UserChannelStatusItem
  nowSeconds: number
}) {
  const { t } = useTranslation()
  if (item.display_status === 'error') {
    const label =
      item.health_status === 'probing'
        ? t('Probing')
        : item.auto_probe_enabled
          ? `${t('Error')} ${formatCountdown(
              remainingSeconds(item, nowSeconds)
            )}`
          : t('Auto probe disabled')
    return (
      <StatusBadge
        variant='danger'
        size='sm'
        icon={item.health_status === 'probing' ? Loader2 : AlertTriangle}
        copyable={false}
        label={label}
        className={
          item.health_status === 'probing' ? '[&_svg]:animate-spin' : undefined
        }
      />
    )
  }
  if (item.display_status === 'disabled') {
    return (
      <StatusBadge
        variant='neutral'
        size='sm'
        copyable={false}
        label={t('Unavailable')}
      />
    )
  }
  return (
    <StatusBadge
      variant='success'
      size='sm'
      icon={Activity}
      copyable={false}
      label={t('Normal')}
    />
  )
}

function ModelsCell({ models }: { models: string[] }) {
  const visible = models.slice(0, 3)
  const hidden = models.length - visible.length
  if (models.length === 0) {
    return <span className='text-muted-foreground'>-</span>
  }
  return (
    <div className='flex max-w-full flex-wrap gap-1 overflow-hidden'>
      {visible.map((model) => (
        <Badge key={model} variant='secondary' className='max-w-44 truncate'>
          {model}
        </Badge>
      ))}
      {hidden > 0 && <Badge variant='outline'>+{hidden}</Badge>}
    </div>
  )
}

function ProbeModelsCell({
  item,
  isAdmin,
  saving,
  onChange,
}: {
  item: UserChannelStatusItem
  isAdmin: boolean
  saving: boolean
  onChange: (item: UserChannelStatusItem, models: string[]) => void
}) {
  const { t } = useTranslation()
  const selected = item.probe_models ?? []
  const selectedSet = new Set(selected)
  const results = item.probe_model_results ?? []
  const resultByModel = new Map(results.map((result) => [result.model, result]))

  const toggleModel = (model: string) => {
    const next = selectedSet.has(model)
      ? selected.filter((candidate) => candidate !== model)
      : [...selected, model]
    if (next.length === 0) {
      toast.error(t('Select at least one probe model'))
      return
    }
    onChange(item, next)
  }

  return (
    <div className='flex min-w-0 flex-col gap-1.5'>
      <div className='flex max-w-full flex-wrap gap-1 overflow-hidden'>
        {selected.length === 0 && (
          <span className='text-muted-foreground text-sm'>-</span>
        )}
        {selected.slice(0, 3).map((model) => {
          const result = resultByModel.get(model)
          const success = result?.status === 'healthy'
          const failed = result?.status === 'unhealthy'
          return (
            <Badge
              key={model}
              variant={failed ? 'destructive' : success ? 'secondary' : 'outline'}
              className='max-w-44 truncate'
              title={
                failed
                  ? `${model}: ${result?.last_error || t('Probe failed')}`
                  : success
                    ? `${model}: ${t('Probe succeeded')}`
                    : model
              }
            >
              {success && <CheckCircle2 />}
              {failed && <XCircle />}
              <span className='truncate'>{model}</span>
            </Badge>
          )
        })}
        {selected.length > 3 && <Badge variant='outline'>+{selected.length - 3}</Badge>}
      </div>
      {isAdmin && (
        <Popover>
          <PopoverTrigger
            render={
              <Button
                type='button'
                variant='outline'
                size='sm'
                disabled={saving}
                className='h-7 w-fit gap-1.5 px-2'
              />
            }
          >
            {saving ? (
              <Loader2 className='animate-spin' />
            ) : (
              <ChevronDown className='size-3.5' />
            )}
            {t('Probe models')}
          </PopoverTrigger>
          <PopoverContent align='start' className='max-h-80 w-80 overflow-auto'>
            <div className='flex flex-col gap-1'>
              {item.models.map((model) => (
                <label
                  key={model}
                  className='hover:bg-muted flex min-w-0 cursor-pointer items-center gap-2 rounded-md px-2 py-1.5 text-sm'
                >
                  <Checkbox
                    checked={selectedSet.has(model)}
                    disabled={saving}
                    onCheckedChange={() => toggleModel(model)}
                  />
                  <span className='truncate'>{model}</span>
                </label>
              ))}
            </div>
          </PopoverContent>
        </Popover>
      )}
    </div>
  )
}

function NumberConfigInput({
  value,
  disabled,
  saving,
  ariaLabel,
  onCommit,
}: {
  value: number
  disabled?: boolean
  saving?: boolean
  ariaLabel: string
  onCommit: (value: number) => void
}) {
  const [draft, setDraft] = useState(String(value))

  useEffect(() => {
    setDraft(String(value))
  }, [value])

  const commit = () => {
    const trimmed = draft.trim()
    if (trimmed === '') {
      setDraft(String(value))
      return
    }
    const nextValue = Number(trimmed)
    if (!Number.isInteger(nextValue)) {
      setDraft(String(value))
      return
    }
    if (nextValue !== value) {
      onCommit(nextValue)
    }
  }

  return (
    <div className='relative w-28'>
      <Input
        type='number'
        inputMode='numeric'
        value={draft}
        aria-label={ariaLabel}
        disabled={disabled || saving}
        className='h-7 pr-7 text-right tabular-nums'
        onChange={(event) => setDraft(event.target.value)}
        onBlur={commit}
        onKeyDown={(event) => {
          if (event.key === 'Enter') {
            event.currentTarget.blur()
          }
          if (event.key === 'Escape') {
            setDraft(String(value))
            event.currentTarget.blur()
          }
        }}
      />
      {saving && (
        <Loader2 className='text-muted-foreground absolute top-1.5 right-2 size-4 animate-spin' />
      )}
    </div>
  )
}

function ChannelGroupTable({
  group,
  nowSeconds,
  isAdmin,
  probingChannelID,
  savingProbeIntervalID,
  savingProbeModelsID,
  savingGroupThreshold,
  onProbe,
  onProbeIntervalChange,
  onProbeModelsChange,
  onGroupThresholdChange,
}: {
  group: UserChannelStatusGroup
  nowSeconds: number
  isAdmin: boolean
  probingChannelID: number | null
  savingProbeIntervalID: number | null
  savingProbeModelsID: number | null
  savingGroupThreshold: string | null
  onProbe: (item: UserChannelStatusItem) => void
  onProbeIntervalChange: (item: UserChannelStatusItem, value: number) => void
  onProbeModelsChange: (item: UserChannelStatusItem, models: string[]) => void
  onGroupThresholdChange: (group: UserChannelStatusGroup, value: number) => void
}) {
  const { t } = useTranslation()
  return (
    <section className='rounded-lg border bg-card'>
      <div className='flex flex-wrap items-center justify-between gap-3 border-b px-4 py-3'>
        <div className='min-w-0'>
          <div className='flex items-center gap-2'>
            <Radio className='text-muted-foreground size-4' />
            <h2 className='truncate text-sm font-semibold'>{group.group}</h2>
          </div>
          {group.group_name && (
            <p className='text-muted-foreground mt-1 text-xs'>
              {group.group_name}
            </p>
          )}
        </div>
        <div className='flex flex-wrap items-center gap-2'>
          {isAdmin && (
            <div className='flex items-center gap-2 rounded-md border px-2 py-1'>
              <span className='text-muted-foreground text-xs'>
                {t('Failure threshold')}
              </span>
              <NumberConfigInput
                value={group.failure_threshold}
                ariaLabel={t('Failure threshold')}
                saving={savingGroupThreshold === group.group}
                onCommit={(value) => onGroupThresholdChange(group, value)}
              />
            </div>
          )}
          <StatusBadge
            variant={group.display_status === 'error' ? 'danger' : 'success'}
            size='sm'
            copyable={false}
            label={
              group.display_status === 'error'
                ? t('All channels unavailable')
                : t('{{count}} available', {
                    count: group.available_count,
                  })
            }
          />
          {group.error_count > 0 && (
            <Badge variant='destructive'>
              {t('{{count}} error', { count: group.error_count })}
            </Badge>
          )}
          <Badge variant='outline'>{group.total}</Badge>
        </div>
      </div>
      <div className='overflow-x-auto'>
        <Table className='min-w-[1280px] table-fixed'>
          <TableHeader>
            <TableRow>
              <TableHead className='w-[20%]'>{t('Channel')}</TableHead>
              <TableHead className='w-48'>{t('Status')}</TableHead>
              <TableHead className='w-[22%]'>{t('Models')}</TableHead>
              <TableHead className='w-[22%]'>{t('Probe models')}</TableHead>
              <TableHead className='w-48'>{t('Last failure')}</TableHead>
              <TableHead className='w-36'>{t('Probe interval')}</TableHead>
              <TableHead className='w-20 text-right'>{t('Actions')}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {group.channels.map((item) => (
              <TableRow key={`${group.group}-${item.channel_id}`}>
                <TableCell className='min-w-0 font-medium'>
                  <div className='truncate'>
                    {item.channel_name || `#${item.channel_id}`}
                  </div>
                </TableCell>
                <TableCell className='w-44'>
                  <ChannelStatusBadge item={item} nowSeconds={nowSeconds} />
                </TableCell>
                <TableCell className='whitespace-normal'>
                  <ModelsCell models={item.models} />
                </TableCell>
                <TableCell className='whitespace-normal'>
                  <ProbeModelsCell
                    item={item}
                    isAdmin={isAdmin}
                    saving={savingProbeModelsID === item.channel_id}
                    onChange={onProbeModelsChange}
                  />
                </TableCell>
                <TableCell className='text-muted-foreground w-48'>
                  {item.last_failure_at
                    ? formatTimestampToDate(item.last_failure_at)
                    : '-'}
                </TableCell>
                <TableCell className='w-36'>
                  {isAdmin ? (
                    <NumberConfigInput
                      value={item.probe_interval_seconds}
                      ariaLabel={t('Probe interval')}
                      saving={savingProbeIntervalID === item.channel_id}
                      onCommit={(value) =>
                        onProbeIntervalChange(item, value)
                      }
                    />
                  ) : (
                    <span className='text-muted-foreground text-sm tabular-nums'>
                      {item.probe_interval_seconds}
                    </span>
                  )}
                </TableCell>
                <TableCell className='w-20 text-right'>
                  {isAdmin && item.can_probe && (
                    <TooltipProvider delay={100}>
                      <Tooltip>
                        <TooltipTrigger
                          render={
                            <Button
                              type='button'
                              variant='ghost'
                              size='icon-sm'
                              onClick={() => onProbe(item)}
                              disabled={probingChannelID === item.channel_id}
                            />
                          }
                        >
                          {probingChannelID === item.channel_id ? (
                            <Loader2 className='animate-spin' />
                          ) : (
                            <RefreshCw />
                          )}
                        </TooltipTrigger>
                        <TooltipContent side='left'>
                          {t('Probe now')}
                        </TooltipContent>
                      </Tooltip>
                    </TooltipProvider>
                  )}
                </TableCell>
              </TableRow>
            ))}
            {group.channels.length === 0 && (
              <TableRow>
                <TableCell
                  colSpan={7}
                  className='text-muted-foreground h-24 text-center'
                >
                  {t('No channels available')}
                </TableCell>
              </TableRow>
            )}
          </TableBody>
        </Table>
      </div>
    </section>
  )
}

export function ChannelStatus() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const userRole = useAuthStore((state) => state.auth.user?.role)
  const isAdmin = Boolean(userRole && userRole >= ROLE.ADMIN)
  const [nowSeconds, setNowSeconds] = useState(() =>
    Math.floor(Date.now() / 1000)
  )
  const [probingChannelID, setProbingChannelID] = useState<number | null>(null)
  const [savingProbeIntervalID, setSavingProbeIntervalID] = useState<
    number | null
  >(null)
  const [savingProbeModelsID, setSavingProbeModelsID] = useState<number | null>(
    null
  )
  const [savingGroupThreshold, setSavingGroupThreshold] = useState<
    string | null
  >(null)
  const { data, isLoading } = useQuery({
    queryKey: ['channel-status'],
    queryFn: getUserChannelStatus,
    refetchInterval: (query) => {
      const groups = query.state.data?.data?.groups ?? []
      const hasError = groups.some((group) =>
        group.channels.some((item) => item.display_status === 'error')
      )
      return hasError ? 5_000 : 30_000
    },
  })
  const probeMutation = useMutation({
    mutationFn: async (item: UserChannelStatusItem) => {
      setProbingChannelID(item.channel_id)
      return probeChannelHealth(item.channel_id)
    },
    onSuccess: (response) => {
      if (response.success) {
        toast.success(t('Channel recovered'))
      } else {
        toast.error(response.message || t('Channel probe failed'))
      }
      queryClient.invalidateQueries({ queryKey: ['channel-status'] })
      queryClient.invalidateQueries({ queryKey: ['channels'] })
    },
    onError: () => {
      toast.error(t('Channel probe failed'))
    },
    onSettled: () => {
      setProbingChannelID(null)
    },
  })
  const probeIntervalMutation = useMutation({
    mutationFn: async ({
      channelID,
      value,
    }: {
      channelID: number
      value: number
    }) => {
      setSavingProbeIntervalID(channelID)
      return updateChannelHealthProbeInterval(channelID, value)
    },
    onSuccess: (response) => {
      if (response.success) {
        toast.success(t('Probe interval updated'))
      } else {
        toast.error(response.message || t('Failed to update probe interval'))
      }
      queryClient.invalidateQueries({ queryKey: ['channel-status'] })
      queryClient.invalidateQueries({ queryKey: ['channels'] })
    },
    onError: () => {
      toast.error(t('Failed to update probe interval'))
    },
    onSettled: () => {
      setSavingProbeIntervalID(null)
    },
  })
  const groupThresholdMutation = useMutation({
    mutationFn: async ({
      group,
      value,
    }: {
      group: string
      value: number
    }) => {
      setSavingGroupThreshold(group)
      return updateChannelHealthGroupThreshold(group, value)
    },
    onSuccess: (response) => {
      if (response.success) {
        toast.success(t('Failure threshold updated'))
      } else {
        toast.error(response.message || t('Failed to update failure threshold'))
      }
      queryClient.invalidateQueries({ queryKey: ['channel-status'] })
      queryClient.invalidateQueries({ queryKey: ['channels'] })
    },
    onError: () => {
      toast.error(t('Failed to update failure threshold'))
    },
    onSettled: () => {
      setSavingGroupThreshold(null)
    },
  })
  const probeModelsMutation = useMutation({
    mutationFn: async ({
      channelID,
      models,
    }: {
      channelID: number
      models: string[]
    }) => {
      setSavingProbeModelsID(channelID)
      return updateChannelHealthProbeModels(channelID, models)
    },
    onSuccess: (response) => {
      if (response.success) {
        toast.success(t('Probe models updated'))
      } else {
        toast.error(response.message || t('Failed to update probe models'))
      }
      queryClient.invalidateQueries({ queryKey: ['channel-status'] })
      queryClient.invalidateQueries({ queryKey: ['channels'] })
    },
    onError: () => {
      toast.error(t('Failed to update probe models'))
    },
    onSettled: () => {
      setSavingProbeModelsID(null)
    },
  })

  useEffect(() => {
    const timer = window.setInterval(() => {
      setNowSeconds(Math.floor(Date.now() / 1000))
    }, 1000)
    return () => window.clearInterval(timer)
  }, [])

  const groups = useMemo(
    () => data?.data?.groups ?? [],
    [data?.data?.groups]
  )
  const serverClientOffset = useMemo(() => {
    const serverUpdatedAt = data?.data?.updated_at
    if (!serverUpdatedAt) return 0
    return Math.floor(Date.now() / 1000) - serverUpdatedAt
  }, [data?.data?.updated_at])
  const displayNowSeconds = nowSeconds - serverClientOffset

  useEffect(() => {
    if (!groups.length) return
    const shouldRefetchSoon = groups.some((group) =>
      group.channels.some(
        (item) =>
          item.display_status === 'error' &&
          item.auto_probe_enabled &&
          item.health_status !== 'probing' &&
          remainingSeconds(item, displayNowSeconds) <= 0
      )
    )
    if (!shouldRefetchSoon) return
    const timer = window.setTimeout(() => {
      queryClient.invalidateQueries({ queryKey: ['channel-status'] })
    }, 1500)
    return () => window.clearTimeout(timer)
  }, [displayNowSeconds, groups, queryClient])

  return (
    <SectionPageLayout fixedContent>
      <SectionPageLayout.Title>{t('Channel Status')}</SectionPageLayout.Title>
      <SectionPageLayout.Content>
        <div className='flex h-full min-h-0 flex-col gap-4 overflow-auto'>
          {isLoading && (
            <div className='text-muted-foreground rounded-lg border p-6 text-sm'>
              {t('Loading...')}
            </div>
          )}
          {!isLoading && groups.length === 0 && (
            <div className='text-muted-foreground rounded-lg border p-6 text-sm'>
              {t('No channels available')}
            </div>
          )}
          {groups.map((group) => (
            <ChannelGroupTable
              key={group.group}
              group={group}
              nowSeconds={displayNowSeconds}
              isAdmin={isAdmin}
              probingChannelID={probingChannelID}
              savingProbeIntervalID={savingProbeIntervalID}
              savingProbeModelsID={savingProbeModelsID}
              savingGroupThreshold={savingGroupThreshold}
              onProbe={(item) => probeMutation.mutate(item)}
              onProbeIntervalChange={(item, value) =>
                probeIntervalMutation.mutate({
                  channelID: item.channel_id,
                  value,
                })
              }
              onProbeModelsChange={(item, models) =>
                probeModelsMutation.mutate({
                  channelID: item.channel_id,
                  models,
                })
              }
              onGroupThresholdChange={(group, value) =>
                groupThresholdMutation.mutate({
                  group: group.group,
                  value,
                })
              }
            />
          ))}
        </div>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}
