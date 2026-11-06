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
import type { QuotaDataItem } from '../types'

/**
 * One row of the model ranking table.
 *
 * The dashboard's quota rows are stored per (user, model, hour), so every
 * figure here is an aggregate over the selected window.
 */
export type ModelRankingRow = {
  model_name: string
  /** Completed requests attributed to the model. */
  requests: number
  /** Total tokens (prompt + completion) attributed to the model. */
  tokens: number
  /** Charged quota in the site's quota unit (500000 = $1 by default). */
  quota: number
  /** Share of the window's total cost, 0..1. */
  share: number
  /**
   * Cost change against the equally sized window immediately before the
   * selected one. `null` when that window has no traffic for the model, so a
   * brand-new model is not reported as a meaningless "+100%".
   */
  growth_pct: number | null
  /** Cost per time bucket, oldest first; drives the inline sparkline. */
  trend: number[]
}

export type ModelRankingSortKey = 'quota' | 'tokens' | 'requests'

const HOUR_SECONDS = 3600
const DAY_SECONDS = 86400
const MIN_BUCKETS = 6
const MAX_BUCKETS = 48
/** Windows up to this length get hourly sparkline buckets. */
const HOURLY_BUCKET_MAX_SPAN = 4 * DAY_SECONDS

type Accumulator = {
  requests: number
  tokens: number
  quota: number
  trend: number[]
}

/**
 * Sparkline resolution follows the window length: hourly buckets for short
 * windows, daily for long ones, so a row always renders a handful of points
 * regardless of whether the user picked a day or a quarter.
 */
function bucketCountForSpan(spanSeconds: number): number {
  const bucketSize =
    spanSeconds <= HOURLY_BUCKET_MAX_SPAN ? HOUR_SECONDS : DAY_SECONDS
  const raw = Math.ceil(spanSeconds / bucketSize)
  if (!Number.isFinite(raw) || raw <= 0) return MIN_BUCKETS
  return Math.min(MAX_BUCKETS, Math.max(MIN_BUCKETS, raw))
}

function bucketIndexOf(
  timestamp: number,
  start: number,
  span: number,
  bucketCount: number
): number {
  if (span <= 0) return 0
  const ratio = (timestamp - start) / span
  const index = Math.floor(ratio * bucketCount)
  if (index < 0) return 0
  if (index >= bucketCount) return bucketCount - 1
  return index
}

/**
 * Cost change in percent. A previous window with no cost yields `null` rather
 * than Infinity, which is what happens for a model that just started being
 * used and has nothing to compare against.
 */
function growthPercent(current: number, previous: number): number | null {
  if (previous <= 0) return null
  return ((current - previous) / previous) * 100
}

function sumQuotaByModel(rows: QuotaDataItem[]): Map<string, number> {
  const totals = new Map<string, number>()
  for (const row of rows) {
    const model = row.model_name ?? ''
    if (!model) continue
    totals.set(model, (totals.get(model) ?? 0) + (Number(row.quota) || 0))
  }
  return totals
}

/**
 * Aggregate raw quota rows into one row per model, ranked by cost.
 *
 * `previous` holds the rows of the window immediately before `range` and is
 * used only for the change column; pass an empty array to skip comparisons.
 */
export function buildModelRankingRows(
  current: QuotaDataItem[],
  previous: QuotaDataItem[],
  range: { start_timestamp: number; end_timestamp: number }
): ModelRankingRow[] {
  const start = Number(range.start_timestamp) || 0
  const end = Number(range.end_timestamp) || 0
  const span = Math.max(0, end - start)
  const bucketCount = bucketCountForSpan(span)

  const acc = new Map<string, Accumulator>()
  for (const row of current) {
    const model = row.model_name ?? ''
    if (!model) continue
    let entry = acc.get(model)
    if (!entry) {
      entry = {
        requests: 0,
        tokens: 0,
        quota: 0,
        trend: Array.from({ length: bucketCount }, () => 0),
      }
      acc.set(model, entry)
    }
    const quota = Number(row.quota) || 0
    entry.requests += Number(row.count) || 0
    entry.tokens += Number(row.token_used) || 0
    entry.quota += quota
    entry.trend[bucketIndexOf(Number(row.created_at) || 0, start, span, bucketCount)] +=
      quota
  }

  const totalQuota = [...acc.values()].reduce((sum, e) => sum + e.quota, 0)
  const previousQuota = sumQuotaByModel(previous)

  const rows: ModelRankingRow[] = []
  for (const [model, entry] of acc) {
    rows.push({
      model_name: model,
      requests: entry.requests,
      tokens: entry.tokens,
      quota: entry.quota,
      share: totalQuota > 0 ? entry.quota / totalQuota : 0,
      growth_pct: growthPercent(entry.quota, previousQuota.get(model) ?? 0),
      trend: entry.trend,
    })
  }

  rows.sort((a, b) => {
    if (b.quota !== a.quota) return b.quota - a.quota
    return a.model_name.localeCompare(b.model_name)
  })
  return rows
}

/**
 * Re-rank rows by a table column. Cost is the default because that is the
 * figure the operator is usually hunting for; ties fall back to cost order so
 * the list never reshuffles for no reason.
 */
export function sortModelRankingRows(
  rows: ModelRankingRow[],
  key: ModelRankingSortKey
): ModelRankingRow[] {
  const sorted = [...rows]
  sorted.sort((a, b) => {
    if (b[key] !== a[key]) return b[key] - a[key]
    return a.model_name.localeCompare(b.model_name)
  })
  return sorted
}
