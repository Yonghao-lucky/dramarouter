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
import { useMutation } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
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
import { Textarea } from '@/components/ui/textarea'
import { parseQuotaFromDollars } from '@/lib/format'

import { grantResellerWallet } from './api'

const REMARK_MAX_LENGTH = 255

type GrantKind = 'recharge' | 'precredit'

interface GrantDialogProps {
  open: boolean
  resellerId: number | null
  resellerLabel: string
  onClose: () => void
  onGranted: () => void
}

export function GrantDialog(props: GrantDialogProps) {
  const { t } = useTranslation()
  const [amount, setAmount] = useState('')
  const [kind, setKind] = useState<GrantKind>('recharge')
  const [remark, setRemark] = useState('')

  // The input holds a display amount the operator typed, so keep the raw string
  // instead of a number: clearing the field must not collapse to "0" on screen.
  const amountValue = Number(amount)
  const quota = Number.isFinite(amountValue)
    ? parseQuotaFromDollars(amountValue)
    : 0

  const resetForm = () => {
    setAmount('')
    setKind('recharge')
    setRemark('')
  }

  const grant = useMutation({
    mutationFn: () =>
      grantResellerWallet({
        reseller_id: props.resellerId as number,
        quota,
        kind,
        remark: remark.trim() ? remark.trim() : undefined,
      }),
    onSuccess: () => {
      toast.success(t('Balance added'))
      resetForm()
      props.onGranted()
      props.onClose()
    },
    onError: (error: Error) => toast.error(error.message),
  })

  const canSubmit =
    props.resellerId != null && quota > 0 && !grant.isPending

  return (
    <Dialog
      open={props.open}
      onOpenChange={(open) => {
        if (!open) {
          resetForm()
          props.onClose()
        }
      }}
    >
      <DialogContent>
        <DialogHeader>
          <DialogTitle>
            {t('Add balance to {{name}}', { name: props.resellerLabel })}
          </DialogTitle>
        </DialogHeader>

        <div className='flex flex-col gap-4'>
          <div className='flex flex-col gap-2'>
            <Label htmlFor='reseller-grant-amount'>{t('Amount (USD)')}</Label>
            <Input
              id='reseller-grant-amount'
              type='number'
              min={0}
              step={0.01}
              inputMode='decimal'
              placeholder={t('Amount (USD)')}
              value={amount}
              onChange={(event) => setAmount(event.target.value)}
            />
          </div>

          <div className='flex flex-col gap-2'>
            <Label>{t('Grant type')}</Label>
            <Select
              items={[
                { value: 'recharge', label: t('Recharge') },
                { value: 'precredit', label: t('Pre-credit') },
              ]}
              value={kind}
              onValueChange={(value) => {
                if (value === 'recharge' || value === 'precredit') {
                  setKind(value)
                }
              }}
            >
              <SelectTrigger className='w-full'>
                <SelectValue placeholder={t('Recharge')} />
              </SelectTrigger>
              <SelectContent alignItemWithTrigger={false}>
                <SelectGroup>
                  <SelectItem value='recharge'>{t('Recharge')}</SelectItem>
                  <SelectItem value='precredit'>{t('Pre-credit')}</SelectItem>
                </SelectGroup>
              </SelectContent>
            </Select>
            <p className='text-muted-foreground text-xs'>
              {t(
                'Recharge means the reseller has already paid; pre-credit means the platform fronts the balance before payment arrives.'
              )}
            </p>
          </div>

          <div className='flex flex-col gap-2'>
            <Label htmlFor='reseller-grant-remark'>{t('Remark')}</Label>
            <Textarea
              id='reseller-grant-remark'
              placeholder={t('Remark (optional)')}
              maxLength={REMARK_MAX_LENGTH}
              value={remark}
              onChange={(event) => setRemark(event.target.value)}
            />
            <div className='text-muted-foreground text-right text-xs tabular-nums'>
              {REMARK_MAX_LENGTH - remark.length}
            </div>
          </div>

          <div className='flex justify-end gap-2'>
            <Button
              variant='outline'
              disabled={grant.isPending}
              onClick={() => {
                resetForm()
                props.onClose()
              }}
            >
              {t('Cancel')}
            </Button>
            <Button disabled={!canSubmit} onClick={() => grant.mutate()}>
              {grant.isPending ? t('Saving') : t('Add balance')}
            </Button>
          </div>
        </div>
      </DialogContent>
    </Dialog>
  )
}

export default GrantDialog
