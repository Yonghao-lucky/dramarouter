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
import { FilmIcon } from 'lucide-react'
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import { formatCurrencyFromUSD } from '@/lib/currency'

import {
  parseTaskUsageDraft,
  taskUsageFields,
  taskUsageTier,
} from '../lib/task-usage'
import type { BillingUsageSchema } from '../types'

function formatUnitPrice(value: string): string {
  const price = Number(value)
  if (!Number.isFinite(price) || price <= 0) return '-'
  return formatCurrencyFromUSD(price, { digitsSmall: 4 })
}

function unitSuffix(unit: string, t: (key: string) => string): string {
  if (unit === 'second') return `/ ${t('second')}`
  if (unit === 'token') return `/ ${t('token')}`
  return `/ ${t('count')}`
}

/**
 * 任务类模型（视频、音频等）的用量计价表：档位是插件声明的枚举维度
 * （Hailuo / MiniMax-H3 的分辨率），每档再按用量维度列出单价与免费额度。
 * 价格来自 billing_expr，所以和实际扣费永远一致。
 */
export function ModelTaskUsagePricingSection(props: {
  schema?: BillingUsageSchema
  billingExpr?: string
}) {
  const { t, i18n } = useTranslation()
  const schema = props.schema
  const billingExpr = props.billingExpr || ''

  const tier = useMemo(
    () => (schema ? taskUsageTier(schema, i18n.language) : null),
    [schema, i18n.language]
  )
  const fields = useMemo(
    () => (schema ? taskUsageFields(schema, i18n.language) : []),
    [schema, i18n.language]
  )
  const draft = useMemo(
    () => (tier ? parseTaskUsageDraft(billingExpr, tier, fields) : {}),
    [billingExpr, tier, fields]
  )

  if (!schema || !tier || fields.length === 0) return null

  const priced = tier.values.some((value) =>
    fields.some(
      (field) => Number(draft[value]?.[field.key]?.price || 0) > 0
    )
  )
  if (!priced) return null

  return (
    <section>
      <div className='mb-3 flex flex-wrap items-center justify-between gap-2'>
        <h2 className='text-muted-foreground flex items-center gap-1.5 text-xs font-semibold tracking-wider uppercase'>
          <FilmIcon className='size-3.5' aria-hidden='true' />
          {tier.label ? `${tier.label} ${t('Pricing')}` : t('Usage Pricing')}
        </h2>
        <span className='text-muted-foreground text-[11px]'>
          {t('Price per usage unit, before group ratio')}
        </span>
      </div>

      <div className='overflow-hidden rounded-lg border'>
        <table className='w-full text-sm'>
          <thead className='bg-muted/40'>
            <tr className='text-muted-foreground text-[10px] tracking-wider uppercase'>
              <th className='px-3 py-2 text-left font-medium'>
                {tier.key ? tier.label : t('Usage')}
              </th>
              {fields.map((field) => (
                <th key={field.key} className='px-3 py-2 text-right font-medium'>
                  {field.label}
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {tier.values.map((value) => (
              <tr key={value} className='border-t'>
                <td className='px-3 py-2 font-medium'>
                  {tier.key ? value : t('Default')}
                </td>
                {fields.map((field) => {
                  const row = draft[value]?.[field.key]
                  const price = row?.price || ''
                  const free = Number(row?.free || 0)
                  return (
                    <td
                      key={field.key}
                      className='px-3 py-2 text-right font-mono tabular-nums'
                    >
                      <div>{formatUnitPrice(price)}</div>
                      <div className='text-muted-foreground/70 text-[11px]'>
                        {unitSuffix(field.unit, t)}
                        {free > 0
                          ? ` · ${t('first {{count}} free', { count: free })}`
                          : ''}
                      </div>
                    </td>
                  )
                })}
              </tr>
            ))}
          </tbody>
        </table>
      </div>

      <p className='text-muted-foreground mt-2 text-[11px]'>
        {t(
          'Each row is one selectable request option; the total is the sum of its billed usage units.'
        )}
      </p>
    </section>
  )
}
