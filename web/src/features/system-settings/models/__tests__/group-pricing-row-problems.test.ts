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
import { describe, expect, test } from 'vitest'

import { groupPricingRowProblems } from '../group-ratio-visual-editor'

const row = (name: string, ratio = '1', topupRatio = '') => ({
  name,
  ratio,
  topupRatio,
})

// Each of these rows would silently change data when collapsed into maps.
describe('group pricing row problems', () => {
  test('a blank name blocks the save instead of being dropped', () => {
    expect(groupPricingRowProblems([row('default'), row('  ')])).toEqual([
      { kind: 'empty-name', name: '' },
    ])
  })

  test('a repeated name blocks the save instead of overwriting the first row', () => {
    expect(
      groupPricingRowProblems([row('vip', '0.9'), row('vip', '1')])
    ).toEqual([{ kind: 'duplicate-name', name: 'vip' }])
  })

  test('a cleared ratio blocks the save instead of billing the group for free', () => {
    expect(groupPricingRowProblems([row('hs10', '')])).toEqual([
      { kind: 'invalid-ratio', name: 'hs10' },
    ])
  })

  test('a negative top-up ratio is rejected; a blank one means unset', () => {
    expect(
      groupPricingRowProblems([row('a', '1', '-1'), row('b', '1', '')])
    ).toEqual([{ kind: 'invalid-topup', name: 'a' }])
  })

  test('valid rows, including ratio 0, have no problems', () => {
    expect(
      groupPricingRowProblems([row('free', '0'), row('vip', '0.9', '1')])
    ).toEqual([])
  })
})
