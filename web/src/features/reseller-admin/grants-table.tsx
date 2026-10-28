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
import { Skeleton } from '@/components/ui/skeleton'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { formatQuota } from '@/lib/format'

import type { ResellerWalletGrant } from './api'

interface GrantsTableProps {
  grants: ResellerWalletGrant[]
  loading: boolean
}

// Skeleton rows keep the table shell in place while the grants load, so the
// surrounding panel does not jump when the response lands.
function LoadingRows() {
  return (
    <>
      {[0, 1, 2].map((index) => (
        <TableRow key={index}>
          {[0, 1, 2, 3, 4].map((cell) => (
            <TableCell key={cell}>
              <Skeleton className='h-4 w-full' />
            </TableCell>
          ))}
        </TableRow>
      ))}
    </>
  )
}

export function GrantsTable({ grants, loading }: GrantsTableProps) {
  const { t } = useTranslation()

  return (
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead>{t('Time')}</TableHead>
          <TableHead>{t('Type')}</TableHead>
          <TableHead>{t('Amount')}</TableHead>
          <TableHead>{t('Remark')}</TableHead>
          <TableHead>{t('Operator')}</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        <GrantRows grants={grants} loading={loading} />
      </TableBody>
    </Table>
  )
}

// GrantRows keeps the three body states as plain branches instead of a nested
// ternary, which reads badly once the row markup grows.
function GrantRows({ grants, loading }: GrantsTableProps) {
  const { t } = useTranslation()

  if (loading) {
    return <LoadingRows />
  }
  if (grants.length === 0) {
    return (
      <TableRow>
        <TableCell
          colSpan={5}
          className='text-muted-foreground py-8 text-center text-sm'
        >
          {t('No top-ups yet')}
        </TableCell>
      </TableRow>
    )
  }
  return (
    <>
      {grants.map((grant) => (
        <TableRow key={grant.id}>
          <TableCell>
            {new Date(grant.created_at * 1000).toLocaleString()}
          </TableCell>
          <TableCell>
            <Badge
              variant={grant.kind === 'recharge' ? 'default' : 'secondary'}
            >
              {grant.kind === 'recharge' ? t('Recharge') : t('Pre-credit')}
            </Badge>
          </TableCell>
          <TableCell className='tabular-nums'>
            {formatQuota(grant.amount)}
          </TableCell>
          <TableCell
            className='text-muted-foreground max-w-[24rem] truncate'
            title={grant.remark}
          >
            {grant.remark || '—'}
          </TableCell>
          <TableCell>#{grant.operator_id}</TableCell>
        </TableRow>
      ))}
    </>
  )
}
