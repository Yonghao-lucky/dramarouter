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
import { ROLE } from '@/lib/roles'

interface Response<T> {
  success: boolean
  data: T
  message?: string
}

export interface ResellerProfile {
  id: number
  user_id: number
  name: string
  code: string
  status: number
  created_at: number
  updated_at: number
}

export interface ResellerBook {
  costs: {
    id: number
    reseller_id: number
    group: string
    cost_ratio: string
    enabled: boolean
    updated_at: number
  }[]
  defaults: {
    id: number
    reseller_id: number
    group: string
    ratio: string
    updated_at: number
  }[]
  /** The platform's own ratio per group: what an ordinary account pays. */
  platform_ratios?: Record<string, number>
}

export interface AdminResellerCustomer {
  user_id: number
  username: string
  display_name: string
  group: string
  status: number
  wallet_quota: number
  funded_quota: number
  funded_cost_quota: number
  used_quota: number
  request_count: number
  last_activity_at: number
}

export interface ResellerOverviewRow {
  reseller_id: number
  username: string
  display_name: string
  name: string
  code: string
  status: number
  wallet_quota: number
  customer_count: number
  customer_balance_quota: number
  funded_quota: number
  revenue_quota: number
  margin_quota: number
  pending_margin_quota: number
  granted_recharge_quota: number
  granted_precredit_quota: number
  last_activity_at: number
  created_at: number
}

export interface ResellerWalletGrant {
  id: number
  event_key: string
  reseller_id: number
  amount: number
  kind: 'recharge' | 'precredit'
  remark: string
  operator_id: number
  created_at: number
}

function unwrap<T>(response: Response<T>): T {
  if (!response.success) {
    throw new Error(response.message || 'Something went wrong!')
  }
  return response.data
}

export async function listResellerProfiles() {
  return unwrap(
    (await api.get<Response<ResellerProfile[]>>('/api/user/reseller/profiles'))
      .data
  )
}

export async function upsertResellerProfile(payload: {
  user_id: number
  name?: string
  code?: string
  status?: number
}) {
  return (
    await api.put<Response<ResellerProfile>>(
      '/api/user/reseller/profile',
      payload
    )
  ).data
}

export async function setResellerCost(payload: {
  reseller_id: number
  group: string
  ratio: string
  enabled?: boolean
}) {
  return (await api.put<Response<null>>('/api/user/reseller/cost', payload))
    .data
}

export async function setResellerDefaultPrice(payload: {
  reseller_id: number
  group: string
  ratio: string
}) {
  return (await api.put<Response<null>>('/api/user/reseller/price', payload))
    .data
}

export async function getResellerBook(resellerId: number) {
  return unwrap(
    (
      await api.get<Response<ResellerBook>>('/api/user/reseller/book', {
        params: { reseller_id: resellerId },
      })
    ).data
  )
}

export async function getResellerCustomers(resellerId: number) {
  return unwrap(
    (
      await api.get<Response<AdminResellerCustomer[]>>(
        '/api/user/reseller/customers',
        { params: { reseller_id: resellerId } }
      )
    ).data
  )
}

export async function setCustomerGroupPrice(
  userId: number,
  group: string,
  ratio: string
) {
  return (
    await api.put<Response<null>>(`/api/user/${userId}/group-pricing`, {
      group,
      ratio,
    })
  ).data
}

export async function setCustomerOwner(
  userId: number,
  resellerId: number,
  reason: string
) {
  return (
    await api.put<Response<null>>(`/api/user/${userId}/owner`, {
      reseller_id: resellerId,
      reason,
    })
  ).data
}

export async function transferResellerQuota(payload: {
  reseller_id: number
  customer_user_id: number
  direction: 'fund' | 'reclaim'
  retail_quota: number
  remark?: string
}) {
  return (
    await api.post<Response<null>>('/api/user/reseller/transfer', payload)
  ).data
}

export async function getGroups() {
  return unwrap((await api.get<Response<string[]>>('/api/group/')).data)
}

export async function listResellerOverviews() {
  return unwrap(
    (
      await api.get<Response<ResellerOverviewRow[]>>(
        '/api/user/reseller/overview'
      )
    ).data
  )
}

export async function listResellerGrants(resellerId: number, limit = 50) {
  return unwrap(
    (
      await api.get<Response<ResellerWalletGrant[]>>(
        '/api/user/reseller/grants',
        { params: { reseller_id: resellerId, limit } }
      )
    ).data
  )
}

export async function grantResellerWallet(payload: {
  reseller_id: number
  quota: number
  kind: 'recharge' | 'precredit'
  remark?: string
  event_key?: string
}) {
  return unwrap(
    (
      await api.post<Response<ResellerWalletGrant>>(
        '/api/user/reseller/grant',
        payload
      )
    ).data
  )
}

export { ROLE }

// ---------------------------------------------------------------------------
// Settlement ledger
// ---------------------------------------------------------------------------

export interface ResellerSettlementRow {
  id: number
  event_key: string
  reseller_id: number
  direction: 'in' | 'out'
  status: 'pending' | 'confirmed' | 'void'
  currency: string
  amount_minor: number
  exchange_rate: string
  quota_amount: number
  kind: 'recharge' | 'precredit'
  method: string
  external_ref: string
  proof_url: string
  remark: string
  grant_id: number
  created_by: number
  confirmed_by: number
  confirmed_at: number
  created_at: number
}

export interface ResellerSettlementTotalsRow {
  received_minor: number
  confirmed_minor: number
  pending_minor: number
  credited_quota: number
  pending_count: number
}

export interface ResellerSettlementBundle {
  items: ResellerSettlementRow[]
  totals: ResellerSettlementTotalsRow
}

export async function listResellerSettlements(resellerId: number, limit = 50) {
  return unwrap(
    (
      await api.get<Response<ResellerSettlementBundle>>(
        '/api/user/reseller/settlements',
        { params: { reseller_id: resellerId, limit } }
      )
    ).data
  )
}

export async function createResellerSettlement(payload: {
  reseller_id: number
  direction: 'in' | 'out'
  currency: string
  amount_minor: number
  exchange_rate?: string
  quota: number
  kind: 'recharge' | 'precredit'
  method?: string
  external_ref?: string
  proof_url?: string
  remark?: string
  event_key?: string
}) {
  return unwrap(
    (
      await api.post<Response<ResellerSettlementRow>>(
        '/api/user/reseller/settlements',
        payload
      )
    ).data
  )
}

export async function confirmResellerSettlement(id: number) {
  return unwrap(
    (
      await api.post<Response<ResellerSettlementRow>>(
        `/api/user/reseller/settlements/${id}/confirm`
      )
    ).data
  )
}

export async function voidResellerSettlement(id: number) {
  return unwrap(
    (
      await api.post<Response<ResellerSettlementRow>>(
        `/api/user/reseller/settlements/${id}/void`
      )
    ).data
  )
}
