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
import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { formatQuota, formatTimestampRelative } from '@/lib/format'
import { cn } from '@/lib/utils'

import type { ResellerOverviewRow } from './api'

interface OverviewPanelProps {
  rows: ResellerOverviewRow[]
  loading: boolean
  selectedId: number | null
  onSelect: (resellerId: number) => void
  onTopUp: (row: ResellerOverviewRow) => void
}

const COLUMN_COUNT = 12

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

export function OverviewPanel(props: OverviewPanelProps) {
  const { t } = useTranslation()

  const totals = props.rows.reduce(
    (acc, row) => ({
      wallet: acc.wallet + row.wallet_quota,
      customers: acc.customers + row.customer_count,
      precredit: acc.precredit + row.granted_precredit_quota,
      pendingMargin: acc.pendingMargin + row.pending_margin_quota,
    }),
    { wallet: 0, customers: 0, precredit: 0, pendingMargin: 0 }
  )

  return (
    <div className='flex flex-col gap-4'>
      <div className='grid grid-cols-2 gap-3 lg:grid-cols-5'>
        <StatCard
          title={t('Resellers')}
          value={props.rows.length.toLocaleString()}
        />
        <StatCard
          title={t('Total wallet')}
          value={formatQuota(totals.wallet)}
        />
        <StatCard
          title={t('Total customers')}
          value={totals.customers.toLocaleString()}
        />
        <StatCard
          title={t('Unpaid credit')}
          value={formatQuota(totals.precredit)}
          hint={t('Pre-authorized credit not yet paid for')}
        />
        <StatCard
          title={t('Pending margin')}
          value={formatQuota(totals.pendingMargin)}
          hint={t('Earned margin still inside reseller wallets')}
        />
      </div>

      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>{t('Reseller')}</TableHead>
            <TableHead>{t('Status')}</TableHead>
            <TableHead>{t('Wallet')}</TableHead>
            <TableHead>{t('Customers')}</TableHead>
            <TableHead>{t('Customer balance')}</TableHead>
            <TableHead>{t('Funded')}</TableHead>
            <TableHead>{t('Revenue')}</TableHead>
            <TableHead>{t('Margin')}</TableHead>
            <TableHead>{t('Pending margin')}</TableHead>
            <TableHead>{t('Grants')}</TableHead>
            <TableHead>{t('Last activity')}</TableHead>
            <TableHead>{t('Actions')}</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {props.loading
            ? [0, 1, 2].map((index) => (
                <TableRow key={`skeleton-${index}`}>
                  <TableCell colSpan={COLUMN_COUNT}>
                    <Skeleton className='h-8 w-full' />
                  </TableCell>
                </TableRow>
              ))
            : null}

          {!props.loading
            ? props.rows.map((row) => {
                const subtitle = row.display_name || row.name
                return (
                  <TableRow
                    key={row.reseller_id}
                    className={cn(
                      props.selectedId === row.reseller_id && 'bg-muted/40'
                    )}
                  >
                    <TableCell>
                      <div className='flex flex-col'>
                        <span>{row.username}</span>
                        <span className='text-muted-foreground text-xs'>
                          {subtitle || `#${row.reseller_id}`}
                        </span>
                      </div>
                    </TableCell>
                    <TableCell>
                      <Badge
                        variant={row.status === 2 ? 'default' : 'secondary'}
                      >
                        {row.status === 2 ? t('Enabled') : t('Disabled')}
                      </Badge>
                    </TableCell>
                    <TableCell className='tabular-nums'>
                      {formatQuota(row.wallet_quota)}
                    </TableCell>
                    <TableCell className='tabular-nums'>
                      {row.customer_count.toLocaleString()}
                    </TableCell>
                    <TableCell className='tabular-nums'>
                      {formatQuota(row.customer_balance_quota)}
                    </TableCell>
                    <TableCell className='tabular-nums'>
                      {formatQuota(row.funded_quota)}
                    </TableCell>
                    <TableCell className='tabular-nums'>
                      {formatQuota(row.revenue_quota)}
                    </TableCell>
                    <TableCell className='tabular-nums'>
                      {formatQuota(row.margin_quota)}
                    </TableCell>
                    <TableCell className='tabular-nums'>
                      {formatQuota(row.pending_margin_quota)}
                    </TableCell>
                    <TableCell className='text-xs tabular-nums'>
                      <div className='flex flex-col'>
                        <span>
                          <span className='text-muted-foreground'>
                            {t('Recharge')}
                          </span>{' '}
                          {formatQuota(row.granted_recharge_quota)}
                        </span>
                        <span>
                          <span className='text-muted-foreground'>
                            {t('Precredit')}
                          </span>{' '}
                          {formatQuota(row.granted_precredit_quota)}
                        </span>
                      </div>
                    </TableCell>
                    <TableCell className='text-muted-foreground text-xs whitespace-nowrap'>
                      {row.last_activity_at === 0
                        ? '—'
                        : formatTimestampRelative(row.last_activity_at)}
                    </TableCell>
                    <TableCell>
                      <div className='flex flex-wrap gap-1'>
                        <Button
                          size='sm'
                          variant='outline'
                          onClick={() => props.onSelect(row.reseller_id)}
                        >
                          {t('Manage')}
                        </Button>
                        <Button
                          size='sm'
                          variant='ghost'
                          onClick={() => props.onTopUp(row)}
                        >
                          {t('Top up')}
                        </Button>
                      </div>
                    </TableCell>
                  </TableRow>
                )
              })
            : null}

          {!props.loading && props.rows.length === 0 ? (
            <TableRow>
              <TableCell
                colSpan={COLUMN_COUNT}
                className='text-muted-foreground py-8 text-center text-sm'
              >
                {t('No resellers yet')}
              </TableCell>
            </TableRow>
          ) : null}
        </TableBody>
      </Table>
    </div>
  )
}
