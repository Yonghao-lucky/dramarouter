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

import {
  Card,
  CardHeader,
  CardTitle,
  CardContent,
  CardDescription,
} from '@/components/ui/card'
import { toIntlLocale } from '@/i18n/languages'

import type { UsageAuditInputRecalculation } from '../usage-audit-api'
import { UsageAuditMoney } from './usage-audit-cost-table'

export function UsageAuditRecalculation(props: {
  value: UsageAuditInputRecalculation
  symbol: string
}) {
  const { t, i18n } = useTranslation()
  const value = props.value
  const locale = toIntlLocale(i18n.language)
  return (
    <Card aria-label={t('Cache read repricing')}>
      <CardHeader>
        <CardTitle>{t('Cache read repricing')}</CardTitle>
        <CardDescription>
          {t(
            'Assuming historical upstream usage used cache_write_tokens, reprice only cache reads in requests without cache writes. Requests with writes are excluded. Amounts use historical ordinary-input prices, with and without the group multiplier.'
          )}
        </CardDescription>
      </CardHeader>
      <CardContent className='space-y-3'>
        <dl className='grid gap-4 sm:grid-cols-3'>
          <div>
            <dt className='text-muted-foreground text-xs'>
              {t('Cache read tokens to reprice')}
            </dt>
            <dd className='mt-2 text-2xl font-semibold tabular-nums'>
              {value.tokens.toLocaleString(locale)}
            </dd>
          </div>
          {[
            [t('At platform input price'), value.platform_amount],
            [t('At base input price'), value.list_amount],
          ].map(([label, amount]) => (
            <div key={label}>
              <dt className='text-muted-foreground text-xs'>{label}</dt>
              <dd className='mt-2 text-lg font-semibold break-all tabular-nums'>
                {value.unpriced_tokens > 0 ? (
                  t('Historical input price unavailable')
                ) : (
                  <UsageAuditMoney value={amount} symbol={props.symbol} />
                )}
              </dd>
            </div>
          ))}
        </dl>
        <div className='bg-muted/40 space-y-2 rounded-lg p-3'>
          <p className='text-sm leading-relaxed break-words tabular-nums'>
            {t(
              'Extra input tokens = Σ(old input_tokens − correct uncached input) = Σ(cache read tokens) = {{tokens}} tokens.',
              { tokens: value.tokens.toLocaleString(locale) }
            )}
          </p>
          <p className='text-muted-foreground text-xs leading-relaxed'>
            {t(
              'Only requests without cache writes are included. Normal cache-read fees are retained; only the extra ordinary-input charge is repriced.'
            )}
          </p>
        </div>
        <dl className='grid gap-3 rounded-lg border p-3 sm:grid-cols-2'>
          {[
            [t('Requests to reprice'), value.request_count],
            [
              t('Excluded requests with cache writes'),
              value.excluded_cache_write_request_count,
            ],
          ].map(([label, count]) => (
            <div key={label}>
              <dt className='text-muted-foreground text-xs'>{label}</dt>
              <dd className='mt-1 font-semibold tabular-nums'>
                {Number(count).toLocaleString(locale)}
              </dd>
            </div>
          ))}
        </dl>
        {value.unpriced_tokens > 0 && (
          <p className='text-muted-foreground text-xs'>
            {t('{{tokens}} tokens have no historical input price.', {
              tokens: value.unpriced_tokens.toLocaleString(locale),
            })}
          </p>
        )}
      </CardContent>
    </Card>
  )
}
