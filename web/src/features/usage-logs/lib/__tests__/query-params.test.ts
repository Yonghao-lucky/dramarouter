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
import { describe, expect, it } from 'vitest'

import { isSameSearch } from '../query-params'

describe('isSameSearch', () => {
  it('treats an unchanged filter set as the same search', () => {
    const current = {
      page: 1,
      startTime: 1000,
      endTime: 2000,
      type: ['0'],
      model: 'gpt-5',
    }
    expect(
      isSameSearch(
        { ...current, model: 'gpt-5', type: ['0'] },
        { ...current }
      )
    ).toBe(true)
  })

  it('ignores keys that are absent on both sides and blanks versus absent', () => {
    const current = { page: 1, startTime: 1000, endTime: 2000 }
    expect(isSameSearch({ ...current, model: '' }, current)).toBe(true)
    expect(isSameSearch(current, { ...current, channel: undefined })).toBe(true)
  })

  it('detects a changed scalar filter', () => {
    const current = { page: 1, startTime: 1000, endTime: 2000, model: 'gpt-5' }
    expect(isSameSearch({ ...current, model: 'claude' }, current)).toBe(false)
  })

  it('detects a changed array filter and a different page', () => {
    const current = { page: 1, type: ['0'] }
    expect(isSameSearch({ ...current, type: ['2'] }, current)).toBe(false)
    expect(isSameSearch({ ...current, page: 2 }, current)).toBe(false)
  })
})
