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
  ResellerMarginDay,
  ResellerPriceBook,
  ResellerSummary,
  ResellerUsageRow,
} from './types'

interface Response<T> {
  success: boolean
  data: T
  message?: string
}

function unwrap<T>(response: Response<T>): T {
  if (!response.success) {
    throw new Error(response.message || 'Something went wrong!')
  }
  return response.data
}

export async function getResellerSummary() {
  return unwrap(
    (await api.get<Response<ResellerSummary>>('/api/reseller/summary')).data
  )
}

export async function getResellerCustomers(afterId = 0, limit = 50) {
  return unwrap(
    (
      await api.get<Response<ResellerCustomer[]>>('/api/reseller/customers', {
        params: { after_id: afterId, limit },
      })
    ).data
  )
}

export async function createResellerCustomer(username: string, password: string) {
  return (
    await api.post<Response<{ user_id: number; username: string }>>(
      '/api/reseller/customers',
      { username, password }
    )
  ).data
}

export async function setResellerAllocation(
  customerId: number,
  allocatedQuota: number
) {
  return (
    await api.put<Response<null>>(
      `/api/reseller/customers/${customerId}/allocation`,
      { allocated_quota: allocatedQuota }
    )
  ).data
}

export async function setResellerCustomerPrice(
  customerId: number,
  group: string,
  ratio: string
) {
  return (
    await api.put<Response<null>>(
      `/api/reseller/customers/${customerId}/price`,
      { group, ratio }
    )
  ).data
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
    (
      await api.get<Response<ResellerPriceBook>>('/api/reseller/price-book')
    ).data
  )
}
