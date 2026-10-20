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

import type { BillingUsageSchema } from '@/features/pricing/types'

import {
  buildTaskUsageExpression,
  parseTaskUsageDraft,
  taskUsageFields,
  taskUsageTier,
} from '../task-usage-pricing-fields'

const h3Schema: BillingUsageSchema = {
  seconds: { type: 'number', unit: 'second', description: { en: 'Video' } },
  resolution: { enum: ['768P', '2K'] },
  input_images: { type: 'number', unit: 'count', description: { en: 'Image' } },
  input_video_seconds: {
    type: 'number',
    unit: 'second',
    description: { en: 'Input video' },
  },
}

describe('task usage pricing expression', () => {
  const tier = taskUsageTier(h3Schema)
  const fields = taskUsageFields(h3Schema, 'en')

  test('bills images only beyond the free allowance, per resolution tier', () => {
    const expression = buildTaskUsageExpression(tier, fields, {
      '768P': {
        seconds: { price: '0.5', free: '' },
        input_images: { price: '0.2', free: '5' },
        input_video_seconds: { price: '0.5', free: '' },
      },
      '2K': {
        seconds: { price: '0.8', free: '' },
        input_images: { price: '0.2', free: '5' },
        input_video_seconds: { price: '0.8', free: '' },
      },
    })

    expect(expression).toBe(
      'u("resolution") == "768P" ? tier("768P", u("seconds") * 0.5 + max(0, u("input_images") - 5) * 0.2 + u("input_video_seconds") * 0.5) : tier("2K", u("seconds") * 0.8 + max(0, u("input_images") - 5) * 0.2 + u("input_video_seconds") * 0.8)'
    )
  })

  test('round-trips a stored expression back into the form', () => {
    const draft = {
      '768P': {
        seconds: { price: '0.5', free: '' },
        input_images: { price: '0.2', free: '2' },
      },
      '2K': { seconds: { price: '0.8', free: '' } },
    }
    const expression = buildTaskUsageExpression(tier, fields, draft)
    const parsed = parseTaskUsageDraft(expression, tier, fields)

    expect(parsed['768P'].input_images).toEqual({ price: '0.2', free: '2' })
    expect(parsed['768P'].seconds).toEqual({ price: '0.5', free: '' })
    expect(parsed['2K'].seconds).toEqual({ price: '0.8', free: '' })
  })

  test('an untouched tier still produces a valid zero-cost tier', () => {
    expect(buildTaskUsageExpression(tier, fields, {})).toBe(
      'u("resolution") == "768P" ? tier("768P", 0) : tier("2K", 0)'
    )
  })
})
