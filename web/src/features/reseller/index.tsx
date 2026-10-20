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
import { Link } from '@tanstack/react-router'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { SectionPageLayout } from '@/components/layout'
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
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import {
  createResellerCustomer,
  getResellerCustomers,
  getResellerMargins,
  getResellerPriceBook,
  getResellerSummary,
  getResellerUsage,
  setResellerAllocation,
  setResellerCustomerPrice,
} from './api'
import type { ResellerCustomer } from './types'

// wallet and usage are quota units; QuotaPerUnit is 500000 by default upstream.
const QUOTA_PER_UNIT = 500000

function formatQuota(value: number) {
  return (value / QUOTA_PER_UNIT).toFixed(4)
}

function SummaryCard(props: { title: string; value: string; hint?: string }) {
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

function CustomersPanel() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [priceDrafts, setPriceDrafts] = useState<Record<number, string>>({})
  const [allocationDrafts, setAllocationDrafts] = useState<
    Record<number, string>
  >({})

  const customers = useQuery({
    queryKey: ['reseller', 'customers'],
    queryFn: () => getResellerCustomers(),
  })

  const refresh = () =>
    queryClient.invalidateQueries({ queryKey: ['reseller'] })

  const createCustomer = useMutation({
    mutationFn: () => createResellerCustomer(username, password),
    onSuccess: () => {
      toast.success(t('Customer created'))
      setUsername('')
      setPassword('')
      void refresh()
    },
    onError: (error: Error) => toast.error(error.message),
  })

  const saveAllocation = useMutation({
    mutationFn: (row: ResellerCustomer) =>
      setResellerAllocation(
        row.user_id,
        Math.round(Number(allocationDrafts[row.user_id] ?? 0))
      ),
    onSuccess: () => {
      toast.success(t('Allocation updated'))
      void refresh()
    },
    onError: (error: Error) => toast.error(error.message),
  })

  const savePrice = useMutation({
    mutationFn: (row: ResellerCustomer) =>
      setResellerCustomerPrice(
        row.user_id,
        row.group,
        priceDrafts[row.user_id] ?? ''
      ),
    onSuccess: () => {
      toast.success(t('Price updated'))
      void refresh()
    },
    onError: (error: Error) => toast.error(error.message),
  })

  return (
    <div className='flex flex-col gap-4'>
      <div className='flex flex-wrap items-end gap-2'>
        <Input
          className='w-44'
          placeholder={t('Username')}
          value={username}
          onChange={(event) => setUsername(event.target.value)}
        />
        <Input
          className='w-44'
          type='password'
          placeholder={t('Password')}
          value={password}
          onChange={(event) => setPassword(event.target.value)}
        />
        <Button
          disabled={!username || !password || createCustomer.isPending}
          onClick={() => createCustomer.mutate()}
        >
          {t('Create customer')}
        </Button>
      </div>

      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>{t('Customer')}</TableHead>
            <TableHead>{t('Wallet')}</TableHead>
            <TableHead>{t('Allocated')}</TableHead>
            <TableHead>{t('Used')}</TableHead>
            <TableHead>{t('Allocation')}</TableHead>
            <TableHead>{t('Retail ratio')}</TableHead>
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
              <TableCell className='tabular-nums'>
                {formatQuota(row.wallet_quota)}
              </TableCell>
              <TableCell className='tabular-nums'>
                {formatQuota(row.allocated_quota)}
              </TableCell>
              <TableCell className='tabular-nums'>
                {formatQuota(row.used_quota)}
              </TableCell>
              <TableCell>
                <div className='flex items-center gap-2'>
                  <Input
                    className='w-28'
                    placeholder={String(row.allocated_quota)}
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
                    onClick={() => saveAllocation.mutate(row)}
                  >
                    {t('Save')}
                  </Button>
                </div>
              </TableCell>
              <TableCell>
                <div className='flex items-center gap-2'>
                  <Input
                    className='w-24'
                    placeholder={t('Default')}
                    value={priceDrafts[row.user_id] ?? ''}
                    onChange={(event) =>
                      setPriceDrafts((prev) => ({
                        ...prev,
                        [row.user_id]: event.target.value,
                      }))
                    }
                  />
                  <Button
                    size='sm'
                    variant='outline'
                    disabled={!priceDrafts[row.user_id]}
                    onClick={() => savePrice.mutate(row)}
                  >
                    {t('Save')}
                  </Button>
                </div>
              </TableCell>
            </TableRow>
          ))}
          {(customers.data ?? []).length === 0 ? (
            <TableRow>
              <TableCell
                colSpan={6}
                className='text-muted-foreground text-center text-sm'
              >
                {t('No customers yet')}
              </TableCell>
            </TableRow>
          ) : null}
        </TableBody>
      </Table>
    </div>
  )
}

function UsagePanel() {
  const { t } = useTranslation()
  const [beforeId, setBeforeId] = useState(0)
  const usage = useQuery({
    queryKey: ['reseller', 'usage', beforeId],
    queryFn: () => getResellerUsage({ before_id: beforeId || undefined }),
  })

  return (
    <div className='flex flex-col gap-3'>
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>{t('Time')}</TableHead>
            <TableHead>{t('Customer')}</TableHead>
            <TableHead>{t('Model')}</TableHead>
            <TableHead>{t('Group')}</TableHead>
            <TableHead>{t('Retail charge')}</TableHead>
            <TableHead>{t('Margin')}</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {(usage.data ?? []).map((row) => (
            <TableRow key={row.id}>
              <TableCell className='whitespace-nowrap text-xs'>
                {new Date(row.occurred_at * 1000).toLocaleString()}
              </TableCell>
              <TableCell>#{row.customer_id}</TableCell>
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
          {(usage.data ?? []).length === 0 ? (
            <TableRow>
              <TableCell
                colSpan={6}
                className='text-muted-foreground text-center text-sm'
              >
                {t('No usage yet')}
              </TableCell>
            </TableRow>
          ) : null}
        </TableBody>
      </Table>
      <Button
        variant='outline'
        disabled={(usage.data ?? []).length === 0}
        onClick={() => {
          const rows = usage.data ?? []
          if (rows.length > 0) setBeforeId(rows[rows.length - 1].id)
        }}
      >
        {t('Load more')}
      </Button>
    </div>
  )
}

function MarginsPanel() {
  const { t } = useTranslation()
  const margins = useQuery({
    queryKey: ['reseller', 'margins'],
    queryFn: () => getResellerMargins(),
  })

  return (
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead>{t('Day')}</TableHead>
          <TableHead>{t('Requests')}</TableHead>
          <TableHead>{t('Retail charge')}</TableHead>
          <TableHead>{t('Margin')}</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {(margins.data ?? []).map((row) => (
          <TableRow key={row.day}>
            <TableCell>
              {new Date(row.day * 86400000).toLocaleDateString()}
            </TableCell>
            <TableCell className='tabular-nums'>{row.request_count}</TableCell>
            <TableCell className='tabular-nums'>
              {formatQuota(row.revenue_quota)}
            </TableCell>
            <TableCell className='tabular-nums'>
              {formatQuota(row.margin_quota)}
            </TableCell>
          </TableRow>
        ))}
        {(margins.data ?? []).length === 0 ? (
          <TableRow>
            <TableCell
              colSpan={4}
              className='text-muted-foreground text-center text-sm'
            >
              {t('No margin recorded yet')}
            </TableCell>
          </TableRow>
        ) : null}
      </TableBody>
    </Table>
  )
}

function PriceBookPanel() {
  const { t } = useTranslation()
  const book = useQuery({
    queryKey: ['reseller', 'price-book'],
    queryFn: () => getResellerPriceBook(),
  })

  return (
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead>{t('Group')}</TableHead>
          <TableHead>{t('Platform cost ratio')}</TableHead>
          <TableHead>{t('Your retail ratio')}</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {(book.data?.costs ?? []).map((cost) => {
          const retail = (book.data?.defaults ?? []).find(
            (item) => item.group === cost.group
          )
          return (
            <TableRow key={cost.group}>
              <TableCell>{cost.group}</TableCell>
              <TableCell>{cost.cost_ratio}</TableCell>
              <TableCell>{retail?.ratio ?? '-'}</TableCell>
            </TableRow>
          )
        })}
        {(book.data?.costs ?? []).length === 0 ? (
          <TableRow>
            <TableCell
              colSpan={3}
              className='text-muted-foreground text-center text-sm'
            >
              {t('No cost price configured')}
            </TableCell>
          </TableRow>
        ) : null}
      </TableBody>
    </Table>
  )
}

export function Reseller() {
  const { t } = useTranslation()
  const role = useAuthStore((state) => state.auth.user?.role ?? 0)
  const admin = role >= ROLE.ADMIN
  const summary = useQuery({
    queryKey: ['reseller', 'summary'],
    queryFn: getResellerSummary,
    retry: false,
  })

  if (summary.isError) {
    return (
      <SectionPageLayout>
        <SectionPageLayout.Title>{t('Reseller console')}</SectionPageLayout.Title>
        <SectionPageLayout.Content>
          <Card>
            <CardContent className='flex flex-col items-center gap-4 py-8 text-center text-sm'>
              {admin
                ? t(
                    'Administrator accounts are not resellers. Manage resellers from the administration page.'
                  )
                : t(
                    'This account is not an enabled reseller. Please contact the administrator.'
                  )}
              {admin ? (
                <Button render={<Link to='/reseller-admin' />}>
                  {t('Reseller management')}
                </Button>
              ) : null}
            </CardContent>
          </Card>
        </SectionPageLayout.Content>
      </SectionPageLayout>
    )
  }

  const data = summary.data

  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>{t('Reseller console')}</SectionPageLayout.Title>
      <SectionPageLayout.Content>
        <div className='flex flex-col gap-4'>
          <div className='grid gap-3 sm:grid-cols-2 xl:grid-cols-4'>
            <SummaryCard
              title={t('Wallet balance')}
              value={formatQuota(data?.wallet_quota ?? 0)}
            />
            <SummaryCard
              title={t('Customers')}
              value={String(data?.customer_count ?? 0)}
            />
            <SummaryCard
              title={t('Allocated')}
              value={formatQuota(data?.allocated_quota ?? 0)}
              hint={`${t('Used')} ${formatQuota(data?.used_quota ?? 0)}`}
            />
            <SummaryCard
              title={t('Total margin')}
              value={formatQuota(data?.margin_quota ?? 0)}
              hint={`${t('Pool')} ${formatQuota(data?.pool_margin_quota ?? 0)} · ${t('Self-pay')} ${formatQuota(data?.self_pay_margin_quota ?? 0)}`}
            />
          </div>

          <Tabs defaultValue='customers'>
            <TabsList>
              <TabsTrigger value='customers'>{t('Customers')}</TabsTrigger>
              <TabsTrigger value='usage'>{t('Usage')}</TabsTrigger>
              <TabsTrigger value='margins'>{t('Margin report')}</TabsTrigger>
              <TabsTrigger value='prices'>{t('Price book')}</TabsTrigger>
            </TabsList>
            <TabsContent value='customers' className='mt-4'>
              <CustomersPanel />
            </TabsContent>
            <TabsContent value='usage' className='mt-4'>
              <UsagePanel />
            </TabsContent>
            <TabsContent value='margins' className='mt-4'>
              <MarginsPanel />
            </TabsContent>
            <TabsContent value='prices' className='mt-4'>
              <PriceBookPanel />
            </TabsContent>
          </Tabs>
        </div>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}
