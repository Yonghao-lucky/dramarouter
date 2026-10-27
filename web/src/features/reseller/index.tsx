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
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { SectionPageLayout } from '@/components/layout'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { formatQuota, parseQuotaFromDollars } from '@/lib/format'

import {
  createResellerCustomer,
  getResellerCustomerDetail,
  getResellerCustomerGroups,
  getResellerCustomers,
  getResellerLogs,
  getResellerMargins,
  getResellerPriceBook,
  getResellerSummary,
  getResellerTransfers,
  getResellerUsage,
  resetResellerCustomerPassword,
  setResellerCustomerGroup,
  setResellerCustomerPrice,
  setResellerCustomerStatus,
  transferResellerQuota,
} from './api'
import type { ResellerCustomer, ResellerCustomerDetail } from './types'

function StatCard(props: { title: string; value: string; hint?: string }) {
  return (
    <Card>
      <CardHeader className='pb-1'>
        <CardTitle className='text-muted-foreground text-xs font-medium'>
          {props.title}
        </CardTitle>
      </CardHeader>
      <CardContent>
        <div className='text-2xl font-semibold tabular-nums'>{props.value}</div>
        {props.hint ? (
          <div className='text-muted-foreground mt-1 text-xs'>{props.hint}</div>
        ) : null}
      </CardContent>
    </Card>
  )
}

function EmptyRow(props: { colSpan: number }) {
  const { t } = useTranslation()
  return (
    <TableRow>
      <TableCell
        colSpan={props.colSpan}
        className='text-muted-foreground py-8 text-center text-sm'
      >
        {t('No records yet')}
      </TableCell>
    </TableRow>
  )
}

function OverviewPanel() {
  const { t } = useTranslation()
  const summary = useQuery({
    queryKey: ['reseller', 'summary'],
    queryFn: getResellerSummary,
  })
  const data = summary.data
  const netCost =
    (data?.funded_cost_quota ?? 0) - (data?.reclaimed_cost_quota ?? 0)

  return (
    <div className='flex flex-col gap-4'>
      <div className='grid gap-3 sm:grid-cols-2 lg:grid-cols-4'>
        <StatCard
          title={t('My wallet')}
          value={formatQuota(data?.wallet_quota ?? 0)}
        />
        <StatCard
          title={t('Customer wallets')}
          value={formatQuota(data?.customer_balance_quota ?? 0)}
          hint={t('{{count}} customers', { count: data?.customer_count ?? 0 })}
        />
        <StatCard
          title={t('Funded (retail)')}
          value={formatQuota(data?.funded_quota ?? 0)}
        />
        <StatCard
          title={t('Net cost')}
          value={formatQuota(netCost)}
          hint={t('What you actually paid after reclaims')}
        />
      </div>
      <div className='grid gap-3 sm:grid-cols-2'>
        <StatCard
          title={t('Customer consumption')}
          value={formatQuota(data?.revenue_quota ?? 0)}
          hint={t('At your retail prices')}
        />
        <StatCard
          title={t('Realized margin')}
          value={formatQuota(data?.margin_quota ?? 0)}
          hint={t('Retail minus your wholesale cost')}
        />
      </div>
    </div>
  )
}

function CustomerDetailDialog(props: {
  customerId: number | null
  onClose: () => void
}) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [fundAmount, setFundAmount] = useState('')
  const [priceDrafts, setPriceDrafts] = useState<Record<string, string>>({})

  const detail = useQuery({
    queryKey: ['reseller', 'customer', props.customerId],
    queryFn: () => getResellerCustomerDetail(props.customerId as number),
    enabled: props.customerId != null,
  })
  const groups = useQuery({
    queryKey: ['reseller', 'customer-groups', props.customerId],
    queryFn: () => getResellerCustomerGroups(props.customerId as number),
    enabled: props.customerId != null,
  })

  const authorizedGroups = groups.data ?? []
  const customerGroupPriced = authorizedGroups.some(
    (group) => group.group === detail.data?.group
  )

  const refresh = () =>
    queryClient.invalidateQueries({ queryKey: ['reseller'] })

  const transfer = useMutation({
    mutationFn: (direction: 'fund' | 'reclaim') =>
      transferResellerQuota(props.customerId as number, {
        direction,
        retail_quota: parseQuotaFromDollars(Number(fundAmount)),
      }),
    onSuccess: (_data, direction) => {
      toast.success(
        direction === 'fund' ? t('Quota transferred') : t('Quota reclaimed')
      )
      setFundAmount('')
      void refresh()
    },
  })

  const moveCustomer = useMutation({
    mutationFn: (group: string) =>
      setResellerCustomerGroup(props.customerId as number, group),
    onSuccess: () => {
      toast.success(t('Group updated'))
      void refresh()
    },
  })

  const savePrice = useMutation({
    mutationFn: (group: string) =>
      setResellerCustomerPrice(
        props.customerId as number,
        group,
        priceDrafts[group] ?? ''
      ),
    onSuccess: () => {
      toast.success(t('Price updated'))
      void refresh()
    },
  })

  const data: ResellerCustomerDetail | undefined = detail.data

  return (
    <Dialog
      open={props.customerId != null}
      onOpenChange={() => props.onClose()}
    >
      <DialogContent className='max-h-[85vh] max-w-4xl overflow-y-auto'>
        <DialogHeader>
          <DialogTitle>
            {data
              ? `${data.username} · #${data.user_id}`
              : t('Customer detail')}
          </DialogTitle>
        </DialogHeader>

        {data ? (
          <div className='flex flex-col gap-5'>
            <div className='grid gap-3 sm:grid-cols-4'>
              <StatCard
                title={t('Wallet')}
                value={formatQuota(data.wallet_quota)}
              />
              <StatCard
                title={t('Funded')}
                value={formatQuota(data.funded_quota)}
              />
              <StatCard
                title={t('Reclaimable')}
                value={formatQuota(data.reclaimable_quota)}
              />
              <StatCard
                title={t('Used')}
                value={formatQuota(data.used_quota)}
              />
            </div>

            <div className='rounded-lg border p-3'>
              <div className='mb-2 text-sm font-medium'>
                {t('Transfer quota')}
              </div>
              <div className='flex flex-wrap items-end gap-2'>
                <Input
                  className='w-40'
                  type='number'
                  min={0}
                  placeholder={t('Retail quota')}
                  value={fundAmount}
                  onChange={(event) => setFundAmount(event.target.value)}
                />
                <Badge variant='secondary' className='h-9 px-3 leading-9'>
                  {data.group}
                </Badge>
                <Button
                  disabled={!fundAmount || transfer.isPending}
                  onClick={() => transfer.mutate('fund')}
                >
                  {t('Fund customer')}
                </Button>
                <Button
                  variant='outline'
                  disabled={!fundAmount || transfer.isPending}
                  onClick={() => transfer.mutate('reclaim')}
                >
                  {t('Reclaim')}
                </Button>
              </div>
              <p className='text-muted-foreground mt-2 text-xs'>
                {t(
                  'Funding charges your wallet the wholesale cost of that quota. Reclaiming returns what is left, at the average cost you paid.'
                )}
              </p>
              {customerGroupPriced ? null : (
                <div className='mt-2 flex flex-wrap items-center gap-2'>
                  <p className='text-destructive text-xs'>
                    {t(
                      'This customer sits in a group that is not in your price book, so you cannot fund them. Move the customer to one of your groups first.'
                    )}
                  </p>
                  {authorizedGroups.map((group) => (
                    <Button
                      key={group.group}
                      size='sm'
                      variant='outline'
                      disabled={moveCustomer.isPending}
                      onClick={() => moveCustomer.mutate(group.group)}
                    >
                      {group.group}
                    </Button>
                  ))}
                </div>
              )}
            </div>

            <div>
              <div className='mb-2 text-sm font-medium'>
                {t('Group pricing')}
              </div>
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>{t('Group')}</TableHead>
                    <TableHead>{t('My cost')}</TableHead>
                    <TableHead>{t('Customer price')}</TableHead>
                    <TableHead />
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {(groups.data ?? []).map((group) => (
                    <TableRow key={group.group}>
                      <TableCell>{group.group}</TableCell>
                      <TableCell className='tabular-nums'>
                        {group.cost_ratio}
                      </TableCell>
                      <TableCell>
                        <Input
                          className='w-24'
                          placeholder={group.ratio}
                          value={priceDrafts[group.group] ?? ''}
                          onChange={(event) =>
                            setPriceDrafts((prev) => ({
                              ...prev,
                              [group.group]: event.target.value,
                            }))
                          }
                        />
                      </TableCell>
                      <TableCell>
                        <Button
                          size='sm'
                          variant='outline'
                          disabled={!priceDrafts[group.group]}
                          onClick={() => savePrice.mutate(group.group)}
                        >
                          {t('Save')}
                        </Button>
                      </TableCell>
                    </TableRow>
                  ))}
                  {(groups.data ?? []).length === 0 ? (
                    <EmptyRow colSpan={4} />
                  ) : null}
                </TableBody>
              </Table>
            </div>

            <div>
              <div className='mb-2 text-sm font-medium'>
                {t('Recent transfers')}
              </div>
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>{t('Time')}</TableHead>
                    <TableHead>{t('Direction')}</TableHead>
                    <TableHead>{t('Retail')}</TableHead>
                    <TableHead>{t('Cost')}</TableHead>
                    <TableHead>{t('Group')}</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {(data.recent_transfers ?? []).map((row) => (
                    <TableRow key={row.id}>
                      <TableCell>
                        {new Date(row.created_at * 1000).toLocaleString()}
                      </TableCell>
                      <TableCell>
                        <Badge
                          variant={
                            row.direction === 'fund' ? 'default' : 'secondary'
                          }
                        >
                          {row.direction === 'fund' ? t('Fund') : t('Reclaim')}
                        </Badge>
                      </TableCell>
                      <TableCell className='tabular-nums'>
                        {formatQuota(row.retail_quota)}
                      </TableCell>
                      <TableCell className='tabular-nums'>
                        {formatQuota(row.cost_quota)}
                      </TableCell>
                      <TableCell>{row.group}</TableCell>
                    </TableRow>
                  ))}
                  {(data.recent_transfers ?? []).length === 0 ? (
                    <EmptyRow colSpan={5} />
                  ) : null}
                </TableBody>
              </Table>
            </div>
          </div>
        ) : null}
      </DialogContent>
    </Dialog>
  )
}

function CustomersPanel() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [keyword, setKeyword] = useState('')
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [displayName, setDisplayName] = useState('')
  const [openCustomer, setOpenCustomer] = useState<number | null>(null)

  const customers = useQuery({
    queryKey: ['reseller', 'customers', keyword],
    queryFn: () => getResellerCustomers(keyword),
  })
  const refresh = () =>
    queryClient.invalidateQueries({ queryKey: ['reseller'] })

  const createCustomer = useMutation({
    mutationFn: () =>
      createResellerCustomer({
        username,
        password,
        display_name: displayName || undefined,
      }),
    onSuccess: () => {
      toast.success(t('Customer created'))
      setUsername('')
      setPassword('')
      setDisplayName('')
      void refresh()
    },
  })

  const toggleStatus = useMutation({
    mutationFn: (row: ResellerCustomer) =>
      setResellerCustomerStatus(row.user_id, row.status === 1 ? 2 : 1),
    onSuccess: () => {
      toast.success(t('Status updated'))
      void refresh()
    },
  })

  const resetPassword = useMutation({
    mutationFn: (payload: { id: number; password: string }) =>
      resetResellerCustomerPassword(payload.id, payload.password),
    onSuccess: () => {
      toast.success(t('Password reset'))
    },
  })

  const moveGroup = useMutation({
    mutationFn: (payload: { id: number; group: string }) =>
      setResellerCustomerGroup(payload.id, payload.group),
    onSuccess: () => {
      toast.success(t('Group updated'))
      void refresh()
    },
  })

  return (
    <div className='flex flex-col gap-4'>
      <Card>
        <CardHeader className='pb-2'>
          <CardTitle className='text-sm'>{t('Create customer')}</CardTitle>
        </CardHeader>
        <CardContent className='flex flex-wrap items-end gap-2'>
          <Input
            className='w-40'
            placeholder={t('Username')}
            value={username}
            onChange={(event) => setUsername(event.target.value)}
          />
          <Input
            className='w-40'
            placeholder={t('Display name')}
            value={displayName}
            onChange={(event) => setDisplayName(event.target.value)}
          />
          <Input
            className='w-40'
            type='password'
            placeholder={t('Password (8-20 chars)')}
            value={password}
            onChange={(event) => setPassword(event.target.value)}
          />
          <Button
            disabled={
              !username || password.length < 8 || createCustomer.isPending
            }
            onClick={() => createCustomer.mutate()}
          >
            {t('Create')}
          </Button>
        </CardContent>
      </Card>

      <div className='flex items-center gap-2'>
        <Input
          className='w-56'
          placeholder={t('Search username')}
          value={keyword}
          onChange={(event) => setKeyword(event.target.value)}
        />
        <Button variant='outline' onClick={() => void customers.refetch()}>
          {t('Refresh')}
        </Button>
      </div>

      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>{t('Customer')}</TableHead>
            <TableHead>{t('Group')}</TableHead>
            <TableHead>{t('Status')}</TableHead>
            <TableHead>{t('Wallet')}</TableHead>
            <TableHead>{t('Funded')}</TableHead>
            <TableHead>{t('Used')}</TableHead>
            <TableHead>{t('Actions')}</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {(customers.data ?? []).map((row) => (
            <TableRow key={row.user_id}>
              <TableCell>
                <button
                  type='button'
                  className='text-left hover:underline'
                  onClick={() => setOpenCustomer(row.user_id)}
                >
                  {row.username}
                  <span className='text-muted-foreground ml-2 text-xs'>
                    #{row.user_id}
                  </span>
                </button>
              </TableCell>
              <TableCell>
                <select
                  className='border-input bg-background h-8 rounded-md border px-2 text-sm'
                  value={row.group}
                  onChange={(event) =>
                    moveGroup.mutate({
                      id: row.user_id,
                      group: event.target.value,
                    })
                  }
                >
                  <option value={row.group}>{row.group}</option>
                  <MoveGroupOptions
                    customerId={row.user_id}
                    current={row.group}
                  />
                </select>
              </TableCell>
              <TableCell>
                <Badge variant={row.status === 1 ? 'default' : 'secondary'}>
                  {row.status === 1 ? t('Enabled') : t('Disabled')}
                </Badge>
              </TableCell>
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
                <div className='flex flex-wrap gap-1'>
                  <Button
                    size='sm'
                    variant='outline'
                    onClick={() => setOpenCustomer(row.user_id)}
                  >
                    {t('Manage')}
                  </Button>
                  <Button
                    size='sm'
                    variant='ghost'
                    onClick={() => toggleStatus.mutate(row)}
                  >
                    {row.status === 1 ? t('Disable') : t('Enable')}
                  </Button>
                  <Button
                    size='sm'
                    variant='ghost'
                    onClick={() => {
                      const next = window.prompt(t('New password'))
                      if (next && next.length >= 8) {
                        resetPassword.mutate({
                          id: row.user_id,
                          password: next,
                        })
                      }
                    }}
                  >
                    {t('Reset password')}
                  </Button>
                </div>
              </TableCell>
            </TableRow>
          ))}
          {(customers.data ?? []).length === 0 ? (
            <EmptyRow colSpan={7} />
          ) : null}
        </TableBody>
      </Table>

      <CustomerDetailDialog
        customerId={openCustomer}
        onClose={() => setOpenCustomer(null)}
      />
    </div>
  )
}

function MoveGroupOptions(props: { customerId: number; current: string }) {
  const groups = useQuery({
    queryKey: ['reseller', 'customer-groups', props.customerId],
    queryFn: () => getResellerCustomerGroups(props.customerId),
  })
  return (
    <>
      {(groups.data ?? [])
        .filter((group) => group.group !== props.current)
        .map((group) => (
          <option key={group.group} value={group.group}>
            {group.group}
          </option>
        ))}
    </>
  )
}

function TransfersPanel() {
  const { t } = useTranslation()
  const [customerId, setCustomerId] = useState('')
  const transfers = useQuery({
    queryKey: ['reseller', 'transfers', customerId],
    queryFn: () =>
      getResellerTransfers({
        customer_id: customerId ? Number(customerId) : undefined,
        limit: 100,
      }),
  })

  return (
    <div className='flex flex-col gap-3'>
      <div className='flex items-center gap-2'>
        <Input
          className='w-48'
          placeholder={t('Customer id (optional)')}
          value={customerId}
          onChange={(event) => setCustomerId(event.target.value)}
        />
        <Button variant='outline' onClick={() => void transfers.refetch()}>
          {t('Refresh')}
        </Button>
      </div>
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>{t('Time')}</TableHead>
            <TableHead>{t('Customer')}</TableHead>
            <TableHead>{t('Direction')}</TableHead>
            <TableHead>{t('Group')}</TableHead>
            <TableHead>{t('Retail')}</TableHead>
            <TableHead>{t('Cost')}</TableHead>
            <TableHead>{t('Rates')}</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {(transfers.data ?? []).map((row) => (
            <TableRow key={row.id}>
              <TableCell>
                {new Date(row.created_at * 1000).toLocaleString()}
              </TableCell>
              <TableCell>#{row.customer_user_id}</TableCell>
              <TableCell>
                <Badge
                  variant={row.direction === 'fund' ? 'default' : 'secondary'}
                >
                  {row.direction === 'fund' ? t('Fund') : t('Reclaim')}
                </Badge>
              </TableCell>
              <TableCell>{row.group}</TableCell>
              <TableCell className='tabular-nums'>
                {formatQuota(row.retail_quota)}
              </TableCell>
              <TableCell className='tabular-nums'>
                {formatQuota(row.cost_quota)}
              </TableCell>
              <TableCell className='text-muted-foreground text-xs'>
                {row.cost_ratio} / {row.retail_ratio}
              </TableCell>
            </TableRow>
          ))}
          {(transfers.data ?? []).length === 0 ? (
            <EmptyRow colSpan={7} />
          ) : null}
        </TableBody>
      </Table>
    </div>
  )
}

function UsagePanel() {
  const { t } = useTranslation()
  const [customerId, setCustomerId] = useState('')
  const usage = useQuery({
    queryKey: ['reseller', 'usage', customerId],
    queryFn: () =>
      getResellerUsage({
        customer_id: customerId ? Number(customerId) : undefined,
        limit: 100,
      }),
  })

  return (
    <div className='flex flex-col gap-3'>
      <div className='flex items-center gap-2'>
        <Input
          className='w-48'
          placeholder={t('Customer id (optional)')}
          value={customerId}
          onChange={(event) => setCustomerId(event.target.value)}
        />
        <Button variant='outline' onClick={() => void usage.refetch()}>
          {t('Refresh')}
        </Button>
      </div>
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>{t('Time')}</TableHead>
            <TableHead>{t('Customer')}</TableHead>
            <TableHead>{t('Model')}</TableHead>
            <TableHead>{t('Group')}</TableHead>
            <TableHead>{t('Charged')}</TableHead>
            <TableHead>{t('Margin')}</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {(usage.data ?? []).map((row) => (
            <TableRow key={row.id}>
              <TableCell>
                {new Date(row.occurred_at * 1000).toLocaleString()}
              </TableCell>
              <TableCell>{row.username || `#${row.customer_id}`}</TableCell>
              <TableCell>{row.model_name}</TableCell>
              <TableCell>{row.group}</TableCell>
              <TableCell className='tabular-nums'>
                {formatQuota(row.revenue_quota)}
              </TableCell>
              <TableCell className='tabular-nums'>
                {formatQuota(row.margin_quota)}
              </TableCell>
            </TableRow>
          ))}
          {(usage.data ?? []).length === 0 ? <EmptyRow colSpan={6} /> : null}
        </TableBody>
      </Table>
    </div>
  )
}

function LogsPanel() {
  const { t } = useTranslation()
  const [customerId, setCustomerId] = useState('')
  const logs = useQuery({
    queryKey: ['reseller', 'logs', customerId],
    queryFn: () =>
      getResellerLogs({
        customer_id: customerId ? Number(customerId) : undefined,
        page_size: 50,
      }),
  })

  return (
    <div className='flex flex-col gap-3'>
      <div className='flex items-center gap-2'>
        <Input
          className='w-48'
          placeholder={t('Customer id (optional)')}
          value={customerId}
          onChange={(event) => setCustomerId(event.target.value)}
        />
        <Button variant='outline' onClick={() => void logs.refetch()}>
          {t('Refresh')}
        </Button>
      </div>
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>{t('Time')}</TableHead>
            <TableHead>{t('Customer')}</TableHead>
            <TableHead>{t('Model')}</TableHead>
            <TableHead>{t('Quota')}</TableHead>
            <TableHead>{t('Content')}</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {(logs.data?.items ?? []).map((row) => (
            <TableRow key={row.id}>
              <TableCell>
                {new Date(row.created_at * 1000).toLocaleString()}
              </TableCell>
              <TableCell>#{row.user_id}</TableCell>
              <TableCell>{row.model_name}</TableCell>
              <TableCell className='tabular-nums'>
                {formatQuota(row.quota)}
              </TableCell>
              <TableCell className='text-muted-foreground max-w-md truncate text-xs'>
                {row.content}
              </TableCell>
            </TableRow>
          ))}
          {(logs.data?.items ?? []).length === 0 ? (
            <EmptyRow colSpan={5} />
          ) : null}
        </TableBody>
      </Table>
    </div>
  )
}

function ProfitPanel() {
  const { t } = useTranslation()
  const margins = useQuery({
    queryKey: ['reseller', 'margins'],
    queryFn: () => getResellerMargins(),
  })
  const total = useMemo(
    () =>
      (margins.data ?? []).reduce(
        (acc, row) => ({
          revenue: acc.revenue + row.revenue_quota,
          margin: acc.margin + row.margin_quota,
          requests: acc.requests + row.request_count,
        }),
        { revenue: 0, margin: 0, requests: 0 }
      ),
    [margins.data]
  )

  return (
    <div className='flex flex-col gap-3'>
      <div className='grid gap-3 sm:grid-cols-3'>
        <StatCard
          title={t('Consumption (31d)')}
          value={formatQuota(total.revenue)}
        />
        <StatCard title={t('Margin (31d)')} value={formatQuota(total.margin)} />
        <StatCard title={t('Requests')} value={String(total.requests)} />
      </div>
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>{t('Day')}</TableHead>
            <TableHead>{t('Requests')}</TableHead>
            <TableHead>{t('Consumption')}</TableHead>
            <TableHead>{t('Margin')}</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {(margins.data ?? []).map((row) => (
            <TableRow key={row.day}>
              <TableCell>
                {new Date(row.day * 86400 * 1000).toLocaleDateString()}
              </TableCell>
              <TableCell className='tabular-nums'>
                {row.request_count}
              </TableCell>
              <TableCell className='tabular-nums'>
                {formatQuota(row.revenue_quota)}
              </TableCell>
              <TableCell className='tabular-nums'>
                {formatQuota(row.margin_quota)}
              </TableCell>
            </TableRow>
          ))}
          {(margins.data ?? []).length === 0 ? <EmptyRow colSpan={4} /> : null}
        </TableBody>
      </Table>
    </div>
  )
}

function PriceBookPanel() {
  const { t } = useTranslation()
  const book = useQuery({
    queryKey: ['reseller', 'price-book'],
    queryFn: getResellerPriceBook,
  })

  return (
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead>{t('Group')}</TableHead>
          <TableHead>{t('My cost')}</TableHead>
          <TableHead>{t('Standing price')}</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {(book.data?.costs ?? []).map((cost) => {
          const standing = (book.data?.defaults ?? []).find(
            (row) => row.group === cost.group
          )
          return (
            <TableRow key={cost.group}>
              <TableCell>{cost.group}</TableCell>
              <TableCell className='tabular-nums'>{cost.cost_ratio}</TableCell>
              <TableCell className='tabular-nums'>
                {standing?.ratio ?? cost.cost_ratio}
              </TableCell>
            </TableRow>
          )
        })}
        {(book.data?.costs ?? []).length === 0 ? (
          <EmptyRow colSpan={3} />
        ) : null}
      </TableBody>
    </Table>
  )
}

export function Reseller() {
  const { t } = useTranslation()
  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>{t('Reseller console')}</SectionPageLayout.Title>
      <SectionPageLayout.Content>
        <Tabs defaultValue='overview' className='flex flex-col gap-4'>
          <TabsList className='flex-wrap'>
            <TabsTrigger value='overview'>{t('Overview')}</TabsTrigger>
            <TabsTrigger value='customers'>{t('Customers')}</TabsTrigger>
            <TabsTrigger value='transfers'>{t('Transfers')}</TabsTrigger>
            <TabsTrigger value='usage'>{t('Usage')}</TabsTrigger>
            <TabsTrigger value='logs'>{t('Logs')}</TabsTrigger>
            <TabsTrigger value='profit'>{t('Profit')}</TabsTrigger>
            <TabsTrigger value='prices'>{t('Price book')}</TabsTrigger>
          </TabsList>
          <TabsContent value='overview'>
            <OverviewPanel />
          </TabsContent>
          <TabsContent value='customers'>
            <CustomersPanel />
          </TabsContent>
          <TabsContent value='transfers'>
            <TransfersPanel />
          </TabsContent>
          <TabsContent value='usage'>
            <UsagePanel />
          </TabsContent>
          <TabsContent value='logs'>
            <LogsPanel />
          </TabsContent>
          <TabsContent value='profit'>
            <ProfitPanel />
          </TabsContent>
          <TabsContent value='prices'>
            <PriceBookPanel />
          </TabsContent>
        </Tabs>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}
