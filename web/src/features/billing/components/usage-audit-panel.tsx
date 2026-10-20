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
import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation, useQuery } from '@tanstack/react-query'
import { useId, useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'

import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  Field,
  FieldError,
  FieldGroup,
  FieldLabel,
} from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import { useDebounce } from '@/hooks/use-debounce'
import { handleServerError } from '@/lib/handle-server-error'

import { billingToday } from '../api'
import {
  usageAuditSchema,
  usageAuditSelection,
  type UsageAuditValues,
} from '../lib/usage-audit'
import { queryUsageAudit, searchUsageAuditKeys } from '../usage-audit-api'
import { UsageAuditResults } from './usage-audit-results'

export function UsageAuditPanel(props: { userId: number }) {
  const { t } = useTranslation()
  const id = useId()
  const today = billingToday()
  const tomorrow = new Date(Date.parse(`${today}T00:00:00Z`) + 86400000)
    .toISOString()
    .slice(0, 10)
  const [keySearch, setKeySearch] = useState('')
  const keyword = useDebounce(keySearch, 300)
  const form = useForm<UsageAuditValues>({
    resolver: zodResolver(usageAuditSchema),
    defaultValues: {
      scope: 'customer',
      start: `${today.slice(0, 7)}-01T00:00`,
      end: `${tomorrow}T00:00`,
      token: '0',
      channel: '',
      model: '',
      group: '',
      protocol: '',
    },
  })
  const values = form.watch()
  const userId = values.scope === 'all' ? 0 : props.userId
  const keys = useQuery({
    queryKey: ['billing', 'usage-audit-keys', userId, keyword],
    queryFn: () => searchUsageAuditKeys(userId, keyword),
  })
  const query = useMutation({
    mutationFn: queryUsageAudit,
    onError: handleServerError,
  })
  const selection = usageAuditSelection(props.userId, values)
  const stale = JSON.stringify(selection) !== JSON.stringify(query.variables)
  const keyOptions = keys.data ?? []
  return (
    <div className='flex min-w-0 flex-col gap-4'>
      <p className='text-sm'>
        {t(
          'Audit token usage and historical prices by customer, key and period. Up to 93 days and 10000 records per query.'
        )}
      </p>
      <form
        onSubmit={form.handleSubmit((input) =>
          query.mutate(usageAuditSelection(props.userId, input))
        )}
      >
        <fieldset disabled={query.isPending}>
          <FieldGroup className='grid gap-4 sm:grid-cols-2 xl:grid-cols-3'>
            <Field>
              <FieldLabel htmlFor={`${id}-scope`}>
                {t('Search scope')}
              </FieldLabel>
              <NativeSelect
                id={`${id}-scope`}
                {...form.register('scope', {
                  onChange: () => {
                    form.setValue('token', '0')
                    setKeySearch('')
                  },
                })}
              >
                <NativeSelectOption value='customer'>
                  {t('Selected billing customer')}
                </NativeSelectOption>
                <NativeSelectOption value='all'>
                  {t('All customers')}
                </NativeSelectOption>
              </NativeSelect>
            </Field>
            <Field>
              <FieldLabel htmlFor={`${id}-start`}>
                {t('Start (Asia/Shanghai)')}
              </FieldLabel>
              <Input
                id={`${id}-start`}
                type='datetime-local'
                {...form.register('start')}
              />
              <FieldError>
                {form.formState.errors.start &&
                  t('Select a valid period of up to 93 days')}
              </FieldError>
            </Field>
            <Field>
              <FieldLabel htmlFor={`${id}-end`}>
                {t('End, exclusive (Asia/Shanghai)')}
              </FieldLabel>
              <Input
                id={`${id}-end`}
                type='datetime-local'
                {...form.register('end')}
              />
              <FieldError>
                {form.formState.errors.end &&
                  t('Select a valid period of up to 93 days')}
              </FieldError>
            </Field>
            <Field>
              <FieldLabel htmlFor={`${id}-key-search`}>
                {t('Key name or ID')}
              </FieldLabel>
              <Input
                id={`${id}-key-search`}
                value={keySearch}
                onChange={(event) => setKeySearch(event.target.value)}
                maxLength={100}
              />
            </Field>
            <Field>
              <FieldLabel htmlFor={`${id}-token`}>{t('API key')}</FieldLabel>
              <NativeSelect
                id={`${id}-token`}
                value={values.token}
                onChange={(event) =>
                  form.setValue('token', event.target.value, {
                    shouldValidate: true,
                  })
                }
              >
                <NativeSelectOption value='0'>
                  {t('All keys')}
                </NativeSelectOption>
                {keyOptions.map((key) => (
                  <NativeSelectOption key={key.id} value={String(key.id)}>
                    #{key.id} / {key.name} / {t('User')} #{key.user_id}
                  </NativeSelectOption>
                ))}
                {values.token !== '0' &&
                  !keyOptions.some(
                    (key) => String(key.id) === values.token
                  ) && (
                    <NativeSelectOption value={values.token}>
                      #{values.token}
                    </NativeSelectOption>
                  )}
              </NativeSelect>
              {keys.isError && (
                <span role='alert'>{t('Key search failed')}</span>
              )}
            </Field>
            <Field>
              <FieldLabel htmlFor={`${id}-manual-token`}>
                {t('Key ID (including deleted keys)')}
              </FieldLabel>
              <Input
                id={`${id}-manual-token`}
                type='number'
                min='0'
                step='1'
                {...form.register('token')}
              />
              <FieldError>
                {form.formState.errors.token && t('Invalid ID')}
              </FieldError>
            </Field>
            <Field>
              <FieldLabel htmlFor={`${id}-model`}>{t('Model')}</FieldLabel>
              <Input
                id={`${id}-model`}
                {...form.register('model')}
                maxLength={255}
              />
            </Field>
            <Field>
              <FieldLabel htmlFor={`${id}-group`}>
                {t('Billing group')}
              </FieldLabel>
              <Input
                id={`${id}-group`}
                {...form.register('group')}
                maxLength={64}
              />
            </Field>
            <Field>
              <FieldLabel htmlFor={`${id}-channel`}>
                {t('Channel ID')}
              </FieldLabel>
              <Input
                id={`${id}-channel`}
                type='number'
                min='0'
                step='1'
                {...form.register('channel')}
              />
              <FieldError>
                {form.formState.errors.channel && t('Invalid ID')}
              </FieldError>
            </Field>
            <Field>
              <FieldLabel htmlFor={`${id}-protocol`}>
                {t('Protocol')}
              </FieldLabel>
              <NativeSelect
                id={`${id}-protocol`}
                {...form.register('protocol')}
              >
                <NativeSelectOption value=''>
                  {t('All protocols')}
                </NativeSelectOption>
                <NativeSelectOption value='anthropic'>
                  Anthropic Messages
                </NativeSelectOption>
                <NativeSelectOption value='openai'>OpenAI</NativeSelectOption>
              </NativeSelect>
            </Field>
          </FieldGroup>
          <Button type='submit' className='mt-4'>
            {query.isPending ? t('Calculating…') : t('Run usage audit')}
          </Button>
        </fieldset>
      </form>
      {query.isPending && <p role='status'>{t('Calculating…')}</p>}
      {query.isError && (
        <Alert variant='destructive'>
          <AlertDescription>
            {t('Usage audit failed. Narrow the filters or retry.')}{' '}
            {query.error instanceof Error ? query.error.message : ''}
          </AlertDescription>
        </Alert>
      )}
      {query.data && (
        <UsageAuditResults
          key={query.data.sha256}
          report={query.data}
          stale={stale}
        />
      )}
    </div>
  )
}
