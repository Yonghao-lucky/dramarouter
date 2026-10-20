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
import { useMemo, type KeyboardEvent, type PointerEvent } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Switch } from '@/components/ui/switch'
import { getLobeIcon } from '@/lib/lobe-icon'
import { cn } from '@/lib/utils'

import type { VendorAutoGroupOrder } from '../types'
import { GroupRatioBadge } from './auto-group-visuals'

export type VendorGroupCatalogEntry = {
  id: number
  name: string
  icon?: string
  groups: { name: string; ratio: number; model_count: number }[]
}

type VendorRouteEditorProps = {
  catalogs: VendorGroupCatalogEntry[]
  value: VendorAutoGroupOrder[]
  maxCount: number
  onChange: (value: VendorAutoGroupOrder[]) => void
}

type VendorOrderItemProps = {
  group: string
  index: number
  count: number
  ratio?: number
  modelCount?: number
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
      {props.modelCount !== undefined && (
        <span className='text-muted-foreground shrink-0 text-[11px] tabular-nums'>
          {t('{{count}} models', { count: props.modelCount })}
        </span>
      )}
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

/**
 * 按模型厂商配置有序分组列表。
 * 一条厂商记录 = 该厂商模型的尝试顺序；未配置的厂商沿用通用 Auto 顺序。
 */
export function VendorRouteEditor(props: VendorRouteEditorProps) {
  const { t } = useTranslation()
  const limit =
    Number.isInteger(props.maxCount) && props.maxCount > 0 ? props.maxCount : 5

  const ordersByVendor = useMemo(() => {
    const map = new Map<number, string[]>()
    for (const order of props.value) {
      if (order.groups.length > 0) {
        map.set(order.vendor_id, order.groups)
      }
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

  const moveGroup = (vendorId: number, groups: string[], index: number, direction: 'up' | 'down') => {
    const target = direction === 'up' ? index - 1 : index + 1
    if (target < 0 || target >= groups.length) return
    const next = [...groups]
    ;[next[index], next[target]] = [next[target], next[index]]
    updateVendor(vendorId, next)
  }

  if (props.catalogs.length === 0) {
    return (
      <p className='text-muted-foreground text-xs'>
        {t('No model vendor catalog is available yet.')}
      </p>
    )
  }

  return (
    <div className='flex flex-col gap-3'>
      {props.catalogs.map((vendor) => {
        const selected = ordersByVendor.get(vendor.id) ?? []
        const enabled = selected.length > 0
        const candidates = vendor.groups.filter(
          (group) => !selected.includes(group.name)
        )
        const ratioByName = new Map(
          vendor.groups.map((group) => [group.name, group.ratio])
        )
        const modelCountByName = new Map(
          vendor.groups.map((group) => [group.name, group.model_count])
        )

        return (
          <div
            key={vendor.id}
            className={cn(
              'rounded-xl border transition-colors',
              enabled ? 'bg-muted/20' : 'bg-background'
            )}
          >
            <div className='flex items-center gap-3 px-3 py-2.5'>
              <span className='bg-muted flex size-7 shrink-0 items-center justify-center rounded-md'>
                {vendor.icon ? (
                  getLobeIcon(vendor.icon, 16)
                ) : (
                  <span className='text-[11px] font-semibold'>
                    {vendor.name.slice(0, 1).toUpperCase()}
                  </span>
                )}
              </span>
              <span className='min-w-0 flex-1 truncate text-sm font-medium'>
                {vendor.name}
              </span>
              <span className='text-muted-foreground shrink-0 text-xs tabular-nums'>
                {enabled
                  ? t('{{count}} routes', { count: selected.length })
                  : t('Default order')}
              </span>
              <Switch
                checked={enabled}
                aria-label={t('Customize {{vendor}} order', {
                  vendor: vendor.name,
                })}
                onCheckedChange={(checked) => {
                  if (!checked) {
                    updateVendor(vendor.id, [])
                    return
                  }
                  // 默认勾选倍率最低的 2 个分组，避免"开启即全选最贵"。
                  const defaults = [...vendor.groups]
                    .sort((a, b) => a.ratio - b.ratio)
                    .slice(0, Math.min(2, limit))
                    .map((group) => group.name)
                  updateVendor(vendor.id, defaults)
                }}
              />
            </div>

            {enabled && (
              <div className='flex flex-col gap-2 px-3 pb-3'>
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
                      modelCount={modelCountByName.get(group)}
                      onMove={(targetIndex, direction) =>
                        moveGroup(vendor.id, selected, targetIndex, direction)
                      }
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
                  <div className='flex flex-wrap gap-1.5'>
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

                {selected.length >= limit && (
                  <p className='text-muted-foreground text-[11px]'>
                    {t('At most {{max}} routes per vendor.', { max: limit })}
                  </p>
                )}
              </div>
            )}
          </div>
        )
      })}
      <p className='text-muted-foreground text-[11px] leading-relaxed'>
        {t(
          'Routes are tried top to bottom. Vendors without a custom order follow the default Auto order below.'
        )}
      </p>
    </div>
  )
}
