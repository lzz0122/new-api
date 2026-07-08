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
import type { TFunction } from 'i18next'
import { z } from 'zod'

import { parseQuotaFromDollars, quotaUnitsToDollars } from '@/lib/format'

import { DEFAULT_GROUP } from '../constants'
import {
  type ApiKeyFormData,
  type ApiKey,
  type ApiKeyGroupConfig,
  type ApiKeyGroupRoute,
} from '../types'

// ============================================================================
// Form Schema
// ============================================================================

export function getApiKeyFormSchema(t: TFunction) {
  return z
    .object({
      name: z.string().min(1, t('Please enter a name')),
      remain_quota_dollars: z.number().optional(),
      expired_time: z.date().optional(),
      unlimited_quota: z.boolean(),
      model_limits: z.array(z.string()),
      allow_ips: z.string().optional(),
      group: z.string().optional(),
      group_routes: z
        .array(
          z.object({
            group: z.string().min(1),
            order: z.number().min(1),
            failover_strategy: z.enum(['fallback', 'return_error']).optional(),
          })
        )
        .min(1, t('Please select at least one group')),
      failover_strategy: z.enum(['fallback', 'return_error']),
      cross_group_retry: z.boolean().optional(),
      tokenCount: z.number().min(1).optional(),
    })
    .superRefine((data, ctx) => {
      if (data.unlimited_quota) {
        return
      }

      if (
        data.remain_quota_dollars === undefined ||
        data.remain_quota_dollars < 0
      ) {
        ctx.addIssue({
          code: 'custom',
          path: ['remain_quota_dollars'],
          message: t('Quota must be zero or greater'),
        })
      }
    })
}

export type ApiKeyFormValues = z.infer<ReturnType<typeof getApiKeyFormSchema>>

// ============================================================================
// Form Defaults
// ============================================================================

export const API_KEY_FORM_DEFAULT_VALUES: ApiKeyFormValues = {
  name: '',
  remain_quota_dollars: 10,
  expired_time: undefined,
  unlimited_quota: true,
  model_limits: [],
  allow_ips: '',
  group: DEFAULT_GROUP,
  group_routes: [{ group: DEFAULT_GROUP, order: 1 }],
  failover_strategy: 'fallback',
  cross_group_retry: true,
  tokenCount: 1,
}

const DEFAULT_GROUP_ROUTE_SETTINGS = {
  failover_strategy: 'fallback' as const,
}

function defaultGroupRoute(group: string, order: number): ApiKeyGroupRoute {
  return {
    group,
    order,
    ...DEFAULT_GROUP_ROUTE_SETTINGS,
  }
}

export function getApiKeyFormDefaultValues(
  defaultUseAutoGroup: boolean
): ApiKeyFormValues {
  return {
    ...API_KEY_FORM_DEFAULT_VALUES,
    group: defaultUseAutoGroup ? 'auto' : DEFAULT_GROUP,
    group_routes: [defaultGroupRoute(defaultUseAutoGroup ? 'auto' : DEFAULT_GROUP, 1)],
    cross_group_retry: defaultUseAutoGroup,
  }
}

// ============================================================================
// Form Data Transformation
// ============================================================================

/**
 * Transform form data to API payload
 */
export function transformFormDataToPayload(
  data: ApiKeyFormValues
): ApiKeyFormData {
  const routes = normalizeGroupRoutes(data.group_routes, data.group)
  const firstRoute = routes[0] || defaultGroupRoute(data.group || DEFAULT_GROUP, 1)
  const groupConfig: ApiKeyGroupConfig = {
    groups: routes,
    failover_strategy: firstRoute.failover_strategy || data.failover_strategy,
  }
  return {
    name: data.name,
    remain_quota: data.unlimited_quota
      ? 0
      : parseQuotaFromDollars(data.remain_quota_dollars || 0),
    expired_time: data.expired_time
      ? Math.floor(data.expired_time.getTime() / 1000)
      : -1,
    unlimited_quota: data.unlimited_quota,
    model_limits_enabled: data.model_limits.length > 0,
    model_limits: data.model_limits.join(','),
    allow_ips: data.allow_ips || '',
    group: routes[0]?.group || data.group || '',
    group_config: JSON.stringify(groupConfig),
    cross_group_retry: data.group === 'auto' ? !!data.cross_group_retry : false,
  }
}

/**
 * Transform API key data to form defaults
 */
export function transformApiKeyToFormDefaults(
  apiKey: ApiKey
): ApiKeyFormValues {
  const groupConfig = parseGroupConfig(apiKey)
  const routes = normalizeGroupRoutes(
    groupConfig.groups,
    apiKey.group || undefined
  )
  return {
    name: apiKey.name,
    remain_quota_dollars: apiKey.unlimited_quota
      ? 0
      : quotaUnitsToDollars(apiKey.remain_quota),
    expired_time:
      apiKey.expired_time > 0
        ? new Date(apiKey.expired_time * 1000)
        : undefined,
    unlimited_quota: apiKey.unlimited_quota,
    model_limits: apiKey.model_limits
      ? apiKey.model_limits.split(',').filter(Boolean)
      : [],
    allow_ips: apiKey.allow_ips || '',
    group: routes[0]?.group || apiKey.group || DEFAULT_GROUP,
    group_routes: routes,
    failover_strategy: groupConfig.failover_strategy,
    cross_group_retry: !!apiKey.cross_group_retry,
    tokenCount: 1,
  }
}

export function normalizeGroupRoutes(
  routes?: ApiKeyGroupRoute[],
  fallbackGroup?: string
): ApiKeyGroupRoute[] {
  const seen = new Set<string>()
  const normalized = (routes || [])
    .map((route, index) => ({
      group: route.group.trim(),
      order: route.order > 0 ? route.order : index + 1,
      failover_strategy:
        route.failover_strategy || DEFAULT_GROUP_ROUTE_SETTINGS.failover_strategy,
    }))
    .filter((route) => {
      if (!route.group || seen.has(route.group)) return false
      seen.add(route.group)
      return true
    })
    .sort((a, b) => a.order - b.order)
    .map((route, index) => ({ ...route, order: index + 1 }))

  if (normalized.length > 0) return normalized
  const group = fallbackGroup?.trim() || DEFAULT_GROUP
  return [defaultGroupRoute(group, 1)]
}

function parseGroupConfig(apiKey: ApiKey): ApiKeyGroupConfig {
  const fallback: ApiKeyGroupConfig = {
    groups: normalizeGroupRoutes(undefined, apiKey.group || DEFAULT_GROUP),
    failover_strategy: 'fallback',
  }
  if (!apiKey.group_config) return fallback
  try {
    const parsed = JSON.parse(apiKey.group_config) as Partial<ApiKeyGroupConfig>
    const cfg: Omit<ApiKeyGroupConfig, 'groups'> = {
      failover_strategy:
        parsed.failover_strategy === 'return_error'
          ? 'return_error'
          : 'fallback',
    }
    const routes = (parsed.groups || []).map((route) => ({
      ...route,
      failover_strategy: route.failover_strategy || cfg.failover_strategy,
    }))
    return {
      groups: normalizeGroupRoutes(routes, apiKey.group || DEFAULT_GROUP),
      ...cfg,
    }
  } catch {
    return fallback
  }
}
