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
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Reorder, useDragControls } from 'motion/react'
import { useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { SectionPageLayout } from '@/components/layout'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'

import {
  getResellerBook,
  getResellerCustomers,
  listResellerGrants,
  listResellerOverviews,
  listResellerProfiles,
  setCustomerGroupPrice,
  setCustomerOwner,
  setResellerCost,
  setResellerDefaultPrice,
  transferResellerQuota,
  type ResellerOverviewRow,
  type ResellerProfile,
  upsertResellerProfile,
  setResellerGroupOrder,
} from './api'
import { PanelBoundary } from '@/features/reseller/panel-boundary'

import { GrantDialog } from './grant-dialog'
import { GrantsTable } from './grants-table'
import { OverviewPanel } from './overview-panel'
import { getLobeIcon } from '@/lib/lobe-icon'

import { SettlementPanel } from './settlement-panel'

const QUOTA_PER_UNIT = 500000

function formatQuota(value: number) {
  return (value / QUOTA_PER_UNIT).toFixed(4)
}

// GroupOrderItem is one draggable row in the price book. Dragging is bound to
// the handle only, so the inputs inside stay clickable and editable.
function GroupOrderItem(props: {
  group: string
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
      {props.children}
    </Reorder.Item>
  )
}

function ProfileBook({ resellerId }: { resellerId: number }) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const book = useQuery({
    queryKey: ['reseller-admin', 'book', resellerId],
    queryFn: () => getResellerBook(resellerId),
  })
  const customers = useQuery({
    queryKey: ['reseller-admin', 'customers', resellerId],
    queryFn: () => getResellerCustomers(resellerId),
  })
  const [costDrafts, setCostDrafts] = useState<Record<string, string>>({})
  const [priceDrafts, setPriceDrafts] = useState<Record<string, string>>({})
  const [allocationDrafts, setAllocationDrafts] = useState<
    Record<number, string>
  >({})
  const [customerPriceDrafts, setCustomerPriceDrafts] = useState<
    Record<number, string>
  >({})

  const refresh = () => {
    void queryClient.invalidateQueries({ queryKey: ['reseller-admin'] })
  }

  const saveCost = useMutation({
    mutationFn: (group: string) =>
      setResellerCost({
        reseller_id: resellerId,
        group,
        ratio: costDrafts[group] ?? '',
        enabled: true,
      }),
    onSuccess: () => {
      toast.success(t('Wholesale price saved'))
      refresh()
    },
    onError: (error: Error) => toast.error(error.message),
  })

  const savePrice = useMutation({
    mutationFn: (group: string) =>
      setResellerDefaultPrice({
        reseller_id: resellerId,
        group,
        ratio: priceDrafts[group] ?? '',
      }),
    onSuccess: () => {
      toast.success(t('Default retail price saved'))
      refresh()
    },
    onError: (error: Error) => toast.error(error.message),
  })

  const saveCustomerPrice = useMutation({
    mutationFn: (payload: { userId: number; group: string }) =>
      setCustomerGroupPrice(
        payload.userId,
        payload.group,
        customerPriceDrafts[payload.userId] ?? ''
      ),
    onSuccess: () => {
      toast.success(t('Customer price saved'))
      refresh()
    },
    onError: (error: Error) => toast.error(error.message),
  })

  const saveAllocation = useMutation({
    mutationFn: (payload: { userId: number; direction: 'fund' | 'reclaim' }) =>
      transferResellerQuota({
        reseller_id: resellerId,
        customer_user_id: payload.userId,
        direction: payload.direction,
        retail_quota: Number(allocationDrafts[payload.userId] ?? 0),
      }),
    onSuccess: () => {
      toast.success(t('Quota transferred'))
      refresh()
    },
    onError: (error: Error) => toast.error(error.message),
  })

  const releaseOwner = useMutation({
    mutationFn: (userId: number) =>
      setCustomerOwner(userId, 0, 'admin release'),
    onSuccess: () => {
      toast.success(t('Ownership released'))
      refresh()
    },
    onError: (error: Error) => toast.error(error.message),
  })

  // Use the server's filtered list: it already drops groups hidden from
  // customers and groups no enabled channel serves any more. Fetching every
  // group from /api/group/ showed both.
  // The server returns the rows in the saved order. Keep a local copy so a drag
  // feels instant, and rewrite it whenever the server list changes (for example
  // after another administrator reorders it).
  const serverRows = book.data?.groups ?? []
  type GroupRow = NonNullable<NonNullable<typeof book.data>['groups']>[number]
  const [orderedRows, setOrderedRows] = useState<GroupRow[]>([])
  useEffect(() => {
    setOrderedRows(serverRows)
  }, [serverRows])

  const saveOrder = useMutation({
    mutationFn: (groups: string[]) =>
      setResellerGroupOrder({ reseller_id: resellerId, groups }),
    onError: (error: Error) => {
      toast.error(error.message)
      // Reverting to the server order keeps the screen honest after a failure.
      setOrderedRows(serverRows)
    },
  })

  const groupSections = orderedRows.reduce<
    { vendor: string; icon?: string; groups: GroupRow[] }[]
  >((sections, row) => {
    const vendor = row.vendor || t('Other')
    const last = sections[sections.length - 1]
    if (last && last.vendor === vendor) last.groups.push(row)
    else sections.push({ vendor, icon: row.vendor_icon, groups: [row] })
    return sections
  }, [])

  // Dragging inside a card reorders that card's groups; the flat list is
  // rebuilt in section order so a later drag of the cards themselves keeps
  // working off the same source of truth.
  const moveGroup = (vendor: string, groups: GroupRow[]) => {
    const next = groupSections.flatMap((section) =>
      section.vendor === vendor ? groups : section.groups
    )
    setOrderedRows(next)
    saveOrder.mutate(next.map((row) => row.group))
  }

  const costByGroup = new Map(
    (book.data?.costs ?? []).map((row) => [row.group, row.cost_ratio])
  )
  const retailByGroup = new Map(
    (book.data?.defaults ?? []).map((row) => [row.group, row.ratio])
  )
  // The platform ratio is what the platform itself charges for the group. It is
  // the reference point when deciding a wholesale price, so it sits next to the
  // columns being edited rather than on another page.
  const platformByGroup = book.data?.platform_ratios ?? {}

  return (
    <div className='flex flex-col gap-4'>
      {/* One card per provider: the flat table mixed every group into a single
          alphabetical list, which is hard to scan once a reseller covers many
          providers. Each group keeps its customer-facing description. */}
      <div className='grid gap-3 xl:grid-cols-2'>
        {groupSections.map((section) => (
          <Card key={section.vendor}>
            <CardHeader className='pb-2'>
              <CardTitle className='flex items-center gap-2 text-sm'>
                {section.icon ? getLobeIcon(section.icon, 18) : null}
                {section.vendor}
                <span className='text-muted-foreground text-xs font-normal'>
                  {t('{{count}} groups', { count: section.groups.length })}
                </span>
              </CardTitle>
            </CardHeader>
            <CardContent className='flex flex-col gap-3'>
              <Reorder.Group
                axis='y'
                values={section.groups.map((row) => row.group)}
                onReorder={(order) => {
                  const byName = new Map(
                    section.groups.map((row) => [row.group, row])
                  )
                  const next = order
                    .map((name) => byName.get(name))
                    .filter((row): row is GroupRow => row !== undefined)
                  moveGroup(section.vendor, next)
                }}
                className='flex flex-col gap-3'
              >
                {section.groups.map((row) => (
                  <GroupOrderItem key={row.group} group={row.group}>
                    <div className='flex flex-col gap-2 rounded-lg border p-3'>
                  <div className='flex flex-wrap items-baseline gap-x-3 gap-y-1'>
                    <span className='text-base font-semibold tracking-tight'>
                      {row.group}
                    </span>
                    <Badge variant='outline' className='tabular-nums'>
                      {t('Platform ratio')} {platformByGroup[row.group] ?? '-'}
                    </Badge>
                  </div>
                  {row.description ? (
                    <p className='text-muted-foreground text-xs leading-relaxed'>
                      {row.description}
                    </p>
                  ) : null}
                  <div className='grid gap-2 sm:grid-cols-2'>
                    <div className='flex flex-col gap-1'>
                      <span className='text-foreground/70 text-xs font-medium'>
                        {t('Wholesale')}{' '}
                        {costByGroup.get(row.group)
                          ? `(${costByGroup.get(row.group)})`
                          : ''}
                      </span>
                      <div className='flex items-center gap-2'>
                        <Input
                          className='h-8'
                          value={costDrafts[row.group] ?? ''}
                          onChange={(event) =>
                            setCostDrafts((prev) => ({
                              ...prev,
                              [row.group]: event.target.value,
                            }))
                          }
                        />
                        <Button
                          size='sm'
                          variant='outline'
                          disabled={!costDrafts[row.group]}
                          onClick={() => saveCost.mutate(row.group)}
                        >
                          {t('Save')}
                        </Button>
                      </div>
                    </div>
                    <div className='flex flex-col gap-1'>
                      <span className='text-foreground/70 text-xs font-medium'>
                        {t('Retail')}{' '}
                        {retailByGroup.get(row.group)
                          ? `(${retailByGroup.get(row.group)})`
                          : ''}
                      </span>
                      <div className='flex items-center gap-2'>
                        <Input
                          className='h-8'
                          value={priceDrafts[row.group] ?? ''}
                          onChange={(event) =>
                            setPriceDrafts((prev) => ({
                              ...prev,
                              [row.group]: event.target.value,
                            }))
                          }
                        />
                        <Button
                          size='sm'
                          variant='outline'
                          disabled={!priceDrafts[row.group]}
                          onClick={() => savePrice.mutate(row.group)}
                        >
                          {t('Save')}
                        </Button>
                      </div>
                    </div>
                  </div>
                    </div>
                  </GroupOrderItem>
                ))}
              </Reorder.Group>
            </CardContent>
          </Card>
        ))}
        {groupSections.length === 0 ? (
          <Card>
            <CardContent className='text-muted-foreground py-8 text-center text-sm'>
              {t('No records yet')}
            </CardContent>
          </Card>
        ) : null}
      </div>

      <Card>
        <CardHeader className='pb-2'>
          <CardTitle className='text-sm'>{t('Owned customers')}</CardTitle>
        </CardHeader>
        <CardContent>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t('Customer')}</TableHead>
                <TableHead>{t('Group')}</TableHead>
                <TableHead>{t('Wallet')}</TableHead>
                <TableHead>{t('Funded')}</TableHead>
                <TableHead>{t('Used')}</TableHead>
                <TableHead>{t('Allocation')}</TableHead>
                <TableHead>{t('Retail ratio')}</TableHead>
                <TableHead />
              </TableRow>
            </TableHeader>
            <TableBody>
              {(customers.data ?? []).map((row) => (
                <TableRow key={row.user_id}>
                  <TableCell>
                    {row.username}
                    <span className='text-muted-foreground ml-2 text-xs'>
                      #{row.user_id}
                    </span>
                  </TableCell>
                  <TableCell>{row.group}</TableCell>
                  <TableCell className='tabular-nums'>
                    {formatQuota(row.wallet_quota)}
                  </TableCell>
                  <TableCell className='tabular-nums'>
                    {formatQuota(row.funded_quota)}
                  </TableCell>
                  <TableCell className='tabular-nums'>
                    {formatQuota(row.used_quota)}
                  </TableCell>
                  <TableCell>
                    <div className='flex items-center gap-2'>
                      <Input
                        className='w-28'
                        placeholder={t('Quota')}
                        value={allocationDrafts[row.user_id] ?? ''}
                        onChange={(event) =>
                          setAllocationDrafts((prev) => ({
                            ...prev,
                            [row.user_id]: event.target.value,
                          }))
                        }
                      />
                      <Button
                        size='sm'
                        variant='outline'
                        disabled={!allocationDrafts[row.user_id]}
                        onClick={() =>
                          saveAllocation.mutate({
                            userId: row.user_id,
                            direction: 'fund',
                          })
                        }
                      >
                        {t('Fund')}
                      </Button>
                      <Button
                        size='sm'
                        variant='ghost'
                        disabled={!allocationDrafts[row.user_id]}
                        onClick={() =>
                          saveAllocation.mutate({
                            userId: row.user_id,
                            direction: 'reclaim',
                          })
                        }
                      >
                        {t('Reclaim')}
                      </Button>
                    </div>
                  </TableCell>
                  <TableCell>
                    <div className='flex items-center gap-2'>
                      <Input
                        className='w-20'
                        value={customerPriceDrafts[row.user_id] ?? ''}
                        onChange={(event) =>
                          setCustomerPriceDrafts((prev) => ({
                            ...prev,
                            [row.user_id]: event.target.value,
                          }))
                        }
                      />
                      <Button
                        size='sm'
                        variant='outline'
                        disabled={!customerPriceDrafts[row.user_id]}
                        onClick={() =>
                          saveCustomerPrice.mutate({
                            userId: row.user_id,
                            group: row.group,
                          })
                        }
                      >
                        {t('Save')}
                      </Button>
                    </div>
                  </TableCell>
                  <TableCell>
                    <Button
                      size='sm'
                      variant='ghost'
                      onClick={() => releaseOwner.mutate(row.user_id)}
                    >
                      {t('Release')}
                    </Button>
                  </TableCell>
                </TableRow>
              ))}
              {(customers.data ?? []).length === 0 ? (
                <TableRow>
                  <TableCell
                    colSpan={8}
                    className='text-muted-foreground text-center text-sm'
                  >
                    {t('No customers yet')}
                  </TableCell>
                </TableRow>
              ) : null}
            </TableBody>
          </Table>
        </CardContent>
      </Card>
    </div>
  )
}

export function ResellerAdmin() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const profiles = useQuery({
    queryKey: ['reseller-admin', 'profiles'],
    queryFn: listResellerProfiles,
  })
  const overview = useQuery({
    queryKey: ['reseller-admin', 'overview'],
    queryFn: listResellerOverviews,
  })
  const [selectedId, setSelectedId] = useState<number | null>(null)
  const [topUp, setTopUp] = useState<{
    resellerId: number
    label: string
  } | null>(null)
  const [userId, setUserId] = useState('')
  const [name, setName] = useState('')

  const selected = useMemo(
    () =>
      (overview.data ?? []).find((row) => row.reseller_id === selectedId) ??
      null,
    [overview.data, selectedId]
  )
  const selectedProfile = useMemo(
    () =>
      (profiles.data ?? []).find((row) => row.user_id === selectedId) ?? null,
    [profiles.data, selectedId]
  )
  const grants = useQuery({
    queryKey: ['reseller-admin', 'grants', selectedId],
    queryFn: () => listResellerGrants(selectedId as number),
    enabled: selectedId != null,
  })

  const refresh = () =>
    queryClient.invalidateQueries({ queryKey: ['reseller-admin'] })

  const enable = useMutation({
    mutationFn: () =>
      upsertResellerProfile({
        user_id: Number(userId),
        name: name || undefined,
        status: 2,
      }),
    onSuccess: () => {
      toast.success(t('Reseller enabled'))
      setUserId('')
      setName('')
      void refresh()
    },
    onError: (error: Error) => toast.error(error.message),
  })

  const toggleStatus = useMutation({
    mutationFn: (profile: ResellerProfile) =>
      upsertResellerProfile({
        user_id: profile.user_id,
        name: profile.name,
        status: profile.status === 2 ? 1 : 2,
      }),
    onSuccess: () => {
      toast.success(t('Reseller status updated'))
      void refresh()
    },
    onError: (error: Error) => toast.error(error.message),
  })

  const labelOf = (row: ResellerOverviewRow) =>
    row.display_name || row.name || row.username || `#${row.reseller_id}`

  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>
        {t('Reseller management')}
      </SectionPageLayout.Title>
      <SectionPageLayout.Content>
        <div className='flex flex-col gap-4'>
          <Card>
            <CardHeader className='pb-2'>
              <CardTitle className='text-sm'>{t('Enable reseller')}</CardTitle>
            </CardHeader>
            <CardContent className='flex flex-wrap items-end gap-2'>
              <Input
                className='w-32'
                placeholder={t('User ID')}
                value={userId}
                onChange={(event) => setUserId(event.target.value)}
              />
              <Input
                className='w-48'
                placeholder={t('Display name')}
                value={name}
                onChange={(event) => setName(event.target.value)}
              />
              <Button
                disabled={!userId || enable.isPending}
                onClick={() => enable.mutate()}
              >
                {enable.isPending ? t('Saving') : t('Enable reseller')}
              </Button>
              <span className='text-muted-foreground text-xs'>
                {t(
                  'The user id is an existing platform account; enabling only adds the reseller capability.'
                )}
              </span>
            </CardContent>
          </Card>

          <OverviewPanel
            rows={overview.data ?? []}
            loading={overview.isPending}
            selectedId={selectedId}
            onSelect={(resellerId) =>
              setSelectedId((current) =>
                current === resellerId ? null : resellerId
              )
            }
            onTopUp={(row) =>
              setTopUp({ resellerId: row.reseller_id, label: labelOf(row) })
            }
          />

          <GrantDialog
            open={topUp != null}
            resellerId={topUp?.resellerId ?? null}
            resellerLabel={topUp?.label ?? ''}
            onClose={() => setTopUp(null)}
            onGranted={() => void refresh()}
          />

          {selected ? (
            <div className='flex flex-col gap-3'>
              <div className='flex flex-wrap items-center gap-2'>
                <span className='text-sm font-medium'>
                  {t('Managing')}: {labelOf(selected)} · #{selected.reseller_id}
                </span>
                {selectedProfile ? (
                  <Button
                    size='sm'
                    variant='outline'
                    disabled={toggleStatus.isPending}
                    onClick={() => toggleStatus.mutate(selectedProfile)}
                  >
                    {selectedProfile.status === 2 ? t('Disable') : t('Enable')}
                  </Button>
                ) : null}
              </div>
              <Card>
                <CardHeader className='pb-2'>
                  <CardTitle className='text-sm'>{t('Top-ups')}</CardTitle>
                </CardHeader>
                <CardContent>
                  <GrantsTable
                    grants={grants.data ?? []}
                    loading={grants.isPending}
                  />
                </CardContent>
              </Card>
              <SettlementPanel resellerId={selected.reseller_id} />
              <PanelBoundary title={t('Price book')}>
                <ProfileBook resellerId={selected.reseller_id} />
              </PanelBoundary>
            </div>
          ) : null}
        </div>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}
