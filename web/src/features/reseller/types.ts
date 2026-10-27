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
export interface ResellerSummary {
  reseller_id: number
  wallet_quota: number
  customer_count: number
  funded_quota: number
  reclaimed_quota: number
  funded_cost_quota: number
  reclaimed_cost_quota: number
  customer_balance_quota: number
  revenue_quota: number
  margin_quota: number
  last_activity_at: number
}

export interface ResellerCustomer {
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

export interface ResellerTransfer {
  id: number
  event_key: string
  reseller_id: number
  customer_user_id: number
  direction: 'fund' | 'reclaim'
  group: string
  retail_quota: number
  cost_quota: number
  retail_ratio: string
  cost_ratio: string
  operator_id: number
  remark: string
  created_at: number
}

export interface ResellerCustomerDetail extends ResellerCustomer {
  email: string
  reclaimable_quota: number
  prices: {
    id: number
    user_id: number
    group: string
    ratio: string
    source: string
    updated_at: number
  }[]
  recent_transfers: ResellerTransfer[]
}

export interface ResellerCustomerGroup {
  group: string
  cost_ratio: string
  ratio: string
  source: string
}

export interface ResellerUsageRow {
  id: number
  customer_id: number
  username: string
  model_name: string
  group: string
  log_type: number
  occurred_at: number
  revenue_quota: number
  margin_quota: number
  request_id: string
}

export interface ResellerMarginDay {
  day: number
  revenue_quota: number
  margin_quota: number
  request_count: number
}

export interface ResellerPriceBook {
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
}

export interface ResellerLogRow {
  id: number
  user_id: number
  created_at: number
  type: number
  model_name: string
  token_name: string
  quota: number
  content: string
  duration?: number
  is_stream?: boolean
}
