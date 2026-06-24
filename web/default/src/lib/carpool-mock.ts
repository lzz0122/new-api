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
import type {
  AxiosInstance,
  AxiosResponse,
  InternalAxiosRequestConfig,
} from 'axios'

const MOCK_STORAGE_KEY = 'new-api-carpool-mock'
const CARPOOL_GROUP = '拼车'

type MockResponse = {
  status?: number
  data: unknown
}

declare global {
  interface Window {
    __carpoolMockFetchInstalled?: boolean
  }
}

function getSearchParamValue() {
  if (typeof window === 'undefined') return null
  return new URLSearchParams(window.location.search).get('mockCarpool')
}

export function isCarpoolMockEnabled() {
  if (!import.meta.env.DEV || typeof window === 'undefined') return false

  const paramValue = getSearchParamValue()
  if (paramValue === '1' || paramValue === 'true') {
    window.localStorage.setItem(MOCK_STORAGE_KEY, '1')
    return true
  }
  if (paramValue === '0' || paramValue === 'false') {
    window.localStorage.removeItem(MOCK_STORAGE_KEY)
    return false
  }

  return window.localStorage.getItem(MOCK_STORAGE_KEY) === '1'
}

export function getCarpoolMockUser() {
  if (!isCarpoolMockEnabled()) return null
  return {
    id: 1,
    username: 'mock-admin',
    display_name: 'Mock Admin',
    email: 'mock-admin@example.test',
    role: 100,
    status: 1,
    group: CARPOOL_GROUP,
    quota: 1000000,
    used_quota: 12800,
    request_count: 320,
    permissions: {
      sidebar_settings: false,
    },
  }
}

function currentMonth(now: Date) {
  return `${now.getFullYear()}-${String(now.getMonth() + 1).padStart(2, '0')}`
}

function addDays(date: Date, days: number) {
  const next = new Date(date)
  next.setDate(next.getDate() + days)
  return next
}

function formatDate(date: Date) {
  return date.toISOString().slice(0, 10)
}

function toUnixSeconds(date: Date) {
  return Math.floor(date.getTime() / 1000)
}

function formatIso(timestamp: number) {
  return new Date(timestamp * 1000).toISOString()
}

function buildDaily(base: number) {
  const today = new Date()
  return Array.from({ length: 7 }, (_, index) => ({
    date: formatDate(addDays(today, index - 6)),
    quota: Number((base * (0.55 + index * 0.09)).toFixed(6)),
  }))
}

function getPeriodRange(period: string) {
  const now = new Date()
  if (period === 'month') {
    return {
      start: formatDate(new Date(now.getFullYear(), now.getMonth(), 1)),
      end: formatDate(now),
    }
  }

  const day = now.getDay() || 7
  return {
    start: formatDate(addDays(now, -(day - 1))),
    end: formatDate(now),
  }
}

function buildCarpoolUsageSummary(period = 'week') {
  const normalizedPeriod = period === 'month' ? 'month' : 'week'
  const periodFactor = normalizedPeriod === 'month' ? 3.2 : 1
  const range = getPeriodRange(normalizedPeriod)
  const now = Math.floor(Date.now() / 1000)

  const users = [
    {
      user_id: 11,
      username: 'Alice',
      email: 'alice@example.test',
      period_quota: 3.42 * periodFactor,
      cumulative_quota: 28.64,
      gross_period_quota: 4.17 * periodFactor,
      gross_cumulative_quota: 33.74,
      carnival_period_quota: 0.75 * periodFactor,
      carnival_cumulative_quota: 5.1,
      current_carnival_quota: 1.28,
      period_token_used: Math.round(456000 * periodFactor),
      cumulative_token_used: 3180000,
      period_request_count: Math.round(44 * periodFactor),
      cumulative_request_count: 362,
      active_tokens: 2,
      known_tokens: 2,
      daily: buildDaily(0.44 * periodFactor),
      tokens: [
        {
          token_id: 101,
          user_id: 11,
          name: 'alice-main',
          period_quota: 2.16 * periodFactor,
          cumulative_quota: 18.22,
          gross_period_quota: 2.66 * periodFactor,
          gross_cumulative_quota: 21.42,
          carnival_period_quota: 0.5 * periodFactor,
          carnival_cumulative_quota: 3.2,
          current_carnival_quota: 0.84,
          period_token_used: Math.round(286000 * periodFactor),
          cumulative_token_used: 2040000,
          period_request_count: Math.round(28 * periodFactor),
          cumulative_request_count: 240,
          active: true,
          last_seen_at: formatIso(now - 320),
        },
        {
          token_id: 102,
          user_id: 11,
          name: 'alice-backup',
          period_quota: 1.26 * periodFactor,
          cumulative_quota: 10.42,
          gross_period_quota: 1.51 * periodFactor,
          gross_cumulative_quota: 12.32,
          carnival_period_quota: 0.25 * periodFactor,
          carnival_cumulative_quota: 1.9,
          current_carnival_quota: 0.44,
          period_token_used: Math.round(170000 * periodFactor),
          cumulative_token_used: 1140000,
          period_request_count: Math.round(16 * periodFactor),
          cumulative_request_count: 122,
          active: true,
          last_seen_at: formatIso(now - 870),
        },
      ],
    },
    {
      user_id: 18,
      username: 'Bob',
      email: 'bob@example.test',
      period_quota: 2.74 * periodFactor,
      cumulative_quota: 19.92,
      gross_period_quota: 3.12 * periodFactor,
      gross_cumulative_quota: 22.62,
      carnival_period_quota: 0.38 * periodFactor,
      carnival_cumulative_quota: 2.7,
      current_carnival_quota: 0.64,
      period_token_used: Math.round(339000 * periodFactor),
      cumulative_token_used: 2280000,
      period_request_count: Math.round(39 * periodFactor),
      cumulative_request_count: 286,
      active_tokens: 1,
      known_tokens: 2,
      daily: buildDaily(0.36 * periodFactor),
      tokens: [
        {
          token_id: 201,
          user_id: 18,
          name: 'bob-main',
          period_quota: 2.74 * periodFactor,
          cumulative_quota: 19.92,
          gross_period_quota: 3.12 * periodFactor,
          gross_cumulative_quota: 22.62,
          carnival_period_quota: 0.38 * periodFactor,
          carnival_cumulative_quota: 2.7,
          current_carnival_quota: 0.64,
          period_token_used: Math.round(339000 * periodFactor),
          cumulative_token_used: 2280000,
          period_request_count: Math.round(39 * periodFactor),
          cumulative_request_count: 286,
          active: true,
          last_seen_at: formatIso(now - 180),
        },
        {
          token_id: 202,
          user_id: 18,
          name: 'bob-disabled',
          period_quota: 0,
          cumulative_quota: 0,
          gross_period_quota: 0,
          gross_cumulative_quota: 0,
          carnival_period_quota: 0,
          carnival_cumulative_quota: 0,
          current_carnival_quota: 0,
          period_token_used: 0,
          cumulative_token_used: 0,
          period_request_count: 0,
          cumulative_request_count: 0,
          active: false,
          last_seen_at: formatIso(now - 86400),
        },
      ],
    },
    {
      user_id: 27,
      username: 'Chen',
      email: 'chen@example.test',
      period_quota: 1.08 * periodFactor,
      cumulative_quota: 8.36,
      gross_period_quota: 1.08 * periodFactor,
      gross_cumulative_quota: 8.36,
      carnival_period_quota: 0,
      carnival_cumulative_quota: 0,
      current_carnival_quota: 0,
      period_token_used: Math.round(92000 * periodFactor),
      cumulative_token_used: 830000,
      period_request_count: Math.round(14 * periodFactor),
      cumulative_request_count: 96,
      active_tokens: 1,
      known_tokens: 1,
      daily: buildDaily(0.17 * periodFactor),
      tokens: [
        {
          token_id: 301,
          user_id: 27,
          name: 'chen-main',
          period_quota: 1.08 * periodFactor,
          cumulative_quota: 8.36,
          gross_period_quota: 1.08 * periodFactor,
          gross_cumulative_quota: 8.36,
          carnival_period_quota: 0,
          carnival_cumulative_quota: 0,
          current_carnival_quota: 0,
          period_token_used: Math.round(92000 * periodFactor),
          cumulative_token_used: 830000,
          period_request_count: Math.round(14 * periodFactor),
          cumulative_request_count: 96,
          active: true,
          last_seen_at: formatIso(now - 1420),
        },
      ],
    },
  ]

  const totals = users.reduce(
    (acc, user) => ({
      period_quota: acc.period_quota + user.period_quota,
      cumulative_quota: acc.cumulative_quota + user.cumulative_quota,
      gross_period_quota: acc.gross_period_quota + user.gross_period_quota,
      gross_cumulative_quota:
        acc.gross_cumulative_quota + user.gross_cumulative_quota,
      carnival_period_quota:
        acc.carnival_period_quota + user.carnival_period_quota,
      carnival_cumulative_quota:
        acc.carnival_cumulative_quota + user.carnival_cumulative_quota,
      current_carnival_quota:
        acc.current_carnival_quota + user.current_carnival_quota,
      period_token_used: acc.period_token_used + user.period_token_used,
      cumulative_token_used:
        acc.cumulative_token_used + user.cumulative_token_used,
      period_request_count:
        acc.period_request_count + user.period_request_count,
      cumulative_request_count:
        acc.cumulative_request_count + user.cumulative_request_count,
      users: acc.users + 1,
      active_tokens: acc.active_tokens + user.active_tokens,
      known_tokens: acc.known_tokens + user.known_tokens,
    }),
    {
      period_quota: 0,
      cumulative_quota: 0,
      gross_period_quota: 0,
      gross_cumulative_quota: 0,
      carnival_period_quota: 0,
      carnival_cumulative_quota: 0,
      current_carnival_quota: 0,
      period_token_used: 0,
      cumulative_token_used: 0,
      period_request_count: 0,
      cumulative_request_count: 0,
      users: 0,
      active_tokens: 0,
      known_tokens: 0,
    }
  )

  return {
    group: CARPOOL_GROUP,
    period: normalizedPeriod,
    start_date: range.start,
    end_date: range.end,
    last_run_at: new Date().toISOString(),
    quota_per_unit: 1,
    totals,
    last_sync: {
      delta_quota: 0.92,
    },
    users,
  }
}

function buildCarnivalUser(user_id: number, username: string, quota: number) {
  return {
    user_id,
    username,
    quota,
    token_used: Math.round(quota * 120000),
    request_count: quota > 0 ? Math.max(1, Math.round(quota * 12)) : 0,
  }
}

function buildCarnivalSessions() {
  const now = new Date()
  const activeStarted = toUnixSeconds(addDays(now, 0)) - 7200
  const lastStarted = toUnixSeconds(addDays(now, -2))
  const lastEnded = lastStarted + 5400
  const monthStarted = toUnixSeconds(addDays(now, -6)) + 3600
  const monthEnded = monthStarted + 3600
  const oldStarted = toUnixSeconds(addDays(now, -24))
  const oldEnded = oldStarted + 4200

  return {
    active: {
      id: 7,
      group: CARPOOL_GROUP,
      started_at: activeStarted,
      ended_at: 0,
      duration_seconds: Math.floor(Date.now() / 1000) - activeStarted,
      since_end_seconds: 0,
      total_quota: 1.92,
      total_tokens: 230400,
      request_count: 23,
      users: [
        buildCarnivalUser(11, 'Alice', 1.28),
        buildCarnivalUser(18, 'Bob', 0.64),
        buildCarnivalUser(27, 'Chen', 0),
      ],
    },
    history: [
      {
        id: 6,
        group: CARPOOL_GROUP,
        started_at: lastStarted,
        ended_at: lastEnded,
        duration_seconds: lastEnded - lastStarted,
        since_end_seconds: Math.floor(Date.now() / 1000) - lastEnded,
        total_quota: 2.85,
        total_tokens: 342000,
        request_count: 34,
        users: [
          buildCarnivalUser(11, 'Alice', 1.7),
          buildCarnivalUser(18, 'Bob', 1.15),
        ],
      },
      {
        id: 5,
        group: CARPOOL_GROUP,
        started_at: monthStarted,
        ended_at: monthEnded,
        duration_seconds: monthEnded - monthStarted,
        since_end_seconds: Math.floor(Date.now() / 1000) - monthEnded,
        total_quota: 1.68,
        total_tokens: 201600,
        request_count: 20,
        users: [
          buildCarnivalUser(11, 'Alice', 0.96),
          buildCarnivalUser(18, 'Bob', 0.48),
          buildCarnivalUser(27, 'Chen', 0.24),
        ],
      },
      {
        id: 4,
        group: CARPOOL_GROUP,
        started_at: oldStarted,
        ended_at: oldEnded,
        duration_seconds: oldEnded - oldStarted,
        since_end_seconds: Math.floor(Date.now() / 1000) - oldEnded,
        total_quota: 4.95,
        total_tokens: 594000,
        request_count: 59,
        users: [
          buildCarnivalUser(11, 'Alice', 3.4),
          buildCarnivalUser(18, 'Bob', 1.55),
        ],
      },
    ],
  }
}

function aggregateSessions(
  sessions: ReturnType<typeof buildCarnivalSessions>['history']
) {
  const userMap = new Map<number, ReturnType<typeof buildCarnivalUser>>()
  const aggregate = {
    total_quota: 0,
    total_tokens: 0,
    request_count: 0,
    users: [] as ReturnType<typeof buildCarnivalUser>[],
  }

  sessions.forEach((session) => {
    aggregate.total_quota += session.total_quota
    aggregate.total_tokens += session.total_tokens
    aggregate.request_count += session.request_count
    session.users.forEach((user) => {
      const existing = userMap.get(user.user_id)
      if (existing) {
        existing.quota += user.quota
        existing.token_used += user.token_used
        existing.request_count += user.request_count
      } else {
        userMap.set(user.user_id, { ...user })
      }
    })
  })

  aggregate.users = Array.from(userMap.values()).sort(
    (left, right) => right.quota - left.quota
  )
  return aggregate
}

function buildCarnivalStatus() {
  const sessions = buildCarnivalSessions()
  return {
    group: CARPOOL_GROUP,
    active: sessions.active,
    last: sessions.history[0],
    server_time: Math.floor(Date.now() / 1000),
  }
}

function buildCarnivalHistory(month = 'all') {
  const now = new Date()
  const selectedMonth = month || 'all'
  const sessions = buildCarnivalSessions()
  const allSessions = sessions.history
  const visibleSessions =
    selectedMonth === 'all'
      ? allSessions
      : allSessions.filter((session) => {
          const started = new Date(session.started_at * 1000)
          return currentMonth(started) === selectedMonth
        })

  return {
    group: CARPOOL_GROUP,
    months: [currentMonth(now), currentMonth(addDays(now, -32))],
    selected_month: selectedMonth,
    sessions: visibleSessions,
    month_total: aggregateSessions(visibleSessions),
    all_total: aggregateSessions(allSessions),
  }
}

function buildUpstreamUsage() {
  const now = Math.floor(Date.now() / 1000)
  return {
    success: true,
    data: {
      group: CARPOOL_GROUP,
      base_url: 'https://sub2api.example.test',
      key_name: 'mock-carpool-upstream',
      masked_key: 'sk-car...mock',
      key_status: 'active',
      upstream_group: 'mock-upstream',
      updated_at: now - 90,
      next_refresh_at: now + 45,
      cached: false,
      rate_limits: [
        {
          window: '7d',
          limit: 50,
          used: 18.42,
          remaining: 31.58,
          reset_at: new Date(Date.now() + 3 * 86400 * 1000).toISOString(),
        },
        {
          window: '24h',
          limit: 12,
          used: 4.36,
          remaining: 7.64,
          reset_at: new Date(Date.now() + 8 * 3600 * 1000).toISOString(),
        },
      ],
    },
  }
}

function buildStatusResponse() {
  return {
    success: true,
    data: {
      system_name: 'New API Carpool Mock',
      SidebarModulesAdmin: '',
      announcements_enabled: false,
    },
  }
}

function getMockUrl(config: InternalAxiosRequestConfig) {
  const rawUrl = config.url || '/'
  return new URL(rawUrl, window.location.origin)
}

function resolveMockResponse(
  config: InternalAxiosRequestConfig
): MockResponse | null {
  if (!isCarpoolMockEnabled()) return null

  const url = getMockUrl(config)
  const path = url.pathname
  const method = (config.method || 'get').toLowerCase()

  if (method === 'get' && path === '/api/status') {
    return { data: buildStatusResponse() }
  }

  if (method === 'get' && path === '/api/setup') {
    return {
      data: {
        success: true,
        data: {
          status: true,
          root_init: true,
          database_type: 'sqlite',
          SelfUseModeEnabled: false,
          DemoSiteEnabled: false,
        },
      },
    }
  }

  if (method === 'get' && path === '/api/notice') {
    return { data: { success: true, data: '' } }
  }

  if (method === 'get' && path === '/api/user/self') {
    return { data: { success: true, data: getCarpoolMockUser() } }
  }

  if (method === 'get' && path === '/api/user/self/groups') {
    return {
      data: {
        success: true,
        data: {
          [CARPOOL_GROUP]: { desc: CARPOOL_GROUP, ratio: 1 },
        },
      },
    }
  }

  if (method === 'get' && path === '/api/user/models') {
    return { data: { success: true, data: ['gpt-5', 'claude-sonnet-4.5'] } }
  }

  if (method === 'get' && path === '/api/log/upstream-usage') {
    return { data: buildUpstreamUsage() }
  }

  if (method === 'get' && path === '/api/log/carnival') {
    return { data: { success: true, data: buildCarnivalStatus() } }
  }

  if (method === 'get' && path === '/api/log/carnival/history') {
    return {
      data: {
        success: true,
        data: buildCarnivalHistory(url.searchParams.get('month') || 'all'),
      },
    }
  }

  if (
    (method === 'post' && path === '/api/log/carnival/start') ||
    (method === 'post' && path === '/api/log/carnival/finish')
  ) {
    return { data: { success: true, data: buildCarnivalStatus() } }
  }

  if (method === 'get' && path === '/api/carpool-usage/summary') {
    return {
      data: {
        success: true,
        data: buildCarpoolUsageSummary(
          url.searchParams.get('period') || 'week'
        ),
      },
    }
  }

  return null
}

function buildAxiosResponse(
  config: InternalAxiosRequestConfig,
  mock: MockResponse
): AxiosResponse {
  return {
    data: mock.data,
    status: mock.status || 200,
    statusText: 'OK',
    headers: {},
    config,
    request: { mocked: true },
  }
}

function installCarpoolMockFetch() {
  if (
    !import.meta.env.DEV ||
    typeof window === 'undefined' ||
    window.__carpoolMockFetchInstalled
  ) {
    return
  }

  const originalFetch = window.fetch.bind(window)
  window.fetch = async (input, init) => {
    if (!isCarpoolMockEnabled()) {
      return originalFetch(input, init)
    }

    const rawUrl =
      typeof input === 'string'
        ? input
        : input instanceof URL
          ? input.toString()
          : input.url
    const url = new URL(rawUrl, window.location.origin)
    const method = (init?.method || 'get').toLowerCase()

    if (method === 'get' && url.pathname === '/api/status') {
      return new Response(JSON.stringify(buildStatusResponse()), {
        status: 200,
        headers: { 'Content-Type': 'application/json' },
      })
    }

    return originalFetch(input, init)
  }
  window.__carpoolMockFetchInstalled = true
}

export function installCarpoolMockApi(api: AxiosInstance) {
  if (!import.meta.env.DEV) return

  installCarpoolMockFetch()

  api.interceptors.request.use((config) => {
    const mock = resolveMockResponse(config)
    if (!mock) return config

    config.adapter = async () => buildAxiosResponse(config, mock)
    return config
  })
}
