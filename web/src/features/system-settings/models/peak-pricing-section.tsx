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
import { Plus, Trash2 } from 'lucide-react'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'
import { SettingsSection } from '../components/settings-section'
import { useUpdateOption } from '../hooks/use-update-option'

type PeakPricingRule = {
  name: string
  days: number[]
  start: string
  end: string
  input_factor: string
  output_factor: string
  cache_factor: string
  cache_write_factor: string
  cache_write_5m_factor: string
  cache_write_1h_factor: string
}

type PeakPricingModelOverride = {
  model: string
  rules: PeakPricingRule[]
  off_peak: PeakPricingRule
}

type PeakPricingSetting = {
  enabled: boolean
  timezone: string
  models: string[]
  rules: PeakPricingRule[]
  off_peak: PeakPricingRule
  model_overrides: PeakPricingModelOverride[]
}

const EMPTY_RULE: PeakPricingRule = {
  name: '',
  days: [],
  start: '00:00',
  end: '00:00',
  input_factor: '1',
  output_factor: '1',
  cache_factor: '1',
  cache_write_factor: '1',
  cache_write_5m_factor: '',
  cache_write_1h_factor: '',
}

const FACTOR_FIELDS = [
  ['input_factor', 'Input factor'],
  ['output_factor', 'Output factor'],
  ['cache_factor', 'Cache read factor'],
  ['cache_write_factor', 'Cache write factor'],
  ['cache_write_5m_factor', 'Cache write 5m factor'],
  ['cache_write_1h_factor', 'Cache write 1h factor'],
] as const

// 常用作息预设，避免管理员逐个个点星期。
const DAY_PRESETS = [
  { label: 'Every day', days: [] as number[] },
  { label: 'Weekdays', days: [1, 2, 3, 4, 5] },
  { label: 'Weekends', days: [0, 6] },
]

const WEEKDAYS = [
  { value: 0, label: 'Sun' },
  { value: 1, label: 'Mon' },
  { value: 2, label: 'Tue' },
  { value: 3, label: 'Wed' },
  { value: 4, label: 'Thu' },
  { value: 5, label: 'Fri' },
  { value: 6, label: 'Sat' },
]

function normalizeRule(
  rule: Partial<PeakPricingRule> | undefined
): PeakPricingRule {
  return { ...EMPTY_RULE, ...(rule ?? {}) }
}

function normalizeOverride(
  item: Partial<PeakPricingModelOverride> | undefined
): PeakPricingModelOverride {
  return {
    model: item?.model ?? '',
    rules: Array.isArray(item?.rules) ? item.rules.map(normalizeRule) : [],
    off_peak: normalizeRule(item?.off_peak),
  }
}

function parseSetting(raw: string | undefined): PeakPricingSetting {
  const fallback: PeakPricingSetting = {
    enabled: false,
    timezone: 'Asia/Shanghai',
    models: [],
    rules: [],
    off_peak: { ...EMPTY_RULE, name: 'Off-peak' },
    model_overrides: [],
  }
  if (!raw) return fallback
  try {
    const parsed = JSON.parse(raw) as Partial<PeakPricingSetting>
    return {
      enabled: parsed.enabled ?? false,
      timezone: parsed.timezone || 'Asia/Shanghai',
      models: Array.isArray(parsed.models) ? parsed.models : [],
      rules: Array.isArray(parsed.rules) ? parsed.rules.map(normalizeRule) : [],
      off_peak: normalizeRule(parsed.off_peak),
      model_overrides: Array.isArray(parsed.model_overrides)
        ? parsed.model_overrides.map(normalizeOverride)
        : [],
    }
  } catch {
    return fallback
  }
}

type RuleEditorProps = {
  rules: PeakPricingRule[]
  onChange: (rules: PeakPricingRule[]) => void
  idPrefix: string
}

function RuleEditor({ rules, onChange, idPrefix }: RuleEditorProps) {
  const { t } = useTranslation()

  const patchRule = (index: number, patch: Partial<PeakPricingRule>) => {
    onChange(rules.map((rule, i) => (i === index ? { ...rule, ...patch } : rule)))
  }

  return (
    <div className='flex flex-col gap-3'>
      <div className='flex items-center justify-between'>
        <div>
          <p className='text-sm font-medium'>{t('Peak windows')}</p>
          <p className='text-muted-foreground text-xs'>
            {t(
              'A request matching no window uses the off-peak factors. End earlier than start means it crosses midnight.'
            )}
          </p>
        </div>
        <Button
          type='button'
          variant='outline'
          size='sm'
          onClick={() =>
            onChange([
              ...rules,
              { ...EMPTY_RULE, name: `Peak ${rules.length + 1}` },
            ])
          }
        >
          <Plus data-icon='inline-start' />
          {t('Add window')}
        </Button>
      </div>

      {rules.length === 0 && (
        <p className='text-muted-foreground rounded-lg border border-dashed p-4 text-xs'>
          {t('No peak windows. All requests use the off-peak factors.')}
        </p>
      )}

      {rules.map((rule, index) => (
        <div key={`${idPrefix}-${index}`} className='rounded-xl border p-4'>
          <div className='flex flex-wrap items-end gap-3'>
            <div className='flex min-w-32 flex-1 flex-col gap-1'>
              <Label className='text-xs'>{t('Name')}</Label>
              <Input
                value={rule.name}
                onChange={(event) =>
                  patchRule(index, { name: event.target.value })
                }
              />
            </div>
            <div className='flex flex-col gap-1'>
              <Label className='text-xs'>{t('Start')}</Label>
              <Input
                type='time'
                value={rule.start}
                onChange={(event) =>
                  patchRule(index, { start: event.target.value })
                }
                className='w-32'
              />
            </div>
            <div className='flex flex-col gap-1'>
              <Label className='text-xs'>{t('End')}</Label>
              <Input
                type='time'
                value={rule.end}
                onChange={(event) =>
                  patchRule(index, { end: event.target.value })
                }
                className='w-32'
              />
            </div>
            <Button
              type='button'
              variant='ghost'
              size='icon-sm'
              aria-label={t('Remove window')}
              onClick={() => onChange(rules.filter((_, i) => i !== index))}
            >
              <Trash2 className='size-4' />
            </Button>
          </div>

          <div className='mt-3 flex flex-wrap items-center gap-1.5'>
            {DAY_PRESETS.map((preset) => {
              const active =
                preset.days.length === 0
                  ? rule.days.length === 0
                  : preset.days.length === rule.days.length &&
                    preset.days.every((day) => rule.days.includes(day))
              return (
                <Button
                  key={preset.label}
                  type='button'
                  size='sm'
                  variant={active ? 'secondary' : 'ghost'}
                  className='h-7 px-2 text-xs'
                  onClick={() => patchRule(index, { days: preset.days })}
                >
                  {t(preset.label)}
                </Button>
              )
            })}
            <span className='bg-border mx-1 h-4 w-px' aria-hidden='true' />
            {WEEKDAYS.map((day) => {
              const active = rule.days.includes(day.value)
              return (
                <Button
                  key={day.value}
                  type='button'
                  size='sm'
                  variant={active ? 'default' : 'outline'}
                  className='h-7 px-2 text-xs'
                  onClick={() =>
                    patchRule(index, {
                      days: active
                        ? rule.days.filter((value) => value !== day.value)
                        : [...rule.days, day.value].sort(),
                    })
                  }
                >
                  {t(day.label)}
                </Button>
              )
            })}
            <span className='text-muted-foreground self-center text-[11px]'>
              {rule.days.length === 0
                ? t('Every day')
                : t('{{count}} days selected', { count: rule.days.length })}
            </span>
          </div>

          <div className='mt-3 grid gap-3 sm:grid-cols-2 lg:grid-cols-4'>
            {FACTOR_FIELDS.map(([field, label]) => (
              <div key={field} className='flex flex-col gap-1'>
                <Label className='text-xs'>{t(label)}</Label>
                <Input
                  value={rule[field]}
                  onChange={(event) =>
                    patchRule(index, { [field]: event.target.value })
                  }
                  className='font-mono text-xs'
                  placeholder='1.2'
                />
              </div>
            ))}
          </div>
        </div>
      ))}
    </div>
  )
}

type OffPeakEditorProps = {
  rule: PeakPricingRule
  onChange: (rule: PeakPricingRule) => void
}

function OffPeakEditor({ rule, onChange }: OffPeakEditorProps) {
  const { t } = useTranslation()
  return (
    <div className='rounded-xl border p-4'>
      <p className='text-sm font-medium'>{t('Off-peak factors')}</p>
      <p className='text-muted-foreground mt-0.5 text-xs'>
        {t('Applied when the request matches no peak window.')}
      </p>
      <div className='mt-3 grid gap-3 sm:grid-cols-2 lg:grid-cols-4'>
        {FACTOR_FIELDS.map(([field, label]) => (
          <div key={field} className='flex flex-col gap-1'>
            <Label className='text-xs'>{t(label)}</Label>
            <Input
              value={rule[field]}
              onChange={(event) =>
                onChange({ ...rule, [field]: event.target.value })
              }
              className='font-mono text-xs'
            />
          </div>
        ))}
      </div>
    </div>
  )
}

type PeakPricingSectionProps = {
  defaultValue?: string
}

/**
 * 峰谷定价配置：全局规则作为默认策略，按模型覆盖可让不同模型有各自的作息与谷时系数。
 * 系数可写数字或表达式（可用 hour / weekday / utc_hour 等变量）。
 */
export function PeakPricingSection({ defaultValue }: PeakPricingSectionProps) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()
  const initial = useMemo(() => parseSetting(defaultValue), [defaultValue])
  const [setting, setSetting] = useState<PeakPricingSetting>(initial)
  const [modelsText, setModelsText] = useState(initial.models.join('\n'))

  const patchOverride = (
    index: number,
    patch: Partial<PeakPricingModelOverride>
  ) => {
    setSetting((prev) => ({
      ...prev,
      model_overrides: prev.model_overrides.map((item, i) =>
        i === index ? { ...item, ...patch } : item
      ),
    }))
  }

  const save = async () => {
    const payload: PeakPricingSetting = {
      ...setting,
      models: modelsText
        .split(/[\n,]/)
        .map((item) => item.trim())
        .filter(Boolean),
      model_overrides: setting.model_overrides.filter((item) =>
        item.model.trim()
      ),
    }
    await updateOption.mutateAsync({
      key: 'PeakPricing',
      value: JSON.stringify(payload),
    })
  }

  return (
    <SettingsSection title={t('Peak & Off-peak Pricing')}>
      <div className='flex flex-col gap-5'>
        <div className='rounded-xl border p-4'>
          <div className='flex items-start justify-between gap-4'>
            <div className='min-w-0'>
              <p className='text-sm font-medium'>
                {t('Enable peak & off-peak pricing')}
              </p>
              <p className='text-muted-foreground mt-0.5 text-xs leading-relaxed'>
                {t(
                  'Input, output and cache prices are multiplied by separate factors depending on when the request arrives. The factors are locked when the request starts.'
                )}
              </p>
            </div>
            <Switch
              checked={setting.enabled}
              onCheckedChange={(enabled) =>
                setSetting((prev) => ({ ...prev, enabled }))
              }
            />
          </div>

          <div className='mt-4 grid gap-4 sm:grid-cols-2'>
            <div className='flex flex-col gap-1.5'>
              <Label className='text-xs'>{t('Timezone')}</Label>
              <Select
                value={setting.timezone}
                onValueChange={(timezone) =>
                  setSetting((prev) => ({ ...prev, timezone }))
                }
              >
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value='Asia/Shanghai'>
                    {t('Beijing Time (UTC+8)')}
                  </SelectItem>
                  <SelectItem value='UTC'>{t('UTC')}</SelectItem>
                </SelectContent>
              </Select>
            </div>
            <div className='flex flex-col gap-1.5'>
              <Label className='text-xs'>{t('Models (one per line)')}</Label>
              <Textarea
                value={modelsText}
                onChange={(event) => setModelsText(event.target.value)}
                placeholder={t(
                  'Leave empty to apply to all models, e.g. claude-*'
                )}
                className='min-h-16 font-mono text-xs'
              />
              <p className='text-muted-foreground text-[11px]'>
                {t(
                  'These models follow the global windows below unless they have their own override.'
                )}
              </p>
            </div>
          </div>
        </div>

        <RuleEditor
          idPrefix='global'
          rules={setting.rules}
          onChange={(rules) => setSetting((prev) => ({ ...prev, rules }))}
        />

        <OffPeakEditor
          rule={setting.off_peak}
          onChange={(off_peak) => setSetting((prev) => ({ ...prev, off_peak }))}
        />

        <div className='flex flex-col gap-3'>
          <div className='flex items-center justify-between'>
            <div>
              <p className='text-sm font-medium'>{t('Per-model overrides')}</p>
              <p className='text-muted-foreground text-xs'>
                {t(
                  'A model with an override uses only its own windows and off-peak factors. Exact names or a trailing * are supported; the first match wins.'
                )}
              </p>
            </div>
            <Button
              type='button'
              variant='outline'
              size='sm'
              onClick={() =>
                setSetting((prev) => ({
                  ...prev,
                  model_overrides: [
                    ...prev.model_overrides,
                    {
                      model: '',
                      rules: [],
                      off_peak: { ...EMPTY_RULE, name: 'Off-peak' },
                    },
                  ],
                }))
              }
            >
              <Plus data-icon='inline-start' />
              {t('Add model override')}
            </Button>
          </div>

          {setting.model_overrides.length === 0 && (
            <p className='text-muted-foreground rounded-lg border border-dashed p-4 text-xs'>
              {t(
                'No per-model overrides. Every model in range uses the global windows.'
              )}
            </p>
          )}

          {setting.model_overrides.map((override, index) => (
            <div key={`override-${index}`} className='rounded-xl border p-4'>
              <div className='flex flex-wrap items-end gap-3'>
                <div className='flex min-w-48 flex-1 flex-col gap-1'>
                  <Label className='text-xs'>{t('Model name or pattern')}</Label>
                  <Input
                    value={override.model}
                    onChange={(event) =>
                      patchOverride(index, { model: event.target.value })
                    }
                    placeholder='claude-*'
                    className='font-mono text-xs'
                  />
                </div>
                <Button
                  type='button'
                  variant='ghost'
                  size='icon-sm'
                  aria-label={t('Remove model override')}
                  onClick={() =>
                    setSetting((prev) => ({
                      ...prev,
                      model_overrides: prev.model_overrides.filter(
                        (_, i) => i !== index
                      ),
                    }))
                  }
                >
                  <Trash2 className='size-4' />
                </Button>
              </div>

              <div className='mt-4 flex flex-col gap-3'>
                <RuleEditor
                  idPrefix={`override-${index}`}
                  rules={override.rules}
                  onChange={(rules) => patchOverride(index, { rules })}
                />
                <OffPeakEditor
                  rule={override.off_peak}
                  onChange={(off_peak) => patchOverride(index, { off_peak })}
                />
              </div>
            </div>
          ))}
        </div>

        <div className='bg-muted/40 rounded-xl p-4 text-xs leading-relaxed'>
          <p className='font-medium'>{t('Factor expressions')}</p>
          <p className='text-muted-foreground mt-1'>
            {t(
              'A factor can be a number or an expression. Available variables: hour, minute, weekday (0=Sunday), month, day in the configured timezone; utc_hour, utc_minute, utc_weekday in UTC. Example: hour >= 22 || hour < 6 ? 0.8 : 1.2'
            )}
          </p>
        </div>

        <div className='flex justify-end'>
          <Button
            type='button'
            onClick={() => void save()}
            disabled={updateOption.isPending}
          >
            {updateOption.isPending ? t('Saving...') : t('Save Changes')}
          </Button>
        </div>
      </div>
    </SettingsSection>
  )
}
