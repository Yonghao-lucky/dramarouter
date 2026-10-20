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
import { useMutation } from '@tanstack/react-query'
import { useId, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Label } from '@/components/ui/label'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { handleServerError } from '@/lib/handle-server-error'

import {
  downloadUsageAudit,
  usageAuditCsv,
  usageAuditDetailsCsv,
} from '../lib/usage-audit'
import {
  downloadUsageAuditPdf,
  type UsageAuditReport,
} from '../usage-audit-api'
import { UsageAuditCostTable } from './usage-audit-cost-table'
import { UsageAuditOverview } from './usage-audit-overview'
import { UsageAuditRecalculation } from './usage-audit-recalculation'

export function UsageAuditResults(props: {
  report: UsageAuditReport
  stale: boolean
}) {
  const { t } = useTranslation()
  const [view, setView] = useState('models')
  const [page, setPage] = useState(0)
  const [showAll, setShowAll] = useState(false)
  const [selected, setSelected] = useState(props.report.models[0]?.label ?? '')
  const modelId = useId()
  const report = props.report
  const selectedModel =
    report.models.find((model) => model.label === selected) ?? report.models[0]
  let visibleModels = selectedModel ? [selectedModel] : []
  if (showAll) {
    visibleModels = report.models.slice(page * 10, (page + 1) * 10)
  }
  const symbol = report.currency.symbol
  const filename = `usage-audit-${report.sha256.slice(0, 12)}`
  const pdf = useMutation({
    mutationFn: downloadUsageAuditPdf,
    onError: handleServerError,
  })
  const pageSize = 10
  function downloadSummary() {
    const csv = usageAuditCsv([
      [
        'report_sha256',
        'dimension',
        'label',
        'records',
        'priced_records',
        'input_tokens',
        'cache_read_input_tokens',
        'cache_creation_input_tokens',
        'output_tokens',
        'platform_amount',
        'list_amount',
        'list_minus_platform',
        'currency',
        'input_repricing_tokens',
        'input_repricing_cache_read_tokens',
        'input_repricing_cache_write_tokens',
        'input_repricing_platform_amount',
        'input_repricing_base_amount',
        'input_repricing_unpriced_tokens',
        'period_start_utc',
        'period_end_exclusive_utc',
        'input_repricing_requests',
        'excluded_cache_write_requests',
        'recalculation_basis',
      ],
      ...(['days', 'models', 'users', 'keys'] as const).flatMap((dimension) =>
        report[dimension].map((row) => [
          report.sha256,
          dimension,
          row.label,
          row.count,
          row.priced_count,
          row.tokens.input_tokens,
          row.tokens.cache_read_input_tokens,
          row.tokens.cache_creation_input_tokens,
          row.tokens.output_tokens,
          row.comparison?.platform.total,
          row.comparison?.list.total,
          row.comparison?.difference.total,
          report.currency.code,
          row.input_recalculation.tokens,
          row.input_recalculation.cache_read_input_tokens,
          row.input_recalculation.cache_creation_input_tokens,
          row.input_recalculation.platform_amount,
          row.input_recalculation.list_amount,
          row.input_recalculation.unpriced_tokens,
          new Date(report.filter.start_at * 1000).toISOString(),
          new Date(report.filter.end_at * 1000).toISOString(),
          row.input_recalculation.request_count,
          row.input_recalculation.excluded_cache_write_request_count,
          report.recalculation_basis,
        ])
      ),
    ])
    downloadUsageAudit(csv, `${filename}-summary.csv`, 'text/csv;charset=utf-8')
  }
  return (
    <div className='flex min-w-0 flex-col gap-5'>
      {props.stale && (
        <Alert>
          <AlertDescription>
            {t('Filters changed. Run the audit again before exporting.')}
          </AlertDescription>
        </Alert>
      )}
      <div className='flex flex-wrap items-center justify-between gap-3'>
        <div>
          <h3 className='text-lg font-semibold'>
            {t('Token usage and cost comparison')}
          </h3>
          <p className='text-muted-foreground text-sm'>
            {new Date((report.filter.start_at + 28800) * 1000)
              .toISOString()
              .slice(0, 19)
              .replace('T', ' ')}{' '}
            -{' '}
            {new Date((report.filter.end_at + 28800) * 1000)
              .toISOString()
              .slice(0, 19)
              .replace('T', ' ')}{' '}
            · Asia/Shanghai · {report.currency.code}
          </p>
        </div>
        <div className='flex flex-wrap gap-2'>
          <Button
            disabled={props.stale || pdf.isPending}
            onClick={() => pdf.mutate(report)}
          >
            {pdf.isPending ? t('Generating PDF…') : t('Export PDF')}
          </Button>
          <Button
            variant='outline'
            disabled={props.stale}
            onClick={downloadSummary}
          >
            {t('Export summary CSV')}
          </Button>
          <Button
            variant='outline'
            disabled={props.stale}
            onClick={() =>
              downloadUsageAudit(
                usageAuditDetailsCsv(report),
                `${filename}-details.csv`,
                'text/csv;charset=utf-8'
              )
            }
          >
            {t('Export details CSV')}
          </Button>
          <Button
            variant='outline'
            disabled={props.stale}
            onClick={() =>
              downloadUsageAudit(
                JSON.stringify(report, null, 2),
                `${filename}.json`,
                'application/json'
              )
            }
          >
            {t('Export JSON')}
          </Button>
        </div>
      </div>
      {pdf.isError && (
        <Alert variant='destructive'>
          <AlertDescription>
            {t('PDF export failed. Refresh the audit and retry.')}
          </AlertDescription>
        </Alert>
      )}
      <Tabs
        value={view}
        onValueChange={(value) => {
          setView(String(value))
          setPage(0)
        }}
      >
        <TabsList>
          <TabsTrigger value='models'>{t('By model')}</TabsTrigger>
          <TabsTrigger value='all'>{t('All models total')}</TabsTrigger>
        </TabsList>
        <TabsContent value='models' className='flex flex-col gap-4'>
          <div className='flex flex-wrap items-end gap-3'>
            <div className='min-w-0 flex-1 space-y-2 sm:max-w-sm'>
              <Label htmlFor={modelId}>{t('View model')}</Label>
              <NativeSelect
                id={modelId}
                value={selected}
                onChange={(event) => {
                  setSelected(event.target.value)
                  setShowAll(false)
                  setPage(0)
                }}
              >
                {report.models.map((model) => (
                  <NativeSelectOption key={model.label} value={model.label}>
                    {model.label}
                  </NativeSelectOption>
                ))}
              </NativeSelect>
            </div>
            <Button
              variant='outline'
              aria-pressed={showAll}
              disabled={report.models.length === 0}
              onClick={() => {
                setShowAll((value) => !value)
                setPage(0)
              }}
            >
              {showAll ? t('Collapse all models') : t('Show all models')}
            </Button>
          </div>
          {visibleModels.map((model) => (
            <Card key={model.label} className='min-w-0'>
              <CardHeader>
                <CardTitle className='break-all'>{model.label}</CardTitle>
                <CardDescription>
                  {t(
                    '{{priced}} of {{total}} records have comparable prices.',
                    {
                      priced: model.priced_count,
                      total: model.count,
                    }
                  )}
                </CardDescription>
                <div className='flex flex-wrap gap-2'>
                  {(report.rates ?? [])
                    .filter((rate) => rate.model === model.label)
                    .map((rate) => (
                      <Badge
                        key={`${rate.group}-${rate.ratio}`}
                        variant='secondary'
                      >
                        {t('Billing multiplier')}: {rate.group || '—'} ×{' '}
                        {rate.ratio}
                      </Badge>
                    ))}
                </div>
              </CardHeader>
              <CardContent className='flex flex-col gap-5'>
                <UsageAuditOverview summary={model} symbol={symbol} />
                <UsageAuditCostTable
                  summary={model}
                  symbol={symbol}
                  label={model.label}
                />
              </CardContent>
            </Card>
          ))}
          {report.models.length === 0 && (
            <p className='text-muted-foreground py-6 text-center'>
              {t('No matching records')}
            </p>
          )}
          {showAll && report.models.length > pageSize && (
            <div className='flex items-center justify-between'>
              <Button
                variant='outline'
                disabled={page === 0}
                onClick={() => setPage((value) => value - 1)}
              >
                {t('Previous')}
              </Button>
              <span>
                {page + 1} / {Math.ceil(report.models.length / pageSize)}
              </span>
              <Button
                variant='outline'
                disabled={(page + 1) * pageSize >= report.models.length}
                onClick={() => setPage((value) => value + 1)}
              >
                {t('Next')}
              </Button>
            </div>
          )}
        </TabsContent>
        <TabsContent value='all'>
          <Card>
            <CardHeader>
              <CardTitle>{t('All models total')}</CardTitle>
              <CardDescription>
                {t(
                  'Combined token usage and category costs for all models in the selected period.'
                )}
              </CardDescription>
            </CardHeader>
            <CardContent className='flex flex-col gap-5'>
              <UsageAuditOverview summary={report.summary} symbol={symbol} />
              <UsageAuditCostTable
                summary={report.summary}
                symbol={symbol}
                label={t('All models total')}
              />
            </CardContent>
          </Card>
        </TabsContent>
      </Tabs>
      <UsageAuditRecalculation
        value={
          showAll || view === 'all'
            ? report.summary.input_recalculation
            : (selectedModel?.input_recalculation ??
              report.summary.input_recalculation)
        }
        symbol={symbol}
      />
      <p className='text-muted-foreground text-xs'>
        {t(
          'Platform cost includes the historical group multiplier. List price uses the historical model base price. Difference = list price cost minus platform cost; it is not a refund amount.'
        )}
      </p>
      <p className='text-muted-foreground text-xs'>
        {t(
          'Priced {{priced}} of {{total}} records; {{unknown}} records lack reliable token evidence.',
          {
            priced: report.summary.priced_count,
            total: report.summary.count,
            unknown: report.summary.unknown_usage_count,
          }
        )}
      </p>
    </div>
  )
}
