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

const RELOAD_GUARD_KEY = 'reseller-chunk-reload-at'
const RELOAD_GUARD_WINDOW_MS = 30_000

// A stale hashed chunk after a redeploy is recoverable: the newest build is
// already on the server, so one silent reload fixes it. The guard key stops an
// endless reload loop when the failure is not actually a stale chunk (for
// example a genuinely broken deploy, where reloading can never help).
export function reloadOnceAfterChunkError() {
  try {
    const last = Number(window.sessionStorage.getItem(RELOAD_GUARD_KEY) ?? 0)
    if (Date.now() - last < RELOAD_GUARD_WINDOW_MS) {
      return false
    }
    window.sessionStorage.setItem(RELOAD_GUARD_KEY, String(Date.now()))
  } catch {
    // Storage can be unavailable in private modes; skip the guard and reload.
  }
  window.location.reload()
  return true
}

function isChunkLoadError(error: unknown): boolean {
  const message = error instanceof Error ? error.message : String(error ?? '')
  return (
    /ChunkLoadError|Loading chunk|Importing a module script failed|dynamically imported module/i.test(
      message
    ) || /Unable to preload CSS|Failed to fetch dynamically imported module/i.test(message)
  )
}

// RouteErrorCard is the route-level fallback. It is now the last resort: the
// reseller admin page wraps its heavy panels in their own boundaries, so a
// single failed query no longer lands here.
export function RouteErrorCard({ error }: { error?: unknown }) {
  const { t } = useTranslation()
  const chunkFailure = isChunkLoadError(error)

  return (
    <Card className='mx-auto my-12 max-w-lg'>
      <CardContent className='flex flex-col items-center gap-4 py-10 text-center'>
        <div className='text-base font-medium'>
          {chunkFailure
            ? t('Loading the latest version')
            : t('This page failed to load')}
        </div>
        <p className='text-muted-foreground text-sm'>
          {chunkFailure
            ? t(
                'A new version was deployed. Reloading will pick it up automatically.'
              )
            : t(
                'The page may have been updated after a deployment. Please refresh to load the latest version.'
              )}
        </p>
        <Button
          onClick={() => {
            if (chunkFailure) {
              reloadOnceAfterChunkError()
              return
            }
            window.location.reload()
          }}
        >
          {t('Refresh')}
        </Button>
      </CardContent>
    </Card>
  )
}
