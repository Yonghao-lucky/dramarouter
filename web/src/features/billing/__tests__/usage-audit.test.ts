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
import { describe, expect, test } from 'vitest'

import {
  sumUsageAuditAmounts,
  usageAuditCsv,
  usageAuditDetailsCsv,
  usageAuditSchema,
  usageAuditSelection,
} from '../lib/usage-audit'
import type { UsageAuditRow } from '../usage-audit-api'

const cacheRow = {
  input_recalculation: {
    request_count: 1,
    excluded_cache_write_request_count: 0,
    tokens: 4825,
    cache_read_input_tokens: 4825,
    cache_creation_input_tokens: 0,
    unpriced_tokens: 0,
    platform_amount: '0.096500000',
    list_amount: '0.096500000',
  },
  id: 51,
  user_id: 8,
  username: 'customer',
  token_id: 12,
  token_name: 'production',
  created_at: 1790784000,
  request_id: 'request-cached',
  upstream_request_id: 'upstream-one',
  model: 'kimi-k3',
  group: 'kimi',
  channel_id: 40,
  is_stream: true,
  kind: 'consumption',
  protocol: 'anthropic',
  usage_semantic: 'openai',
  group_ratio: 1,
  tokens: {
    input_tokens: 96,
    cache_read_input_tokens: 4825,
    cache_creation_input_tokens: 0,
    output_tokens: 86,
    total_input_tokens: 4921,
  },
  actual_amount: '0.020174000',
  list_prices_per_million: {
    input: '20',
    cache_read: '2',
    cache_write: '25',
    output: '100',
    rounding: '0',
    total: '0',
  },
  list_cost: {
    input: '0.001920000',
    cache_read: '0.009650000',
    cache_write: '0.000000000',
    output: '0.008600000',
    rounding: '0.000004000',
    total: '0.020174000',
  },
  discounted_cost: {
    input: '0.001920000',
    cache_read: '0.009650000',
    cache_write: '0.000000000',
    output: '0.008600000',
    rounding: '0.000004000',
    total: '0.020174000',
  },
  recalculation_difference: '0.000000000',
  possible_duplicate_input_tokens: 4825,
  duplicate_status: 'needs_response_evidence',
  issue: '',
  evidence: {
    request_path: '/v1/messages',
    request_conversion: ['Claude Messages', 'OpenAI Compatible'],
    billing_path: 'billing-usage-openai',
    model_ratio: 1.4285715,
    completion_ratio: 5,
    cache_ratio: 0.1,
    cache_creation_ratio: null,
    cache_creation_ratio_5m: null,
    cache_creation_ratio_1h: null,
    cache_creation_tokens_5m: 0,
    cache_creation_tokens_1h: 0,
  },
} satisfies UsageAuditRow

describe('Usage audit selection and exports', () => {
  const values = {
    scope: 'customer' as const,
    start: '2026-10-01T00:00',
    end: '2026-10-02T00:00',
    token: '12',
    channel: '',
    model: ' kimi-k3 ',
    group: '',
    protocol: 'anthropic' as const,
  }
  test('uses Shanghai time and exact key identity for a customer or all accounts', () => {
    expect(usageAuditSelection(8, values)).toEqual({
      user_id: 8,
      token_id: 12,
      channel_id: 0,
      start_at: 1790784000,
      end_at: 1790870400,
      model: 'kimi-k3',
      group: '',
      protocol: 'anthropic',
    })
    expect(usageAuditSelection(8, { ...values, scope: 'all' }).user_id).toBe(0)
  })
  test('rejects reversed periods and unsafe key IDs', () => {
    expect(
      usageAuditSchema.safeParse({ ...values, end: values.start }).success
    ).toBe(false)
    expect(
      usageAuditSchema.safeParse({ ...values, token: '9007199254740992' })
        .success
    ).toBe(false)
  })
  test('escapes formulas and commas without turning a negative amount into text', () => {
    expect(
      usageAuditCsv([
        ['=HYPERLINK("x")', 'key,name', '-0.020174', 'line\nbreak'],
      ])
    ).toBe('\uFEFF"\'=HYPERLINK(""x"")","key,name","-0.020174","line\nbreak"')
  })
  test('exports one complete line per log with pricing evidence and automatic historical input repricing', () => {
    const report = {
      recalculation_basis: 'legacy_openai_cache_write_tokens',
      filter: {
        user_id: 8,
        token_id: 12,
        channel_id: 40,
        start_at: 1790784000,
        end_at: 1790870400,
        model: '',
        group: '',
        protocol: '',
      },
      generated_at: 1790870400,
      sha256: 'original-server-evidence',
      currency: {
        code: 'CNY',
        symbol: '¥',
        rate: '7',
        quota_per_unit: '500000',
      },
      rows: [
        cacheRow,
        {
          ...cacheRow,
          id: 52,
          request_id: 'request-missing-price',
          model: 'other-model',
          list_cost: null,
          discounted_cost: null,
          issue: 'missing_price_evidence',
          input_recalculation: {
            ...cacheRow.input_recalculation,
            unpriced_tokens: 4825,
            platform_amount: '',
            list_amount: '',
          },
        },
        {
          ...cacheRow,
          id: 53,
          request_id: 'request-write-excluded',
          tokens: { ...cacheRow.tokens, cache_creation_input_tokens: 100 },
          possible_duplicate_input_tokens: 0,
          duplicate_status: 'excluded_cache_write_present',
          input_recalculation: null,
        },
      ],
    }
    const csv = usageAuditDetailsCsv(report)
    const [headers, ...lines] = csv
      .slice(1)
      .split('\r\n')
      .map((line) => line.slice(1, -1).split('","'))
    expect(lines).toHaveLength(3)
    const records = lines.map((line) =>
      Object.fromEntries(headers.map((header, index) => [header, line[index]]))
    )
    expect(records[0]).toMatchObject({
      log_id: '51',
      request_id: 'request-cached',
      channel_id: '40',
      is_stream: 'true',
      input_tokens: '96',
      cache_read_input_tokens: '4825',
      actual_amount: '0.020174000',
      list_cache_read_amount: '0.009650000',
      platform_rounding_amount: '0.000004000',
      recalculation_basis: 'legacy_openai_cache_write_tokens',
      input_repricing_tokens: '4825',
      input_repricing_platform_amount: '0.096500000',
      input_repricing_base_amount: '0.096500000',
      input_repricing_unpriced_tokens: '0',
      currency: 'CNY',
      request_path: '/v1/messages',
      cache_read_ratio: '0.1',
    })
    expect(records[2]).toMatchObject({
      cache_creation_input_tokens: '100',
      duplicate_status: 'excluded_cache_write_present',
      possible_duplicate_input_tokens: '0',
      input_repricing_tokens: '0',
      input_repricing_platform_amount: '',
      input_repricing_base_amount: '',
      recalculation_basis: 'legacy_openai_cache_write_tokens',
    })
    expect(records[1]).toMatchObject({
      log_id: '52',
      actual_amount: '0.020174000',
      list_amount: '',
      recalculation_basis: 'legacy_openai_cache_write_tokens',
      input_repricing_tokens: '4825',
      input_repricing_platform_amount: '',
      input_repricing_base_amount: '',
      input_repricing_unpriced_tokens: '4825',
      issue: 'missing_price_evidence',
    })
  })
})

describe('Cache cost subtotals', () => {
  test('adds historical amounts exactly and retains signed differences', () => {
    expect(sumUsageAuditAmounts('577.309850281', '858.476531455')).toBe(
      '1435.786381736'
    )
    expect(sumUsageAuditAmounts('-0.000000001', '0')).toBe('-0.000000001')
  })
  test('leaves the subtotal unknown if either historical cost is missing', () => {
    expect(sumUsageAuditAmounts('0.1', undefined)).toBeNull()
  })
})
