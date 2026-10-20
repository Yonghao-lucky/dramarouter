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
import {
  ArrowDown01Icon,
  ArrowUp01Icon,
  Cancel01Icon,
  Drag01Icon,
} from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { Reorder, useDragControls } from 'motion/react'
import {
  useMemo,
  useState,
  type KeyboardEvent,
  type PointerEvent,
} from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Switch } from '@/components/ui/switch'
import { getLobeIcon } from '@/lib/lobe-icon'
import { cn } from '@/lib/utils'

import type { VendorAutoGroupOrder } from '../types'
import { GroupRatioBadge } from './auto-group-visuals'
import type { VendorGroupCatalogEntry } from './vendor-route-editor'

type VendorOrderItemProps = {
  group: string
  index: number
  count: number
  ratio?: number
  onMove: (index: number, direction: 'up' | 'down') => void
  onRemove: (group: string) => void
}

function VendorOrderItem(props: VendorOrderItemProps) {
  const { t } = useTranslation()
  const dragControls = useDragControls()

  const startDrag = (event: PointerEvent<HTMLButtonElement>) => {
    dragControls.start(event)
  }
  const handleKeyDown = (event: KeyboardEvent<HTMLButtonElement>) => {
    if (event.key === 'ArrowUp') {
      event.preventDefault()
      props.onMove(props.index, 'up')
    }
    if (event.key === 'ArrowDown') {
      event.preventDefault()
      props.onMove(props.index, 'down')
    }
  }

  return (
    <Reorder.Item
      value={props.group}
      dragListener={false}
      dragControls={dragControls}
      className='bg-background flex items-center gap-2 rounded-lg border px-2 py-1.5'
    >
      <Button
        type='button'
        variant='ghost'
        size='icon-sm'
        className='text-muted-foreground cursor-grab touch-none active:cursor-grabbing'
        aria-label={t('Drag {{group}} to reorder', { group: props.group })}
        onPointerDown={startDrag}
        onKeyDown={handleKeyDown}
      >
        <HugeiconsIcon icon={Drag01Icon} strokeWidth={2} aria-hidden='true' />
      </Button>
      <span
        aria-hidden='true'
        className='bg-primary/10 text-primary flex size-5 shrink-0 items-center justify-center rounded-full text-[10px] font-semibold tabular-nums'
      >
        {props.index + 1}
      </span>
      <span className='min-w-0 flex-1 truncate text-sm font-medium'>
        {props.group}
      </span>
      <GroupRatioBadge ratio={props.ratio} />
      <div className='flex shrink-0'>
        <Button
          type='button'
          variant='ghost'
          size='icon-sm'
          disabled={props.index === 0}
          aria-label={t('Move {{group}} up', { group: props.group })}
          onClick={() => props.onMove(props.index, 'up')}
        >
          <HugeiconsIcon icon={ArrowUp01Icon} strokeWidth={2} aria-hidden='true' />
        </Button>
        <Button
          type='button'
          variant='ghost'
          size='icon-sm'
          disabled={props.index === props.count - 1}
          aria-label={t('Move {{group}} down', { group: props.group })}
          onClick={() => props.onMove(props.index, 'down')}
        >
          <HugeiconsIcon
            icon={ArrowDown01Icon}
            strokeWidth={2}
            aria-hidden='true'
          />
        </Button>
        <Button
          type='button'
          variant='ghost'
          size='icon-sm'
          aria-label={t('Remove {{group}}', { group: props.group })}
          onClick={() => props.onRemove(props.group)}
        >
          <HugeiconsIcon icon={Cancel01Icon} strokeWidth={2} aria-hidden='true' />
        </Button>
      </div>
    </Reorder.Item>
  )
}

type VendorRoutePickerProps = {
  catalogs: VendorGroupCatalogEntry[]
  value: VendorAutoGroupOrder[]
  maxCount: number
  onChange: (value: VendorAutoGroupOrder[]) => void
}

/**
 * 按模型厂商选择并排序分组。
 * 厂商只作展示与归类：真正参与投标的是每条线路所属的分组。
 */
export function VendorRoutePicker(props: VendorRoutePickerProps) {
  const { t } = useTranslation()
  const limit =
    Number.isInteger(props.maxCount) && props.maxCount > 0 ? props.maxCount : 5
  const [expanded, setExpanded] = useState<number | null>(null)

  const ordersByVendor = useMemo(() => {
    const map = new Map<number, string[]>()
    for (const order of props.value) {
      if (order.groups.length > 0) map.set(order.vendor_id, order.groups)
    }
    return map
  }, [props.value])

  const updateVendor = (vendorId: number, groups: string[]) => {
    const next = props.value.filter((order) => order.vendor_id !== vendorId)
    if (groups.length > 0) {
      next.push({ vendor_id: vendorId, groups: groups.slice(0, limit) })
    }
    const vendorOrder = new Map(props.catalogs.map((item, index) => [item.id, index]))
    next.sort(
      (a, b) =>
        (vendorOrder.get(a.vendor_id) ?? Number.MAX_SAFE_INTEGER) -
        (vendorOrder.get(b.vendor_id) ?? Number.MAX_SAFE_INTEGER)
    )
    props.onChange(next)
  }

  const enableVendor = (vendor: VendorGroupCatalogEntry) => {
    const cheapest = [...vendor.groups]
      .sort((a, b) => a.ratio - b.ratio)
      .slice(0, Math.min(2, limit))
      .map((group) => group.name)
    updateVendor(vendor.id, cheapest)
    setExpanded(vendor.id)
  }

  const totalSelected = props.value.reduce(
    (sum, order) => sum + order.groups.length,
    0
  )

  return (
    <div className='flex flex-col gap-3'>
      <div className='grid grid-cols-2 gap-2 sm:grid-cols-3'>
        {props.catalogs.map((vendor) => {
          const selected = ordersByVendor.get(vendor.id) ?? []
          const enabled = selected.length > 0
          const isExpanded = expanded === vendor.id
          return (
            <button
              key={vendor.id}
              type='button'
              onClick={() => {
                if (!enabled) {
                  enableVendor(vendor)
                  return
                }
                setExpanded(isExpanded ? null : vendor.id)
              }}
              className={cn(
                'flex flex-col gap-1.5 rounded-xl border p-2.5 text-start transition-colors',
                enabled
                  ? 'border-primary/40 bg-primary/5'
                  : 'hover:bg-muted/40',
                isExpanded && 'ring-primary/30 ring-2'
              )}
            >
              <span className='flex items-center gap-2'>
                <span className='bg-muted flex size-6 shrink-0 items-center justify-center rounded-md'>
                  {vendor.icon ? (
                    getLobeIcon(vendor.icon, 14)
                  ) : (
                    <span className='text-[10px] font-semibold'>
                      {vendor.name.slice(0, 1).toUpperCase()}
                    </span>
                  )}
                </span>
                <span className='min-w-0 flex-1 truncate text-xs font-medium'>
                  {vendor.name}
                </span>
                <Switch
                  checked={enabled}
                  aria-label={t('Customize {{vendor}} order', {
                    vendor: vendor.name,
                  })}
                  onClick={(event) => event.stopPropagation()}
                  onCheckedChange={(checked) => {
                    if (!checked) {
                      updateVendor(vendor.id, [])
                      setExpanded(null)
                      return
                    }
                    enableVendor(vendor)
                  }}
                />
              </span>
              <span className='text-muted-foreground truncate text-[11px] tabular-nums'>
                {enabled
                  ? selected.join(' → ')
                  : t('{{count}} groups available', {
                      count: vendor.groups.length,
                    })}
              </span>
            </button>
          )
        })}
      </div>

      {expanded !== null &&
        (() => {
          const vendor = props.catalogs.find((item) => item.id === expanded)
          if (!vendor) return null
          const selected = ordersByVendor.get(vendor.id) ?? []
          const candidates = vendor.groups.filter(
            (group) => !selected.includes(group.name)
          )
          const ratioByName = new Map(
            vendor.groups.map((group) => [group.name, group.ratio])
          )

          return (
            <div className='rounded-xl border p-3'>
              <div className='mb-2 flex items-center justify-between gap-2'>
                <span className='text-sm font-medium'>
                  {t('{{vendor}} routes', { vendor: vendor.name })}
                </span>
                <span className='text-muted-foreground text-[11px]'>
                  {t('Tried top to bottom')}
                </span>
              </div>

              <Reorder.Group
                axis='y'
                values={selected}
                onReorder={(groups) => updateVendor(vendor.id, groups)}
                className='flex flex-col gap-1.5'
              >
                {selected.map((group, index) => (
                  <VendorOrderItem
                    key={group}
                    group={group}
                    index={index}
                    count={selected.length}
                    ratio={ratioByName.get(group)}
                    onMove={(targetIndex, direction) => {
                      const target = direction === 'up' ? targetIndex - 1 : targetIndex + 1
                      if (target < 0 || target >= selected.length) return
                      const next = [...selected]
                      ;[next[targetIndex], next[target]] = [next[target], next[targetIndex]]
                      updateVendor(vendor.id, next)
                    }}
                    onRemove={(group) =>
                      updateVendor(
                        vendor.id,
                        selected.filter((item) => item !== group)
                      )
                    }
                  />
                ))}
              </Reorder.Group>

              {selected.length < limit && candidates.length > 0 && (
                <div className='mt-2 flex flex-wrap gap-1.5'>
                  {candidates.map((group) => (
                    <Button
                      key={group.name}
                      type='button'
                      variant='outline'
                      size='sm'
                      className='h-7 gap-1.5 px-2 text-xs'
                      onClick={() =>
                        updateVendor(vendor.id, [...selected, group.name])
                      }
                    >
                      <span>+ {group.name}</span>
                      <GroupRatioBadge ratio={group.ratio} />
                    </Button>
                  ))}
                </div>
              )}
            </div>
          )
        })()}

      <p className='text-muted-foreground text-[11px] leading-relaxed'>
        {totalSelected > 0
          ? t(
              '{{count}} routes selected. Requests are retried in this order.'
            , { count: totalSelected })
          : t(
              'No custom route yet. Vendors without a selection follow the default Auto order.'
            )}
      </p>
    </div>
  )
}
