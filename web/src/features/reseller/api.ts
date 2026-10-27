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
import { api } from '@/lib/api'

import type {
  ResellerCustomer,
  ResellerCustomerDetail,
  ResellerCustomerGroup,
  ResellerLogRow,
  ResellerMarginDay,
  ResellerPriceBook,
  ResellerSummary,
  ResellerTransfer,
  ResellerUsageRow,
} from './types'

interface Response<T> {
  success: boolean
  data: T
  message?: string
}

interface PageResponse<T> {
  success: boolean
  data: T[] | { items: T[]; total: number; page: number; page_size: number }
  message?: string
}

function unwrap<T>(response: Response<T>): T {
  if (!response.success) {
    throw new Error(response.message || 'Something went wrong!')
  }
  return response.data
}

function unwrapPage<T>(response: PageResponse<T>): {
  items: T[]
  total: number
} {
  if (!response.success) {
    throw new Error(response.message || 'Something went wrong!')
  }
  if (Array.isArray(response.data)) {
    return { items: response.data, total: response.data.length }
  }
  return { items: response.data.items ?? [], total: response.data.total ?? 0 }
}

export async function getResellerSummary() {
  return unwrap(
    (await api.get<Response<ResellerSummary>>('/api/reseller/summary')).data
  )
}

export async function getResellerCustomers(
  keyword = '',
  afterId = 0,
  limit = 50
) {
  return unwrap(
    (
      await api.get<Response<ResellerCustomer[]>>('/api/reseller/customers', {
        params: { keyword, after_id: afterId, limit },
      })
    ).data
  )
}

export async function getResellerCustomerDetail(customerId: number) {
  return unwrap(
    (
      await api.get<Response<ResellerCustomerDetail>>(
        `/api/reseller/customers/${customerId}`
      )
    ).data
  )
}

export async function createResellerCustomer(payload: {
  username: string
  password: string
  display_name?: string
  email?: string
  group?: string
}) {
  return unwrap(
    (
      await api.post<Response<{ user_id: number; username: string }>>(
        '/api/reseller/customers',
        payload
      )
    ).data
  )
}

export async function setResellerCustomerStatus(
  customerId: number,
  status: number
) {
  return (
    await api.put<Response<null>>(
      `/api/reseller/customers/${customerId}/status`,
      {
        status,
      }
    )
  ).data
}

export async function resetResellerCustomerPassword(
  customerId: number,
  password: string
) {
  return (
    await api.put<Response<null>>(
      `/api/reseller/customers/${customerId}/password`,
      { password }
    )
  ).data
}

export async function setResellerCustomerGroup(
  customerId: number,
  group: string
) {
  return (
    await api.put<Response<null>>(
      `/api/reseller/customers/${customerId}/group`,
      {
        group,
      }
    )
  ).data
}

export async function getResellerCustomerGroups(customerId: number) {
  return unwrap(
    (
      await api.get<Response<ResellerCustomerGroup[]>>(
        `/api/reseller/customers/${customerId}/groups`
      )
    ).data
  )
}

export async function setResellerCustomerPrice(
  customerId: number,
  group: string,
  ratio: string
) {
  return (
    await api.put<Response<null>>(
      `/api/reseller/customers/${customerId}/price`,
      {
        group,
        ratio,
      }
    )
  ).data
}

export async function transferResellerQuota(
  customerId: number,
  payload: {
    direction: 'fund' | 'reclaim'
    // Pricing always follows the customer's own group on the server; the
    // payload deliberately carries no group so a reseller cannot bill one
    // group's cost while the customer consumes at another.
    retail_quota: number
    remark?: string
    event_key?: string
  }
) {
  return unwrap(
    (
      await api.post<Response<ResellerTransfer>>(
        `/api/reseller/customers/${customerId}/transfer`,
        payload
      )
    ).data
  )
}

export async function getResellerTransfers(params: {
  customer_id?: number
  before_id?: number
  limit?: number
}) {
  return unwrap(
    (
      await api.get<Response<ResellerTransfer[]>>('/api/reseller/transfers', {
        params,
      })
    ).data
  )
}

export async function getResellerUsage(params: {
  customer_id?: number
  before_id?: number
  start?: number
  end?: number
  limit?: number
}) {
  return unwrap(
    (
      await api.get<Response<ResellerUsageRow[]>>('/api/reseller/usage', {
        params,
      })
    ).data
  )
}

export async function getResellerMargins(start?: number, end?: number) {
  return unwrap(
    (
      await api.get<Response<ResellerMarginDay[]>>('/api/reseller/margins', {
        params: { start, end },
      })
    ).data
  )
}

export async function getResellerPriceBook() {
  return unwrap(
    (await api.get<Response<ResellerPriceBook>>('/api/reseller/price-book'))
      .data
  )
}

export async function getResellerLogs(params: {
  customer_id?: number
  type?: number
  model_name?: string
  start_timestamp?: number
  end_timestamp?: number
  p?: number
  page_size?: number
}) {
  return unwrapPage(
    (
      await api.get<PageResponse<ResellerLogRow>>('/api/reseller/logs', {
        params,
      })
    ).data
  )
}

export interface ResellerTaskRow {
  id: number
  created_at: number
  task_id: string
  platform: string
  user_id: number
  group: string
  quota: number
  action: string
  status: string
  fail_reason: string
  model_name: string
  progress: string
}

export async function getResellerTasks(params: {
  customer_id?: number
  model_name?: string
  status?: string
  start_timestamp?: number
  end_timestamp?: number
  p?: number
  page_size?: number
}) {
  return unwrapPage<ResellerTaskRow>(
    (
      await api.get<PageResponse<ResellerTaskRow>>('/api/reseller/tasks', {
        params,
      })
    ).data
  )
}
