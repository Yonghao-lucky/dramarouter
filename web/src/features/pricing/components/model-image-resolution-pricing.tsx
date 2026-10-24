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
import { ImageIcon } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { formatCurrencyFromUSD } from '@/lib/currency'

import type { ImageResolutionPricingView } from '../types'

const MEGAPIXEL = 1_000_000

/** 像素上限换算成面积文案，0 表示兜底档。 */
export function formatPixelLimit(maxPixels: number): string {
  if (!Number.isFinite(maxPixels) || maxPixels <= 0) return '∞'
  const mp = maxPixels / MEGAPIXEL
  if (mp >= 1) return `${Math.round(mp * 10) / 10} MP`
  return `${Math.round(maxPixels / 1000)} KP`
}

function formatPrice(value: number): string {
  if (!Number.isFinite(value)) return '-'
  return formatCurrencyFromUSD(value, { digitsSmall: 4 })
}

/**
 * 模型卡片上的分辨率计费徽标：只提示「按分辨率计价」并给出最低档价格，
 * 具体档位在详情页展开，避免卡片拥挤。
 */
export function ModelImageResolutionBadge(props: {
  view?: ImageResolutionPricingView
}) {
  const { t } = useTranslation()
  const view = props.view
  if (!view || view.tiers.length === 0) return null

  const cheapest = view.tiers.reduce(
    (lowest, tier) => (tier.price < lowest ? tier.price : lowest),
    view.tiers[0].price
  )

  return (
    <span
      title={t(
        'Billed per image by requested resolution; see details for the tier table.'
      )}
      className='inline-flex items-center gap-1 rounded-md border border-sky-300/70 bg-sky-50 px-1.5 py-0.5 text-[10px] font-medium text-sky-700 dark:border-sky-500/30 dark:bg-sky-500/10 dark:text-sky-300'
    >
      <ImageIcon className='size-3' aria-hidden='true' />
      {t('Per resolution')}
      <span className='font-mono tabular-nums'>{formatPrice(cheapest)}+</span>
    </span>
  )
}

/**
 * 模型详情里的分辨率价格表：档位、像素上限、单价。
 */
export function ModelImageResolutionSection(props: {
  view?: ImageResolutionPricingView
}) {
  const { t } = useTranslation()
  const view = props.view
  if (!view || view.tiers.length === 0) return null

  return (
    <section>
      <div className='mb-3 flex flex-wrap items-center justify-between gap-2'>
        <h2 className='text-muted-foreground text-xs font-semibold tracking-wider uppercase'>
          {t('Image Resolution Pricing')}
        </h2>
        <span className='text-muted-foreground text-[11px]'>
          {t('Price per image, before group ratio and peak factors')}
        </span>
      </div>

      <div className='overflow-hidden rounded-lg border'>
        <table className='w-full text-sm'>
          <thead className='bg-muted/40'>
            <tr className='text-muted-foreground text-[10px] tracking-wider uppercase'>
              <th className='px-3 py-2 text-left font-medium'>
                {t('Tier name')}
              </th>
              <th className='px-3 py-2 text-right font-medium'>
                {t('Max pixels')}
              </th>
              <th className='px-3 py-2 text-right font-medium'>
                {t('Price per image')}
              </th>
            </tr>
          </thead>
          <tbody>
            {view.tiers.map((tier) => (
              <tr key={tier.name} className='border-t'>
                <td className='px-3 py-2 font-medium'>{tier.name}</td>
                <td className='text-muted-foreground px-3 py-2 text-right font-mono tabular-nums'>
                  {formatPixelLimit(tier.max_pixels)}
                </td>
                <td className='px-3 py-2 text-right font-mono tabular-nums'>
                  {formatPrice(tier.price)}
                </td>
              </tr>
            ))}
            {view.default_price ? (
              <tr className='text-muted-foreground border-t'>
                <td className='px-3 py-2'>{t('Default price (USD)')}</td>
                <td className='px-3 py-2 text-right font-mono tabular-nums'>
                  {t('unknown size')}
                </td>
                <td className='px-3 py-2 text-right font-mono tabular-nums'>
                  {formatPrice(view.default_price)}
                </td>
              </tr>
            ) : null}
          </tbody>
        </table>
      </div>
      <p className='text-muted-foreground mt-2 text-[11px]'>
        {t(
          'Tiers are matched by pixel area (width x height). The first tier whose limit covers the requested size wins; sizes that are missing or set to auto use the default price.'
        )}
      </p>
    </section>
  )
}
