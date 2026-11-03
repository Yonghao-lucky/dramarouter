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
import { Link2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { CopyButton } from '@/components/copy-button'
import { Button } from '@/components/ui/button'
import {
  Popover,
  PopoverContent,
  PopoverDescription,
  PopoverHeader,
  PopoverTitle,
  PopoverTrigger,
} from '@/components/ui/popover'
import { useStatus } from '@/hooks/use-status'

/**
 * API 地址入口：客户经常需要把本站地址复制到 SDK / 客户端的 base_url。
 * 地址来源与站内其他位置保持一致：优先 status.server_address，缺失时退回当前源。
 */
export function ApiAddressPopover() {
  const { t } = useTranslation()
  const { status } = useStatus()
  const serverAddress =
    (status?.server_address as string | undefined) || window.location.origin

  return (
    <Popover>
      <PopoverTrigger
        render={
          <Button size='sm' variant='outline'>
            <Link2 className='h-4 w-4' />
            {t('API Address')}
          </Button>
        }
      />
      <PopoverContent align='end' className='w-80'>
        <PopoverHeader>
          <PopoverTitle>{t('API Address')}</PopoverTitle>
          <PopoverDescription>
            {t('Use this address as the base URL in your client.')}
          </PopoverDescription>
        </PopoverHeader>
        <div className='mt-3 flex items-center gap-2 rounded-md border p-2'>
          <code className='min-w-0 flex-1 truncate text-xs'>
            {serverAddress}
          </code>
          <CopyButton
            value={serverAddress}
            size='icon'
            tooltip={t('Copy API address')}
            aria-label={t('Copy API address')}
          />
        </div>
      </PopoverContent>
    </Popover>
  )
}
