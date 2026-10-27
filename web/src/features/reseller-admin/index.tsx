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
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
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
  getGroups,
  getResellerBook,
  getResellerCustomers,
  listResellerProfiles,
  setCustomerGroupPrice,
  setCustomerOwner,
  setResellerCost,
  setResellerDefaultPrice,
  transferResellerQuota,
  type ResellerProfile,
  upsertResellerProfile,
} from './api'

const QUOTA_PER_UNIT = 500000

function formatQuota(value: number) {
  return (value / QUOTA_PER_UNIT).toFixed(4)
}

function ProfileBook({ profile }: { profile: ResellerProfile }) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const groups = useQuery({ queryKey: ['groups'], queryFn: getGroups })
  const book = useQuery({
    queryKey: ['reseller-admin', 'book', profile.id],
    queryFn: () => getResellerBook(profile.id),
  })
  const customers = useQuery({
    queryKey: ['reseller-admin', 'customers', profile.id],
    queryFn: () => getResellerCustomers(profile.id),
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
        reseller_id: profile.id,
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
        reseller_id: profile.id,
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
        reseller_id: profile.user_id,
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

  const groupNames = groups.data ?? []
  const costByGroup = new Map(
    (book.data?.costs ?? []).map((row) => [row.group, row.cost_ratio])
  )
  const retailByGroup = new Map(
    (book.data?.defaults ?? []).map((row) => [row.group, row.ratio])
  )

  return (
    <div className='flex flex-col gap-4'>
      <Card>
        <CardHeader className='pb-2'>
          <CardTitle className='text-sm'>
            {t('Wholesale and standing retail price')}
          </CardTitle>
        </CardHeader>
        <CardContent>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t('Group')}</TableHead>
                <TableHead>{t('Wholesale')}</TableHead>
                <TableHead>{t('Set wholesale')}</TableHead>
                <TableHead>{t('Retail')}</TableHead>
                <TableHead>{t('Set retail')}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {groupNames.map((group) => (
                <TableRow key={group}>
                  <TableCell>{group}</TableCell>
                  <TableCell className='tabular-nums'>
                    {costByGroup.get(group) ?? '-'}
                  </TableCell>
                  <TableCell>
                    <div className='flex items-center gap-2'>
                      <Input
                        className='w-24'
                        value={costDrafts[group] ?? ''}
                        onChange={(event) =>
                          setCostDrafts((prev) => ({
                            ...prev,
                            [group]: event.target.value,
                          }))
                        }
                      />
                      <Button
                        size='sm'
                        variant='outline'
                        disabled={!costDrafts[group]}
                        onClick={() => saveCost.mutate(group)}
                      >
                        {t('Save')}
                      </Button>
                    </div>
                  </TableCell>
                  <TableCell className='tabular-nums'>
                    {retailByGroup.get(group) ?? '-'}
                  </TableCell>
                  <TableCell>
                    <div className='flex items-center gap-2'>
                      <Input
                        className='w-24'
                        value={priceDrafts[group] ?? ''}
                        onChange={(event) =>
                          setPriceDrafts((prev) => ({
                            ...prev,
                            [group]: event.target.value,
                          }))
                        }
                      />
                      <Button
                        size='sm'
                        variant='outline'
                        disabled={!priceDrafts[group]}
                        onClick={() => savePrice.mutate(group)}
                      >
                        {t('Save')}
                      </Button>
                    </div>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </CardContent>
      </Card>

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
  const [selected, setSelected] = useState<ResellerProfile | null>(null)
  const [userId, setUserId] = useState('')
  const [name, setName] = useState('')

  const grant = useMutation({
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
      void queryClient.invalidateQueries({ queryKey: ['reseller-admin'] })
    },
    onError: (error: Error) => toast.error(error.message),
  })

  const disable = useMutation({
    mutationFn: (profile: ResellerProfile) =>
      upsertResellerProfile({
        user_id: profile.user_id,
        name: profile.name,
        status: profile.status === 2 ? 1 : 2,
      }),
    onSuccess: () => {
      toast.success(t('Reseller status updated'))
      void queryClient.invalidateQueries({ queryKey: ['reseller-admin'] })
    },
    onError: (error: Error) => toast.error(error.message),
  })

  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>
        {t('Reseller management')}
      </SectionPageLayout.Title>
      <SectionPageLayout.Content>
        <div className='flex flex-col gap-4'>
          <div className='flex flex-wrap items-end gap-2'>
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
              disabled={!userId || grant.isPending}
              onClick={() => grant.mutate()}
            >
              {t('Enable reseller')}
            </Button>
          </div>

          <Card>
            <CardContent className='pt-4'>
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>{t('Reseller')}</TableHead>
                    <TableHead>{t('User ID')}</TableHead>
                    <TableHead>{t('Status')}</TableHead>
                    <TableHead />
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {(profiles.data ?? []).map((profile) => (
                    <TableRow key={profile.id}>
                      <TableCell>
                        {profile.name || profile.code || '-'}
                      </TableCell>
                      <TableCell>#{profile.user_id}</TableCell>
                      <TableCell>
                        <Badge
                          variant={
                            profile.status === 2 ? 'default' : 'secondary'
                          }
                        >
                          {profile.status === 2 ? t('Enabled') : t('Disabled')}
                        </Badge>
                      </TableCell>
                      <TableCell>
                        <div className='flex items-center gap-2'>
                          <Button
                            size='sm'
                            variant='outline'
                            onClick={() => setSelected(profile)}
                          >
                            {t('Manage')}
                          </Button>
                          <Button
                            size='sm'
                            variant='ghost'
                            onClick={() => disable.mutate(profile)}
                          >
                            {profile.status === 2 ? t('Disable') : t('Enable')}
                          </Button>
                        </div>
                      </TableCell>
                    </TableRow>
                  ))}
                  {(profiles.data ?? []).length === 0 ? (
                    <TableRow>
                      <TableCell
                        colSpan={4}
                        className='text-muted-foreground text-center text-sm'
                      >
                        {t('No resellers yet')}
                      </TableCell>
                    </TableRow>
                  ) : null}
                </TableBody>
              </Table>
            </CardContent>
          </Card>

          {selected ? (
            <div className='flex flex-col gap-3'>
              <div className='text-sm font-medium'>
                {t('Managing')}: {selected.name || `#${selected.user_id}`}
              </div>
              <ProfileBook profile={selected} />
            </div>
          ) : null}
        </div>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}
