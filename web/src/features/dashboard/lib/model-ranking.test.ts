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

import type { QuotaDataItem } from '../types'
import {
  buildModelRankingRows,
  sortModelRankingRows,
} from './model-ranking'

const HOUR = 3600
const DAY = 24 * HOUR
const START = 1_700_000_000

function row(
  model: string,
  offsetSeconds: number,
  quota: number,
  count = 1,
  tokens = 100
): QuotaDataItem {
  return {
    model_name: model,
    created_at: START + offsetSeconds,
    quota,
    count,
    token_used: tokens,
  }
}

describe('buildModelRankingRows', () => {
  test('sums every hour of the window into one row per model', () => {
    const rows = buildModelRankingRows(
      [
        row('gpt-a', 0, 100, 2, 200),
        row('gpt-a', HOUR, 300, 3, 400),
        row('claude-b', HOUR, 600, 1, 50),
      ],
      [],
      { start_timestamp: START, end_timestamp: START + DAY }
    )

    expect(rows).toHaveLength(2)
    expect(rows[0]).toMatchObject({
      model_name: 'claude-b',
      quota: 600,
      requests: 1,
      tokens: 50,
      share: 0.6,
      growth_pct: null,
    })
    expect(rows[1]).toMatchObject({
      model_name: 'gpt-a',
      quota: 400,
      requests: 5,
      tokens: 600,
      share: 0.4,
    })
  })

  test('ranks by cost descending regardless of input order', () => {
    const rows = buildModelRankingRows(
      [row('cheap', 0, 10), row('expensive', 0, 900), row('middle', 0, 100)],
      [],
      { start_timestamp: START, end_timestamp: START + DAY }
    )

    expect(rows.map((r) => r.model_name)).toEqual([
      'expensive',
      'middle',
      'cheap',
    ])
  })

  test('reports growth against the comparison window and null for new models', () => {
    const rows = buildModelRankingRows(
      [row('growing', 0, 150), row('brand-new', 0, 80)],
      [row('growing', 0, 100)],
      { start_timestamp: START, end_timestamp: START + DAY }
    )

    const growing = rows.find((r) => r.model_name === 'growing')
    const brandNew = rows.find((r) => r.model_name === 'brand-new')

    expect(growing?.growth_pct).toBeCloseTo(50)
    // A model with no traffic in the comparison window has nothing to compare
    // against; "+100%" would be a fabrication.
    expect(brandNew?.growth_pct).toBeNull()
  })

  test('reports a decline as a negative percentage', () => {
    const rows = buildModelRankingRows(
      [row('shrinking', 0, 40)],
      [row('shrinking', 0, 100)],
      { start_timestamp: START, end_timestamp: START + DAY }
    )

    expect(rows[0].growth_pct).toBeCloseTo(-60)
  })

  test('places cost into the bucket matching its hour', () => {
    // A 24h window buckets hourly, so an event at +0s and one at +5h land in
    // different points of the sparkline.
    const [entry] = buildModelRankingRows(
      [row('m', 0, 100), row('m', 5 * HOUR, 700)],
      [],
      { start_timestamp: START, end_timestamp: START + DAY }
    )

    expect(entry.trend).toHaveLength(24)
    expect(entry.trend[0]).toBe(100)
    expect(entry.trend[5]).toBe(700)
    expect(entry.trend.reduce((a, b) => a + b, 0)).toBe(800)
  })

  test('caps sparkline resolution for long windows', () => {
    const [entry] = buildModelRankingRows(
      [row('m', 0, 100)],
      [],
      { start_timestamp: START, end_timestamp: START + 365 * DAY }
    )

    expect(entry.trend.length).toBeLessThanOrEqual(48)
    expect(entry.trend.length).toBeGreaterThanOrEqual(6)
  })

  test('ignores rows without a model name', () => {
    const rows = buildModelRankingRows(
      [row('', 0, 500), row('real', 0, 100)],
      [],
      { start_timestamp: START, end_timestamp: START + DAY }
    )

    expect(rows).toHaveLength(1)
    expect(rows[0].model_name).toBe('real')
    expect(rows[0].share).toBe(1)
  })

  test('avoids dividing by zero when the window has no cost', () => {
    const rows = buildModelRankingRows(
      [row('free', 0, 0)],
      [],
      { start_timestamp: START, end_timestamp: START + DAY }
    )

    expect(rows[0].share).toBe(0)
  })
})

describe('sortModelRankingRows', () => {
  const rows = buildModelRankingRows(
    [
      row('a', 0, 100, 1, 9000),
      row('b', 0, 300, 5, 1000),
      row('c', 0, 200, 3, 5000),
    ],
    [],
    { start_timestamp: START, end_timestamp: START + DAY }
  )

  test('sorts by whatever column the operator picked', () => {
    expect(sortModelRankingRows(rows, 'quota').map((r) => r.model_name)).toEqual(
      ['b', 'c', 'a']
    )
    expect(
      sortModelRankingRows(rows, 'requests').map((r) => r.model_name)
    ).toEqual(['b', 'c', 'a'])
    expect(
      sortModelRankingRows(rows, 'tokens').map((r) => r.model_name)
    ).toEqual(['a', 'c', 'b'])
  })

  test('leaves the caller array untouched', () => {
    const before = rows.map((r) => r.model_name)
    sortModelRankingRows(rows, 'tokens')
    expect(rows.map((r) => r.model_name)).toEqual(before)
  })
})
