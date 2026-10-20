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
import { useId, useState } from 'react'
import { useTranslation } from 'react-i18next'

import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { toIntlLocale } from '@/i18n/languages'

import { sumUsageAuditAmounts } from '../lib/usage-audit'
import type { UsageAuditSummary } from '../usage-audit-api'

export function UsageAuditMoney(props: {
  value?: string | null
  symbol: string
}) {
  const { i18n } = useTranslation()
  if (props.value == null || props.value === '') return <span>—</span>
  return (
    <span title={props.value}>
      {props.symbol}{' '}
      {Number(props.value).toLocaleString(toIntlLocale(i18n.language), {
        minimumFractionDigits: 6,
        maximumFractionDigits: 9,
      })}
    </span>
  )
}

export function UsageAuditCostTable(props: {
  summary: UsageAuditSummary
  symbol: string
  label: string
}) {
  const { t } = useTranslation()
  const summary = props.summary
  const comparison = summary.comparison
  const [cacheExpanded, setCacheExpanded] = useState(false)
  const cacheDetailsId = useId()
  const rows = [
    {
      label: t('Input'),
      tokens: summary.tokens.input_tokens,
      field: 'input' as const,
    },
    {
      label: t('Cache'),
      tokens:
        summary.tokens.cache_read_input_tokens +
        summary.tokens.cache_creation_input_tokens,
      field: 'cache' as const,
    },
    ...(cacheExpanded
      ? [
          {
            label: t('Cache read'),
            tokens: summary.tokens.cache_read_input_tokens,
            field: 'cache_read' as const,
          },
          {
            label: t('Cache write'),
            tokens: summary.tokens.cache_creation_input_tokens,
            field: 'cache_write' as const,
          },
        ]
      : []),
    {
      label: t('Output'),
      tokens: summary.tokens.output_tokens,
      field: 'output' as const,
    },
  ]
  const adjustments = [
    { label: t('Rounding adjustment'), field: 'rounding' as const },
    { label: t('Accounting adjustment'), field: 'adjustment' as const },
  ].filter(
    (row) =>
      comparison &&
      (Number(comparison.platform[row.field] || 0) !== 0 ||
        Number(comparison.list[row.field] || 0) !== 0)
  )
  return (
    <Table aria-label={props.label}>
      <TableHeader>
        <TableRow>
          <TableHead>{t('Token category')}</TableHead>
          <TableHead className='text-right'>{t('Tokens')}</TableHead>
          <TableHead className='text-right'>{t('Platform cost')}</TableHead>
          <TableHead className='text-right'>{t('List price cost')}</TableHead>
          <TableHead className='text-right'>{t('Price difference')}</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {rows.map((row) => (
          <TableRow
            key={row.field}
            id={row.field === 'cache_read' ? cacheDetailsId : undefined}
          >
            <TableCell
              className={
                row.field === 'cache_read' || row.field === 'cache_write'
                  ? 'text-muted-foreground pl-7'
                  : undefined
              }
            >
              {row.field === 'cache' ? (
                <button
                  type='button'
                  className='focus-visible:outline-ring inline-flex cursor-pointer items-center gap-2 rounded-sm focus-visible:outline-2'
                  aria-expanded={cacheExpanded}
                  aria-controls={cacheExpanded ? cacheDetailsId : undefined}
                  onClick={() => setCacheExpanded(!cacheExpanded)}
                >
                  <span aria-hidden='true'>{cacheExpanded ? '▾' : '▸'}</span>
                  {row.label}
                </button>
              ) : (
                row.label
              )}
            </TableCell>
            <TableCell className='text-right tabular-nums'>
              {row.tokens.toLocaleString()}
            </TableCell>
            <TableCell className='text-right'>
              <UsageAuditMoney
                value={
                  row.field === 'cache'
                    ? sumUsageAuditAmounts(
                        comparison?.platform.cache_read,
                        comparison?.platform.cache_write
                      )
                    : comparison?.platform[row.field]
                }
                symbol={props.symbol}
              />
            </TableCell>
            <TableCell className='text-right'>
              <UsageAuditMoney
                value={
                  row.field === 'cache'
                    ? sumUsageAuditAmounts(
                        comparison?.list.cache_read,
                        comparison?.list.cache_write
                      )
                    : comparison?.list[row.field]
                }
                symbol={props.symbol}
              />
            </TableCell>
            <TableCell className='text-right'>
              <UsageAuditMoney
                value={
                  row.field === 'cache'
                    ? sumUsageAuditAmounts(
                        comparison?.difference.cache_read,
                        comparison?.difference.cache_write
                      )
                    : comparison?.difference[row.field]
                }
                symbol={props.symbol}
              />
            </TableCell>
          </TableRow>
        ))}
        {adjustments.map((row) => (
          <TableRow key={row.field}>
            <TableCell>{row.label}</TableCell>
            <TableCell className='text-right'>—</TableCell>
            <TableCell className='text-right'>
              <UsageAuditMoney
                value={comparison?.platform[row.field]}
                symbol={props.symbol}
              />
            </TableCell>
            <TableCell className='text-right'>
              <UsageAuditMoney
                value={comparison?.list[row.field]}
                symbol={props.symbol}
              />
            </TableCell>
            <TableCell className='text-right'>
              <UsageAuditMoney
                value={comparison?.difference[row.field]}
                symbol={props.symbol}
              />
            </TableCell>
          </TableRow>
        ))}
        <TableRow className='bg-muted/40 font-medium'>
          <TableCell>{t('Total')}</TableCell>
          <TableCell className='text-right'>
            {(
              summary.tokens.total_input_tokens + summary.tokens.output_tokens
            ).toLocaleString()}
          </TableCell>
          <TableCell className='text-right'>
            <UsageAuditMoney
              value={comparison?.platform.total}
              symbol={props.symbol}
            />
          </TableCell>
          <TableCell className='text-right'>
            <UsageAuditMoney
              value={comparison?.list.total}
              symbol={props.symbol}
            />
          </TableCell>
          <TableCell className='text-right'>
            <UsageAuditMoney
              value={comparison?.difference.total}
              symbol={props.symbol}
            />
          </TableCell>
        </TableRow>
      </TableBody>
    </Table>
  )
}
