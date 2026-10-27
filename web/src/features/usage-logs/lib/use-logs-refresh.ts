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
import { useQueryClient } from '@tanstack/react-query'

/**
 * Marks every usage-log query stale so the next render fetches instead of
 * replaying the cache. Searching with unchanged filters produces an identical
 * query key, which React Query would otherwise serve straight from the cache;
 * this is what makes the Search button actually reload the list.
 */
export function useRefreshLogs() {
  const queryClient = useQueryClient()

  return async () => {
    await Promise.all([
      queryClient.invalidateQueries({ queryKey: ['logs'] }),
      queryClient.invalidateQueries({ queryKey: ['usage-logs-stats'] }),
    ])
  }
}
