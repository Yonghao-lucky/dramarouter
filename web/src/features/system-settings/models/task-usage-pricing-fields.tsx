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
import { useEffect, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Field, FieldDescription, FieldLabel } from '@/components/ui/field'
import {
  InputGroup,
  InputGroupAddon,
  InputGroupInput,
} from '@/components/ui/input-group'
import type { BillingUsageSchema } from '@/features/pricing/types'
import {
  buildTaskUsageExpression,
  parseTaskUsageDraft,
  taskUsageFields,
  taskUsageTier,
  type TaskUsageDraft,
} from '@/features/pricing/lib/task-usage'

export {
  buildTaskUsageExpression,
  parseTaskUsageDraft,
  taskUsageFields,
  taskUsageTier,
  type TaskUsageDraft,
  type TaskUsageField,
  type TaskUsageTier,
} from '@/features/pricing/lib/task-usage'

type TaskUsagePricingFieldsProps = {
  schema: BillingUsageSchema
  billingExpr: string
  onBillingExprChange: (next: string) => void
}

/**
 * 任务类模型的用量计价表单：按输出档位填写每单位价格，
 * 计数类用量额外填写「前 N 个免费」。生成的表达式仍走 tier() 语法。
 */
export function TaskUsagePricingFields(props: TaskUsagePricingFieldsProps) {
  const { t, i18n } = useTranslation()
  const tier = useMemo(
    () => taskUsageTier(props.schema, i18n.language),
    [props.schema, i18n.language]
  )
  const fields = useMemo(
    () => taskUsageFields(props.schema, i18n.language),
    [props.schema, i18n.language]
  )
  const [draft, setDraft] = useState<TaskUsageDraft>(() =>
    parseTaskUsageDraft(props.billingExpr, tier, fields)
  )
  const lastEmitted = useRef(props.billingExpr)

  useEffect(() => {
    if (props.billingExpr === lastEmitted.current) return
    lastEmitted.current = props.billingExpr
    setDraft(parseTaskUsageDraft(props.billingExpr, tier, fields))
  }, [props.billingExpr, tier, fields])

  const update = (
    value: string,
    key: string,
    patch: Partial<{ price: string; free: string }>
  ) => {
    const next: TaskUsageDraft = { ...draft }
    const rows = { ...(next[value] ?? {}) }
    const row = rows[key] ?? { price: '', free: '' }
    rows[key] = { ...row, ...patch }
    next[value] = rows
    setDraft(next)
    const expression = buildTaskUsageExpression(tier, fields, next)
    lastEmitted.current = expression
    props.onBillingExprChange(expression)
  }

  return (
    <div className='space-y-3'>
      <p className='text-muted-foreground text-xs'>
        {t(
          'Prices are charged in USD per usage unit. Fields declared as counts accept a free allowance: only units above it are billed.'
        )}
      </p>
      {tier.values.map((value) => (
        <div
          key={value}
          className='bg-muted/30 space-y-3 rounded-md border p-3'
        >
          <h4 className='text-sm font-medium'>
            {tier.key ? `${tier.label}: ${value}` : t('Base price')}
          </h4>
          {fields.map((field) => {
            const row = draft[value]?.[field.key] ?? { price: '', free: '' }
            return (
              <div
                key={field.key}
                className='grid gap-2 sm:grid-cols-[minmax(0,1fr)_auto_auto]'
              >
                <Field className='gap-1'>
                  <FieldLabel className='text-xs font-normal'>
                    {field.label}
                  </FieldLabel>
                  {field.unit === 'count' ? (
                    <FieldDescription className='text-[11px]'>
                      {t('Charged per unit beyond the free allowance.')}
                    </FieldDescription>
                  ) : null}
                </Field>
                <Field className='w-32 gap-1'>
                  <FieldLabel className='text-[11px] font-normal'>
                    {t('Free included')}
                  </FieldLabel>
                  <InputGroup>
                    <InputGroupInput
                      inputMode='decimal'
                      value={row.free}
                      disabled={field.unit !== 'count'}
                      placeholder='0'
                      onChange={(event) =>
                        update(value, field.key, { free: event.target.value })
                      }
                    />
                    <InputGroupAddon align='inline-end'>
                      {t('units')}
                    </InputGroupAddon>
                  </InputGroup>
                </Field>
                <Field className='w-40 gap-1'>
                  <FieldLabel className='text-[11px] font-normal'>
                    {t('Price per unit')}
                  </FieldLabel>
                  <InputGroup>
                    <InputGroupAddon>$</InputGroupAddon>
                    <InputGroupInput
                      inputMode='decimal'
                      value={row.price}
                      placeholder='0'
                      onChange={(event) =>
                        update(value, field.key, { price: event.target.value })
                      }
                    />
                    <InputGroupAddon align='inline-end'>
                      /{field.unit}
                    </InputGroupAddon>
                  </InputGroup>
                </Field>
              </div>
            )
          })}
        </div>
      ))}
    </div>
  )
}
