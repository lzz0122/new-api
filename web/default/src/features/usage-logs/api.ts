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
import { api } from '@/lib/api'

import { buildQueryParams } from './lib/utils'
import type {
  GetLogsParams,
  GetLogsResponse,
  GetLogStatsParams,
  GetLogStatsResponse,
  GetMidjourneyLogsParams,
  GetTaskLogsParams,
  UserInfo,
  CarnivalHistorySnapshot,
  CarnivalStatusSnapshot,
  CarpoolGroupsSnapshot,
  CarpoolHistorySnapshot,
  CarpoolStatusSnapshot,
  CarpoolUsageSummarySnapshot,
  UpstreamUsageSnapshot,
} from './types'

// ============================================================================
// Generic API Helpers
// ============================================================================

function buildApiPath(endpoint: string, isAdmin: boolean): string {
  return isAdmin ? endpoint : `${endpoint}/self`
}

async function fetchLogs<T>(
  endpoint: string,
  params: T,
  isAdmin: boolean
): Promise<GetLogsResponse> {
  const paramRecord = params as unknown as Record<string, unknown>
  const queryParams = buildQueryParams({
    p: paramRecord.p || 1,
    page_size: paramRecord.page_size || 20,
    ...params,
  })
  const path = buildApiPath(endpoint, isAdmin)
  const res = await api.get(`${path}?${queryParams}`)
  return res.data
}

async function fetchLogStats<T>(
  endpoint: string,
  params: T,
  isAdmin: boolean
): Promise<GetLogStatsResponse> {
  const queryParams = buildQueryParams(
    params as unknown as Record<string, unknown>
  )
  const path = buildApiPath(endpoint, isAdmin)
  const res = await api.get(`${path}/stat?${queryParams}`)
  return res.data
}

// ============================================================================
// Common Log APIs
// ============================================================================

export const getAllLogs = (params: GetLogsParams = {}) =>
  fetchLogs('/api/log', params, true)

export const getUserLogs = (
  params: Omit<GetLogsParams, 'username' | 'channel'> = {}
) => fetchLogs('/api/log', params, false)

export const getLogStats = (params: GetLogStatsParams = {}) =>
  fetchLogStats('/api/log', params, true)

export const getUserLogStats = (
  params: Omit<GetLogStatsParams, 'username' | 'channel'> = {}
) => fetchLogStats('/api/log', params, false)

export async function getUpstreamUsage(params: {
  group?: string
  refresh?: boolean
}): Promise<{
  success: boolean
  message?: string
  data?: UpstreamUsageSnapshot
}> {
  const queryParams = buildQueryParams({
    group: params.group || '拼车',
    refresh: params.refresh ? 1 : undefined,
  })
  const res = await api.get(`/api/log/upstream-usage?${queryParams}`)
  return res.data
}

export async function getCarnivalStatus(params: { group?: string }): Promise<{
  success: boolean
  message?: string
  data?: CarnivalStatusSnapshot
}> {
  const queryParams = buildQueryParams({
    group: params.group || '拼车',
  })
  const res = await api.get(`/api/log/carnival?${queryParams}`)
  return res.data
}

export async function getCarnivalHistory(params: {
  group?: string
  month?: string
}): Promise<{
  success: boolean
  message?: string
  data?: CarnivalHistorySnapshot
}> {
  const queryParams = buildQueryParams({
    group: params.group || '拼车',
    month: params.month || undefined,
  })
  const res = await api.get(`/api/log/carnival/history?${queryParams}`)
  return res.data
}

export async function startCarnival(params: { group?: string }): Promise<{
  success: boolean
  message?: string
  data?: CarnivalStatusSnapshot
}> {
  const queryParams = buildQueryParams({
    group: params.group || '拼车',
  })
  const res = await api.post(`/api/log/carnival/start?${queryParams}`)
  return res.data
}

export async function finishCarnival(params: { group?: string }): Promise<{
  success: boolean
  message?: string
  data?: CarnivalStatusSnapshot
}> {
  const queryParams = buildQueryParams({
    group: params.group || '拼车',
  })
  const res = await api.post(`/api/log/carnival/finish?${queryParams}`)
  return res.data
}

export async function getCarpoolUsageSummary(params: {
  group?: string
  period?: 'day' | 'week' | 'month'
  scope?: 'session'
  session_id?: number
}): Promise<CarpoolUsageSummarySnapshot> {
  const sessionId =
    params.session_id && params.session_id > 0 ? params.session_id : undefined
  const queryParams = buildQueryParams({
    group: params.group || '拼车',
    period: params.period || 'week',
    scope: sessionId ? params.scope : undefined,
    session_id: sessionId,
  })
  const res = await api.get(`/api/carpool-usage/summary?${queryParams}`)
  return res.data?.data || res.data
}

export async function getCarpoolGroups(): Promise<CarpoolGroupsSnapshot> {
  const res = await api.get('/api/carpool-usage/groups')
  return res.data?.data || res.data
}

export async function getCarpoolStatus(params: {
  group?: string
}): Promise<CarpoolStatusSnapshot> {
  const queryParams = buildQueryParams({
    group: params.group || '拼车',
  })
  const res = await api.get(`/api/carpool-usage/status?${queryParams}`)
  return res.data?.data || res.data
}

export async function getCarpoolHistory(params: {
  group?: string
  month?: string
}): Promise<CarpoolHistorySnapshot> {
  const queryParams = buildQueryParams({
    group: params.group || undefined,
    month: params.month || undefined,
  })
  const res = await api.get(`/api/carpool-usage/history?${queryParams}`)
  return res.data?.data || res.data
}

export async function startCarpool(params: { group?: string }): Promise<{
  success: boolean
  message?: string
  data?: CarpoolStatusSnapshot
}> {
  const queryParams = buildQueryParams({
    group: params.group || '拼车',
  })
  const res = await api.post(`/api/carpool-usage/start?${queryParams}`)
  return res.data
}

export async function finishCarpool(params: {
  group?: string
  code: string
}): Promise<{
  success: boolean
  message?: string
  data?: CarpoolStatusSnapshot
}> {
  const res = await api.post('/api/carpool-usage/finish', {
    group: params.group || '拼车',
    code: params.code,
  })
  return res.data
}

export async function getUserInfo(
  userId: number
): Promise<{ success: boolean; message?: string; data?: UserInfo }> {
  const res = await api.get(`/api/user/${userId}`)
  return res.data
}

// ============================================================================
// MjProxy (Drawing) Logs API
// ============================================================================

export const getAllMidjourneyLogs = (params: GetMidjourneyLogsParams) =>
  fetchLogs('/api/mj', params, true)

export const getUserMidjourneyLogs = (params: GetMidjourneyLogsParams) =>
  fetchLogs('/api/mj', params, false)

// ============================================================================
// Task Logs API
// ============================================================================

export const getAllTaskLogs = (params: GetTaskLogsParams) =>
  fetchLogs('/api/task', params, true)

export const getUserTaskLogs = (params: GetTaskLogsParams) =>
  fetchLogs('/api/task', params, false)
