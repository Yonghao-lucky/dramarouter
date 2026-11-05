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
import { useEffect, useMemo, useState } from 'react'
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
import {
  formatQuota,
  parseQuotaFromDollars,
  quotaUnitsToEditableAmount,
} from '@/lib/format'

import {
  claimResellerMargin,
  createResellerCustomer,
  getResellerCustomerDetail,
  getResellerCustomerGroups,
  getResellerCustomers,
  getResellerAudit,
  getResellerDashboard,
  getResellerGrants,
  getResellerLogs,
  getResellerMargins,
  getResellerQuotaAlert,
  getResellerSettlements,
  getResellerTemplates,
  resellerSettlementExportURL,
  saveResellerTemplate,
  setResellerQuotaAlert,
  applyResellerTemplate,
  deleteResellerTemplate,
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
            className='w-full sm:w-40'
            placeholder={t('Username')}
            autoComplete='off'
            value={username}
            onChange={(event) => setUsername(event.target.value)}
          />
          <Input
            className='w-full sm:w-40'
            placeholder={t('Display name')}
            autoComplete='off'
            value={displayName}
            onChange={(event) => setDisplayName(event.target.value)}
          />
          <Input
            className='w-full sm:w-40'
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
            <SelectTrigger className='w-full sm:w-40'>
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
          className='w-full sm:w-56'
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

      {/* Phones get cards: seven columns of numbers in a horizontally scrolled
          table is unusable on a 375px screen. */}
      <div className='flex flex-col gap-2 md:hidden'>
        {(customers.data ?? []).map((row) => (
          <Card key={row.user_id}>
            <CardContent className='flex flex-col gap-2 py-3'>
              <div className='flex items-start justify-between gap-2'>
                <button
                  type='button'
                  className='text-left font-medium hover:underline'
                  onClick={() => setOpenCustomer(row.user_id)}
                >
                  {row.username}
                  <span className='text-muted-foreground ml-1 text-xs'>
                    #{row.user_id}
                  </span>
                </button>
                <Badge variant={row.status === 1 ? 'default' : 'secondary'}>
                  {row.status === 1 ? t('Enabled') : t('Disabled')}
                </Badge>
              </div>
              <div className='grid grid-cols-3 gap-2 text-xs'>
                <div>
                  <div className='text-muted-foreground'>{t('Wallet')}</div>
                  <div className='tabular-nums'>
                    {formatQuota(row.wallet_quota)}
                  </div>
                </div>
                <div>
                  <div className='text-muted-foreground'>{t('Funded')}</div>
                  <div className='tabular-nums'>
                    {formatQuota(row.funded_quota)}
                  </div>
                </div>
                <div>
                  <div className='text-muted-foreground'>{t('Used')}</div>
                  <div className='tabular-nums'>
                    {formatQuota(row.used_quota)}
                  </div>
                </div>
              </div>
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
              </div>
            </CardContent>
          </Card>
        ))}
        {(customers.data ?? []).length === 0 ? (
          <Card>
            <CardContent className='text-muted-foreground py-8 text-center text-sm'>
              {t('No records yet')}
            </CardContent>
          </Card>
        ) : null}
      </div>

      <div className='hidden md:block'>
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
      </div>

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
          {/* Tab labels wrap on desktop but scroll horizontally on phones,
              where wrapping six-plus tabs would push content off screen. */}
          <TabsList className='max-w-full flex-nowrap overflow-x-auto sm:flex-wrap sm:overflow-visible'>
            <TabsTrigger value='overview'>{t('Overview')}</TabsTrigger>
            <TabsTrigger value='customers'>{t('Customers')}</TabsTrigger>
            <TabsTrigger value='transfers'>{t('Transfers')}</TabsTrigger>
            <TabsTrigger value='usage'>{t('Usage')}</TabsTrigger>
            <TabsTrigger value='logs'>{t('Logs')}</TabsTrigger>
            <TabsTrigger value='profit'>{t('Profit')}</TabsTrigger>
            <TabsTrigger value='prices'>{t('Price book')}</TabsTrigger>
            <TabsTrigger value='wallet'>{t('Wallet history')}</TabsTrigger>
            <TabsTrigger value='settlements'>{t('Settlements')}</TabsTrigger>
            <TabsTrigger value='dashboard'>{t('Customer data')}</TabsTrigger>
            <TabsTrigger value='templates'>{t('Price templates')}</TabsTrigger>
            <TabsTrigger value='alert'>{t('Alerts')}</TabsTrigger>
            <TabsTrigger value='audit'>{t('Audit')}</TabsTrigger>
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
          <TabsContent value='wallet'>
            <WalletPanel />
          </TabsContent>
          <TabsContent value='settlements'>
            <SettlementsPanel />
          </TabsContent>
          <TabsContent value='dashboard'>
            <DashboardPanel />
          </TabsContent>
          <TabsContent value='templates'>
            <TemplatesPanel />
          </TabsContent>
          <TabsContent value='alert'>
            <AlertPanel />
          </TabsContent>
          <TabsContent value='audit'>
            <AuditPanel />
          </TabsContent>
        </Tabs>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}

// ---------------------------------------------------------------------------
// Wallet history: what the platform credited, with pre-credit clearly labelled.
// ---------------------------------------------------------------------------
function WalletPanel() {
  const { t } = useTranslation()
  const grants = useQuery({
    queryKey: ['reseller', 'grants'],
    queryFn: () => getResellerGrants(100),
  })
  const totals = grants.data?.totals

  return (
    <div className='flex flex-col gap-4'>
      <div className='grid gap-3 sm:grid-cols-2 lg:grid-cols-4'>
        <StatCard
          title={t('Total recharged')}
          value={formatQuota(
            (grants.data?.items ?? [])
              .filter((row) => row.kind === 'recharge')
              .reduce((sum, row) => sum + row.amount, 0)
          )}
        />
        <StatCard
          title={t('Pre-credit (not yet settled)')}
          value={formatQuota(
            (grants.data?.items ?? [])
              .filter((row) => row.kind === 'precredit')
              .reduce((sum, row) => sum + row.amount, 0)
          )}
          hint={t('Granted by the platform ahead of payment')}
        />
        <StatCard
          title={t('Confirmed payments')}
          value={formatQuota(totals?.credited_quota ?? 0)}
        />
        <StatCard
          title={t('Pending settlements')}
          value={String(totals?.pending_count ?? 0)}
          hint={formatQuota(totals?.pending_minor ?? 0)}
        />
      </div>

      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>{t('Time')}</TableHead>
            <TableHead>{t('Type')}</TableHead>
            <TableHead>{t('Amount')}</TableHead>
            <TableHead>{t('Remark')}</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {(grants.data?.items ?? []).map((row) => (
            <TableRow key={row.id}>
              <TableCell>
                {new Date(row.created_at * 1000).toLocaleString()}
              </TableCell>
              <TableCell>
                <Badge
                  variant={row.kind === 'recharge' ? 'default' : 'secondary'}
                >
                  {row.kind === 'recharge'
                    ? t('Recharge')
                    : t('Pre-credit')}
                </Badge>
              </TableCell>
              <TableCell className='tabular-nums'>
                {formatQuota(row.amount)}
              </TableCell>
              <TableCell className='text-muted-foreground text-sm'>
                {row.remark || '-'}
              </TableCell>
            </TableRow>
          ))}
          {(grants.data?.items ?? []).length === 0 ? (
            <EmptyRow colSpan={4} />
          ) : null}
        </TableBody>
      </Table>
    </div>
  )
}

// ---------------------------------------------------------------------------
// Customer analytics. The server scopes every figure to this reseller.
// ---------------------------------------------------------------------------
function DashboardPanel() {
  const { t } = useTranslation()
  const [days, setDays] = useState('7')
  const dashboard = useQuery({
    queryKey: ['reseller', 'dashboard', days],
    queryFn: () => getResellerDashboard(Number(days)),
  })
  const data = dashboard.data

  return (
    <div className='flex flex-col gap-4'>
      <div className='flex flex-wrap items-center gap-2'>
        <Select value={days} onValueChange={setDays}>
          <SelectTrigger className='w-32'>
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectGroup>
              <SelectItem value='7'>{t('Last 7 days')}</SelectItem>
              <SelectItem value='30'>{t('Last 30 days')}</SelectItem>
              <SelectItem value='90'>{t('Last 90 days')}</SelectItem>
            </SelectGroup>
          </SelectContent>
        </Select>
        <span className='text-muted-foreground text-xs'>
          {t('Only customers you own are included')}
        </span>
      </div>

      <div className='grid gap-3 sm:grid-cols-2 lg:grid-cols-4'>
        <StatCard
          title={t('My customers')}
          value={String(data?.customer_count ?? 0)}
          hint={t('{{count}} active', { count: data?.active_count ?? 0 })}
        />
        <StatCard
          title={t('Their consumption')}
          value={formatQuota(data?.revenue_quota ?? 0)}
        />
        <StatCard
          title={t('Requests')}
          value={String(data?.request_count ?? 0)}
          hint={t('Avg {{value}} per request', {
            value: formatQuota(data?.average_per_request ?? 0),
          })}
        />
        <StatCard
          title={t('Their wallet total')}
          value={formatQuota(data?.total_balance_quota ?? 0)}
        />
      </div>

      <div className='grid gap-4 lg:grid-cols-2'>
        <Card>
          <CardHeader className='pb-2'>
            <CardTitle className='text-sm'>{t('Top customers')}</CardTitle>
          </CardHeader>
          <CardContent>
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t('Customer')}</TableHead>
                  <TableHead>{t('Consumption')}</TableHead>
                  <TableHead>{t('Requests')}</TableHead>
                  <TableHead>{t('Balance')}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {(data?.top_customers ?? []).map((row) => (
                  <TableRow key={row.user_id}>
                    <TableCell>
                      {row.username}
                      <span className='text-muted-foreground ml-1 text-xs'>
                        #{row.user_id}
                      </span>
                    </TableCell>
                    <TableCell className='tabular-nums'>
                      {formatQuota(row.revenue_quota)}
                    </TableCell>
                    <TableCell className='tabular-nums'>
                      {row.request_count}
                    </TableCell>
                    <TableCell className='tabular-nums'>
                      {formatQuota(row.balance_quota)}
                    </TableCell>
                  </TableRow>
                ))}
                {(data?.top_customers ?? []).length === 0 ? (
                  <EmptyRow colSpan={4} />
                ) : null}
              </TableBody>
            </Table>
          </CardContent>
        </Card>

        <Card>
          <CardHeader className='pb-2'>
            <CardTitle className='text-sm'>{t('By model')}</CardTitle>
          </CardHeader>
          <CardContent>
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t('Model')}</TableHead>
                  <TableHead>{t('Consumption')}</TableHead>
                  <TableHead>{t('Requests')}</TableHead>
                  <TableHead>{t('Customers')}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {(data?.by_model ?? []).map((row) => (
                  <TableRow key={row.model_name}>
                    <TableCell>{row.model_name}</TableCell>
                    <TableCell className='tabular-nums'>
                      {formatQuota(row.revenue_quota)}
                    </TableCell>
                    <TableCell className='tabular-nums'>
                      {row.request_count}
                    </TableCell>
                    <TableCell className='tabular-nums'>
                      {row.customer_count}
                    </TableCell>
                  </TableRow>
                ))}
                {(data?.by_model ?? []).length === 0 ? (
                  <EmptyRow colSpan={4} />
                ) : null}
              </TableBody>
            </Table>
          </CardContent>
        </Card>
      </div>
    </div>
  )
}

// ---------------------------------------------------------------------------
// Settlement ledger and its CSV export.
// ---------------------------------------------------------------------------
function SettlementsPanel() {
  const { t } = useTranslation()
  const [status, setStatus] = useState('')
  const settlements = useQuery({
    queryKey: ['reseller', 'settlements', status],
    queryFn: () =>
      getResellerSettlements({ status: status || undefined, limit: 100 }),
  })

  return (
    <div className='flex flex-col gap-3'>
      <div className='flex flex-wrap items-center gap-2'>
        <Select
          value={status || 'all'}
          onValueChange={(value) => setStatus(value === 'all' ? '' : value)}
        >
          <SelectTrigger className='w-36'>
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectGroup>
              <SelectItem value='all'>{t('All statuses')}</SelectItem>
              <SelectItem value='pending'>{t('Pending')}</SelectItem>
              <SelectItem value='confirmed'>{t('Confirmed')}</SelectItem>
              <SelectItem value='void'>{t('Void')}</SelectItem>
            </SelectGroup>
          </SelectContent>
        </Select>
        <Button
          size='sm'
          variant='outline'
          onClick={() => {
            window.open(resellerSettlementExportURL(), '_blank')
          }}
        >
          {t('Export CSV')}
        </Button>
      </div>

      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>{t('Time')}</TableHead>
            <TableHead>{t('Direction')}</TableHead>
            <TableHead>{t('Status')}</TableHead>
            <TableHead>{t('Paid amount')}</TableHead>
            <TableHead>{t('Quota credited')}</TableHead>
            <TableHead>{t('Method')}</TableHead>
            <TableHead>{t('Reference')}</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {(settlements.data ?? []).map((row) => (
            <TableRow key={row.id}>
              <TableCell>
                {new Date(row.created_at * 1000).toLocaleString()}
              </TableCell>
              <TableCell>
                {row.direction === 'in' ? t('I paid') : t('Refund')}
              </TableCell>
              <TableCell>
                <Badge
                  variant={
                    row.status === 'confirmed'
                      ? 'default'
                      : row.status === 'void'
                        ? 'destructive'
                        : 'secondary'
                  }
                >
                  {row.status === 'confirmed'
                    ? t('Confirmed')
                    : row.status === 'void'
                      ? t('Void')
                      : t('Pending')}
                </Badge>
              </TableCell>
              <TableCell className='tabular-nums'>
                {(row.amount_minor / 100).toFixed(2)} {row.currency}
              </TableCell>
              <TableCell className='tabular-nums'>
                {formatQuota(row.quota_amount)}
              </TableCell>
              <TableCell>{row.method || '-'}</TableCell>
              <TableCell className='text-muted-foreground text-xs'>
                {row.external_ref || '-'}
              </TableCell>
            </TableRow>
          ))}
          {(settlements.data ?? []).length === 0 ? (
            <EmptyRow colSpan={7} />
          ) : null}
        </TableBody>
      </Table>
    </div>
  )
}

// ---------------------------------------------------------------------------
// Operation audit: who did what, from which IP.
// ---------------------------------------------------------------------------
function AuditPanel() {
  const { t } = useTranslation()
  const audit = useQuery({
    queryKey: ['reseller', 'audit'],
    queryFn: () => getResellerAudit(100),
  })

  return (
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead>{t('Time')}</TableHead>
          <TableHead>{t('Action')}</TableHead>
          <TableHead>{t('Detail')}</TableHead>
          <TableHead>{t('IP')}</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {(audit.data ?? []).map((row) => (
          <TableRow key={row.id}>
            <TableCell>
              {new Date(row.created_at * 1000).toLocaleString()}
            </TableCell>
            <TableCell className='font-mono text-xs'>{row.action}</TableCell>
            <TableCell className='text-muted-foreground max-w-md truncate text-xs'>
              {row.detail}
            </TableCell>
            <TableCell className='font-mono text-xs'>{row.ip}</TableCell>
          </TableRow>
        ))}
        {(audit.data ?? []).length === 0 ? <EmptyRow colSpan={4} /> : null}
      </TableBody>
    </Table>
  )
}

// ---------------------------------------------------------------------------
// Reusable price templates: build once, apply to selected customers.
// ---------------------------------------------------------------------------
function TemplatesPanel() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [name, setName] = useState('')
  const [draft, setDraft] = useState<Record<string, string>>({})
  const [applyTemplate, setApplyTemplate] = useState('')
  const [applyCustomers, setApplyCustomers] = useState('')

  const groups = useQuery({
    queryKey: ['reseller', 'groups'],
    queryFn: getResellerPlatformGroups,
  })
  const templates = useQuery({
    queryKey: ['reseller', 'templates'],
    queryFn: getResellerTemplates,
  })

  const refresh = () =>
    queryClient.invalidateQueries({ queryKey: ['reseller', 'templates'] })

  const save = useMutation({
    mutationFn: () =>
      saveResellerTemplate({
        name,
        items: Object.fromEntries(
          Object.entries(draft).filter(([, value]) => value.trim() !== '')
        ),
      }),
    onSuccess: () => {
      toast.success(t('Template saved'))
      setName('')
      setDraft({})
      void refresh()
    },
    onError: (error: Error) => toast.error(error.message),
  })

  const remove = useMutation({
    mutationFn: (id: number) => deleteResellerTemplate(id),
    onSuccess: () => {
      toast.success(t('Template deleted'))
      void refresh()
    },
    onError: (error: Error) => toast.error(error.message),
  })

  const apply = useMutation({
    mutationFn: () =>
      applyResellerTemplate({
        template_id: Number(applyTemplate),
        customer_ids: applyCustomers
          .split(/[\s,]+/)
          .map((value) => Number(value))
          .filter((value) => Number.isInteger(value) && value > 0),
      }),
    onSuccess: (result) => {
      toast.success(t('Applied to {{count}} customers', { count: result.applied }))
      setApplyCustomers('')
    },
    onError: (error: Error) => toast.error(error.message),
  })

  return (
    <div className='flex flex-col gap-4'>
      <Card>
        <CardHeader className='pb-2'>
          <CardTitle className='text-sm'>{t('New template')}</CardTitle>
        </CardHeader>
        <CardContent className='flex flex-col gap-3'>
          <Input
            className='max-w-sm'
            placeholder={t('Template name')}
            value={name}
            onChange={(event) => setName(event.target.value)}
          />
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t('Group')}</TableHead>
                <TableHead>{t('Your cost')}</TableHead>
                <TableHead>{t('Retail ratio')}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {(groups.data ?? []).map((row) => (
                <TableRow key={row.group}>
                  <TableCell>{row.group}</TableCell>
                  <TableCell className='tabular-nums'>
                    {row.cost_ratio || '-'}
                  </TableCell>
                  <TableCell>
                    <Input
                      className='w-28'
                      value={draft[row.group] ?? ''}
                      onChange={(event) =>
                        setDraft((prev) => ({
                          ...prev,
                          [row.group]: event.target.value,
                        }))
                      }
                    />
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
          <Button
            disabled={!name || save.isPending}
            onClick={() => save.mutate()}
          >
            {t('Save template')}
          </Button>
        </CardContent>
      </Card>

      <Card>
        <CardHeader className='pb-2'>
          <CardTitle className='text-sm'>{t('Apply a template')}</CardTitle>
        </CardHeader>
        <CardContent className='flex flex-col gap-3'>
          <Select value={applyTemplate} onValueChange={setApplyTemplate}>
            <SelectTrigger className='max-w-sm'>
              <SelectValue placeholder={t('Choose a template')} />
            </SelectTrigger>
            <SelectContent>
              <SelectGroup>
                {(templates.data?.templates ?? []).map((row) => (
                  <SelectItem key={row.id} value={String(row.id)}>
                    {row.name}
                  </SelectItem>
                ))}
              </SelectGroup>
            </SelectContent>
          </Select>
          <Input
            placeholder={t('Customer IDs, comma separated')}
            value={applyCustomers}
            onChange={(event) => setApplyCustomers(event.target.value)}
          />
          <Button
            disabled={!applyTemplate || !applyCustomers || apply.isPending}
            onClick={() => apply.mutate()}
          >
            {t('Apply')}
          </Button>
          <span className='text-muted-foreground text-xs'>
            {t('Only customers you own are applied; the rest are rejected')}
          </span>
        </CardContent>
      </Card>

      <Card>
        <CardHeader className='pb-2'>
          <CardTitle className='text-sm'>{t('My templates')}</CardTitle>
        </CardHeader>
        <CardContent>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t('Name')}</TableHead>
                <TableHead>{t('Groups')}</TableHead>
                <TableHead />
              </TableRow>
            </TableHeader>
            <TableBody>
              {(templates.data?.templates ?? []).map((row) => (
                <TableRow key={row.id}>
                  <TableCell>{row.name}</TableCell>
                  <TableCell className='text-muted-foreground text-xs'>
                    {(templates.data?.items[String(row.id)] ?? [])
                      .map((item) => `${item.group}=${item.ratio}`)
                      .join(', ') || '-'}
                  </TableCell>
                  <TableCell>
                    <Button
                      size='sm'
                      variant='ghost'
                      onClick={() => remove.mutate(row.id)}
                    >
                      {t('Delete')}
                    </Button>
                  </TableCell>
                </TableRow>
              ))}
              {(templates.data?.templates ?? []).length === 0 ? (
                <EmptyRow colSpan={3} />
              ) : null}
            </TableBody>
          </Table>
        </CardContent>
      </Card>
    </div>
  )
}

// ---------------------------------------------------------------------------
// Low-balance mail alert rule.
// ---------------------------------------------------------------------------
function AlertPanel() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const rule = useQuery({
    queryKey: ['reseller', 'alert'],
    queryFn: getResellerQuotaAlert,
  })
  const [enabled, setEnabled] = useState(false)
  const [threshold, setThreshold] = useState('')
  const [email, setEmail] = useState('')
  const [cooldown, setCooldown] = useState('24')

  // Seed the form once the saved rule arrives. Writing state during render
  // would re-render on every pass and can loop, so it belongs in an effect.
  useEffect(() => {
    if (!rule.data) return
    setEnabled(rule.data.enabled)
    setThreshold(String(quotaUnitsToEditableAmount(rule.data.threshold_quota)))
    setEmail(rule.data.notify_email ?? '')
    setCooldown(String(rule.data.cooldown_hours || 24))
  }, [rule.data])

  const save = useMutation({
    mutationFn: () =>
      setResellerQuotaAlert({
        enabled,
        threshold_quota: parseQuotaFromDollars(Number(threshold) || 0),
        notify_email: email.trim() || undefined,
        cooldown_hours: Number(cooldown) || 24,
      }),
    onSuccess: () => {
      toast.success(t('Alert rule saved'))
      void queryClient.invalidateQueries({ queryKey: ['reseller', 'alert'] })
    },
    onError: (error: Error) => toast.error(error.message),
  })

  return (
    <Card>
      <CardHeader className='pb-2'>
        <CardTitle className='text-sm'>{t('Low balance email alert')}</CardTitle>
      </CardHeader>
      <CardContent className='flex flex-col gap-4'>
        <label className='flex items-center gap-2 text-sm'>
          <input
            type='checkbox'
            className='size-4'
            checked={enabled}
            onChange={(event) => setEnabled(event.target.checked)}
          />
          {t('Notify me when a customer runs low')}
        </label>
        <div className='flex flex-col gap-2'>
          <span className='text-sm'>{t('Threshold (USD)')}</span>
          <Input
            className='max-w-xs'
            type='number'
            min={0}
            value={threshold}
            onChange={(event) => setThreshold(event.target.value)}
          />
        </div>
        <div className='flex flex-col gap-2'>
          <span className='text-sm'>{t('Notify email')}</span>
          <Input
            className='max-w-xs'
            placeholder={t('Defaults to your account email')}
            value={email}
            onChange={(event) => setEmail(event.target.value)}
          />
        </div>
        <div className='flex flex-col gap-2'>
          <span className='text-sm'>{t('Cooldown (hours)')}</span>
          <Input
            className='max-w-xs'
            type='number'
            min={1}
            max={720}
            value={cooldown}
            onChange={(event) => setCooldown(event.target.value)}
          />
        </div>
        <Button
          className='w-fit'
          disabled={save.isPending}
          onClick={() => save.mutate()}
        >
          {t('Save')}
        </Button>
      </CardContent>
    </Card>
  )
}
