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
import { z } from 'zod'

import type { UsageAuditFilter, UsageAuditReport } from '../usage-audit-api'

export const usageAuditSchema = z
  .object({
    scope: z.enum(['customer', 'all']),
    start: z.string().min(16),
    end: z.string().min(16),
    token: z.string().regex(/^\d+$/),
    channel: z.string().regex(/^\d*$/),
    model: z.string().trim().max(255),
    group: z.string().trim().max(64),
    protocol: z.enum(['', 'anthropic', 'openai']),
  })
  .superRefine((value, context) => {
    const start = Date.parse(`${value.start}+08:00`)
    const end = Date.parse(`${value.end}+08:00`)
    if (
      !Number.isFinite(start) ||
      !Number.isFinite(end) ||
      end <= start ||
      end - start > 93 * 86400000
    ) {
      context.addIssue({
        code: 'custom',
        path: ['end'],
        message: 'Select a valid period of up to 93 days',
      })
    }
    for (const field of ['token', 'channel'] as const) {
      if (
        !Number.isSafeInteger(Number(value[field])) ||
        Number(value[field]) > 2147483647
      ) {
        context.addIssue({
          code: 'custom',
          path: [field],
          message: 'Invalid ID',
        })
      }
    }
  })
export type UsageAuditValues = z.infer<typeof usageAuditSchema>

export function usageAuditSelection(
  userId: number,
  value: UsageAuditValues
): UsageAuditFilter {
  return {
    user_id: value.scope === 'all' ? 0 : userId,
    token_id: Number(value.token),
    channel_id: Number(value.channel),
    start_at: Date.parse(`${value.start}+08:00`) / 1000,
    end_at: Date.parse(`${value.end}+08:00`) / 1000,
    model: value.model.trim(),
    group: value.group.trim(),
    protocol: value.protocol,
  }
}

// Quote every cell, preserving numeric amounts while neutralizing spreadsheet
// formulas in customer-controlled model, account and key names.
export function usageAuditCsv(
  records: (string | number | null | undefined)[][]
) {
  return (
    '\uFEFF' +
    records
      .map((row) =>
        row
          .map((value) => {
            let cell = value == null ? '' : String(value)
            if (/^[=+\-@\t\r]/.test(cell) && !/^-?\d+(\.\d+)?$/.test(cell)) {
              cell = `'${cell}`
            }
            return '"' + cell.replaceAll('"', '""') + '"'
          })
          .join(',')
      )
      .join('\r\n')
  )
}

export function usageAuditDetailsCsv(
  report: Pick<
    UsageAuditReport,
    | 'filter'
    | 'generated_at'
    | 'sha256'
    | 'rows'
    | 'currency'
    | 'recalculation_basis'
  >
) {
  const records = [
    [
      'report_sha256',
      'user_id',
      'username',
      'token_id',
      'token_name',
      'created_at_utc',
      'request_id',
      'upstream_request_id',
      'model',
      'group',
      'group_ratio',
      'protocol',
      'usage_semantic',
      'input_tokens',
      'cache_read_input_tokens',
      'cache_creation_input_tokens',
      'output_tokens',
      'total_input_tokens',
      'actual_amount',
      'list_amount',
      'discounted_amount',
      'actual_minus_recalculated',
      'list_input_per_million',
      'list_cache_read_per_million',
      'list_cache_write_per_million',
      'list_output_per_million',
      'discounted_input_amount',
      'discounted_cache_read_amount',
      'discounted_cache_write_amount',
      'discounted_output_amount',
      'possible_duplicate_input_tokens',
      'duplicate_status',
      'issue',
      'currency',
      'currency_rate',
      'log_id',
      'log_kind',
      'created_at_shanghai',
      'channel_id',
      'is_stream',
      'list_input_amount',
      'list_cache_read_amount',
      'list_cache_write_amount',
      'list_output_amount',
      'list_rounding_amount',
      'platform_rounding_amount',
      'list_minus_platform_amount',
      'possible_duplicate_cache_read_tokens',
      'possible_duplicate_cache_write_tokens',
      'input_repricing_tokens',
      'input_repricing_platform_amount',
      'input_repricing_base_amount',
      'input_repricing_unpriced_tokens',
      'period_start_utc',
      'period_end_exclusive_utc',
      'report_generated_at_utc',
      'request_path',
      'request_conversion',
      'billing_path',
      'model_ratio',
      'completion_ratio',
      'cache_read_ratio',
      'cache_write_ratio',
      'cache_write_5m_ratio',
      'cache_write_1h_ratio',
      'cache_write_5m_tokens',
      'cache_write_1h_tokens',
      'recalculation_basis',
    ],
    ...report.rows.map((row) => {
      let priceDifference: string | null = null
      if (row.list_cost) {
        const units =
          BigInt(row.list_cost.total.replace('.', '')) -
          BigInt(row.actual_amount.replace('.', ''))
        const absolute = units < 0n ? -units : units
        priceDifference = `${units < 0n ? '-' : ''}${absolute / 1_000_000_000n}.${(absolute % 1_000_000_000n).toString().padStart(9, '0')}`
      }
      return [
        report.sha256,
        row.user_id,
        row.username,
        row.token_id,
        row.token_name,
        new Date(row.created_at * 1000).toISOString(),
        row.request_id,
        row.upstream_request_id,
        row.model,
        row.group,
        row.group_ratio,
        row.protocol,
        row.usage_semantic,
        row.tokens?.input_tokens,
        row.tokens?.cache_read_input_tokens,
        row.tokens?.cache_creation_input_tokens,
        row.tokens?.output_tokens,
        row.tokens?.total_input_tokens,
        row.actual_amount,
        row.list_cost?.total,
        row.discounted_cost?.total,
        row.recalculation_difference,
        row.list_prices_per_million?.input,
        row.list_prices_per_million?.cache_read,
        row.list_prices_per_million?.cache_write,
        row.list_prices_per_million?.output,
        row.discounted_cost?.input,
        row.discounted_cost?.cache_read,
        row.discounted_cost?.cache_write,
        row.discounted_cost?.output,
        row.possible_duplicate_input_tokens,
        row.duplicate_status,
        row.issue,
        report.currency.code,
        report.currency.rate,
        row.id,
        row.kind,
        new Date((row.created_at + 28800) * 1000)
          .toISOString()
          .slice(0, 19)
          .replace('T', ' ') + '+08:00',
        row.channel_id,
        row.is_stream ? 'true' : 'false',
        row.list_cost?.input,
        row.list_cost?.cache_read,
        row.list_cost?.cache_write,
        row.list_cost?.output,
        row.list_cost?.rounding,
        row.discounted_cost?.rounding,
        priceDifference,
        row.input_recalculation?.cache_read_input_tokens ?? 0,
        row.input_recalculation?.cache_creation_input_tokens ?? 0,
        row.input_recalculation?.tokens ?? 0,
        row.input_recalculation?.platform_amount,
        row.input_recalculation?.list_amount,
        row.input_recalculation?.unpriced_tokens ?? 0,
        new Date(report.filter.start_at * 1000).toISOString(),
        new Date(report.filter.end_at * 1000).toISOString(),
        new Date(report.generated_at * 1000).toISOString(),
        row.evidence?.request_path,
        row.evidence?.request_conversion?.join(' -> '),
        row.evidence?.billing_path,
        row.evidence?.model_ratio,
        row.evidence?.completion_ratio,
        row.evidence?.cache_ratio,
        row.evidence?.cache_creation_ratio,
        row.evidence?.cache_creation_ratio_5m,
        row.evidence?.cache_creation_ratio_1h,
        row.evidence?.cache_creation_tokens_5m,
        row.evidence?.cache_creation_tokens_1h,
        report.recalculation_basis,
      ]
    }),
  ]
  return usageAuditCsv(records)
}

export function downloadUsageAudit(
  content: string,
  filename: string,
  type: string
) {
  const url = URL.createObjectURL(new Blob([content], { type }))
  const anchor = document.createElement('a')
  anchor.href = url
  anchor.download = filename
  anchor.click()
  URL.revokeObjectURL(url)
}

// Historical cost components use at most nine decimal places. Sum fixed-point
// values exactly so the cache subtotal preserves the exported amounts.
export function sumUsageAuditAmounts(
  ...values: (string | null | undefined)[]
): string | null {
  let total = 0n
  for (const value of values) {
    if (value == null || !/^-?\d+(?:\.\d{1,9})?$/.test(value)) return null
    const [whole, fraction = ''] = value.replace('-', '').split('.')
    const amount = BigInt(whole) * 1000000000n + BigInt(fraction.padEnd(9, '0'))
    total += value.startsWith('-') ? -amount : amount
  }
  const absolute = total < 0n ? -total : total
  return `${total < 0n ? '-' : ''}${absolute / 1000000000n}.${String(absolute % 1000000000n).padStart(9, '0')}`
}
