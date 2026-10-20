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
import { useQuery } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Dialog } from '@/components/dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { api } from '@/lib/api'

interface PriceRow {
  id: number
  user_id: number
  group: string
  ratio: string
  source: string
}

async function loadPricing(userId: number) {
  const [prices, groups] = await Promise.all([
    api.get('/api/user/' + userId + '/group-pricing'),
    api.get('/api/group/'),
  ])
  const overrides: PriceRow[] = prices.data?.data ?? []
  const groupNames: string[] = groups.data?.data ?? []
  return { overrides, groupNames }
}

export function UserPricingDialog(props: {
  open: boolean
  onOpenChange: (open: boolean) => void
  userId: number
  username: string
}) {
  const { t } = useTranslation()
  const [drafts, setDrafts] = useState<Record<string, string>>({})
  const [saving, setSaving] = useState(false)

  const pricing = useQuery({
    queryKey: ['user-pricing', props.userId],
    queryFn: () => loadPricing(props.userId),
    enabled: props.open,
  })

  const overrides = pricing.data?.overrides ?? []
  const groups = pricing.data?.groupNames ?? []
  const overrideByGroup = new Map(overrides.map((row) => [row.group, row.ratio]))

  const save = async (group: string) => {
    const ratio = (drafts[group] ?? '').trim()
    setSaving(true)
    try {
      const response = await api.put(`/api/user/${props.userId}/group-pricing`, {
        group,
        ratio,
      })
      if (!response.data?.success) {
        throw new Error(response.data?.message || t('Failed to save price'))
      }
      toast.success(ratio ? t('Price saved') : t('Price override cleared'))
      setDrafts((prev) => ({ ...prev, [group]: '' }))
      await pricing.refetch()
    } catch (error) {
      toast.error(error instanceof Error ? error.message : t('Failed to save price'))
    } finally {
      setSaving(false)
    }
  }

  return (
    <Dialog
      open={props.open}
      onOpenChange={props.onOpenChange}
      title={t('Per-user pricing')}
      description={t(
        'Set a retail ratio for one billing group. An empty value clears the override and the group ratio applies again.'
      )}
      contentHeight='auto'
      bodyClassName='space-y-3'
      footer={
        <Button variant='outline' onClick={() => props.onOpenChange(false)}>
          {t('Close')}
        </Button>
      }
    >
      <div className='text-muted-foreground text-sm'>#{props.userId} {props.username}</div>
      <div className='flex flex-col gap-2'>
        {groups.map((group) => (
          <div key={group} className='flex items-end gap-2'>
            <div className='flex-1'>
              <Label className='text-xs'>{group}</Label>
              <Input
                className='h-8'
                placeholder={overrideByGroup.get(group) ?? t('Group ratio')}
                value={drafts[group] ?? ''}
                onChange={(event) =>
                  setDrafts((prev) => ({ ...prev, [group]: event.target.value }))
                }
              />
            </div>
            <Button
              size='sm'
              variant='outline'
              disabled={saving || (drafts[group] ?? '').trim() === ''}
              onClick={() => void save(group)}
            >
              {t('Save')}
            </Button>
            {overrideByGroup.has(group) ? (
              <Button
                size='sm'
                variant='ghost'
                disabled={saving}
                onClick={() => {
                  setDrafts((prev) => ({ ...prev, [group]: ' ' }))
                  void save(group)
                }}
              >
                {t('Clear')}
              </Button>
            ) : null}
          </div>
        ))}
      </div>
    </Dialog>
  )
}
