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
  allocated_quota: number
  used_quota: number
  reserved_quota: number
  revenue_quota: number
  margin_quota: number
  self_pay_margin_quota: number
  pool_margin_quota: number
  last_activity_at: number
}

export interface ResellerCustomer {
  user_id: number
  username: string
  group: string
  status: number
  wallet_quota: number
  allocated_quota: number
  reserved_quota: number
  used_quota: number
  revenue_quota: number
  margin_quota: number
}

export interface ResellerUsageRow {
  id: number
  customer_id: number
  model_name: string
  group: string
  log_type: number
  occurred_at: number
  revenue_quota: number
  margin_quota: number
  settlement: string
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
