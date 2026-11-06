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
import { useQuery } from '@tanstack/react-query'
import { ArrowDownRight, ArrowUpRight, Minus, Trophy } from 'lucide-react'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { IconBadge } from '@/components/ui/icon-badge'
import { Skeleton } from '@/components/ui/skeleton'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { getQuotaRows } from '@/features/dashboard/api'
import { buildQueryParams, getDefaultDays } from '@/features/dashboard/lib'
import {
  buildModelRankingRows,
  sortModelRankingRows,
  type ModelRankingRow,
  type ModelRankingSortKey,
} from '@/features/dashboard/lib/model-ranking'
import type {
  DashboardFilters,
  QuotaDataItem,
} from '@/features/dashboard/types'
import { toIntlLocale } from '@/i18n/languages'
import { formatCompactNumber, formatNumber, formatQuota } from '@/lib/format'
import { computeTimeRange } from '@/lib/time'
import { cn } from '@/lib/utils'
import { useAuthStore } from '@/stores/auth-store'

import { TrendSparkline } from './model-ranking-sparkline'

/** Rows shown before the "show all" toggle; matches a comfortable screen. */
const COLLAPSED_ROW_LIMIT = 10

interface ModelRankingTableProps {
  data: QuotaDataItem[]
  loading?: boolean
  filters?: DashboardFilters
}

/**
 * Per-model ranking of the selected window: cost, volume, share, a cost
 * sparkline and the change against the immediately preceding window.
 *
 * The current window's rows come from the parent (they are the same rows the
 * charts above already fetch), so only the comparison window costs a request.
 */
export function ModelRankingTable(props: ModelRankingTableProps) {
  const { t, i18n } = useTranslation()
  const user = useAuthStore((state) => state.auth.user)
  const isAdmin = !!(user?.role && user.role >= 10)
  const isReseller = user?.reseller_enabled === true
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)

  const [sortKey, setSortKey] = useState<ModelRankingSortKey>('quota')
  const [expanded, setExpanded] = useState(false)

  const timeRange = useMemo(
    () =>
      computeTimeRange(
        getDefaultDays(props.filters?.time_granularity),
        props.filters?.start_timestamp,
        props.filters?.end_timestamp
      ),
    [
      props.filters?.time_granularity,
      props.filters?.start_timestamp,
      props.filters?.end_timestamp,
    ]
  )

  // The comparison window is the same length immediately before the current
  // one, so "+12%" always means "against the previous period of equal size".
  const previousRange = useMemo(() => {
    const span = Math.max(
      0,
      timeRange.end_timestamp - timeRange.start_timestamp
    )
    return {
      start_timestamp: timeRange.start_timestamp - span,
      end_timestamp: timeRange.start_timestamp,
    }
  }, [timeRange.start_timestamp, timeRange.end_timestamp])

  const previousQuery = useQuery({
    queryKey: [
      'dashboard',
      'models',
      'ranking-previous',
      previousRange.start_timestamp,
      previousRange.end_timestamp,
      props.filters?.time_granularity,
      props.filters?.username,
      isAdmin,
      isReseller,
    ],
    queryFn: () =>
      getQuotaRows(
        buildQueryParams(previousRange, props.filters),
        { isAdmin, isReseller }
      ),
    staleTime: 60 * 1000,
  })

  const rows = useMemo(
    () =>
      buildModelRankingRows(
        props.loading ? [] : props.data,
        previousQuery.data ?? [],
        timeRange
      ),
    [props.data, props.loading, previousQuery.data, timeRange]
  )

  const sortedRows = useMemo(
    () => sortModelRankingRows(rows, sortKey),
    [rows, sortKey]
  )
  const visibleRows = expanded
    ? sortedRows
    : sortedRows.slice(0, COLLAPSED_ROW_LIMIT)
  const hasMore = sortedRows.length > COLLAPSED_ROW_LIMIT
  const totalQuota = rows.reduce((sum, row) => sum + row.quota, 0)

  const columns: Array<{
    key: string
    label: string
    sortKey?: ModelRankingSortKey
  }> = [
    { key: 'model', label: t('Model') },
    { key: 'requests', label: t('Requests'), sortKey: 'requests' },
    { key: 'tokens', label: t('Tokens'), sortKey: 'tokens' },
    { key: 'quota', label: t('Cost'), sortKey: 'quota' },
    { key: 'share', label: t('Share') },
    { key: 'trend', label: t('Trend') },
    { key: 'growth', label: t('Change') },
  ]

  let body: React.ReactNode
  if (props.loading) {
    body = (
      <div className='space-y-2 px-3 py-3 sm:px-5 sm:py-4'>
        {Array.from({ length: 5 }, (_, index) => (
          <div key={index} className='flex items-center gap-3'>
            <Skeleton className='h-4 w-48' />
            <Skeleton className='ml-auto h-4 w-16' />
            <Skeleton className='h-4 w-16' />
            <Skeleton className='h-4 w-20' />
          </div>
        ))}
      </div>
    )
  } else if (sortedRows.length === 0) {
    body = (
      <div className='text-muted-foreground px-4 py-8 text-center text-xs'>
        {t('No model usage data')}
      </div>
    )
  } else {
    body = (
      <>
        <Table>
          <TableHeader>
            <TableRow>
              {columns.map((column) => {
                const columnSortKey = column.sortKey
                const isActive =
                  columnSortKey !== undefined && sortKey === columnSortKey
                let sortState: 'ascending' | 'descending' | 'none' | undefined
                if (columnSortKey !== undefined) {
                  sortState = isActive ? 'descending' : 'none'
                }
                return (
                  <TableHead
                    key={column.key}
                    className={cn(
                      'text-muted-foreground h-9 text-xs font-medium',
                      column.key === 'model' && 'pl-3 sm:pl-5',
                      column.key !== 'model' && 'text-right',
                      column.key === 'trend' && 'w-24',
                      columnSortKey !== undefined &&
                        'hover:text-foreground cursor-pointer select-none'
                    )}
                    onClick={
                      columnSortKey
                        ? () => setSortKey(columnSortKey)
                        : undefined
                    }
                    aria-sort={sortState}
                  >
                    {column.label}
                    {isActive && (
                      <span className='text-muted-foreground/60 ml-1'>↓</span>
                    )}
                  </TableHead>
                )
              })}
            </TableRow>
          </TableHeader>
          <TableBody>
            {visibleRows.map((row, index) => (
              <RankingRow
                key={row.model_name}
                row={row}
                rank={index + 1}
                totalQuota={totalQuota}
                locale={locale}
              />
            ))}
          </TableBody>
        </Table>

        {hasMore && (
          <div className='border-t px-3 py-2 text-center sm:px-5'>
            <button
              type='button'
              onClick={() => setExpanded((prev) => !prev)}
              className='text-muted-foreground hover:text-foreground text-xs font-medium transition-colors'
            >
              {expanded
                ? t('Show less')
                : t('Show all {{count}} models', {
                    count: sortedRows.length,
                  })}
            </button>
          </div>
        )}
      </>
    )
  }

  return (
    <div className='overflow-hidden rounded-lg border'>
      <div className='flex flex-wrap items-center gap-x-2 gap-y-1 border-b px-3 py-2 sm:px-5 sm:py-3'>
        <IconBadge tone='chart-2' size='sm'>
          <Trophy />
        </IconBadge>
        <div className='text-sm font-semibold'>{t('Model Ranking')}</div>
        <span className='text-muted-foreground text-xs'>
          {rows.length} {t('models')}
        </span>
        <span className='text-muted-foreground/70 ml-auto hidden text-xs sm:inline'>
          {t('Change compares against the previous period')}
        </span>
      </div>
      {body}
    </div>
  )
}

function RankingRow(props: {
  row: ModelRankingRow
  rank: number
  totalQuota: number
  locale: Intl.LocalesArgument
}) {
  const sharePercent =
    props.totalQuota > 0 ? (props.row.quota / props.totalQuota) * 100 : 0

  return (
    <TableRow className='[&>td]:h-11'>
      <TableCell className='max-w-0 pl-3 sm:pl-5'>
        <div className='flex items-center gap-2'>
          <span className='text-muted-foreground/70 w-4 shrink-0 text-right font-mono text-[11px] tabular-nums'>
            {props.rank}
          </span>
          <span className='text-foreground truncate font-mono text-xs font-medium'>
            {props.row.model_name}
          </span>
        </div>
      </TableCell>
      <TableCell className='text-right font-mono text-xs tabular-nums'>
        {formatNumber(props.row.requests, props.locale)}
      </TableCell>
      <TableCell className='text-right font-mono text-xs tabular-nums'>
        {formatCompactNumber(props.row.tokens, props.locale)}
      </TableCell>
      <TableCell className='text-right font-mono text-xs font-medium tabular-nums'>
        {formatQuota(props.row.quota)}
      </TableCell>
      <TableCell className='text-right'>
        <div className='flex items-center justify-end gap-1.5'>
          <div className='bg-muted h-1 w-10 overflow-hidden rounded-full'>
            <div
              className='bg-chart-1 h-full rounded-full'
              style={{ width: `${Math.min(100, sharePercent)}%` }}
            />
          </div>
          <span className='text-muted-foreground w-9 text-right font-mono text-[11px] tabular-nums'>
            {sharePercent.toFixed(1)}%
          </span>
        </div>
      </TableCell>
      <TableCell className='text-right'>
        <TrendSparkline values={props.row.trend} growth={props.row.growth_pct} />
      </TableCell>
      <TableCell className='text-right'>
        <GrowthCell growth={props.row.growth_pct} />
      </TableCell>
    </TableRow>
  )
}

function GrowthCell(props: { growth: number | null }) {
  const { t } = useTranslation()

  if (props.growth === null) {
    return (
      <span className='text-muted-foreground/60 inline-flex items-center gap-0.5 text-[11px]'>
        <Minus className='size-3' />
        {t('New')}
      </span>
    )
  }

  if (props.growth === 0) {
    return (
      <span className='text-muted-foreground/60 inline-flex items-center gap-0.5 text-[11px] tabular-nums'>
        <Minus className='size-3' />
        0%
      </span>
    )
  }

  const isUp = props.growth > 0
  const Icon = isUp ? ArrowUpRight : ArrowDownRight
  // A model with almost no traffic can post a four-digit percentage, which
  // reads as noise; cap the display so the column stays scannable.
  const display =
    Math.abs(props.growth) > 999
      ? '999+%'
      : `${Math.abs(props.growth).toFixed(1)}%`

  return (
    <span
      className={cn(
        'inline-flex items-center gap-0.5 font-mono text-[11px] tabular-nums',
        isUp ? 'text-success' : 'text-destructive'
      )}
    >
      <Icon className='size-3' />
      {display}
    </span>
  )
}
