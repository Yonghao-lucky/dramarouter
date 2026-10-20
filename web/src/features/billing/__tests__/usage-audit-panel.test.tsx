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
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, test, vi } from 'vitest'

import { UsageAuditPanel } from '../components/usage-audit-panel'
import {
  queryUsageAudit,
  downloadUsageAuditPdf,
  searchUsageAuditKeys,
  type UsageAuditReport,
  type UsageAuditCost,
} from '../usage-audit-api'

vi.mock('../usage-audit-api', () => ({
  queryUsageAudit: vi.fn(),
  searchUsageAuditKeys: vi.fn(),
  downloadUsageAuditPdf: vi.fn(),
}))
const cost: UsageAuditCost = {
  input: '0.000200000',
  cache_read: '0.000160000',
  cache_write: '0.000250000',
  output: '0.000200000',
  rounding: '0',
  total: '0.000810000',
}
const tokens = {
  input_tokens: 100,
  cache_read_input_tokens: 800,
  cache_creation_input_tokens: 100,
  output_tokens: 20,
  total_input_tokens: 1000,
}
const summary = {
  count: 1,
  priced_count: 1,
  unknown_usage_count: 0,
  duplicate_review_count: 1,
  tokens,
  possible_duplicate_input_tokens: 900,
  input_recalculation: {
    request_count: 1,
    excluded_cache_write_request_count: 1,
    tokens: 800,
    cache_read_input_tokens: 800,
    cache_creation_input_tokens: 0,
    unpriced_tokens: 0,
    platform_amount: '0.001600000',
    list_amount: '0.003200000',
  },
  actual_amount: '0.000810000',
  priced_actual_amount: '0.000810000',
  list_cost: { ...cost, total: '0.001620000' },
  discounted_cost: cost,
  recalculation_difference: '0',
  comparison: {
    platform: cost,
    list: {
      ...cost,
      input: '0.000400000',
      cache_read: '0.000320000',
      cache_write: '0.000500000',
      output: '0.000400000',
      total: '0.001620000',
    },
    difference: cost,
  },
}
const report: UsageAuditReport = {
  recalculation_basis: 'legacy_openai_cache_write_tokens',
  filter: {
    user_id: 8,
    token_id: 12,
    channel_id: 0,
    start_at: 1790784000,
    end_at: 1790870400,
    model: '',
    group: '',
    protocol: '',
  },
  generated_at: 1790870400,
  timezone: 'Asia/Shanghai',
  price_source: 'historical_model_base_without_group_ratio',
  currency: { code: 'USD', symbol: '$', rate: '1', quota_per_unit: '500000' },
  sha256: 'abc123',
  summary,
  models: [{ label: 'kimi-k3', ...summary }],
  days: [],
  users: [],
  keys: [],
  rows: [
    {
      id: 1,
      input_recalculation: summary.input_recalculation,
      user_id: 8,
      username: 'customer',
      token_id: 12,
      token_name: 'production',
      created_at: 1790784000,
      request_id: 'request-one',
      upstream_request_id: '',
      model: 'kimi-k3',
      group: 'kimi',
      channel_id: 40,
      is_stream: false,
      kind: 'consumption',
      protocol: 'anthropic',
      usage_semantic: 'openai',
      group_ratio: 0.5,
      tokens,
      actual_amount: '0.000810000',
      list_prices_per_million: {
        ...cost,
        input: '4',
        cache_read: '0.4',
        cache_write: '5',
        output: '20',
      },
      list_cost: summary.list_cost,
      discounted_cost: cost,
      recalculation_difference: '0',
      possible_duplicate_input_tokens: 900,
      duplicate_status: 'needs_response_evidence',
      issue: '',
    },
  ],
  rates: [{ model: 'kimi-k3', group: 'kimi', ratio: '0.5' }],
}
function renderPanel() {
  return render(
    <QueryClientProvider
      client={
        new QueryClient({
          defaultOptions: {
            queries: { retry: false },
            mutations: { retry: false },
          },
        })
      }
    >
      <UsageAuditPanel userId={8} />
    </QueryClientProvider>
  )
}
function selectPeriod() {
  fireEvent.change(screen.getByLabelText('Start (Asia/Shanghai)'), {
    target: { value: '2026-10-01T00:00' },
  })
  fireEvent.change(screen.getByLabelText('End, exclusive (Asia/Shanghai)'), {
    target: { value: '2026-10-02T00:00' },
  })
}
describe('Administrator usage audit', () => {
  beforeEach(() => {
    vi.resetAllMocks()
    vi.mocked(searchUsageAuditKeys).mockResolvedValue([
      { id: 12, user_id: 8, name: 'production' },
    ])
    vi.mocked(queryUsageAudit).mockResolvedValue(report)
  })
  test('submits selected customer and key, displays both prices, and prevents stale exports', async () => {
    const user = userEvent.setup()
    renderPanel()
    selectPeriod()
    await screen.findByRole('option', { name: /production/ })
    await user.selectOptions(screen.getByLabelText('API key'), '12')
    await user.click(screen.getByRole('button', { name: 'Run usage audit' }))
    await waitFor(() =>
      expect(queryUsageAudit).toHaveBeenCalledWith(
        report.filter,
        expect.anything()
      )
    )
    const correction = await screen.findByLabelText('Cache read repricing')
    expect(within(correction).getByText('800')).toBeVisible()
    expect(screen.getByRole('group', { name: 'Cost comparison' })).toBeVisible()
    expect(screen.getAllByText('$ 0.001620').length).toBeGreaterThan(0)
    expect(
      screen.getByRole('button', { name: 'Export details CSV' })
    ).toBeEnabled()
    const metrics = screen.getByRole('group', { name: 'Cost comparison' })
    for (const label of [
      'Platform cost',
      'List price cost',
      'Price difference',
    ]) {
      expect(within(metrics).getByText(label)).toBeVisible()
    }
    expect(screen.queryByText('Actual recorded amount')).not.toBeInTheDocument()
    expect(screen.queryByLabelText('Group by')).not.toBeInTheDocument()
    const modelTable = screen.getByRole('table', { name: 'kimi-k3' })
    const cacheToggle = within(modelTable).getByRole('button', {
      name: 'Cache',
    })
    expect(cacheToggle).toHaveAttribute('aria-expanded', 'false')
    expect(
      within(modelTable).queryByRole('row', { name: /Cache read/ })
    ).not.toBeInTheDocument()
    expect(
      within(modelTable).getByRole('row', { name: /Cache/ })
    ).toHaveTextContent('$ 0.000410')
    await user.click(cacheToggle)
    expect(cacheToggle).toHaveAttribute('aria-expanded', 'true')
    expect(
      within(modelTable).getByRole('row', { name: /Cache read/ })
    ).toHaveTextContent('800')
    await user.click(screen.getByRole('tab', { name: 'All models total' }))
    expect(
      screen.getByRole('table', { name: 'All models total' })
    ).toBeVisible()
    await user.click(screen.getByRole('button', { name: 'Export PDF' }))
    await waitFor(() =>
      expect(downloadUsageAuditPdf).toHaveBeenCalledWith(
        report,
        expect.anything()
      )
    )
    fireEvent.change(screen.getByLabelText('Model'), {
      target: { value: 'other-model' },
    })
    expect(
      screen.getByRole('button', { name: 'Export details CSV' })
    ).toBeDisabled()
    expect(screen.getByRole('button', { name: 'Export PDF' })).toBeDisabled()
    expect(
      screen.getByText('Filters changed. Run the audit again before exporting.')
    ).toBeVisible()
  })
  test('all-customer scope clears the selected key and failed queries show an error', async () => {
    const user = userEvent.setup()
    vi.mocked(queryUsageAudit).mockRejectedValue(new Error('request failed'))
    renderPanel()
    selectPeriod()
    fireEvent.change(screen.getByLabelText('Key ID (including deleted keys)'), {
      target: { value: '12' },
    })
    await user.selectOptions(screen.getByLabelText('Search scope'), 'all')
    expect(
      screen.getByLabelText('Key ID (including deleted keys)')
    ).toHaveValue(0)
    await user.click(screen.getByRole('button', { name: 'Run usage audit' }))
    expect(
      await screen.findByText(
        /Usage audit failed. Narrow the filters or retry./
      )
    ).toBeVisible()
    expect(
      screen.queryByRole('button', { name: 'Export details CSV' })
    ).not.toBeInTheDocument()
    expect(vi.mocked(queryUsageAudit).mock.calls[0][0].user_id).toBe(0)
  })
  test('keeps model categories separate and only combines them in the all-model view', async () => {
    const user = userEvent.setup()
    const second = {
      label: 'claude-test',
      ...summary,
      input_recalculation: {
        ...summary.input_recalculation,
        tokens: 1600,
        cache_read_input_tokens: 1600,
        platform_amount: '0.003200000',
        list_amount: '0.006400000',
      },
      tokens: {
        ...tokens,
        cache_read_input_tokens: 1600,
        total_input_tokens: 1800,
      },
    }
    vi.mocked(queryUsageAudit).mockResolvedValue({
      ...report,
      models: [...report.models, second],
      rows: [
        ...report.rows,
        {
          ...report.rows[0],
          id: 2,
          input_recalculation: second.input_recalculation,
          model: 'claude-test',
          tokens: second.tokens,
          possible_duplicate_input_tokens: 1600,
        },
      ],
      summary: {
        ...summary,
        count: 2,
        priced_count: 2,
        input_recalculation: {
          ...summary.input_recalculation,
          tokens: 2400,
          request_count: 2,
          excluded_cache_write_request_count: 2,
          cache_read_input_tokens: 2400,
          cache_creation_input_tokens: 0,
          platform_amount: '0.004800000',
          list_amount: '0.009600000',
        },
        tokens: {
          input_tokens: 200,
          cache_read_input_tokens: 2400,
          cache_creation_input_tokens: 200,
          output_tokens: 40,
          total_input_tokens: 2800,
        },
        comparison: {
          platform: {
            ...cost,
            input: '0.0004',
            cache_read: '0.00032',
            cache_write: '0.0005',
            output: '0.0004',
            total: '0.00162',
          },
          list: {
            ...cost,
            input: '0.0008',
            cache_read: '0.00064',
            cache_write: '0.001',
            output: '0.0008',
            total: '0.00324',
          },
          difference: {
            ...cost,
            input: '0.0004',
            cache_read: '0.00032',
            cache_write: '0.0005',
            output: '0.0004',
            total: '0.00162',
          },
        },
      },
    })
    renderPanel()
    selectPeriod()
    await user.click(screen.getByRole('button', { name: 'Run usage audit' }))
    const firstTable = await screen.findByRole('table', { name: 'kimi-k3' })
    await user.click(within(firstTable).getByRole('button', { name: 'Cache' }))
    expect(
      screen.queryByRole('table', { name: 'claude-test' })
    ).not.toBeInTheDocument()
    const tokenOverview = screen.getByRole('group', { name: 'Token usage' })
    expect(within(tokenOverview).getByText(/800/)).toBeVisible()
    expect(within(tokenOverview).getByText('900')).toBeVisible()
    expect(within(tokenOverview).getAllByRole('term')).toHaveLength(3)
    expect(
      within(firstTable).getByRole('row', { name: /Cache read/ })
    ).toHaveTextContent('800')
    await user.selectOptions(screen.getByLabelText('View model'), 'claude-test')
    expect(
      screen.queryByRole('table', { name: 'kimi-k3' })
    ).not.toBeInTheDocument()
    const secondTable = screen.getByRole('table', { name: 'claude-test' })
    await user.click(within(secondTable).getByRole('button', { name: 'Cache' }))
    expect(
      within(screen.getByRole('group', { name: 'Token usage' })).getByText(
        /1,600/
      )
    ).toBeVisible()
    expect(
      within(secondTable).getByRole('row', { name: /Cache read/ })
    ).toHaveTextContent('1,600')
    expect(
      within(screen.getByLabelText('Cache read repricing')).getByText('1,600')
    ).toBeVisible()
    await user.click(screen.getByRole('button', { name: 'Show all models' }))
    expect(screen.getByRole('table', { name: 'kimi-k3' })).toBeVisible()
    expect(screen.getByRole('table', { name: 'claude-test' })).toBeVisible()
    await user.click(
      screen.getByRole('button', { name: 'Collapse all models' })
    )
    expect(
      screen.queryByRole('table', { name: 'kimi-k3' })
    ).not.toBeInTheDocument()
    expect(screen.getByRole('table', { name: 'claude-test' })).toBeVisible()
    await user.click(screen.getByRole('tab', { name: 'All models total' }))
    expect(
      within(screen.getByLabelText('Cache read repricing')).getByText('2,400')
    ).toBeVisible()
    const totalTable = screen.getByRole('table', {
      name: 'All models total',
    })
    await user.click(within(totalTable).getByRole('button', { name: 'Cache' }))
    expect(
      within(totalTable).getByRole('row', { name: /Cache read/ })
    ).toHaveTextContent('2,400')

    expect(
      within(screen.getByRole('group', { name: 'Token usage' })).getByText(
        /2,400/
      )
    ).toBeVisible()
    expect(
      screen.getByRole('table', { name: 'All models total' })
    ).toBeVisible()
  })
  test('shows PDF export failure and leaves the result available to retry', async () => {
    const user = userEvent.setup()
    vi.mocked(downloadUsageAuditPdf).mockRejectedValue(
      new Error('report changed')
    )
    renderPanel()
    selectPeriod()
    await user.click(screen.getByRole('button', { name: 'Run usage audit' }))
    await user.click(await screen.findByRole('button', { name: 'Export PDF' }))
    expect(
      await screen.findByText('PDF export failed. Refresh the audit and retry.')
    ).toBeVisible()
    expect(screen.getByRole('button', { name: 'Export PDF' })).toBeEnabled()
    expect(screen.getByRole('table', { name: 'kimi-k3' })).toBeVisible()
  })
  test('shows tokens and historical input valuations automatically without downstream price fields', async () => {
    const user = userEvent.setup()
    renderPanel()
    selectPeriod()
    await user.click(screen.getByRole('button', { name: 'Run usage audit' }))
    const repricing = await screen.findByLabelText('Cache read repricing')
    expect(within(repricing).getByText('800')).toBeVisible()
    expect(
      within(repricing).getByText(
        'Extra input tokens = Σ(old input_tokens − correct uncached input) = Σ(cache read tokens) = 800 tokens.'
      )
    ).toBeVisible()
    expect(within(repricing).getByText('Requests to reprice')).toBeVisible()
    expect(
      within(repricing).getByText('Excluded requests with cache writes')
    ).toBeVisible()
    expect(
      within(repricing).queryByText('Cache write counted again as input')
    ).not.toBeInTheDocument()
    expect(within(repricing).getByText('$ 0.001600')).toBeVisible()
    expect(within(repricing).getByText('$ 0.003200')).toBeVisible()
    expect(within(repricing).queryByRole('textbox')).not.toBeInTheDocument()
    expect(within(repricing).queryByRole('combobox')).not.toBeInTheDocument()
    expect(screen.queryByText(/Enter downstream/)).not.toBeInTheDocument()
  })
  test('retains repricing tokens when historical input prices are missing', async () => {
    const unpriced = {
      ...summary,
      input_recalculation: {
        ...summary.input_recalculation,
        unpriced_tokens: 800,
        platform_amount: '0.000000000',
        list_amount: '0.000000000',
      },
    }
    vi.mocked(queryUsageAudit).mockResolvedValue({
      ...report,
      summary: unpriced,
      models: [{ label: 'kimi-k3', ...unpriced }],
    })
    const user = userEvent.setup()
    renderPanel()
    selectPeriod()
    await user.click(screen.getByRole('button', { name: 'Run usage audit' }))
    const repricing = await screen.findByLabelText('Cache read repricing')
    expect(within(repricing).getByText('800')).toBeVisible()
    expect(
      within(repricing).getAllByText('Historical input price unavailable')
    ).toHaveLength(2)
    expect(
      within(repricing).getByText('800 tokens have no historical input price.')
    ).toBeVisible()
  })
})
