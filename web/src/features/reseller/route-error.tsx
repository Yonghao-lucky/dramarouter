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

import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'

// RouteErrorCard keeps a reseller page failure local. A stale hashed chunk after
// a redeploy or a failed query must not replace the whole dashboard with the
// generic 500 page.
export function RouteErrorCard() {
  const { t } = useTranslation()

  return (
    <Card className='mx-auto my-12 max-w-lg'>
      <CardContent className='flex flex-col items-center gap-4 py-10 text-center'>
        <div className='text-base font-medium'>
          {t('This page failed to load')}
        </div>
        <p className='text-muted-foreground text-sm'>
          {t(
            'The page may have been updated after a deployment. Please refresh to load the latest version.'
          )}
        </p>
        <Button onClick={() => window.location.reload()}>{t('Refresh')}</Button>
      </CardContent>
    </Card>
  )
}
