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
import { Sunrise, Sunset } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { cn } from '@/lib/utils'

import type { PeakPricingView, PeakPricingWindow } from '../types'

const WEEKDAY_KEYS = ['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat']

function formatDays(days: number[]): string {
  if (days.length === 0 || days.length === 7) return '*'
  const sorted = [...new Set(days)].sort()
  // 连续区间折叠为 Mon-Fri，其余用逗号连接。
  const isConsecutive = sorted.every(
    (day, index) => index === 0 || day === sorted[index - 1] + 1
  )
  if (isConsecutive && sorted.length > 2) {
    return `${WEEKDAY_KEYS[sorted[0]]}-${WEEKDAY_KEYS[sorted[sorted.length - 1]]}`
  }
  return sorted.map((day) => WEEKDAY_KEYS[day] ?? String(day)).join(',')
}

function formatWindow(window: PeakPricingWindow): string {
  const days = formatDays(window.days ?? [])
  const start = window.start || '00:00'
  const end = window.end || '00:00'
  return `${days} ${start}-${end}`
}

function formatFactor(value: number): string {
  if (!Number.isFinite(value)) return '1x'
  const rounded = Math.round(value * 1000) / 1000
  return `${rounded}x`
}

/** 判断当前时刻是否落在某个窗口内（前端仅用于高亮，最终以服务端计费为准）。 */
function isWindowActive(window: PeakPricingWindow, now: Date): boolean {
  const weekday = now.getDay()
  if (window.days?.length && !window.days.includes(weekday)) return false
  const toMinutes = (value: string) => {
    const [hour, minute] = (value || '00:00').split(':').map(Number)
    return (hour || 0) * 60 + (minute || 0)
  }
  const start = toMinutes(window.start)
  const end = toMinutes(window.end)
  const current = now.getHours() * 60 + now.getMinutes()
  if (start === end) return true
  if (start < end) return current >= start && current < end
  return current >= start || current < end
}

export function isPeakActive(view: PeakPricingView, now = new Date()): boolean {
  return (view.windows ?? []).some((window) => isWindowActive(window, now))
}

/**
 * 模型卡片上的峰谷徽标：显示当前是否处于峰时，以及谷时最低系数。
 */
export function ModelPeakPricingBadge(props: { view?: PeakPricingView }) {
  const { t } = useTranslation()
  const view = props.view
  if (!view) return null

  const active = isPeakActive(view)
  const offPeak = view.off_peak
  const lowest = Math.min(
    offPeak?.input_factor ?? 1,
    offPeak?.output_factor ?? 1,
    offPeak?.cache_factor ?? 1
  )
  const hasDiscount = Number.isFinite(lowest) && lowest < 1

  return (
    <span
      title={t(
        'Peak pricing is billed by request start time; see details for the full schedule.'
      )}
      className={cn(
        'inline-flex items-center gap-1 rounded-md border px-1.5 py-0.5 text-[10px] font-medium',
        active
          ? 'border-amber-300/70 bg-amber-50 text-amber-700 dark:border-amber-500/30 dark:bg-amber-500/10 dark:text-amber-300'
          : 'border-emerald-300/70 bg-emerald-50 text-emerald-700 dark:border-emerald-500/30 dark:bg-emerald-500/10 dark:text-emerald-300'
      )}
    >
      {active ? (
        <Sunrise className='size-3' aria-hidden='true' />
      ) : (
        <Sunset className='size-3' aria-hidden='true' />
      )}
      {active ? t('Peak now') : t('Off-peak now')}
      {!active && hasDiscount && (
        <span className='font-mono tabular-nums'>
          {formatFactor(lowest)}
        </span>
      )}
    </span>
  )
}

/**
 * 模型详情里的峰谷定价表：时段、星期、四项系数，并标注当前生效行。
 */
export function ModelPeakPricingSection(props: { view?: PeakPricingView }) {
  const { t } = useTranslation()
  const view = props.view
  if (!view || (view.windows?.length ?? 0) === 0) return null

  const now = new Date()
  const rows: PeakPricingWindow[] = [...view.windows]

  return (
    <section>
      <div className='mb-3 flex flex-wrap items-center justify-between gap-2'>
        <h2 className='text-muted-foreground text-xs font-semibold tracking-wider uppercase'>
          {t('Peak & Off-peak Pricing')}
        </h2>
        <span className='text-muted-foreground text-[11px]'>
          {t('Timezone: {{tz}}', { tz: view.timezone })}
          {view.source === 'model' ? ` · ${t('Model-specific schedule')}` : ''}
        </span>
      </div>

      <div className='overflow-hidden rounded-lg border'>
        <table className='w-full text-sm'>
          <thead className='bg-muted/40'>
            <tr className='text-muted-foreground text-[10px] tracking-wider uppercase'>
              <th className='px-3 py-2 text-left font-medium'>{t('Window')}</th>
              <th className='px-3 py-2 text-right font-medium'>{t('Input')}</th>
              <th className='px-3 py-2 text-right font-medium'>
                {t('Output')}
              </th>
              <th className='px-3 py-2 text-right font-medium'>
                {t('Cache read')}
              </th>
              <th className='px-3 py-2 text-right font-medium'>
                {t('Cache write')}
              </th>
              <th className='px-3 py-2 text-right font-medium'>
                {t('Cache write 5m')}
              </th>
            </tr>
          </thead>
          <tbody>
            {rows.map((window, index) => {
              const active = isWindowActive(window, now)
              return (
                <tr
                  key={`${window.name}-${index}`}
                  className={cn(
                    'border-t',
                    active && 'bg-amber-50/60 dark:bg-amber-500/10'
                  )}
                >
                  <td className='px-3 py-2'>
                    <div className='flex items-center gap-2'>
                      <span className='font-medium'>
                        {window.name || t('Peak')}
                      </span>
                      <span className='text-muted-foreground font-mono text-xs'>
                        {formatWindow(window)}
                      </span>
                      {active && (
                        <span className='rounded bg-amber-200/70 px-1 text-[10px] font-medium text-amber-900 dark:bg-amber-500/30 dark:text-amber-100'>
                          {t('Now')}
                        </span>
                      )}
                    </div>
                  </td>
                  <td className='px-3 py-2 text-right font-mono tabular-nums'>
                    {formatFactor(window.input_factor)}
                  </td>
                  <td className='px-3 py-2 text-right font-mono tabular-nums'>
                    {formatFactor(window.output_factor)}
                  </td>
                  <td className='px-3 py-2 text-right font-mono tabular-nums'>
                    {formatFactor(window.cache_factor)}
                  </td>
                  <td className='px-3 py-2 text-right font-mono tabular-nums'>
                    {formatFactor(window.cache_write_factor)}
                  </td>
                  <td className='px-3 py-2 text-right font-mono tabular-nums'>
                    {formatFactor(
                      window.cache_write_5m_factor ?? window.cache_write_factor
                    )}
                  </td>
                </tr>
              )
            })}
            <tr className='text-muted-foreground border-t'>
              <td className='px-3 py-2'>
                {t('Off-peak')}
                <span className='ml-2 text-[11px]'>
                  {t('all other times')}
                </span>
              </td>
              <td className='px-3 py-2 text-right font-mono tabular-nums'>
                {formatFactor(view.off_peak?.input_factor ?? 1)}
              </td>
              <td className='px-3 py-2 text-right font-mono tabular-nums'>
                {formatFactor(view.off_peak?.output_factor ?? 1)}
              </td>
              <td className='px-3 py-2 text-right font-mono tabular-nums'>
                {formatFactor(view.off_peak?.cache_factor ?? 1)}
              </td>
              <td className='px-3 py-2 text-right font-mono tabular-nums'>
                {formatFactor(view.off_peak?.cache_write_factor ?? 1)}
              </td>
              <td className='px-3 py-2 text-right font-mono tabular-nums'>
                {formatFactor(
                  view.off_peak?.cache_write_5m_factor ??
                    view.off_peak?.cache_write_factor ??
                    1
                )}
              </td>
            </tr>
          </tbody>
        </table>
      </div>
      <p className='text-muted-foreground mt-2 text-[11px]'>
        {t(
          'Final price = base price × factor. The factor is locked when the request starts, so a request that begins in one window is billed entirely by that window.'
        )}
      </p>
    </section>
  )
}
