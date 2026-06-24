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
import { useQueryClient } from '@tanstack/react-query'
import type {
  ColumnDef,
  RowSelectionState,
  Table as TanStackTable,
} from '@tanstack/react-table'
import {
  Check,
  CheckCircle2,
  Copy,
  Gauge,
  Info,
  Loader2,
  Settings,
  Trash2,
} from 'lucide-react'
import {
  type ChangeEvent,
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
} from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'
import {
  DataTableBulkActions as BulkActionsToolbar,
  DataTablePagination,
  DataTableView,
  useDataTable,
} from '@/components/data-table'
import { Dialog } from '@/components/dialog'
import {
  sideDrawerContentClassName,
  sideDrawerFooterClassName,
  sideDrawerFormClassName,
  sideDrawerHeaderClassName,
} from '@/components/drawer-layout'
import { StatusBadge } from '@/components/status-badge'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetFooter,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import { Switch } from '@/components/ui/switch'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { useCopyToClipboard } from '@/hooks/use-copy-to-clipboard'
import { useIsMobile } from '@/hooks/use-mobile'

import { updateChannel } from '../../api'
import {
  channelsQueryKeys,
  formatResponseTime,
  handleTestChannel,
} from '../../lib'
import type {
  Channel,
  GetChannelsResponse,
  SearchChannelsResponse,
} from '../../types'
import { useChannels } from '../channels-provider'

type ChannelTestDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
}

type ModelRow = {
  model: string
}

type TestStatus = 'idle' | 'testing' | 'success' | 'error'

type TestResult = {
  status: TestStatus
  responseTime?: number
  error?: string
  errorCode?: string
}

const endpointTypeOptions: Array<{ value: string; label: string }> = [
  { value: 'auto', label: 'Auto detect (default)' },
  { value: 'openai', label: 'OpenAI (/v1/chat/completions)' },
  { value: 'openai-response', label: 'OpenAI Responses (/v1/responses)' },
  {
    value: 'openai-response-compact',
    label: 'OpenAI Response Compaction (/v1/responses/compact)',
  },
  { value: 'anthropic', label: 'Anthropic (/v1/messages)' },
  {
    value: 'gemini',
    label: 'Gemini (/v1beta/models/{model}:generateContent)',
  },
  { value: 'jina-rerank', label: 'Jina Rerank (/v1/rerank)' },
  {
    value: 'image-generation',
    label: 'Image Generation (/v1/images/generations)',
  },
  { value: 'embeddings', label: 'Embeddings (/v1/embeddings)' },
]

const endpointSelectContentClass = 'w-[460px] max-w-[calc(100vw-2rem)]'
const endpointSelectItemClass =
  'items-start py-2 [&_[data-slot=select-item-text]]:min-w-0 [&_[data-slot=select-item-text]]:shrink [&_[data-slot=select-item-text]]:whitespace-normal'

const STREAM_INCOMPATIBLE_ENDPOINTS = new Set([
  'embeddings',
  'image-generation',
  'jina-rerank',
  'openai-response-compact',
])

const MODEL_PRICE_ERROR_CODE = 'model_price_error'
const FAILURE_SUMMARY_MAX_LENGTH = 96

type FailureStatusDisplay = {
  summary: string
  details?: string
}

type FailureDetailsState = {
  model: string
  summary: string
  details: string
}

function normalizeInlineError(errorText: string) {
  return errorText.replaceAll(/\s+/g, ' ').trim()
}

function getFirstErrorLine(errorText: string) {
  return errorText
    .split(/\r?\n/)
    .map((line) => line.trim())
    .find(Boolean)
}

function truncateFailureSummary(summary: string) {
  if (summary.length <= FAILURE_SUMMARY_MAX_LENGTH) {
    return summary
  }

  return `${summary.slice(0, FAILURE_SUMMARY_MAX_LENGTH).trimEnd()}...`
}

function getFailureStatusDisplay({
  errorText,
  fallbackSummary,
  isModelPriceError,
  modelPriceSummary,
}: {
  errorText?: string
  fallbackSummary: string
  isModelPriceError: boolean
  modelPriceSummary: string
}): FailureStatusDisplay {
  const rawError = errorText?.trim()

  if (!rawError) {
    return { summary: fallbackSummary }
  }

  if (isModelPriceError) {
    return {
      summary: modelPriceSummary,
      details: rawError === modelPriceSummary ? undefined : rawError,
    }
  }

  const firstLine = getFirstErrorLine(rawError) ?? rawError
  const summary = truncateFailureSummary(normalizeInlineError(firstLine))
  const normalizedRawError = normalizeInlineError(rawError)

  return {
    summary,
    details: summary === normalizedRawError ? undefined : rawError,
  }
}

function getTestTableColumnClass(columnId: string) {
  switch (columnId) {
    case 'select':
      return 'w-10 min-w-10'
    case 'model':
      return 'w-auto min-w-48 whitespace-nowrap'
    case 'status':
      return 'w-28 min-w-28 whitespace-nowrap'
    case 'result':
      return 'w-80 min-w-80 max-w-80 whitespace-normal'
    case 'actions':
      return 'bg-popover w-px whitespace-nowrap'
    default:
      return undefined
  }
}

export function ChannelTestDialog({
  open,
  onOpenChange,
}: ChannelTestDialogProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const currentChannelId = currentRow.id
  const batchStopRequestedRef = useRef(false)
  const batchProgressToastIdRef = useRef<ReturnType<
    typeof toast.loading
  > | null>(null)
  const [endpointType, setEndpointType] = useState('auto')
  const [isStreamTest, setIsStreamTest] = useState(false)
  const [searchTerm, setSearchTerm] = useState('')
  const [testResults, setTestResults] = useState<Record<string, TestResult>>({})
  const [rowSelection, setRowSelection] = useState<RowSelectionState>({})
  const [testingModels, setTestingModels] = useState<Set<string>>(
    () => new Set()
  )
  const [isBatchTesting, setIsBatchTesting] = useState(false)
  const [failureDetails, setFailureDetails] =
    useState<FailureDetailsState | null>(null)
  const [pagination, setPagination] = useState({
    pageIndex: 0,
    pageSize: 10,
  })
  const endpointSelectItems = useMemo(
    () =>
      endpointTypeOptions.map((option) => ({
        value: option.value,
        label: t(option.label),
      })),
    [t]
  )

  const dismissBatchProgressToast = useCallback(() => {
    if (batchProgressToastIdRef.current === null) return

    toast.dismiss(batchProgressToastIdRef.current)
    batchProgressToastIdRef.current = null
  }, [])

  useEffect(() => {
    if (!batchProgress) {
      dismissBatchProgressToast()
      return
    }

    const title = isBatchStopRequested
      ? t('Stopping batch test...')
      : t('Batch testing models...')
    const completedText = t('{{completed}}/{{total}} completed', {
      completed: batchProgress.completed,
      total: batchProgress.total,
    })
    const resultText = t('{{success}} succeeded, {{failed}} failed', {
      success: batchProgress.success,
      failed: batchProgress.failed,
    })

    batchProgressToastIdRef.current = toast.loading(title, {
      id: batchProgressToastIdRef.current ?? undefined,
      description: `${completedText} · ${resultText}`,
    })
  }, [batchProgress, dismissBatchProgressToast, isBatchStopRequested, t])

  useEffect(() => dismissBatchProgressToast, [dismissBatchProgressToast])

  const resetState = useCallback(() => {
    setEndpointType('auto')
    setIsStreamTest(false)
    setSearchTerm('')
    setTestResults({})
    setRowSelection({})
    setTestingModels(() => new Set())
    setIsBatchTesting(false)
    setFailureDetails(null)
    setPagination({ pageIndex: 0, pageSize: 10 })
  }, [])

  useEffect(() => {
    if (open && currentRow) {
      resetState()
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, currentRow?.id, resetState])

  const streamDisabled = STREAM_INCOMPATIBLE_ENDPOINTS.has(endpointType)

  useEffect(() => {
    if (streamDisabled) {
      setIsStreamTest(false)
    }
  }, [streamDisabled])

  const modelsValue = currentRow?.models ?? ''
  const defaultTestModel = currentRow?.test_model?.trim()

  const models = useMemo(() => {
    if (!modelsValue) return []
    return modelsValue
      .split(',')
      .map((model) => model.trim())
      .filter(Boolean)
  }, [modelsValue])

  const filteredModels = useMemo(() => {
    if (!searchTerm) return models
    const keyword = searchTerm.toLowerCase()
    return models.filter((model) => model.toLowerCase().includes(keyword))
  }, [models, searchTerm])

  useEffect(() => {
    setPagination((prev) => ({ ...prev, pageIndex: 0 }))
  }, [searchTerm, modelsValue])

  const tableData = useMemo<ModelRow[]>(
    () => filteredModels.map((model) => ({ model })),
    [filteredModels]
  )

  const markModelTesting = useCallback((key: string, isTesting: boolean) => {
    setTestingModels((prev) => {
      const next = new Set(prev)
      if (isTesting) {
        next.add(key)
      } else {
        next.delete(key)
      }
      return next
    })
  }, [])

  const updateTestResult = useCallback((key: string, result: TestResult) => {
    setFailureDetails((current) => (current?.model === key ? null : current))
    setTestResults((prev) => ({
      ...prev,
      [key]: result,
    }))
  }, [])

  const testSingleModel = useCallback(
    async (model: string, silent = false): Promise<TestResult | undefined> => {
      if (!currentRow) return

      markModelTesting(model, true)
      updateTestResult(model, { status: 'testing' })
      let finalResult: TestResult | undefined

      try {
        await handleTestChannel(
          currentRow.id,
          {
            testModel: model,
            endpointType: endpointType === 'auto' ? undefined : endpointType,
            stream: isStreamTest || undefined,
            silent,
          },
          (success, responseTime, error, errorCode) => {
            finalResult = {
              status: success ? 'success' : 'error',
              responseTime,
              error,
              errorCode,
            }
            updateTestResult(model, finalResult)
          }
        )
      } catch (error: unknown) {
        finalResult = {
          status: 'error',
          error: error instanceof Error ? error.message : t('Test failed'),
        }
        updateTestResult(model, finalResult)
      } finally {
        markModelTesting(model, false)
      }
      return finalResult
    },
    [
      currentRow,
      endpointType,
      isStreamTest,
      markModelTesting,
      t,
      updateTestResult,
    ]
  )

  const handleBatchTest = useCallback(
    async (modelsToTest: string[]) => {
      const uniqueModels = [
        ...new Set(modelsToTest.map((model) => model.trim()).filter(Boolean)),
      ]
      if (!uniqueModels.length) return

      setIsBatchTesting(true)
      try {
        const settled = await Promise.allSettled(
          modelsToTest.map((modelName) => testSingleModel(modelName, true))
        )
        const results = settled
          .map((result) =>
            result.status === 'fulfilled' ? result.value : undefined
          )
          const batchPromises = batch.map(async (modelName) => {
            try {
              const result = await testSingleModel(modelName, true, false)
              const finalResult = result ?? createFallbackResult()
              if (!result) {
                updateTestResult(modelName, finalResult)
              }
              recordBatchResult(finalResult)
              return finalResult
            } catch (error: unknown) {
              const fallbackResult = createFallbackResult(error)
              updateTestResult(modelName, fallbackResult)
              recordBatchResult(fallbackResult)
              return fallbackResult
            }
          })

          await Promise.allSettled(batchPromises)

          if (
            batchStopRequestedRef.current ||
            startIndex + BATCH_TEST_CONCURRENCY >= uniqueModels.length
          ) {
            break
          }

          await sleep(BATCH_TEST_DELAY_MS)
        }

        resultPatch = getLatestChannelTestCachePatch(results)
        const stopped =
          batchStopRequestedRef.current && completedCount < uniqueModels.length

        dismissBatchProgressToast()
        if (stopped) {
          toast.info(
            t(
              'Batch test stopped: {{completed}}/{{total}} completed, {{success}} succeeded, {{failed}} failed',
              {
                completed: completedCount,
                total: uniqueModels.length,
                success: successCount,
                failed: failedCount,
              }
            )
          )
        } else if (failedCount > 0) {
          toast.error(
            t(
              'Batch test completed: {{success}} succeeded, {{failed}} failed',
              {
                success: successCount,
                failed: failedCount,
              }
            )
          )
        } else {
          toast.success(
            t('Batch test completed: {{count}} succeeded', {
              count: successCount,
            })
          )
        }
      } finally {
        setIsBatchTesting(false)
        setRowSelection({})
      }
    },
    [
      dismissBatchProgressToast,
      refreshChannelLists,
      t,
      testSingleModel,
      updateTestResult,
    ]
  )

  const handleClose = () => {
    resetState()
    onOpenChange(false)
  }

  const isAnyTesting = testingModels.size > 0 || isBatchTesting

  const columns = useMemo<ColumnDef<ModelRow>[]>(
    () => [
      {
        id: 'select',
        header: ({ table }) => (
          <Checkbox
            checked={table.getIsAllPageRowsSelected()}
            indeterminate={table.getIsSomePageRowsSelected()}
            onCheckedChange={(value) =>
              table.toggleAllPageRowsSelected(!!value)
            }
            aria-label={t('Select all models')}
          />
        ),
        cell: ({ row }) => (
          <Checkbox
            checked={row.getIsSelected()}
            onCheckedChange={(value) => row.toggleSelected(!!value)}
            aria-label={t('Select model {{model}}', {
              model: row.original.model,
            })}
          />
        ),
        enableSorting: false,
        enableHiding: false,
        size: 40,
      },
      {
        accessorKey: 'model',
        header: t('Model'),
        cell: ({ row }) => {
          const model = row.original.model
          const isDefault = defaultTestModel === model

          return (
            <div className='flex w-max items-center gap-2 whitespace-nowrap'>
              <span className='font-medium whitespace-nowrap' title={model}>
                {model}
              </span>
              {isDefault && (
                <StatusBadge
                  label={t('Default')}
                  variant='info'
                  size='sm'
                  copyable={false}
                />
              )}
            </div>
          )
        },
      },
      {
        id: 'status',
        header: t('Status'),
        cell: ({ row }) => {
          const model = row.original.model
          const result = testResults[model]
          return <TestStatusCell result={result} />
        },
        enableSorting: false,
        size: 112,
      },
      {
        id: 'result',
        header: t('Result'),
        cell: ({ row }) => {
          const model = row.original.model
          const result = testResults[model]
          return (
            <TestResultCell
              result={result}
              model={model}
              onOpenDetails={setFailureDetails}
            />
          )
        },
        enableSorting: false,
        size: 320,
      },
      {
        id: 'actions',
        header: t('Actions'),
        cell: ({ row }) => {
          const model = row.original.model
          const isTestingModel = testingModels.has(model)

          return (
            <Tooltip>
              <TooltipTrigger
                render={
                  <Button
                    variant='ghost'
                    size='icon-sm'
                    onClick={() => testSingleModel(model)}
                    disabled={isTestingModel || isBatchTesting}
                    aria-label={t('Test Connection')}
                  />
                }
              >
                {isTestingModel ? (
                  <Loader2 className='size-4 animate-spin' />
                ) : (
                  <Gauge className='size-4' />
                )}
              </TooltipTrigger>
              <TooltipContent>{t('Test Connection')}</TooltipContent>
            </Tooltip>
          )
        },
        enableSorting: false,
      },
    ],
    [
      defaultTestModel,
      isBatchTesting,
      t,
      testResults,
      testingModels,
      testSingleModel,
    ]
  )

  const { table } = useDataTable({
    data: tableData,
    columns,
    rowSelection,
    pagination,
    enableRowSelection: true,
    onRowSelectionChange: setRowSelection,
    onPaginationChange: setPagination,
    withFilteredRowModel: false,
    withSortedRowModel: false,
    withFacetedRowModel: false,
  })

  if (!currentRow) {
    return null
  }

  return (
    <>
      <Dialog
        open={open}
        onOpenChange={handleDialogOpenChange}
        title={
          <span className='inline-flex max-w-full min-w-0 items-center gap-1.5'>
            <span className='shrink-0'>{t('Test Channel Connection')}:</span>
            <span className='min-w-0 truncate'>{currentRow.name}</span>
          </span>
        }
        contentClassName='max-h-[90vh] overflow-hidden sm:max-w-4xl'
        contentHeight='auto'
        bodyClassName='space-y-4'
        footer={
          <Button variant='outline' onClick={handleClose}>
            {t('Close')}
          </Button>
        }
      >
        <div className='max-h-[78vh] space-y-4 overflow-y-auto py-4 pr-1'>
          <div className='grid gap-4 md:grid-cols-2'>
            <div className='grid gap-2'>
              <Label htmlFor='endpoint-type'>{t('Endpoint Type')}</Label>
              <Select
                items={endpointSelectItems}
                value={endpointType}
                onValueChange={(v) => v !== null && setEndpointType(v)}
              >
                <SelectTrigger id='endpoint-type' className='w-full min-w-0'>
                  <SelectValue
                    className='min-w-0 truncate'
                    placeholder={t('Auto detect (default)')}
                  />
                </SelectTrigger>
                <SelectContent
                  alignItemWithTrigger={false}
                  className={endpointSelectContentClass}
                >
                  <SelectGroup>
                    {endpointSelectItems.map((option) => (
                      <SelectItem
                        key={option.value}
                        value={option.value}
                        className={endpointSelectItemClass}
                      >
                        <span className='min-w-0 leading-snug break-words whitespace-normal'>
                          {option.label}
                        </span>
                      </SelectItem>
                    ))}
                  </SelectGroup>
                </SelectContent>
              </Select>
              <p className='text-muted-foreground text-xs'>
                {t(
                  'Override the endpoint used for testing. Leave empty to auto detect.'
                )}
              </p>
            </div>
            <div className='grid gap-2'>
              <Label htmlFor='stream-toggle'>{t('Stream Mode')}</Label>
              <div className='flex items-center gap-2'>
                <Switch
                  id='stream-toggle'
                  checked={isStreamTest}
                  onCheckedChange={setIsStreamTest}
                  disabled={streamDisabled}
                />
                <span className='text-sm'>
                  {isStreamTest ? t('Enabled') : t('Disabled')}
                </span>
              </div>
              <p className='text-muted-foreground text-xs'>
                {t('Enable streaming mode for the test request.')}
              </p>
            </div>
          </div>

          <div className='space-y-3 max-sm:has-[div[role="toolbar"]]:pb-16'>
            <div className='flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between'>
              <div className='min-w-0 space-y-2'>
                <p className='text-sm font-medium'>{t('Channel models')}</p>
                <p className='text-muted-foreground text-xs'>
                  {t('Select models to run batch tests.')}
                </p>
                <div className='flex flex-wrap items-center gap-2'>
                  {isBatchTesting ? (
                    <Button
                      variant='outline'
                      size='sm'
                      onClick={handleStopBatchTest}
                      disabled={isBatchStopRequested}
                    >
                      {isBatchStopRequested
                        ? t('Stopping...')
                        : t('Stop testing')}
                    </Button>
                  ) : (
                    <>
                      <Button
                        size='sm'
                        onClick={() => handleBatchTest(filteredModels)}
                        disabled={isAnyTesting || filteredModels.length === 0}
                      >
                        {testAllButtonLabel}
                      </Button>
                      {successModels.length > 0 && (
                        <Button
                          variant='outline'
                          size='sm'
                          onClick={handleSelectSuccessfulModels}
                        >
                          <CheckCircle2 data-icon='inline-start' />
                          {t('Select successful models ({{count}})', {
                            count: successModels.length,
                          })}
                        </Button>
                      )}
                      {failedModels.length > 0 && (
                        <Button
                          variant='outline'
                          size='sm'
                          onClick={() => setIsDeleteFailedDialogOpen(true)}
                        >
                          <Trash2 data-icon='inline-start' />
                          {t('Delete failed models ({{count}})', {
                            count: failedModels.length,
                          })}
                        </Button>
                      )}
                    </>
                  )}
                </div>
              </div>
              <div className='flex flex-col gap-2 sm:flex-row sm:items-center'>
                <Input
                  placeholder={t('Filter models...')}
                  value={searchTerm}
                  onChange={handleSearchTermChange}
                  className='sm:w-64'
                />
              </div>
            </div>

            <div className='space-y-3'>
              <DataTableView
                table={table}
                containerClassName='rounded-md'
                containerProps={{
                  role: 'region',
                  'aria-label': t('Channel models'),
                }}
                tableContainerClassName='max-h-90 overflow-auto **:data-[slot=table-container]:overflow-visible'
                tableClassName='w-max min-w-full table-auto'
                pinnedColumns={[
                  {
                    columnId: 'actions',
                    side: 'right',
                    cellClassName: 'bg-popover',
                  },
                ]}
                colgroup={
                  <colgroup>
                    <col className='w-10 min-w-10' />
                    <col className='w-auto' />
                    <col className='w-28' />
                    <col className='w-80' />
                    <col className='w-px' />
                  </colgroup>
                }
                getColumnClassName={(columnId) =>
                  getTestTableColumnClass(columnId)
                }
                emptyContent={
                  models.length
                    ? t('No models matched your search.')
                    : t('This channel has no configured models.')
                }
                emptyCellClassName='text-muted-foreground h-16 text-center text-sm'
              />

              <DataTablePagination table={table} />
            </div>

            <TestModelsBulkActions
              table={table}
              disabled={isAnyTesting}
              onTestSelected={handleBatchTest}
            />
          </div>
        </div>
      </Dialog>
      <FailureDetailsSheet
        details={failureDetails}
        onOpenChange={(sheetOpen) => {
          if (!sheetOpen) {
            setFailureDetails(null)
          }
        }}
      />
    </>
  )
}

function TestStatusCell({ result }: { result?: TestResult }) {
  const { t } = useTranslation()

  if (!result || result.status === 'idle') {
    return (
      <StatusBadge label={t('Not tested')} variant='neutral' copyable={false} />
    )
  }

  if (result.status === 'testing') {
    return (
      <StatusBadge variant='info' copyable={false}>
        <Loader2 className='size-3.5 shrink-0 animate-spin' />
        <span className='min-w-0 truncate leading-normal'>
          {t('Testing...')}
        </span>
      </StatusBadge>
    )
  }

  if (result.status === 'success') {
    return (
      <StatusBadge label={t('Success')} variant='success' copyable={false} />
    )
  }

  return <StatusBadge label={t('Failed')} variant='danger' copyable={false} />
}

function TestResultCell({
  result,
  model,
  onOpenDetails,
}: {
  result?: TestResult
  model: string
  onOpenDetails: (details: FailureDetailsState) => void
}) {
  const { t } = useTranslation()

  if (!result || result.status === 'idle') {
    return <span className='text-muted-foreground text-sm'>-</span>
  }

  if (result.status === 'testing') {
    return (
      <div className='text-muted-foreground flex min-w-0 items-center gap-2 text-sm'>
        <Loader2 className='h-4 w-4 shrink-0 animate-spin' />
        <span className='truncate'>{t('Testing...')}</span>
      </div>
    )
  }

  if (result.status === 'success') {
    return typeof result.responseTime === 'number' ? (
      <span className='text-muted-foreground text-sm'>
        {formatResponseTime(result.responseTime, t)}
      </span>
    ) : (
      <span className='text-muted-foreground text-sm'>-</span>
    )
  }

  return (
    <FailureResultContent
      result={result}
      model={model}
      onOpenDetails={onOpenDetails}
    />
  )
}

function FailureResultContent({
  result,
  model,
  onOpenDetails,
}: {
  result: TestResult
  model: string
  onOpenDetails: (details: FailureDetailsState) => void
}) {
  const { t } = useTranslation()
  const errorText = result.error?.trim()
  const isModelPriceError = result.errorCode === MODEL_PRICE_ERROR_CODE
  const modelPriceSummary = t(
    'Model price is not configured. Please complete model pricing in settings.'
  )
  const { summary, details } = getFailureStatusDisplay({
    errorText,
    fallbackSummary: t('Test failed'),
    isModelPriceError,
    modelPriceSummary,
  })

  return (
    <div className='flex min-w-0 items-center gap-2 text-xs whitespace-normal'>
      <p className='text-muted-foreground line-clamp-2 min-w-0 flex-1 leading-snug wrap-break-word'>
        {summary}
      </p>
      <div className='flex shrink-0 flex-wrap items-center justify-end gap-1.5'>
        {isModelPriceError && (
          <Button
            variant='outline'
            size='sm'
            className='h-7 w-fit px-2 text-xs'
            onClick={() =>
              window.open('/system-settings/billing/model-pricing', '_blank')
            }
          >
            <Settings className='mr-1 h-3 w-3 shrink-0' />
            {t('Go to Settings')}
          </Button>
        )}
        {details && (
          <Button
            variant='ghost'
            size='sm'
            className='h-7 w-fit px-2 text-xs'
            aria-haspopup='dialog'
            onClick={() => onOpenDetails({ model, summary, details })}
          >
            <Info className='mr-1 h-3 w-3 shrink-0' />
            {t('Details')}
          </Button>
        )}
      </div>
    </div>
  )
}

function FailureDetailsSheet({
  details,
  onOpenChange,
}: {
  details: FailureDetailsState | null
  onOpenChange: (open: boolean) => void
}) {
  const { t } = useTranslation()
  const isMobile = useIsMobile()
  const { copiedText, copyToClipboard } = useCopyToClipboard({ notify: false })

  return (
    <Sheet open={Boolean(details)} onOpenChange={onOpenChange}>
      <SheetContent
        side={isMobile ? 'bottom' : 'right'}
        className={
          isMobile
            ? sideDrawerContentClassName('h-auto max-h-[85dvh] rounded-t-xl')
            : sideDrawerContentClassName('sm:max-w-lg')
        }
      >
        {details && (
          <>
            <SheetHeader className={sideDrawerHeaderClassName('sm:px-5')}>
              <SheetTitle className='pr-10'>{t('Details')}</SheetTitle>
              <SheetDescription className='pr-10 wrap-break-word'>
                {details.model}
              </SheetDescription>
            </SheetHeader>
            <div className={sideDrawerFormClassName('gap-4 sm:px-5')}>
              <section className='space-y-1'>
                <div className='text-muted-foreground text-xs font-medium'>
                  {t('Model')}
                </div>
                <p className='text-sm font-medium break-all'>{details.model}</p>
              </section>
              <section className='space-y-1'>
                <div className='text-muted-foreground text-xs font-medium'>
                  {t('Failed')}
                </div>
                <p className='text-muted-foreground text-sm leading-relaxed wrap-break-word'>
                  {details.summary}
                </p>
              </section>
              <section className='space-y-2'>
                <div className='text-muted-foreground text-xs font-medium'>
                  {t('Details')}
                </div>
                <pre className='bg-muted/30 text-muted-foreground m-0 max-w-full rounded-md border p-3 text-xs leading-relaxed wrap-break-word whitespace-pre-wrap'>
                  {details.details}
                </pre>
              </section>
            </div>
            <SheetFooter className={sideDrawerFooterClassName('sm:px-5')}>
              <Button
                variant='outline'
                className='w-full sm:w-auto'
                onClick={() => copyToClipboard(details.details)}
              >
                {copiedText === details.details ? (
                  <Check className='mr-2 h-4 w-4 text-green-600' />
                ) : (
                  <Copy className='mr-2 h-4 w-4' />
                )}
                {t('Copy')}
              </Button>
            </SheetFooter>
          </>
        )}
      </SheetContent>
    </Sheet>
  )
}

function TestModelsBulkActions({ table }: { table: TanStackTable<ModelRow> }) {
  const { t } = useTranslation()
  const selectedRows = table.getFilteredSelectedRowModel().rows
  const selectedModels = selectedRows.map((row) => row.original.model)

  const buttonLabel =
    selectedModels.length > 0
      ? t('Test {{count}} selected', { count: selectedModels.length })
      : t('Test selected models')

  return (
    <BulkActionsToolbar table={table} entityName='model'>
      <Tooltip>
        <TooltipTrigger
          render={
            <Button
              size='sm'
              onClick={() => onTestSelected(selectedModels)}
              disabled={disabled || selectedModels.length === 0}
            />
          }
        >
          {disabled ? (
            <>
              <Loader2 className='mr-2 h-4 w-4 animate-spin' />
              {t('Testing...')}
            </>
          ) : (
            buttonLabel
          )}
        </TooltipTrigger>
        <TooltipContent>
          <p>{t('Run tests for the selected models')}</p>
        </TooltipContent>
      </Tooltip>
    </BulkActionsToolbar>
  )
}
