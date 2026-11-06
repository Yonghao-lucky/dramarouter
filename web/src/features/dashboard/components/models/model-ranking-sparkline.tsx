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
import { useId, useMemo } from 'react'

import { cn } from '@/lib/utils'

const SPARKLINE_WIDTH = 80
const SPARKLINE_HEIGHT = 28
const SPARKLINE_PADDING = 3

interface TrendSparklineProps {
  /** Cost per bucket, oldest first. */
  values: number[]
  /** Drives the colour; `null` (no comparison data) renders neutral. */
  growth: number | null
}

/**
 * Inline cost sparkline for one ranking row. Kept as a plain SVG rather than a
 * VChart instance: a chart per row would mount dozens of chart engines for a
 * decoration that only needs a polyline.
 */
export function TrendSparkline(props: TrendSparklineProps) {
  const rawId = useId()
  const gradientId = `model-trend-${rawId.replaceAll(':', '')}`
  const paths = useMemo(() => buildSparklinePaths(props.values), [props.values])

  if (!paths) {
    return <span className='text-muted-foreground/40 text-[11px]'>-</span>
  }

  let tone = 'text-muted-foreground/50'
  if (props.growth !== null && props.growth !== 0) {
    tone = props.growth > 0 ? 'text-success' : 'text-destructive'
  }

  return (
    <div className={cn('ml-auto h-7 w-20', tone)} aria-hidden='true'>
      <svg viewBox='0 0 80 28' preserveAspectRatio='none' className='size-full'>
        <defs>
          <linearGradient id={gradientId} x1='0' x2='0' y1='0' y2='1'>
            <stop offset='0%' stopColor='currentColor' stopOpacity='0.22' />
            <stop offset='100%' stopColor='currentColor' stopOpacity='0' />
          </linearGradient>
        </defs>
        <path d={paths.area} fill={`url(#${gradientId})`} />
        <path
          d={paths.line}
          fill='none'
          stroke='currentColor'
          strokeLinecap='round'
          strokeLinejoin='round'
          strokeWidth='1.5'
          vectorEffect='non-scaling-stroke'
        />
      </svg>
    </div>
  )
}

function buildSparklinePaths(
  values: number[]
): { line: string; area: string } | null {
  // Fewer than two points cannot express a trend; a flat or all-zero series
  // would render a straight line at the baseline, which reads as real data.
  if (values.length < 2) return null

  const max = Math.max(...values)
  if (max <= 0) return null

  const points = values.map((value, index) => {
    const x = (index / (values.length - 1)) * SPARKLINE_WIDTH
    const normalized = Math.max(0, value) / max
    const y =
      SPARKLINE_HEIGHT -
      SPARKLINE_PADDING -
      normalized * (SPARKLINE_HEIGHT - SPARKLINE_PADDING * 2)
    return { x, y }
  })

  const first = points[0]
  const last = points.at(-1)
  if (!first || !last) return null

  const line = points
    .map((point, index) => `${index === 0 ? 'M' : 'L'} ${point.x} ${point.y}`)
    .join(' ')

  return {
    line,
    area: `${line} L ${last.x} ${SPARKLINE_HEIGHT} L ${first.x} ${SPARKLINE_HEIGHT} Z`,
  }
}
