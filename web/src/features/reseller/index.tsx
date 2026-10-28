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
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
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
  claimResellerMargin,
  createResellerCustomer,
  getResellerCustomerDetail,
  getResellerCustomerGroups,
  getResellerCustomers,
  getResellerLogs,
  getResellerMargins,
  getResellerPlatformGroups,
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
        <StatCard
          title={t('Pending margin')}
          value={formatQuota(data?.pending_margin_quota ?? 0)}
          hint={t('Credit it from the Profit tab')}
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
      <DialogContent className='h-[92vh] max-h-[92vh] w-[96vw] max-w-[1200px] grid-rows-[auto_minmax(0,1fr)] gap-3 overflow-hidden sm:max-w-[1200px]'>
        <DialogHeader>
          <DialogTitle>
            {data
              ? `${data.username} · #${data.user_id}`
              : t('Customer detail')}
          </DialogTitle>
        </DialogHeader>

        {data ? (
          <div className='flex min-h-0 flex-col gap-4 overflow-y-auto pr-1'>
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
                <p className='text-muted-foreground basis-full text-xs'>
                  {t(
                    'Funding moves quota from your wallet into the customer wallet one to one. Your profit is the difference between your retail price and your wholesale cost, collected as pending margin you credit from the Profit tab.'
                  )}
                </p>
              </div>
            </div>

            <div className='grid min-h-0 gap-4 xl:grid-cols-2'>
              <div className='flex min-h-0 flex-col'>
                <div className='mb-1 text-sm font-medium'>
                  {t('Group pricing')}
                </div>
                <p className='text-muted-foreground mb-2 text-xs'>
                  {t(
                    'The default customer price is the platform group price. Set your own price only where you want a different one; the others stay at the platform price.'
                  )}
                </p>
                <div className='min-h-0 flex-1 overflow-y-auto rounded-lg border'>
                  <Table>
                    <TableHeader className='bg-popover sticky top-0 z-10'>
                      <TableRow>
                        <TableHead>{t('Group')}</TableHead>
                        <TableHead>{t('My cost')}</TableHead>
                        <TableHead>{t('Customer price')}</TableHead>
                        <TableHead>{t('Margin')}</TableHead>
                        <TableHead />
                      </TableRow>
                    </TableHeader>
                    <TableBody>
                      {(groups.data ?? []).map((group) => {
                        const retail = Number(
                          priceDrafts[group.group] || group.ratio
                        )
                        const cost = Number(group.cost_ratio)
                        const margin =
                          retail > 0 && cost > 0
                            ? `${(((retail - cost) / retail) * 100).toFixed(1)}%`
                            : '—'
                        return (
                          <TableRow key={group.group}>
                            <TableCell>
                              <div className='flex items-center gap-1.5'>
                                <span>{group.group}</span>
                                {group.group === data.group ? (
                                  <Badge variant='outline'>
                                    {t('Current')}
                                  </Badge>
                                ) : null}
                              </div>
                            </TableCell>
                            <TableCell className='tabular-nums'>
                              {group.cost_ratio || '—'}
                            </TableCell>
                            <TableCell>
                              <Input
                                className='w-24'
                                inputMode='decimal'
                                placeholder={group.ratio || '—'}
                                value={priceDrafts[group.group] ?? ''}
                                onChange={(event) =>
                                  setPriceDrafts((prev) => ({
                                    ...prev,
                                    [group.group]: event.target.value,
                                  }))
                                }
                                onKeyDown={(event) => {
                                  if (event.key === 'Enter') {
                                    event.preventDefault()
                                    savePrice.mutate(group.group)
                                  }
                                }}
                              />
                            </TableCell>
                            <TableCell className='text-muted-foreground tabular-nums'>
                              {margin}
                            </TableCell>
                            <TableCell>
                              <Button
                                size='sm'
                                variant='outline'
                                disabled={
                                  !priceDrafts[group.group] ||
                                  (savePrice.isPending &&
                                    savePrice.variables === group.group)
                                }
                                onClick={() => savePrice.mutate(group.group)}
                              >
                                {savePrice.isPending &&
                                savePrice.variables === group.group
                                  ? t('Saving')
                                  : t('Save')}
                              </Button>
                            </TableCell>
                          </TableRow>
                        )
                      })}
                      {(groups.data ?? []).length === 0 ? (
                        <EmptyRow colSpan={5} />
                      ) : null}
                    </TableBody>
                  </Table>
                </div>
              </div>

              <div className='flex min-h-0 flex-col'>
                <div className='mb-2 text-sm font-medium'>
                  {t('Recent transfers')}
                </div>
                <div className='min-h-0 flex-1 overflow-y-auto rounded-lg border'>
                  <Table>
                    <TableHeader className='bg-popover sticky top-0 z-10'>
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
                                row.direction === 'fund'
                                  ? 'default'
                                  : 'secondary'
                              }
                            >
                              {row.direction === 'fund'
                                ? t('Fund')
                                : t('Reclaim')}
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
  const [group, setGroup] = useState('default')
  const [openCustomer, setOpenCustomer] = useState<number | null>(null)
  const [passwordTarget, setPasswordTarget] = useState<ResellerCustomer | null>(
    null
  )

  const customers = useQuery({
    queryKey: ['reseller', 'customers', keyword],
    queryFn: () => getResellerCustomers(keyword),
  })
  // One request for every group the platform serves, shared by the create form
  // and by every row's group selector. Fetching per row would issue one request
  // per customer on every list render.
  const platformGroups = useQuery({
    queryKey: ['reseller', 'platform-groups'],
    queryFn: getResellerPlatformGroups,
  })
  const refresh = () =>
    queryClient.invalidateQueries({ queryKey: ['reseller'] })

  const createCustomer = useMutation({
    mutationFn: () =>
      createResellerCustomer({
        username,
        password,
        display_name: displayName || undefined,
        group,
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
      setPasswordTarget(null)
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

  const groupOptions = platformGroups.data ?? []

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
            autoComplete='off'
            value={username}
            onChange={(event) => setUsername(event.target.value)}
          />
          <Input
            className='w-40'
            placeholder={t('Display name')}
            autoComplete='off'
            value={displayName}
            onChange={(event) => setDisplayName(event.target.value)}
          />
          <Input
            className='w-40'
            type='password'
            placeholder={t('Password (8-20 chars)')}
            autoComplete='new-password'
            value={password}
            onChange={(event) => setPassword(event.target.value)}
          />
          <Select
            items={groupOptions.map((row) => ({
              value: row.group,
              label: row.group,
            }))}
            value={group}
            onValueChange={(value) => {
              if (value) setGroup(value)
            }}
          >
            <SelectTrigger className='w-40'>
              <SelectValue placeholder={t('Group')} />
            </SelectTrigger>
            <SelectContent alignItemWithTrigger={false}>
              <SelectGroup>
                {groupOptions.map((row) => (
                  <SelectItem key={row.group} value={row.group}>
                    {row.group}
                  </SelectItem>
                ))}
              </SelectGroup>
            </SelectContent>
          </Select>
          <Button
            disabled={
              !username || password.length < 8 || createCustomer.isPending
            }
            onClick={() => createCustomer.mutate()}
          >
            {createCustomer.isPending ? t('Creating') : t('Create')}
          </Button>
        </CardContent>
        <CardContent className='text-muted-foreground pt-0 text-xs'>
          {t(
            'The customer is an ordinary account: they start in the default group and may call any group with their key.'
          )}
        </CardContent>
      </Card>

      <div className='flex items-center gap-2'>
        <Input
          className='w-56'
          placeholder={t('Search username')}
          value={keyword}
          onChange={(event) => setKeyword(event.target.value)}
        />
        <Button
          variant='outline'
          disabled={customers.isFetching}
          onClick={() => void customers.refetch()}
        >
          {t('Refresh')}
        </Button>
        {customers.isFetching ? (
          <span className='text-muted-foreground text-xs'>{t('Loading')}</span>
        ) : null}
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
                <Select
                  items={groupOptions.map((option) => ({
                    value: option.group,
                    label: option.group,
                  }))}
                  value={row.group}
                  onValueChange={(value) => {
                    if (value && value !== row.group) {
                      moveGroup.mutate({ id: row.user_id, group: value })
                    }
                  }}
                >
                  <SelectTrigger className='w-36'>
                    <SelectValue placeholder={row.group} />
                  </SelectTrigger>
                  <SelectContent alignItemWithTrigger={false}>
                    <SelectGroup>
                      {groupOptions.map((option) => (
                        <SelectItem key={option.group} value={option.group}>
                          {option.group}
                        </SelectItem>
                      ))}
                    </SelectGroup>
                  </SelectContent>
                </Select>
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
                    disabled={toggleStatus.isPending}
                    onClick={() => toggleStatus.mutate(row)}
                  >
                    {row.status === 1 ? t('Disable') : t('Enable')}
                  </Button>
                  <Button
                    size='sm'
                    variant='ghost'
                    onClick={() => setPasswordTarget(row)}
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

      <PasswordResetDialog
        customer={passwordTarget}
        pending={resetPassword.isPending}
        onClose={() => setPasswordTarget(null)}
        onSubmit={(next) =>
          passwordTarget &&
          resetPassword.mutate({ id: passwordTarget.user_id, password: next })
        }
      />
    </div>
  )
}

function PasswordResetDialog(props: {
  customer: ResellerCustomer | null
  pending: boolean
  onClose: () => void
  onSubmit: (password: string) => void
}) {
  const { t } = useTranslation()
  const [password, setPassword] = useState('')
  const valid = password.length >= 8 && password.length <= 20

  return (
    <Dialog
      open={props.customer != null}
      onOpenChange={(open) => {
        if (!open) {
          setPassword('')
          props.onClose()
        }
      }}
    >
      <DialogContent className='max-w-sm'>
        <DialogHeader>
          <DialogTitle>
            {t('Reset password for {{name}}', {
              name: props.customer?.username ?? '',
            })}
          </DialogTitle>
        </DialogHeader>
        <div className='flex flex-col gap-3'>
          <Input
            type='password'
            autoComplete='new-password'
            placeholder={t('Password (8-20 chars)')}
            value={password}
            onChange={(event) => setPassword(event.target.value)}
          />
          <div className='flex justify-end gap-2'>
            <Button variant='outline' onClick={props.onClose}>
              {t('Cancel')}
            </Button>
            <Button
              disabled={!valid || props.pending}
              onClick={() => props.onSubmit(password)}
            >
              {t('Save')}
            </Button>
          </div>
        </div>
      </DialogContent>
    </Dialog>
  )
}

function CustomerFilter(props: {
  value: string
  onChange: (value: string) => void
  onRefresh: () => void
  refreshing: boolean
}) {
  const { t } = useTranslation()
  const customers = useQuery({
    queryKey: ['reseller', 'customers', ''],
    queryFn: () => getResellerCustomers('', 0, 200),
  })

  return (
    <div className='flex flex-wrap items-center gap-2'>
      <Select
        items={[
          { value: '0', label: t('All customers') },
          ...(customers.data ?? []).map((row) => ({
            value: String(row.user_id),
            label: `${row.username} · #${row.user_id}`,
          })),
        ]}
        value={props.value || '0'}
        onValueChange={(next) => props.onChange(next === '0' ? '' : (next ?? ''))}
      >
        <SelectTrigger className='w-56'>
          <SelectValue placeholder={t('All customers')} />
        </SelectTrigger>
        <SelectContent alignItemWithTrigger={false}>
          <SelectGroup>
            <SelectItem value='0'>{t('All customers')}</SelectItem>
            {(customers.data ?? []).map((row) => (
              <SelectItem key={row.user_id} value={String(row.user_id)}>
                {row.username} · #{row.user_id}
              </SelectItem>
            ))}
          </SelectGroup>
        </SelectContent>
      </Select>
      <Button
        variant='outline'
        disabled={props.refreshing}
        onClick={props.onRefresh}
      >
        {t('Refresh')}
      </Button>
    </div>
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
      <CustomerFilter
        value={customerId}
        onChange={setCustomerId}
        onRefresh={() => void transfers.refetch()}
        refreshing={transfers.isFetching}
      />
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
      <CustomerFilter
        value={customerId}
        onChange={setCustomerId}
        onRefresh={() => void usage.refetch()}
        refreshing={usage.isFetching}
      />
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
      <CustomerFilter
        value={customerId}
        onChange={setCustomerId}
        onRefresh={() => void logs.refetch()}
        refreshing={logs.isFetching}
      />
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
  const queryClient = useQueryClient()
  const summary = useQuery({
    queryKey: ['reseller', 'summary'],
    queryFn: getResellerSummary,
  })
  const margins = useQuery({
    queryKey: ['reseller', 'margins'],
    queryFn: () => getResellerMargins(),
  })
  // Claiming is explicit: earned margin is listed as pending until the reseller
  // moves it into the wallet themselves.
  const claim = useMutation({
    mutationFn: claimResellerMargin,
    onSuccess: (result) => {
      toast.success(
        t('Credited {{amount}} to your wallet', {
          amount: formatQuota(result.credited_quota),
        })
      )
      void queryClient.invalidateQueries({ queryKey: ['reseller'] })
    },
  })
  const pending = summary.data?.pending_margin_quota ?? 0
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
      <div className='grid gap-3 sm:grid-cols-4'>
        <StatCard
          title={t('Consumption (31d)')}
          value={formatQuota(total.revenue)}
        />
        <StatCard title={t('Margin (31d)')} value={formatQuota(total.margin)} />
        <StatCard title={t('Requests')} value={String(total.requests)} />
        <StatCard
          title={t('Pending margin')}
          value={formatQuota(pending)}
          hint={t('Available to credit into your wallet')}
        />
      </div>
      <div className='flex items-center gap-2'>
        <Button
          disabled={pending <= 0 || claim.isPending}
          onClick={() => claim.mutate()}
        >
          {claim.isPending ? t('Crediting') : t('Credit to wallet')}
        </Button>
        <span className='text-muted-foreground text-xs'>
          {t(
            'Margin is not paid out automatically. It stays pending until you credit it.'
          )}
        </span>
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
  // Every platform group is listed, not just the groups that already have a
  // wholesale cost row: a reseller needs to see which groups they can sell in
  // and which still have no cost configured.
  const groups = useQuery({
    queryKey: ['reseller', 'platform-groups'],
    queryFn: getResellerPlatformGroups,
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
        {(groups.data ?? []).map((row) => (
          <TableRow key={row.group}>
            <TableCell>{row.group}</TableCell>
            <TableCell className='tabular-nums'>
              {row.sellable ? row.cost_ratio : t('No cost set')}
            </TableCell>
            <TableCell className='tabular-nums'>
              {row.sellable ? row.ratio || row.cost_ratio : '—'}
            </TableCell>
          </TableRow>
        ))}
        {(groups.data ?? []).length === 0 ? <EmptyRow colSpan={3} /> : null}
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
