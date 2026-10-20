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

import {
  CHANNEL_TYPE_OPTIONS,
  CHANNEL_TYPE_TASK_PLUGIN,
  CHANNEL_TYPE_VOLC_NATIVE,
  channelTypeOptionsForTaskPluginBind,
} from '../../constants'

describe('channel type options for task plugin bind', () => {
  test('hides the task plugin type when the caller cannot bind', () => {
    const options = channelTypeOptionsForTaskPluginBind(false)

    expect(
      options.some((option) => option.value === CHANNEL_TYPE_TASK_PLUGIN)
    ).toBe(false)
  })

  test('shows the task plugin type when the caller can bind', () => {
    const options = channelTypeOptionsForTaskPluginBind(true)

    expect(options).toEqual(CHANNEL_TYPE_OPTIONS)
    expect(
      options.some((option) => option.value === CHANNEL_TYPE_TASK_PLUGIN)
    ).toBe(true)
  })

  test('keeps the downstream task plugin type id distinct from Volc Native', () => {
    const options = channelTypeOptionsForTaskPluginBind(true)

    expect(CHANNEL_TYPE_TASK_PLUGIN).toBe(62)
    expect(
      options.find((option) => option.value === CHANNEL_TYPE_TASK_PLUGIN)
    ).toEqual({ value: 62, label: 'Task Plugin' })
    expect(
      options.find((option) => option.value === CHANNEL_TYPE_VOLC_NATIVE)
    ).toEqual({ value: 61, label: 'Volc Native' })
  })
})
