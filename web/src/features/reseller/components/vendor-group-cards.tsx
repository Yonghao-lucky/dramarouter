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
import { Drag01Icon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { Reorder, useDragControls } from 'motion/react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { getLobeIcon } from '@/lib/lobe-icon'

/** The fields the price book needs to attribute a group to a provider. */
export type VendorGroupedRow = {
  group: string
  vendor?: string
  vendor_icon?: string
}

export type VendorSection<T> = {
  vendor: string
  icon?: string
  rows: T[]
}

/**
 * Split rows into consecutive runs of the same provider.
 *
 * Runs are consecutive rather than fully bucketed because the server already
 * ordered the rows: saved drag positions first, then provider. Grouping by
 * consecutive run therefore preserves the operator's ordering inside a card
 * while still letting each provider form one card.
 */
function groupRowsByVendor<T extends VendorGroupedRow>(
  rows: T[],
  otherLabel: string
): VendorSection<T>[] {
  return rows.reduce<VendorSection<T>[]>((sections, row) => {
    const vendor = row.vendor || otherLabel
    const last = sections.at(-1)
    if (last && last.vendor === vendor) last.rows.push(row)
    else sections.push({ vendor, icon: row.vendor_icon, rows: [row] })
    return sections
  }, [])
}

interface VendorGroupCardsProps<T extends VendorGroupedRow> {
  rows: T[]
  /** Replaces the whole ordering; omit to make the cards read-only. */
  onReorder?: (groups: string[]) => void
  renderRow: (row: T) => React.ReactNode
}

/**
 * One card per provider, mirroring the admin price book. Shared by the
 * reseller's own price book and the per-customer pricing panel so both read the
 * same way and both support dragging.
 */
export function VendorGroupCards<T extends VendorGroupedRow>(
  props: VendorGroupCardsProps<T>
) {
  const { t } = useTranslation()
  const sections = groupRowsByVendor(props.rows, t('Other'))

  if (sections.length === 0) {
    return (
      <div className='text-muted-foreground rounded-lg border px-4 py-6 text-center text-xs'>
        {t('No groups available')}
      </div>
    )
  }

  /** Move one provider's rows without disturbing the other cards. */
  const reorderWithin = (vendor: string, ordered: string[]) => {
    if (!props.onReorder) return
    const byName = new Map(props.rows.map((row) => [row.group, row.group]))
    const moved = ordered.filter((name) => byName.has(name))
    const next = sections.flatMap((section) =>
      section.vendor === vendor
        ? moved
        : section.rows.map((row) => row.group)
    )
    props.onReorder(next)
  }

  return (
    <div className='grid gap-3 xl:grid-cols-2'>
      {sections.map((section) => (
        <Card key={section.vendor}>
          <CardHeader className='pb-2'>
            <CardTitle className='flex items-center gap-2 text-base'>
              {section.icon ? getLobeIcon(section.icon, 20) : null}
              {section.vendor}
              <span className='text-muted-foreground text-xs font-normal'>
                {t('{{count}} groups', { count: section.rows.length })}
              </span>
            </CardTitle>
          </CardHeader>
          <CardContent className='flex flex-col gap-2'>
            <Reorder.Group
              axis='y'
              values={section.rows.map((row) => row.group)}
              onReorder={(order) => reorderWithin(section.vendor, order)}
              className='flex flex-col gap-2'
            >
              {section.rows.map((row) => (
                <DraggableRow
                  key={row.group}
                  group={row.group}
                  draggable={props.onReorder !== undefined}
                >
                  {props.renderRow(row)}
                </DraggableRow>
              ))}
            </Reorder.Group>
          </CardContent>
        </Card>
      ))}
    </div>
  )
}

/** Binds dragging to a handle so inputs and buttons inside stay usable. */
function DraggableRow(props: {
  group: string
  draggable: boolean
  children: React.ReactNode
}) {
  const { t } = useTranslation()
  const dragControls = useDragControls()

  return (
    <Reorder.Item
      value={props.group}
      dragListener={false}
      dragControls={dragControls}
      className='bg-background relative rounded-lg'
    >
      {props.draggable && (
        <Button
          type='button'
          variant='ghost'
          size='icon-sm'
          className='text-muted-foreground absolute top-2 right-2 cursor-grab touch-none active:cursor-grabbing'
          aria-label={t('Drag {{group}} to reorder', { group: props.group })}
          onPointerDown={(event) => dragControls.start(event)}
        >
          <HugeiconsIcon icon={Drag01Icon} strokeWidth={2} aria-hidden='true' />
        </Button>
      )}
      {props.children}
    </Reorder.Item>
  )
}
