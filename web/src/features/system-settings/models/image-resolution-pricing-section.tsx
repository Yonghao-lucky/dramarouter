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
import { Switch } from '@/components/ui/switch'

import { SettingsSection } from '../components/settings-section'
import { useUpdateOption } from '../hooks/use-update-option'

type ImageResolutionTier = {
  name: string
  // 像素面积上限，0 表示兜底档，必须排在最后
  max_pixels: number
  price: number
  // 仅前端用于列表 key，保存前剔除
  rowKey?: string
}

type ImageResolutionModelPricing = {
  tiers: ImageResolutionTier[]
  default_price: number
}

type ImageResolutionPricingSetting = {
  enabled: boolean
  models: Record<string, ImageResolutionModelPricing>
}

// 常见的 1K / 2K / 4K 分界，按像素面积而非固定尺寸，便于上游新增等比例尺寸。
const DEFAULT_TIERS: ImageResolutionTier[] = [
  { name: '1K', max_pixels: 2_000_000, price: 0.0039 },
  { name: '2K', max_pixels: 6_000_000, price: 0.0166 },
  { name: '4K', max_pixels: 0, price: 0.0292 },
]

function numberOrZero(value: string): number {
  const parsed = Number(value)
  return Number.isFinite(parsed) ? parsed : 0
}

let tierRowSeq = 0

/** 档位行需要稳定的 key：排序与删除都会改变下标，用下标做 key 会错位。 */
function withRowKeys(tiers: ImageResolutionTier[]): ImageResolutionTier[] {
  return tiers.map((tier) => {
    tierRowSeq += 1
    return { ...tier, rowKey: tier.rowKey ?? `tier-${tierRowSeq}` }
  })
}

/** 按像素面积把一组尺寸归类，仅用于设置页的预览提示。 */
function describePixels(maxPixels: number): string {
  if (maxPixels <= 0) return '∞'
  if (maxPixels >= 1_000_000) {
    return `${Math.round((maxPixels / 1_000_000) * 10) / 10} MP`
  }
  return `${Math.round(maxPixels / 1000)} KP`
}

type ImageResolutionPricingSectionProps = {
  defaultValue?: string
}

/**
 * 图片按分辨率计费：同一模型按出图尺寸落在不同价格档。
 * 价格单位为美元/次，最终费用再乘以图片张数、分组倍率与峰谷系数。
 */
export function ImageResolutionPricingSection(
  props: ImageResolutionPricingSectionProps
) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()
  const initial = useMemo((): ImageResolutionPricingSetting => {
    const fallback: ImageResolutionPricingSetting = {
      enabled: false,
      models: {},
    }
    if (!props.defaultValue) return fallback
    try {
      const parsed = JSON.parse(
        props.defaultValue
      ) as Partial<ImageResolutionPricingSetting>
      const models: Record<string, ImageResolutionModelPricing> = {}
      for (const [name, pricing] of Object.entries(parsed.models ?? {})) {
        models[name] = {
          tiers: withRowKeys(pricing.tiers ?? []),
          default_price: pricing.default_price ?? 0,
        }
      }
      return {
        enabled: parsed.enabled ?? false,
        models,
      }
    } catch {
      return fallback
    }
  }, [props.defaultValue])
  const [setting, setSetting] = useState<ImageResolutionPricingSetting>(initial)

  const modelNames = Object.keys(setting.models)

  const patchModel = (
    modelName: string,
    patch: Partial<ImageResolutionModelPricing>
  ) => {
    setSetting((prev) => ({
      ...prev,
      models: {
        ...prev.models,
        [modelName]: { ...prev.models[modelName], ...patch },
      },
    }))
  }

  const renameModel = (from: string, to: string) => {
    setSetting((prev) => {
      const models: Record<string, ImageResolutionModelPricing> = {}
      for (const [name, pricing] of Object.entries(prev.models)) {
        models[name === from ? to : name] = pricing
      }
      return { ...prev, models }
    })
  }

  const removeModel = (modelName: string) => {
    setSetting((prev) => {
      const models = { ...prev.models }
      delete models[modelName]
      return { ...prev, models }
    })
  }

  const save = async () => {
    // 空模型名会直接被后端拒绝，这里先剔除，避免保存失败。
    const models: Record<string, ImageResolutionModelPricing> = {}
    for (const [name, pricing] of Object.entries(setting.models)) {
      const trimmed = name.trim()
      if (!trimmed) continue
      models[trimmed] = {
        tiers: pricing.tiers.map((tier) => ({
          name: tier.name.trim(),
          max_pixels: tier.max_pixels,
          price: tier.price,
        })),
        default_price: pricing.default_price,
      }
    }
    await updateOption.mutateAsync({
      key: 'ImageResolutionPricing',
      value: JSON.stringify({ enabled: setting.enabled, models }),
    })
  }

  return (
    <SettingsSection title={t('Image Resolution Pricing')}>
      <div className='flex flex-col gap-5'>
        <div className='rounded-xl border p-4'>
          <div className='flex items-start justify-between gap-4'>
            <div className='min-w-0'>
              <p className='text-sm font-medium'>
                {t('Bill images by resolution')}
              </p>
              <p className='text-muted-foreground mt-0.5 text-xs leading-relaxed'>
                {t(
                  'Each model gets an ordered tier table keyed by pixel area (width x height). The first tier whose limit covers the requested size wins; a tier with limit 0 is the catch-all and must stay last. Prices are in USD per image and are multiplied by the image count, group ratio and peak factors.'
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
        </div>

        <div className='flex items-center justify-between'>
          <div>
            <p className='text-sm font-medium'>{t('Model tiers')}</p>
            <p className='text-muted-foreground text-xs'>
              {t(
                'A trailing * matches a model prefix, e.g. gpt-image-*. Exact names take priority.'
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
                models: {
                  ...prev.models,
                  '': {
                    tiers: withRowKeys(
                      DEFAULT_TIERS.map((tier) => ({ ...tier }))
                    ),
                    default_price: DEFAULT_TIERS[0].price,
                  },
                },
              }))
            }
          >
            <Plus data-icon='inline-start' />
            {t('Add model')}
          </Button>
        </div>

        {modelNames.length === 0 && (
          <p className='text-muted-foreground rounded-lg border border-dashed p-4 text-xs'>
            {t(
              'No resolution pricing configured. Image models keep their flat per-call price.'
            )}
          </p>
        )}

        {modelNames.map((modelKey) => {
          const pricing = setting.models[modelKey]
          return (
            <div key={modelKey || 'new'} className='rounded-xl border p-4'>
              <div className='flex flex-wrap items-end gap-3'>
                <div className='flex min-w-48 flex-1 flex-col gap-1'>
                  <Label className='text-xs'>
                    {t('Model name or pattern')}
                  </Label>
                  <Input
                    value={modelKey}
                    onChange={(event) =>
                      renameModel(modelKey, event.target.value)
                    }
                    placeholder='gpt-image-2.5'
                    className='font-mono text-xs'
                  />
                </div>
                <div className='flex w-32 flex-col gap-1'>
                  <Label className='text-xs'>{t('Default price (USD)')}</Label>
                  <Input
                    type='number'
                    min={0}
                    step='0.0001'
                    value={String(pricing.default_price)}
                    onChange={(event) =>
                      patchModel(modelKey, {
                        default_price: numberOrZero(event.target.value),
                      })
                    }
                    className='font-mono text-xs'
                  />
                </div>
                <Button
                  type='button'
                  variant='ghost'
                  size='icon-sm'
                  aria-label={t('Remove model')}
                  onClick={() => removeModel(modelKey)}
                >
                  <Trash2 className='size-4' />
                </Button>
              </div>

              <div className='mt-4 flex flex-col gap-2'>
                <div className='text-muted-foreground grid grid-cols-[1fr_1fr_1fr_auto] gap-2 text-[11px]'>
                  <span>{t('Tier name')}</span>
                  <span>{t('Max pixels')}</span>
                  <span>{t('Price (USD)')}</span>
                  <span />
                </div>
                {pricing.tiers.map((tier, index) => (
                  <div
                    key={tier.rowKey ?? `${modelKey}-tier-${index}`}
                    className='grid grid-cols-[1fr_1fr_1fr_auto] items-center gap-2'
                  >
                    <Input
                      value={tier.name}
                      onChange={(event) =>
                        patchModel(modelKey, {
                          tiers: pricing.tiers.map((item, i) =>
                            i === index
                              ? { ...item, name: event.target.value }
                              : item
                          ),
                        })
                      }
                      placeholder='2K'
                      className='font-mono text-xs'
                    />
                    <Input
                      type='number'
                      min={0}
                      step={100000}
                      value={String(tier.max_pixels)}
                      onChange={(event) =>
                        patchModel(modelKey, {
                          tiers: pricing.tiers.map((item, i) =>
                            i === index
                              ? {
                                  ...item,
                                  max_pixels: numberOrZero(event.target.value),
                                }
                              : item
                          ),
                        })
                      }
                      className='font-mono text-xs'
                    />
                    <Input
                      type='number'
                      min={0}
                      step='0.0001'
                      value={String(tier.price)}
                      onChange={(event) =>
                        patchModel(modelKey, {
                          tiers: pricing.tiers.map((item, i) =>
                            i === index
                              ? {
                                  ...item,
                                  price: numberOrZero(event.target.value),
                                }
                              : item
                          ),
                        })
                      }
                      className='font-mono text-xs'
                    />
                    <div className='flex items-center gap-1'>
                      <span className='text-muted-foreground w-12 text-[10px] tabular-nums'>
                        {describePixels(tier.max_pixels)}
                      </span>
                      <Button
                        type='button'
                        variant='ghost'
                        size='icon-sm'
                        aria-label={t('Remove tier')}
                        onClick={() =>
                          patchModel(modelKey, {
                            tiers: pricing.tiers.filter((_, i) => i !== index),
                          })
                        }
                      >
                        <Trash2 className='size-4' />
                      </Button>
                    </div>
                  </div>
                ))}
                <Button
                  type='button'
                  variant='outline'
                  size='sm'
                  onClick={() =>
                    patchModel(modelKey, {
                      tiers: [
                        ...pricing.tiers,
                        { name: '', max_pixels: 0, price: 0 },
                      ],
                    })
                  }
                >
                  <Plus data-icon='inline-start' />
                  {t('Add tier')}
                </Button>
              </div>
            </div>
          )
        })}

        <div className='bg-muted/40 rounded-xl p-4 text-xs leading-relaxed'>
          <p className='font-medium'>{t('How sizes map to tiers')}</p>
          <p className='text-muted-foreground mt-1'>
            {t(
              'Limits are pixel areas: 1024x1024 is about 1.0 MP, 1536x1024 about 1.6 MP, 2048x2048 about 4.2 MP and 3840x2160 about 8.3 MP. Sizes that are missing or set to auto fall back to the default price.'
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
