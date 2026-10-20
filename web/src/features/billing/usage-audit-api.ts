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

import type { BillingCurrency } from './types'

export interface UsageAuditFilter {
  user_id: number
  token_id: number
  channel_id: number
  start_at: number
  end_at: number
  model: string
  group: string
  protocol: string
}
export interface UsageAuditTokens {
  input_tokens: number
  cache_read_input_tokens: number
  cache_creation_input_tokens: number
  output_tokens: number
  total_input_tokens: number
}
export interface UsageAuditCost {
  input: string
  cache_read: string
  cache_write: string
  output: string
  rounding: string
  adjustment?: string
  total: string
}
export interface UsageAuditComparison {
  platform: UsageAuditCost
  list: UsageAuditCost
  difference: UsageAuditCost
}
export interface UsageAuditInputRecalculation {
  request_count: number
  excluded_cache_write_request_count: number
  tokens: number
  cache_read_input_tokens: number
  cache_creation_input_tokens: number
  unpriced_tokens: number
  platform_amount: string
  list_amount: string
}
export interface UsageAuditRow {
  input_recalculation: UsageAuditInputRecalculation | null
  id: number
  user_id: number
  username: string
  token_id: number
  token_name: string
  created_at: number
  request_id: string
  upstream_request_id: string
  model: string
  group: string
  channel_id: number
  is_stream: boolean
  kind: string
  protocol: string
  usage_semantic: string
  group_ratio: number | null
  tokens: UsageAuditTokens | null
  actual_amount: string
  list_prices_per_million: UsageAuditCost | null
  list_cost: UsageAuditCost | null
  discounted_cost: UsageAuditCost | null
  recalculation_difference: string | null
  possible_duplicate_input_tokens: number
  duplicate_status: string
  issue: string
  evidence?: {
    request_path: string
    request_conversion: string[] | null
    billing_path: string
    model_ratio: number | null
    completion_ratio: number | null
    cache_ratio: number | null
    cache_creation_ratio: number | null
    cache_creation_ratio_5m: number | null
    cache_creation_ratio_1h: number | null
    cache_creation_tokens_5m: number
    cache_creation_tokens_1h: number
  } | null
}
export interface UsageAuditSummary {
  input_recalculation: UsageAuditInputRecalculation
  count: number
  priced_count: number
  unknown_usage_count: number
  duplicate_review_count: number
  tokens: UsageAuditTokens
  possible_duplicate_input_tokens: number
  actual_amount: string
  priced_actual_amount: string
  list_cost: UsageAuditCost
  discounted_cost: UsageAuditCost
  recalculation_difference: string
  comparison: UsageAuditComparison | null
}
export interface UsageAuditGroup extends UsageAuditSummary {
  label: string
}
export interface UsageAuditReport {
  recalculation_basis: string
  filter: UsageAuditFilter
  generated_at: number
  timezone: string
  price_source: string
  currency: BillingCurrency
  sha256: string
  summary: UsageAuditSummary
  days: UsageAuditGroup[]
  models: UsageAuditGroup[]
  users: UsageAuditGroup[]
  keys: UsageAuditGroup[]
  rows: UsageAuditRow[]
  rates: { model: string; group: string; ratio: string }[]
}
export interface UsageAuditKey {
  id: number
  user_id: number
  name: string
}
interface AuditResponse<T> {
  success: boolean
  message?: string
  data: T
}
export async function queryUsageAudit(input: UsageAuditFilter) {
  const { data } = await api.post<AuditResponse<UsageAuditReport>>(
    '/api/billing/admin/usage-audit',
    input
  )
  if (!data.success) throw new Error(data.message || 'Something went wrong!')
  return data.data
}
export async function searchUsageAuditKeys(userId: number, keyword: string) {
  const { data } = await api.get<AuditResponse<UsageAuditKey[]>>(
    '/api/billing/admin/usage-audit/keys',
    { params: { user_id: userId, keyword } }
  )
  if (!data.success) throw new Error(data.message || 'Something went wrong!')
  return data.data
}

export async function downloadUsageAuditPdf(report: UsageAuditReport) {
  const { data } = await api.post<Blob>(
    '/api/billing/admin/usage-audit/pdf',
    {
      filter: report.filter,
      generated_at: report.generated_at,
      sha256: report.sha256,
    },
    { responseType: 'blob' }
  )
  const url = URL.createObjectURL(data)
  const anchor = document.createElement('a')
  anchor.href = url
  anchor.download = `token-usage-${report.sha256.slice(0, 12)}.pdf`
  anchor.click()
  URL.revokeObjectURL(url)
}
