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
export function buildQueryParams(
  params: Record<string, unknown>
): URLSearchParams {
  const queryParams = new URLSearchParams()
  Object.entries(params).forEach(([key, value]) => {
    if (value !== undefined && value !== null && value !== '') {
      queryParams.append(key, String(value))
    }
  })
  return queryParams
}

function normalizeSearchValue(value: unknown): string {
  if (value === undefined || value === null) return ''
  return Array.isArray(value) ? value.join(',') : String(value)
}

/**
 * Compares the next search the filter bar is about to navigate to against the
 * one already in the URL. Searching with an unchanged filter set must not push
 * a history entry that looks like a no-op; the caller refreshes the cached logs
 * instead.
 */
export function isSameSearch(
  next: Record<string, unknown>,
  current: Record<string, unknown>
): boolean {
  const keys = new Set([...Object.keys(next), ...Object.keys(current)])
  for (const key of keys) {
    if (normalizeSearchValue(next[key]) !== normalizeSearchValue(current[key])) {
      return false
    }
  }
  return true
}
