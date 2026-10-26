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
  BillingUsageSchema,
  BillingUsageUnit,
} from '@/features/pricing/types'
import { resolveLocalizedText } from '@/lib/localized-text'

export type TaskUsageField = {
  key: string
  unit: BillingUsageUnit
  label: string
}

// 每个计费档位下，每个用量字段的价格与免费额度。
export type TaskUsageDraft = Record<
  string,
  Record<string, { price: string; free: string }>
>

export type TaskUsageTier = {
  key: string | null
  label: string
  values: string[]
}

// 插件把输出分辨率声明为枚举；缺失时退回单一档位。
export function taskUsageTier(
  schema: BillingUsageSchema,
  language = 'en'
): TaskUsageTier {
  for (const [key, field] of Object.entries(schema)) {
    if (field.enum && field.enum.length > 0) {
      const label = resolveLocalizedText(field.description, language).trim()
      return { key, label: label || key, values: field.enum }
    }
  }
  return { key: null, label: '', values: ['base'] }
}

export function taskUsageFields(
  schema: BillingUsageSchema,
  language: string
): TaskUsageField[] {
  const fields: TaskUsageField[] = []
  for (const [key, field] of Object.entries(schema)) {
    if (field.enum && field.enum.length > 0) continue
    if (field.type && field.type !== 'number') continue
    const unit: BillingUsageUnit = field.unit ?? 'count'
    const label =
      resolveLocalizedText(field.description, language).trim() || key
    fields.push({ key, unit, label })
  }
  return fields
}

function tierBody(
  rows: Record<string, { price: string; free: string }> | undefined,
  fields: TaskUsageField[]
): string {
  if (!rows) return '0'
  const terms: string[] = []
  for (const field of fields) {
    const row = rows[field.key]
    if (!row) continue
    const price = Number(row.price)
    if (!Number.isFinite(price) || price <= 0) continue
    if (field.unit === 'count') {
      const free = Number(row.free)
      const allowance = Number.isFinite(free) && free > 0 ? free : 0
      // 前 N 个免费，超出部分按单价计费。
      terms.push(`max(0, u("${field.key}") - ${allowance}) * ${price}`)
      continue
    }
    terms.push(`u("${field.key}") * ${price}`)
  }
  return terms.length > 0 ? terms.join(' + ') : '0'
}

export function buildTaskUsageExpression(
  tier: TaskUsageTier,
  fields: TaskUsageField[],
  draft: TaskUsageDraft
): string {
  const values = tier.values
  if (!tier.key) {
    return `tier("base", ${tierBody(draft.base, fields)})`
  }
  let expression = ''
  for (let index = values.length - 1; index >= 0; index -= 1) {
    const value = values[index]
    const body = `tier("${value}", ${tierBody(draft[value], fields)})`
    expression =
      index === values.length - 1
        ? body
        : `u("${tier.key}") == "${value}" ? ${body} : ${expression}`
  }
  return expression
}

type TierSlice = { name: string; body: string }

// 按括号深度切分 tier("name", body)，避免被 max()/u() 里的逗号或括号误导。
function tierSlices(expression: string): TierSlice[] {
  const slices: TierSlice[] = []
  let cursor = 0
  while (cursor < expression.length) {
    const start = expression.indexOf('tier("', cursor)
    if (start < 0) break
    const nameStart = start + 'tier("'.length
    const nameEnd = expression.indexOf('"', nameStart)
    const comma = expression.indexOf(',', nameEnd)
    if (nameEnd < 0 || comma < 0) break
    // tier( 自身的左括号计为第一层，正文从名称后的逗号开始。
    let depth = 1
    let closed = false
    for (let index = comma + 1; index < expression.length; index += 1) {
      const char = expression[index]
      if (char === '(') depth += 1
      if (char !== ')') continue
      depth -= 1
      if (depth > 0) continue
      slices.push({
        name: expression.slice(nameStart, nameEnd),
        body: expression.slice(comma + 1, index).trim(),
      })
      cursor = index + 1
      closed = true
      break
    }
    if (!closed) break
  }
  return slices
}

const FREE_TERM =
  /max\(\s*0\s*,\s*u\("([^"]+)"\)\s*-\s*([0-9.]+)\s*\)\s*\*\s*([0-9.]+)/g
const UNIT_TERM = /u\("([^"]+)"\)\s*\*\s*([0-9.]+)/g

export function parseTaskUsageDraft(
  expression: string,
  tier: TaskUsageTier,
  fields: TaskUsageField[]
): TaskUsageDraft {
  const draft: TaskUsageDraft = {}
  for (const value of tier.values) draft[value] = {}
  for (const slice of tierSlices(expression)) {
    const slot = draft[slice.name] ?? (draft[slice.name] = {})
    let remainder = slice.body

    for (const match of slice.body.matchAll(FREE_TERM)) {
      const known = fields.some((field) => field.key === match[1])
      if (!known) continue
      slot[match[1]] = { free: match[2], price: match[3] }
      remainder = remainder.replace(match[0], ' ')
    }
    for (const match of remainder.matchAll(UNIT_TERM)) {
      const known = fields.some((field) => field.key === match[1])
      if (!known) continue
      slot[match[1]] = { free: slot[match[1]]?.free ?? '', price: match[2] }
    }
  }
  return draft
}
