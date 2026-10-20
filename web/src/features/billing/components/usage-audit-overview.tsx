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
import { useTranslation } from 'react-i18next'

import { toIntlLocale } from '@/i18n/languages'

import type { UsageAuditSummary } from '../usage-audit-api'
import { UsageAuditMoney } from './usage-audit-cost-table'

export function UsageAuditOverview(props: {
  summary: UsageAuditSummary
  symbol: string
}) {
  const { t, i18n } = useTranslation()
  const summary = props.summary
  const comparison = summary.comparison
  const locale = toIntlLocale(i18n.language)
  return (
    <div className='flex flex-col gap-4'>
      <div className='flex flex-wrap items-baseline justify-between gap-2'>
        <span className='text-sm font-medium'>{t('Token usage')}</span>
        <span className='text-muted-foreground text-xs'>
          {t('Total tokens')}:{' '}
          <strong className='text-foreground tabular-nums'>
            {(
              summary.tokens.total_input_tokens + summary.tokens.output_tokens
            ).toLocaleString(locale)}
          </strong>
        </span>
      </div>
      <dl
        className='grid grid-cols-1 gap-3 sm:grid-cols-3'
        role='group'
        aria-label={t('Token usage')}
      >
        {[
          { label: t('Input'), value: summary.tokens.input_tokens },
          {
            label: t('Cache'),
            value:
              summary.tokens.cache_read_input_tokens +
              summary.tokens.cache_creation_input_tokens,
          },
          { label: t('Output'), value: summary.tokens.output_tokens },
        ].map((metric) => (
          <div
            key={metric.label}
            className='bg-primary/5 border-primary/15 min-w-0 rounded-xl border p-4'
          >
            <dt className='text-muted-foreground text-xs'>{metric.label}</dt>
            <dd className='text-primary mt-2 text-2xl font-semibold break-all tabular-nums'>
              {metric.value.toLocaleString(locale)}
            </dd>
            {metric.label === t('Cache') && (
              <p className='text-muted-foreground mt-2 text-xs'>
                {t('Cache read')}:{' '}
                {summary.tokens.cache_read_input_tokens.toLocaleString(locale)}{' '}
                · {t('Cache write')}:{' '}
                {summary.tokens.cache_creation_input_tokens.toLocaleString(
                  locale
                )}
              </p>
            )}
          </div>
        ))}
      </dl>
      <dl
        className='bg-muted/40 grid grid-cols-3 gap-3 rounded-xl p-4'
        role='group'
        aria-label={t('Cost comparison')}
      >
        {[
          [t('Platform cost'), comparison?.platform.total],
          [t('List price cost'), comparison?.list.total],
          [t('Price difference'), comparison?.difference.total],
        ].map(([label, value]) => (
          <div key={label} className='min-w-0'>
            <dt className='text-muted-foreground text-xs'>{label}</dt>
            <dd className='mt-2 text-sm font-medium break-all tabular-nums sm:text-base'>
              <UsageAuditMoney value={value} symbol={props.symbol} />
            </dd>
          </div>
        ))}
      </dl>
    </div>
  )
}
