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
import { Component, type ErrorInfo, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'

interface PanelBoundaryProps {
  children: ReactNode
  /** Shown as the panel title so the failed area stays identifiable. */
  title?: string
  onReset?: () => void
}

interface PanelBoundaryState {
  error: Error | null
}

// PanelBoundary keeps one reseller panel's render failure inside that panel.
// Without it a single throw from a heavy query unmounts the whole page, which is
// exactly the blank screen reported when opening a reseller's detail view.
export class PanelBoundary extends Component<
  PanelBoundaryProps,
  PanelBoundaryState
> {
  state: PanelBoundaryState = { error: null }

  static getDerivedStateFromError(error: Error): PanelBoundaryState {
    return { error }
  }

  componentDidCatch(error: Error, info: ErrorInfo) {
    // Keep the trace in the console for support without surfacing it to users.
    console.error('reseller panel failed', this.props.title, error, info)
  }

  private reset = () => {
    this.setState({ error: null })
    this.props.onReset?.()
  }

  render() {
    if (this.state.error) {
      return (
        <PanelFallback
          title={this.props.title}
          message={this.state.error.message}
          onRetry={this.reset}
        />
      )
    }
    return this.props.children
  }
}

function PanelFallback(props: {
  title?: string
  message: string
  onRetry: () => void
}) {
  const { t } = useTranslation()
  return (
    <Card className='border-destructive/40'>
      <CardContent className='flex flex-col items-start gap-3 py-6'>
        <div className='text-sm font-medium'>
          {props.title
            ? t('{{title}} could not be loaded', { title: props.title })
            : t('This section could not be loaded')}
        </div>
        <p className='text-muted-foreground text-xs'>{props.message}</p>
        <Button size='sm' variant='outline' onClick={props.onRetry}>
          {t('Retry')}
        </Button>
      </CardContent>
    </Card>
  )
}
