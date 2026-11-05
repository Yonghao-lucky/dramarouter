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

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Skeleton } from '@/components/ui/skeleton'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { formatQuota, parseQuotaFromDollars } from '@/lib/format'

import {
  confirmResellerSettlement,
  createResellerSettlement,
  listResellerSettlements,
  voidResellerSettlement,
  type ResellerSettlementRow,
} from './api'

interface SettlementPanelProps {
  resellerId: number
}

// SettlementPanel is the reconciliation ledger: record what was received, then
// confirm it to credit the wallet. Keeping the two steps apart means a
// mistakenly entered payment never silently becomes spendable balance.
export function SettlementPanel({ resellerId }: SettlementPanelProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [amount, setAmount] = useState('')
  const [quota, setQuota] = useState('')
  const [currency, setCurrency] = useState<'CNY' | 'USD'>('CNY')
  const [method, setMethod] = useState('bank')
  const [reference, setReference] = useState('')
  const [remark, setRemark] = useState('')

  const settlements = useQuery({
    queryKey: ['reseller-admin', 'settlements', resellerId],
    queryFn: () => listResellerSettlements(resellerId, 100),
  })

  const refresh = () => {
    void queryClient.invalidateQueries({ queryKey: ['reseller-admin'] })
  }

  const create = useMutation({
    mutationFn: () =>
      createResellerSettlement({
        reseller_id: resellerId,
        direction: 'in',
        currency,
        // The operator types a human amount; store it in minor units so the
        // ledger never depends on a floating-point column.
        amount_minor: Math.round((Number(amount) || 0) * 100),
        quota: parseQuotaFromDollars(Number(quota) || 0),
        kind: 'recharge',
        method,
        external_ref: reference.trim() || undefined,
        remark: remark.trim() || undefined,
      }),
    onSuccess: () => {
      toast.success(t('Settlement recorded'))
      setAmount('')
      setQuota('')
      setReference('')
      setRemark('')
      refresh()
    },
    onError: (error: Error) => toast.error(error.message),
  })

  const confirm = useMutation({
    mutationFn: (id: number) => confirmResellerSettlement(id),
    onSuccess: () => {
      toast.success(t('Settlement confirmed and credited'))
      refresh()
    },
    onError: (error: Error) => toast.error(error.message),
  })

  const voidRow = useMutation({
    mutationFn: (id: number) => voidResellerSettlement(id),
    onSuccess: () => {
      toast.success(t('Settlement voided'))
      refresh()
    },
    onError: (error: Error) => toast.error(error.message),
  })

  const totals = settlements.data?.totals

  return (
    <div className='flex flex-col gap-3'>
      <Card>
        <CardHeader className='pb-2'>
          <CardTitle className='text-sm'>{t('Record a payment')}</CardTitle>
        </CardHeader>
        <CardContent className='flex flex-col gap-3'>
          <div className='grid gap-3 sm:grid-cols-2 lg:grid-cols-4'>
            <div className='flex flex-col gap-1'>
              <Label className='text-xs'>{t('Amount paid')}</Label>
              <Input
                type='number'
                min={0}
                value={amount}
                onChange={(event) => setAmount(event.target.value)}
              />
            </div>
            <div className='flex flex-col gap-1'>
              <Label className='text-xs'>{t('Currency')}</Label>
              <Select
                value={currency}
                onValueChange={(value) => setCurrency(value as 'CNY' | 'USD')}
              >
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectGroup>
                    <SelectItem value='CNY'>CNY</SelectItem>
                    <SelectItem value='USD'>USD</SelectItem>
                  </SelectGroup>
                </SelectContent>
              </Select>
            </div>
            <div className='flex flex-col gap-1'>
              <Label className='text-xs'>{t('Quota to credit (USD)')}</Label>
              <Input
                type='number'
                min={0}
                value={quota}
                onChange={(event) => setQuota(event.target.value)}
              />
            </div>
            <div className='flex flex-col gap-1'>
              <Label className='text-xs'>{t('Method')}</Label>
              <Input
                value={method}
                onChange={(event) => setMethod(event.target.value)}
              />
            </div>
            <div className='flex flex-col gap-1'>
              <Label className='text-xs'>{t('Reference')}</Label>
              <Input
                placeholder={t('Bank reference or TxID')}
                value={reference}
                onChange={(event) => setReference(event.target.value)}
              />
            </div>
            <div className='flex flex-col gap-1 sm:col-span-2'>
              <Label className='text-xs'>{t('Remark')}</Label>
              <Input
                value={remark}
                onChange={(event) => setRemark(event.target.value)}
              />
            </div>
          </div>
          <Button
            className='w-fit'
            disabled={!amount || !quota || create.isPending}
            onClick={() => create.mutate()}
          >
            {t('Record')}
          </Button>
        </CardContent>
      </Card>

      <div className='grid grid-cols-2 gap-2 text-sm lg:grid-cols-4'>
        <div className='rounded-md border p-2'>
          <div className='text-muted-foreground text-xs'>
            {t('Received (minor units)')}
          </div>
          <div className='tabular-nums'>
            {(totals?.received_minor ?? 0).toLocaleString()}
          </div>
        </div>
        <div className='rounded-md border p-2'>
          <div className='text-muted-foreground text-xs'>
            {t('Confirmed')}
          </div>
          <div className='tabular-nums'>
            {(totals?.confirmed_minor ?? 0).toLocaleString()}
          </div>
        </div>
        <div className='rounded-md border p-2'>
          <div className='text-muted-foreground text-xs'>{t('Pending')}</div>
          <div className='tabular-nums'>
            {(totals?.pending_minor ?? 0).toLocaleString()}
            <span className='text-muted-foreground ml-1 text-xs'>
              ({totals?.pending_count ?? 0})
            </span>
          </div>
        </div>
        <div className='rounded-md border p-2'>
          <div className='text-muted-foreground text-xs'>
            {t('Quota credited')}
          </div>
          <div className='tabular-nums'>
            {formatQuota(totals?.credited_quota ?? 0)}
          </div>
        </div>
      </div>

      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>{t('Time')}</TableHead>
            <TableHead>{t('Amount')}</TableHead>
            <TableHead>{t('Quota')}</TableHead>
            <TableHead>{t('Status')}</TableHead>
            <TableHead>{t('Reference')}</TableHead>
            <TableHead />
          </TableRow>
        </TableHeader>
        <TableBody>
          <SettlementRows
            rows={settlements.data?.items ?? []}
            loading={settlements.isPending}
            onConfirm={(id) => confirm.mutate(id)}
            onVoid={(id) => voidRow.mutate(id)}
            busy={confirm.isPending || voidRow.isPending}
          />
        </TableBody>
      </Table>
    </div>
  )
}

function SettlementRows(props: {
  rows: ResellerSettlementRow[]
  loading: boolean
  busy: boolean
  onConfirm: (id: number) => void
  onVoid: (id: number) => void
}) {
  const { t } = useTranslation()

  if (props.loading) {
    return (
      <>
        {[0, 1, 2].map((index) => (
          <TableRow key={index}>
            <TableCell colSpan={6}>
              <Skeleton className='h-6 w-full' />
            </TableCell>
          </TableRow>
        ))}
      </>
    )
  }
  if (props.rows.length === 0) {
    return (
      <TableRow>
        <TableCell
          colSpan={6}
          className='text-muted-foreground py-6 text-center text-sm'
        >
          {t('No settlements yet')}
        </TableCell>
      </TableRow>
    )
  }
  return (
    <>
      {props.rows.map((row) => (
        <TableRow key={row.id}>
          <TableCell className='whitespace-nowrap'>
            {new Date(row.created_at * 1000).toLocaleString()}
          </TableCell>
          <TableCell className='tabular-nums'>
            {(row.amount_minor / 100).toFixed(2)} {row.currency}
          </TableCell>
          <TableCell className='tabular-nums'>
            {formatQuota(row.quota_amount)}
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
          <TableCell
            className='text-muted-foreground max-w-40 truncate text-xs'
            title={row.external_ref}
          >
            {row.external_ref || '—'}
          </TableCell>
          <TableCell>
            {row.status === 'pending' ? (
              <div className='flex flex-wrap gap-1'>
                <Button
                  size='sm'
                  disabled={props.busy}
                  onClick={() => props.onConfirm(row.id)}
                >
                  {t('Confirm')}
                </Button>
                <Button
                  size='sm'
                  variant='ghost'
                  disabled={props.busy}
                  onClick={() => props.onVoid(row.id)}
                >
                  {t('Void')}
                </Button>
              </div>
            ) : null}
          </TableCell>
        </TableRow>
      ))}
    </>
  )
}
