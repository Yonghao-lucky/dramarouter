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
import { AlertCircle } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'

import { getMissingModels } from '../api'
import { modelsQueryKeys } from '../lib/query-keys'
import { useModels } from './models-provider'

/**
 * 渠道里已经配置、但还没有模型资料的模型不会出现在列表中，
 * 也就没有定价入口。这里把它们露出来，避免藏在「更多」菜单里。
 */
export function MissingModelsBanner() {
  const { t } = useTranslation()
  const { setOpen } = useModels()
  const query = useQuery({
    queryKey: modelsQueryKeys.missing(),
    queryFn: getMissingModels,
    refetchOnWindowFocus: false,
  })
  const missing = query.data?.success ? (query.data.data ?? []) : []
  if (missing.length === 0) return null

  const preview = missing.slice(0, 3).join(', ')
  return (
    <Alert>
      <AlertCircle className='size-4' />
      <AlertDescription className='flex flex-wrap items-center justify-between gap-2'>
        <span>
          {t(
            '{{count}} models are configured in channels but have no model record, so they are not listed and cannot be priced: {{models}}',
            {
              count: missing.length,
              models: missing.length > 3 ? `${preview} …` : preview,
            }
          )}
        </span>
        <Button
          size='sm'
          variant='outline'
          onClick={() => setOpen('missing-models')}
        >
          {t('Add model records')}
        </Button>
      </AlertDescription>
    </Alert>
  )
}
