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
import { useEffect, useMemo, useRef, useState } from 'react'
import { useForm, type SubmitErrorHandler } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { useQuery } from '@tanstack/react-query'
import {
  ArrowDown,
  ArrowUp,
  ChevronDown,
  KeyRound,
  Pencil,
  Plus,
  Settings2,
  Trash2,
  WalletCards,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { getUserModels, getUserGroups } from '@/lib/api'
import { getCurrencyDisplay, getCurrencyLabel } from '@/lib/currency'
import { cn } from '@/lib/utils'
import { useStatus } from '@/hooks/use-status'
import { Button } from '@/components/ui/button'
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import {
  Sheet,
  SheetClose,
  SheetContent,
  SheetDescription,
  SheetFooter,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'
import { DateTimePicker } from '@/components/datetime-picker'
import {
  SideDrawerSection,
  SideDrawerSectionHeader,
  sideDrawerContentClassName,
  sideDrawerFooterClassName,
  sideDrawerFormClassName,
  sideDrawerHeaderClassName,
  sideDrawerSwitchItemClassName,
} from '@/components/drawer-layout'
import { MultiSelect } from '@/components/multi-select'
import { createApiKey, updateApiKey, getApiKey } from '../api'
import { ERROR_MESSAGES, SUCCESS_MESSAGES } from '../constants'
import {
  getApiKeyFormSchema,
  type ApiKeyFormValues,
  getApiKeyFormDefaultValues,
  transformFormDataToPayload,
  transformApiKeyToFormDefaults,
} from '../lib'
import { type ApiKey, type ApiKeyGroupRoute } from '../types'
import {
  ApiKeyGroupCombobox,
  type ApiKeyGroupOption,
} from './api-key-group-combobox'
import { useApiKeys } from './api-keys-provider'

type ApiKeyMutateDrawerProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  currentRow?: ApiKey
}

export function ApiKeysMutateDrawer({
  open,
  onOpenChange,
  currentRow,
}: ApiKeyMutateDrawerProps) {
  const { t } = useTranslation()
  const isUpdate = !!currentRow
  const { triggerRefresh } = useApiKeys()
  const { status } = useStatus()
  const [isSubmitting, setIsSubmitting] = useState(false)
  const [advancedOpen, setAdvancedOpen] = useState(false)
  const defaultUseAutoGroup = status?.default_use_auto_group === true

  // Fetch models
  const { data: modelsData } = useQuery({
    queryKey: ['user-models'],
    queryFn: getUserModels,
    staleTime: 5 * 60 * 1000, // Cache for 5 minutes
  })

  // Fetch groups
  const { data: groupsData } = useQuery({
    queryKey: ['user-groups'],
    queryFn: getUserGroups,
    staleTime: 5 * 60 * 1000,
  })

  const models = modelsData?.data || []
  const groupsRaw = groupsData?.data || {}
  const groups: ApiKeyGroupOption[] = Object.entries(groupsRaw).map(
    ([key, info]) => ({
      value: key,
      label: key,
      desc: info.desc || key,
      ratio: info.ratio,
    })
  )
  const backendHasAuto = groups.some((g) => g.value === 'auto')
  const schema = getApiKeyFormSchema(t)

  const form = useForm<ApiKeyFormValues>({
    resolver: zodResolver(schema),
    defaultValues: getApiKeyFormDefaultValues(defaultUseAutoGroup),
  })

  // Load existing data when updating
  useEffect(() => {
    let ignore = false
    if (open && isUpdate && currentRow) {
      form.reset(transformApiKeyToFormDefaults(currentRow))
      getApiKey(currentRow.id).then((result) => {
        if (!ignore && result.success && result.data) {
          form.reset(transformApiKeyToFormDefaults(result.data))
        }
      })
    } else if (open && !isUpdate) {
      form.reset(
        getApiKeyFormDefaultValues(defaultUseAutoGroup && backendHasAuto)
      )
    }

    return () => {
      ignore = true
    }
  }, [open, isUpdate, currentRow, form, defaultUseAutoGroup, backendHasAuto])

  // Correct groups after groups load: if a value is not available, fall back
  useEffect(() => {
    if (!open) return
    if (groups.length === 0) return
    const available = new Set(groups.map((g) => g.value))
    const fallback =
      groups.find((g) => g.value === 'default')?.value ?? groups[0]?.value ?? ''
    const routes = form.getValues('group_routes') || []
    const normalizedRoutes = routes
      .filter((route) => route.group.trim())
      .map((route, index) => ({ ...route, group: route.group, order: index + 1 }))
    const nextRoutes = isUpdate
      ? normalizedRoutes
      : normalizedRoutes.filter((route) => available.has(route.group))
    if (nextRoutes.length === 0 && fallback) {
      nextRoutes.push({ group: fallback, order: 1 })
    }
    if (JSON.stringify(routes) !== JSON.stringify(nextRoutes)) {
      form.setValue('group_routes', nextRoutes)
      form.setValue('group', nextRoutes[0]?.group || '')
    }
  }, [open, isUpdate, groups, form])

  const onSubmit = async (data: ApiKeyFormValues) => {
    setIsSubmitting(true)
    try {
      const basePayload = transformFormDataToPayload(data)

      if (isUpdate && currentRow) {
        const result = await updateApiKey({
          ...basePayload,
          id: currentRow.id,
        })
        if (result.success) {
          toast.success(t(SUCCESS_MESSAGES.API_KEY_UPDATED))
          onOpenChange(false)
          triggerRefresh()
        } else {
          toast.error(result.message || t(ERROR_MESSAGES.UPDATE_FAILED))
        }
      } else {
        // Create mode - handle batch creation
        const count = data.tokenCount || 1
        let successCount = 0

        for (let i = 0; i < count; i++) {
          const result = await createApiKey({
            ...basePayload,
            name:
              i === 0 && data.name
                ? data.name
                : `${data.name || 'default'}-${Math.random().toString(36).slice(2, 8)}`,
          })
          if (result.success) {
            successCount++
          } else {
            toast.error(result.message || t(ERROR_MESSAGES.CREATE_FAILED))
            break
          }
        }

        if (successCount > 0) {
          toast.success(
            t('Successfully created {{count}} API Key(s)', {
              count: successCount,
            })
          )
          onOpenChange(false)
          triggerRefresh()
        }
      }
    } catch (_error) {
      toast.error(t(ERROR_MESSAGES.UNEXPECTED))
    } finally {
      setIsSubmitting(false)
    }
  }

  const onInvalid: SubmitErrorHandler<ApiKeyFormValues> = () => {
    toast.error(t('Please fix the highlighted fields before saving'))
  }

  const handleSetExpiry = (months: number, days: number, hours: number) => {
    if (months === 0 && days === 0 && hours === 0) {
      form.setValue('expired_time', undefined)
      return
    }

    const now = new Date()
    now.setMonth(now.getMonth() + months)
    now.setDate(now.getDate() + days)
    now.setHours(now.getHours() + hours)

    form.setValue('expired_time', now)
  }

  const { meta: currencyMeta } = getCurrencyDisplay()
  const currencyLabel = getCurrencyLabel()
  const tokensOnly = currencyMeta.kind === 'tokens'
  const quotaLabel = t('Quota ({{currency}})', { currency: currencyLabel })
  const quotaPlaceholder = tokensOnly
    ? t('Enter quota in tokens')
    : t('Enter quota in {{currency}}', { currency: currencyLabel })
  const groupRoutes = form.watch('group_routes') || []
  const selectedGroup = groupRoutes[0]?.group || form.watch('group')
  const unlimitedQuota = form.watch('unlimited_quota')
  const selectedGroupSet = useMemo(
    () => new Set(groupRoutes.map((route) => route.group)),
    [groupRoutes]
  )
  const addableGroups = useMemo(
    () => groups.filter((group) => !selectedGroupSet.has(group.value)),
    [groups, selectedGroupSet]
  )
  const [pendingGroup, setPendingGroup] = useState('')
  const [editingGroupRouteIndex, setEditingGroupRouteIndex] = useState<
    number | null
  >(null)
  const groupEditorRef = useRef<HTMLDivElement | null>(null)

  useEffect(() => {
    if (addableGroups.length === 0) {
      if (pendingGroup) setPendingGroup('')
      return
    }
    if (
      !pendingGroup ||
      !addableGroups.some((group) => group.value === pendingGroup)
    ) {
      setPendingGroup(addableGroups[0]?.value || '')
    }
  }, [addableGroups, pendingGroup])

  useEffect(() => {
    if (groupRoutes.length === 0) {
      setEditingGroupRouteIndex(null)
      return
    }
    if (
      editingGroupRouteIndex !== null &&
      editingGroupRouteIndex >= groupRoutes.length
    ) {
      setEditingGroupRouteIndex(groupRoutes.length - 1)
    }
  }, [editingGroupRouteIndex, groupRoutes.length])

  useEffect(() => {
    if (!open) return
    const handlePointerDown = (event: PointerEvent) => {
      const target = event.target
      if (
        target instanceof Node &&
        groupEditorRef.current &&
        !groupEditorRef.current.contains(target)
      ) {
        setEditingGroupRouteIndex(null)
      }
    }
    document.addEventListener('pointerdown', handlePointerDown)
    return () => document.removeEventListener('pointerdown', handlePointerDown)
  }, [open])

  const setGroupRoutes = (routes: ApiKeyGroupRoute[]) => {
    const normalized = routes.map((route, index) => ({
      ...route,
      group: route.group,
      order: index + 1,
    }))
    form.setValue('group_routes', normalized, { shouldDirty: true })
    form.setValue('group', normalized[0]?.group || '', { shouldDirty: true })
  }

  const addGroupRoute = () => {
    if (!pendingGroup || selectedGroupSet.has(pendingGroup)) return
    const nextRoutes = [
      ...groupRoutes,
      {
        group: pendingGroup,
        order: groupRoutes.length + 1,
        failover_strategy: form.getValues('failover_strategy'),
      },
    ]
    setGroupRoutes(nextRoutes)
    setEditingGroupRouteIndex(nextRoutes.length - 1)
  }

  const updateGroupRoute = (
    index: number,
    updates: Partial<ApiKeyGroupRoute>
  ) => {
    const next = groupRoutes.map((route, routeIndex) =>
      routeIndex === index ? { ...route, ...updates } : route
    )
    setGroupRoutes(next)
  }

  const updateGroupRouteOrder = (index: number, order: number) => {
    const boundedOrder = Math.min(Math.max(order || 1, 1), groupRoutes.length)
    const next = [...groupRoutes]
    const [route] = next.splice(index, 1)
    next.splice(boundedOrder - 1, 0, route)
    setGroupRoutes(next)
    setEditingGroupRouteIndex(boundedOrder - 1)
  }

  const moveGroupRoute = (index: number, direction: -1 | 1) => {
    const nextIndex = index + direction
    if (nextIndex < 0 || nextIndex >= groupRoutes.length) return
    const next = [...groupRoutes]
    const current = next[index]
    next[index] = next[nextIndex]
    next[nextIndex] = current
    setGroupRoutes(next)
  }

  const removeGroupRoute = (index: number) => {
    if (groupRoutes.length <= 1) return
    if (editingGroupRouteIndex === index) setEditingGroupRouteIndex(null)
    if (editingGroupRouteIndex !== null && editingGroupRouteIndex > index) {
      setEditingGroupRouteIndex(editingGroupRouteIndex - 1)
    }
    setGroupRoutes(groupRoutes.filter((_, routeIndex) => routeIndex !== index))
  }

  return (
    <Sheet
      open={open}
      onOpenChange={(v) => {
        onOpenChange(v)
        if (!v) {
          form.reset()
        }
      }}
    >
      <SheetContent
        className={sideDrawerContentClassName('max-w-none sm:!max-w-[620px]')}
      >
        <SheetHeader className={sideDrawerHeaderClassName()}>
          <SheetTitle>
            {isUpdate ? t('Update API Key') : t('Create API Key')}
          </SheetTitle>
          <SheetDescription>
            {isUpdate
              ? t('Update the API key by providing necessary info.')
              : t('Add a new API key by providing necessary info.')}
          </SheetDescription>
        </SheetHeader>
        <Form {...form}>
          <form
            id='api-key-form'
            onSubmit={form.handleSubmit(onSubmit, onInvalid)}
            className={sideDrawerFormClassName('gap-5')}
          >
            <SideDrawerSection>
              <SideDrawerSectionHeader
                title={t('Basic Information')}
                description={t('Set API key basic information')}
                icon={<KeyRound className='size-4' />}
              />
              <FormField
                control={form.control}
                name='name'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Name')}</FormLabel>
                    <FormControl>
                      <Input {...field} placeholder={t('Enter a name')} />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />

              <FormField
                control={form.control}
                name='group_routes'
                render={() => (
                  <FormItem>
                    <FormLabel>{t('Groups')}</FormLabel>
                    <FormControl>
                      <div ref={groupEditorRef} className='space-y-4'>
                        <div className='space-y-2'>
                          <div className='flex items-center justify-between gap-2'>
                            <div className='text-sm font-medium'>
                              {t('Selected groups')}
                            </div>
                            <div className='text-muted-foreground text-xs'>
                              {t(
                                'Click a group to edit its group and priority.'
                              )}
                            </div>
                          </div>
                          {groupRoutes.map((route, index) => {
                            const option = groups.find(
                              (group) => group.value === route.group
                            )
                            const routeOption =
                              option ?? {
                                value: route.group,
                                label: route.group,
                                desc: t('Current group'),
                              }
                            const editing = editingGroupRouteIndex === index
                            const routeOptions = [
                              ...addableGroups,
                              routeOption,
                            ].sort((a, b) => a.label.localeCompare(b.label))
                            return (
                              <div
                                key={route.group}
                                className={cn(
                                  'border-border bg-muted/25 rounded-lg border p-2 transition-colors',
                                  editing &&
                                    'border-primary/50 bg-primary/5 ring-primary/15 ring-2'
                                )}
                                onClick={() => setEditingGroupRouteIndex(index)}
                              >
                                <div className='grid grid-cols-[2rem_minmax(0,1fr)_auto] items-center gap-2'>
                                  <span className='text-muted-foreground text-center text-sm tabular-nums'>
                                    {index + 1}
                                  </span>
                                  <div className='min-w-0'>
                                    <div className='truncate text-sm font-medium'>
                                      {routeOption.label}
                                    </div>
                                    {routeOption.desc && (
                                      <div className='text-muted-foreground truncate text-xs'>
                                        {routeOption.desc}
                                      </div>
                                    )}
                                  </div>
                                  <div className='flex items-center gap-1'>
                                    <Button
                                      type='button'
                                      variant='ghost'
                                      size='icon'
                                      className='size-8'
                                      aria-label={t('Edit group route')}
                                      onClick={(event) => {
                                        event.stopPropagation()
                                        setEditingGroupRouteIndex(index)
                                      }}
                                    >
                                      <Pencil className='size-4' />
                                    </Button>
                                    <Button
                                      type='button'
                                      variant='ghost'
                                      size='icon'
                                      className='size-8'
                                      aria-label={t('Move up')}
                                      disabled={index === 0}
                                      onClick={(event) => {
                                        event.stopPropagation()
                                        moveGroupRoute(index, -1)
                                        setEditingGroupRouteIndex(index - 1)
                                      }}
                                    >
                                      <ArrowUp className='size-4' />
                                    </Button>
                                    <Button
                                      type='button'
                                      variant='ghost'
                                      size='icon'
                                      className='size-8'
                                      aria-label={t('Move down')}
                                      disabled={
                                        index === groupRoutes.length - 1
                                      }
                                      onClick={(event) => {
                                        event.stopPropagation()
                                        moveGroupRoute(index, 1)
                                        setEditingGroupRouteIndex(index + 1)
                                      }}
                                    >
                                      <ArrowDown className='size-4' />
                                    </Button>
                                    <Button
                                      type='button'
                                      variant='ghost'
                                      size='icon'
                                      className='text-destructive hover:text-destructive size-8'
                                      aria-label={t('Remove')}
                                      disabled={groupRoutes.length <= 1}
                                      onClick={(event) => {
                                        event.stopPropagation()
                                        removeGroupRoute(index)
                                      }}
                                    >
                                      <Trash2 className='size-4' />
                                    </Button>
                                  </div>
                                </div>
                                {editing && (
                                  <div
                                    className='border-border mt-3 space-y-3 border-t pt-3'
                                    onClick={(event) => event.stopPropagation()}
                                  >
                                    <div className='text-xs font-medium'>
                                      {t('Selected group settings')}
                                    </div>
                                    <div className='grid gap-3 sm:grid-cols-[minmax(0,1fr)_8rem]'>
                                      <div className='space-y-1.5'>
                                        <FormLabel className='text-xs'>
                                          {t('Group')}
                                        </FormLabel>
                                        <ApiKeyGroupCombobox
                                          options={routeOptions}
                                          value={route.group}
                                          onValueChange={(value) => {
                                            if (
                                              value &&
                                              value !== route.group &&
                                              selectedGroupSet.has(value)
                                            ) {
                                              return
                                            }
                                            updateGroupRoute(index, {
                                              group: value,
                                            })
                                          }}
                                          placeholder={t('Select a group')}
                                        />
                                      </div>
                                      <div className='space-y-1.5'>
                                        <FormLabel className='text-xs'>
                                          {t('Priority')}
                                        </FormLabel>
                                        <Input
                                          type='number'
                                          min='1'
                                          max={groupRoutes.length}
                                          value={index + 1}
                                          onChange={(event) =>
                                            updateGroupRouteOrder(
                                              index,
                                              parseInt(
                                                event.target.value,
                                                10
                                              ) || 1
                                            )
                                          }
                                        />
                                      </div>
                                    </div>
                                    <div className='grid gap-3'>
                                      <div className='space-y-1.5'>
                                        <FormLabel className='text-xs'>
                                          {t('Failure strategy')}
                                        </FormLabel>
                                        <NativeSelect
                                          className='w-full'
                                          value={
                                            route.failover_strategy ||
                                            'fallback'
                                          }
                                          onChange={(event) =>
                                            updateGroupRoute(index, {
                                              failover_strategy: event.target
                                                .value as ApiKeyGroupRoute['failover_strategy'],
                                            })
                                          }
                                        >
                                          <NativeSelectOption value='fallback'>
                                            {t('Switch silently')}
                                          </NativeSelectOption>
                                          <NativeSelectOption value='return_error'>
                                            {t('Return error')}
                                          </NativeSelectOption>
                                        </NativeSelect>
                                      </div>
                                    </div>
                                  </div>
                                )}
                              </div>
                            )
                          })}
                        </div>
                        <div
                          className='border-border space-y-2 rounded-lg border border-dashed p-3'
                          onPointerDown={() => setEditingGroupRouteIndex(null)}
                        >
                          <div className='text-sm font-medium'>
                            {t('Add group')}
                          </div>
                          <div className='grid gap-2 sm:grid-cols-[minmax(0,1fr)_auto]'>
                            <ApiKeyGroupCombobox
                              options={addableGroups}
                              value={pendingGroup}
                              onValueChange={setPendingGroup}
                              placeholder={t('Select a group')}
                              disabled={addableGroups.length === 0}
                            />
                            <Button
                              type='button'
                              variant='outline'
                              className='gap-2'
                              disabled={
                                !pendingGroup || addableGroups.length === 0
                              }
                              onClick={addGroupRoute}
                            >
                              <Plus className='size-4' />
                              {t('Add')}
                            </Button>
                          </div>
                        </div>
                      </div>
                    </FormControl>
                    <FormDescription>
                      {t(
                        'Lower numbers are tried first. Channel health is evaluated globally by the system.'
                      )}
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />

              {selectedGroup === 'auto' && (
                <FormField
                  control={form.control}
                  name='cross_group_retry'
                  render={({ field }) => (
                    <FormItem className={sideDrawerSwitchItemClassName()}>
                      <div className='flex flex-col gap-0.5'>
                        <FormLabel className='text-sm'>
                          {t('Cross-group retry')}
                        </FormLabel>
                        <FormDescription className='line-clamp-2 text-xs sm:line-clamp-none'>
                          {t(
                            'When enabled, if channels in the current group fail, it will try channels in the next group in order.'
                          )}
                        </FormDescription>
                      </div>
                      <FormControl>
                        <Switch
                          checked={!!field.value}
                          onCheckedChange={field.onChange}
                        />
                      </FormControl>
                    </FormItem>
                  )}
                />
              )}

              <FormField
                control={form.control}
                name='expired_time'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Expiration Time')}</FormLabel>
                    <div className='grid gap-2 sm:grid-cols-[minmax(0,1fr)_auto] sm:items-center'>
                      <FormControl>
                        <DateTimePicker
                          value={field.value}
                          onChange={field.onChange}
                          placeholder={t('Never expires')}
                          className='min-w-0 [&_input[type=time]]:w-24 sm:[&_input[type=time]]:w-32'
                        />
                      </FormControl>
                      <div className='grid grid-cols-4 gap-2 sm:flex'>
                        <Button
                          type='button'
                          variant='outline'
                          size='sm'
                          className='px-2 text-xs sm:px-3 sm:text-sm'
                          onClick={() => handleSetExpiry(0, 0, 0)}
                        >
                          {t('Never')}
                        </Button>
                        <Button
                          type='button'
                          variant='outline'
                          size='sm'
                          className='px-2 text-xs sm:px-3 sm:text-sm'
                          onClick={() => handleSetExpiry(1, 0, 0)}
                        >
                          {t('1 Month')}
                        </Button>
                        <Button
                          type='button'
                          variant='outline'
                          size='sm'
                          className='px-2 text-xs sm:px-3 sm:text-sm'
                          onClick={() => handleSetExpiry(0, 1, 0)}
                        >
                          {t('1 Day')}
                        </Button>
                        <Button
                          type='button'
                          variant='outline'
                          size='sm'
                          className='px-2 text-xs sm:px-3 sm:text-sm'
                          onClick={() => handleSetExpiry(0, 0, 1)}
                        >
                          {t('1 Hour')}
                        </Button>
                      </div>
                    </div>
                    <FormMessage />
                  </FormItem>
                )}
              />

              {!isUpdate && (
                <FormField
                  control={form.control}
                  name='tokenCount'
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{t('Quantity')}</FormLabel>
                      <FormControl>
                        <Input
                          {...field}
                          type='number'
                          min='1'
                          placeholder={t('Number of keys to create')}
                          onChange={(e) =>
                            field.onChange(parseInt(e.target.value, 10) || 1)
                          }
                        />
                      </FormControl>
                      <FormDescription>
                        {t(
                          'Create multiple API keys at once (random suffix will be added to names)'
                        )}
                      </FormDescription>
                      <FormMessage />
                    </FormItem>
                  )}
                />
              )}
            </SideDrawerSection>

            <SideDrawerSection>
              <SideDrawerSectionHeader
                title={t('Quota Settings')}
                description={t('Set quota amount and limits')}
                icon={<WalletCards className='size-4' />}
              />
              {!unlimitedQuota && (
                <FormField
                  control={form.control}
                  name='remain_quota_dollars'
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{quotaLabel}</FormLabel>
                      <FormControl>
                        <Input
                          {...field}
                          type='number'
                          step={tokensOnly ? 1 : 0.01}
                          placeholder={quotaPlaceholder}
                          onChange={(e) =>
                            field.onChange(parseFloat(e.target.value) || 0)
                          }
                        />
                      </FormControl>
                      <FormDescription>
                        {tokensOnly
                          ? t('Enter the quota amount in tokens')
                          : t('Enter the quota amount in {{currency}}', {
                              currency: currencyLabel,
                            })}
                      </FormDescription>
                      <FormMessage />
                    </FormItem>
                  )}
                />
              )}

              <FormField
                control={form.control}
                name='unlimited_quota'
                render={({ field }) => (
                  <FormItem className={sideDrawerSwitchItemClassName()}>
                    <div className='flex flex-col gap-0.5'>
                      <FormLabel className='text-sm'>
                        {t('Unlimited Quota')}
                      </FormLabel>
                      <FormDescription className='text-xs'>
                        {t('Enable unlimited quota for this API key')}
                      </FormDescription>
                    </div>
                    <FormControl>
                      <Switch
                        checked={field.value}
                        onCheckedChange={field.onChange}
                      />
                    </FormControl>
                  </FormItem>
                )}
              />
            </SideDrawerSection>

            <Collapsible open={advancedOpen} onOpenChange={setAdvancedOpen}>
              <SideDrawerSection>
                <CollapsibleTrigger
                  render={
                    <button
                      type='button'
                      className='hover:bg-muted/40 flex w-full items-center gap-3 rounded-md py-1.5 text-left transition-colors'
                    />
                  }
                >
                  <SideDrawerSectionHeader
                    className='flex-1'
                    title={t('Advanced Settings')}
                    description={t('Set API key access restrictions')}
                    icon={<Settings2 className='size-4' />}
                  />
                  <ChevronDown
                    className={cn(
                      'text-muted-foreground size-4 shrink-0 transition-transform',
                      advancedOpen && 'rotate-180'
                    )}
                  />
                </CollapsibleTrigger>
                <CollapsibleContent>
                  <div className='flex flex-col gap-4 pt-2'>
                    <FormField
                      control={form.control}
                      name='model_limits'
                      render={({ field }) => (
                        <FormItem>
                          <FormLabel>{t('Model Limits')}</FormLabel>
                          <FormControl>
                            <MultiSelect
                              options={models.map((m) => ({
                                label: m,
                                value: m,
                              }))}
                              selected={field.value}
                              onChange={field.onChange}
                              placeholder={t(
                                'Select models (empty for allow all)'
                              )}
                            />
                          </FormControl>
                          <FormDescription>
                            {t('Limit which models can be used with this key')}
                          </FormDescription>
                          <FormMessage />
                        </FormItem>
                      )}
                    />

                    <FormField
                      control={form.control}
                      name='allow_ips'
                      render={({ field }) => (
                        <FormItem>
                          <FormLabel>
                            {t('IP Whitelist (supports CIDR)')}
                          </FormLabel>
                          <FormControl>
                            <Textarea
                              {...field}
                              className='min-h-20 resize-none'
                              placeholder={t(
                                'One IP per line (empty for no restriction)'
                              )}
                              rows={3}
                            />
                          </FormControl>
                          <FormDescription>
                            {t(
                              'Do not over-trust this feature. IP may be spoofed. Please use with nginx, CDN and other gateways.'
                            )}
                          </FormDescription>
                          <FormMessage />
                        </FormItem>
                      )}
                    />
                  </div>
                </CollapsibleContent>
              </SideDrawerSection>
            </Collapsible>
          </form>
        </Form>
        <SheetFooter className={sideDrawerFooterClassName()}>
          <SheetClose
            render={<Button variant='outline' className='w-full sm:w-auto' />}
          >
            {t('Close')}
          </SheetClose>
          <Button
            type='button'
            onClick={form.handleSubmit(onSubmit, onInvalid)}
            disabled={isSubmitting}
            className='w-full sm:w-auto'
          >
            {isSubmitting ? t('Saving...') : t('Save changes')}
          </Button>
        </SheetFooter>
      </SheetContent>
    </Sheet>
  )
}
