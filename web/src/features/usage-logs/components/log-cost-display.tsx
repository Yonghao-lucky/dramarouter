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
import { Wrench01Icon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useTranslation } from 'react-i18next'

import { StatusBadge } from '@/components/status-badge'
import { Badge } from '@/components/ui/badge'
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { formatLogQuota } from '@/lib/format'

import { hasToolSurcharge } from '../lib/format'
import type { LogOtherData } from '../types'

interface LogCostDisplayProps {
  quota: number
  other: LogOtherData | null
}

function splitQuotaDisplay(value: string): { prefix: string; amount: string } {
  const match = value.match(/^([^0-9+\-.,\s]+)(.+)$/)
  if (!match) return { prefix: '', amount: value }
  return { prefix: match[1], amount: match[2] }
}

function ToolSurchargeMarker() {
  const { t } = useTranslation()
  const label = t('Includes tool-call surcharge')

  return (
    <Tooltip>
      <TooltipTrigger
        render={
          <Badge
            variant='warning'
            className='h-5 min-w-5 cursor-help gap-0 rounded-full px-1'
            role='img'
            aria-label={label}
            tabIndex={0}
            data-tool-surcharge-indicator='true'
          >
            <HugeiconsIcon
              icon={Wrench01Icon}
              strokeWidth={2}
              aria-hidden='true'
            />
            <span
              className='text-[9px] leading-none font-bold'
              aria-hidden='true'
            >
              +
            </span>
          </Badge>
        }
      />
      <TooltipContent>{label}</TooltipContent>
    </Tooltip>
  )
}

function QuotaBadge(props: { quota: number }) {
  const quotaDisplay = splitQuotaDisplay(formatLogQuota(props.quota))

  return (
    <span className='border-border/80 bg-muted/60 inline-flex h-6 w-fit items-center rounded-md border px-2 [font-family:var(--font-body)] text-sm leading-none font-semibold tabular-nums'>
      {quotaDisplay.prefix ? (
        <span className='mr-1'>{quotaDisplay.prefix}</span>
      ) : null}
      <span>{quotaDisplay.amount}</span>
    </span>
  )
}

function formatPeakFactor(value: number): string {
  return `${Math.round(value * 1000) / 1000}x`
}

function formatPeakPrice(value: number | undefined): string {
  return typeof value === 'number' && Number.isFinite(value)
    ? formatPeakFactor(value)
    : '-'
}

/** 峰谷标记：峰时为原价倍率，谷时显示本次实际生效的优惠倍数。 */
function PeakPricingMarker(props: {
  peak: NonNullable<LogOtherData['peak_pricing']>
}) {
  const { t } = useTranslation()
  const { peak } = props
  // 旧日志没有 is_peak 字段，但那时只有命中峰时规则才会记录 rule。
  const isPeak = peak.is_peak ?? Boolean(peak.rule)
  const headline = peak.input

  return (
    <Tooltip>
      <TooltipTrigger
        render={
          <span
            className={
              isPeak
                ? 'inline-flex w-fit items-center gap-1 rounded border border-amber-300/70 bg-amber-50 px-1 text-[10px] leading-4 font-medium text-amber-700 dark:border-amber-500/30 dark:bg-amber-500/10 dark:text-amber-300'
                : 'inline-flex w-fit items-center gap-1 rounded border border-emerald-300/70 bg-emerald-50 px-1 text-[10px] leading-4 font-medium text-emerald-700 dark:border-emerald-500/30 dark:bg-emerald-500/10 dark:text-emerald-300'
            }
            tabIndex={0}
            data-peak-pricing-indicator={isPeak ? 'peak' : 'off-peak'}
          >
            {isPeak ? t('Peak') : t('Off-peak')}
            {headline != null && Number.isFinite(headline) ? (
              <span className='font-mono tabular-nums'>
                {formatPeakFactor(headline)}
              </span>
            ) : null}
          </span>
        }
      />
      <TooltipContent>
        <div className='flex flex-col gap-0.5'>
          <span>
            {isPeak
              ? t('Billed at the peak rate')
              : t('Billed at the off-peak rate')}
          </span>
          {peak.rule ? <span>{peak.rule}</span> : null}
          {peak.holiday ? (
            <span>{t('Off-peak all day on holidays')}</span>
          ) : null}
          <span className='font-mono tabular-nums'>
            {t('Input')} {formatPeakPrice(peak.input)} · {t('Output')}{' '}
            {formatPeakPrice(peak.output)} · {t('Cache read')}{' '}
            {formatPeakPrice(peak.cache)}
          </span>
        </div>
      </TooltipContent>
    </Tooltip>
  )
}

function SubscriptionBadge(props: { quota: number }) {
  const { t } = useTranslation()

  return (
    <Tooltip>
      <TooltipTrigger
        render={
          <StatusBadge
            label={t('Subscription')}
            variant='success'
            size='sm'
            copyable={false}
            className='cursor-help'
          />
        }
      />
      <TooltipContent>
        <span>
          {t('Deducted by subscription')}: {formatLogQuota(props.quota)}
        </span>
      </TooltipContent>
    </Tooltip>
  )
}

export function LogCostDisplay(props: LogCostDisplayProps) {
  const isSubscription = props.other?.billing_source === 'subscription'
  const showToolSurcharge = hasToolSurcharge(props.other)
  const peak = props.other?.peak_pricing
  const showPeak = peak != null

  if (!isSubscription && !showToolSurcharge && !showPeak) {
    return (
      <div className='flex flex-col gap-0.5'>
        <QuotaBadge quota={props.quota} />
      </div>
    )
  }

  return (
    <TooltipProvider>
      <div className='flex flex-col gap-0.5'>
        <div className='inline-flex items-center gap-1'>
          {isSubscription ? (
            <SubscriptionBadge quota={props.quota} />
          ) : (
            <QuotaBadge quota={props.quota} />
          )}
          {showToolSurcharge ? <ToolSurchargeMarker /> : null}
        </div>
        {showPeak ? <PeakPricingMarker peak={peak} /> : null}
      </div>
    </TooltipProvider>
  )
}
