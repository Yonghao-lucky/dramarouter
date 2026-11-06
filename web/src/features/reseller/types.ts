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
  /** Earned but not yet moved into the wallet. Crediting is manual. */
  pending_margin_quota: number
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
  /** Provider that dominates the group's models; the price book groups by it. */
  vendor?: string
  vendor_icon?: string
  /** Customer-facing description, kept in sync with the admin group settings. */
  description?: string
  cost_ratio: string
  ratio: string
  source: string
  platform_ratio: string
  sellable: boolean
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

/** One settlement row: money between the platform and this reseller. */
export interface ResellerSettlement {
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
  voided_at: number
  created_at: number
}

export interface ResellerSettlementTotals {
  received_minor: number
  confirmed_minor: number
  pending_minor: number
  credited_quota: number
  pending_count: number
}

/** A platform top-up credited to this reseller wallet. */
export interface ResellerWalletGrantRow {
  id: number
  event_key: string
  reseller_id: number
  amount: number
  kind: 'recharge' | 'precredit'
  remark: string
  operator_id: number
  created_at: number
}

export interface ResellerGrantBundle {
  items: ResellerWalletGrantRow[]
  totals: ResellerSettlementTotals
}

export interface ResellerDashboardTopCustomer {
  user_id: number
  username: string
  display_name: string
  revenue_quota: number
  request_count: number
  balance_quota: number
}

/** One hour of a customer's model usage, matching the platform analytics rows. */
export interface ResellerQuotaDataRow {
  user_id?: number
  username?: string
  model_name?: string
  created_at: number
  token_used?: number
  count?: number
  quota?: number
}

/** One owned customer's standing figures in the dashboard window. */
export interface ResellerDashboardCustomerRow {
  user_id: number
  username: string
  display_name: string
  group: string
  balance_quota: number
  used_quota: number
  revenue_quota: number
  request_count: number
  last_active_at: number
}

export interface ResellerDashboardModelRow {
  model_name: string
  revenue_quota: number
  request_count: number
  customer_count: number
}

/** Owned-customer analytics. Every figure is scoped to the caller. */
export interface ResellerDashboard {
  customer_count: number
  active_count: number
  total_balance_quota: number
  revenue_quota: number
  request_count: number
  used_quota: number
  average_per_request: number
  window_days: number
  top_customers: ResellerDashboardTopCustomer[]
  by_model: ResellerDashboardModelRow[]
  customers: ResellerDashboardCustomerRow[]
  by_day: ResellerMarginDay[]
}

export interface ResellerAuditRow {
  id: number
  operator_id: number
  username: string
  action: string
  detail: string
  ip: string
  created_at: number
}

export interface ResellerTemplateItem {
  id: number
  template_id: number
  group: string
  ratio: string
}

export interface ResellerPriceTemplate {
  id: number
  reseller_id: number
  name: string
  remark: string
  created_by: number
  created_at: number
  updated_at: number
}

export interface ResellerTemplateBundle {
  templates: ResellerPriceTemplate[]
  items: Record<string, ResellerTemplateItem[]>
}

export interface ResellerQuotaAlert {
  id: number
  reseller_id: number
  enabled: boolean
  threshold_quota: number
  notify_email: string
  cooldown_hours: number
  updated_at: number
}
