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
  createMemoryHistory,
  createRootRouteWithContext,
  createRoute,
  createRouter,
  Outlet,
  RouterProvider,
} from '@tanstack/react-router'
import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { AxiosError } from 'axios'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'
import { Route as DocsRoute } from '@/routes/docs'
import { useAuthStore } from '@/stores/auth-store'
import { useSystemConfigStore } from '@/stores/system-config-store'

const requestBody = `{
  "model": "YOUR_VIDEO_MODEL_ID",
  "prompt": "田野航拍",
  "duration": 5
}`

const mockDocuments = [
  {
    id: 'quickstart',
    title: '快速开始',
    content: `## 首次调用\n\n使用 YOUR_API_KEY。\n\n### 请求体\n\n\`\`\`json\n${requestBody}\n\`\`\``,
  },
  {
    id: 'text',
    title: '文本模型',
    content: '## 流式响应\n\n参数 stream 使用 SSE 流式传输。',
  },
]

const initialAdapter = api.defaults.adapter
const initialConfig = useSystemConfigStore.getState()
let client: QueryClient
let requireAuth: boolean
let enabled: boolean
let failDocs: boolean
let apiDocuments: typeof mockDocuments

beforeEach(() => {
  vi.spyOn(window, 'scrollTo').mockImplementation(() => {})
  localStorage.clear()
  useAuthStore.getState().auth.reset()
  useSystemConfigStore.setState(initialConfig)
  useSystemConfigStore.getState().setLoading(false)
  requireAuth = false
  enabled = true
  failDocs = false
  apiDocuments = mockDocuments
  api.defaults.adapter = async (config) => {
    let data: unknown = ''
    if (config.url === '/api/status') {
      data = {
        HeaderNavModules: JSON.stringify({ docs: { enabled, requireAuth } }),
      }
    }
    if (config.url === '/api/docs') {
      expect(config.headers.get('Cache-Control')).toContain('no-cache')
      if (failDocs) {
        throw new AxiosError(
          'Request failed with status code 404',
          undefined,
          config,
          undefined,
          {
            data: {},
            status: 404,
            statusText: 'Not Found',
            headers: {},
            config,
          }
        )
      }
      data = apiDocuments
    }
    return {
      data: { success: true, data },
      status: 200,
      statusText: 'OK',
      headers: {},
      config,
    }
  }
})

afterEach(async () => {
  cleanup()
  await api.get('/api/status')
  client?.clear()
  api.defaults.adapter = initialAdapter
  useAuthStore.getState().auth.reset()
  useSystemConfigStore.setState(initialConfig)
  localStorage.clear()
})

async function renderDocumentation(path = '/docs') {
  client = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Infinity } },
  })
  const root = createRootRouteWithContext<{ queryClient: QueryClient }>()({
    component: Outlet,
  })
  const docs = DocsRoute.update({
    id: '/docs/',
    path: '/docs/',
    getParentRoute: () => root,
  } as Parameters<typeof DocsRoute.update>[0])
  const home = createRoute({
    getParentRoute: () => root,
    path: '/',
    component: () => <h1>Home</h1>,
  })
  const login = createRoute({
    getParentRoute: () => root,
    path: '/sign-in',
    component: () => <h1>Login</h1>,
  })
  const router = createRouter({
    routeTree: root.addChildren([docs, home, login]),
    context: { queryClient: client },
    history: createMemoryHistory({ initialEntries: [path] }),
  })
  await router.load()
  render(
    <QueryClientProvider client={client}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  )
  return router
}

test('search finds body parameters and navigates to matching articles', async () => {
  const user = userEvent.setup()
  const router = await renderDocumentation()
  await screen.findByRole('heading', { level: 1, name: '快速开始' })

  const menu = screen.getByRole('button', { name: 'Documentation menu' })
  await user.click(menu)
  const input = screen.getByRole('searchbox', { name: 'Search documentation' })
  await user.type(input, 'stream')

  const nav = screen.getByRole('navigation', { name: 'API documentation' })
  const result = within(nav).getByRole('link', { name: /文本模型/ })
  expect(result).toHaveTextContent('stream')
  await user.click(result)

  await screen.findByRole('heading', { level: 1, name: '文本模型' })
  expect(router.state.location.href).toBe('/docs?article=text')
})

test('quickstart presents and copies gateway address while other articles omit it', async () => {
  const user = userEvent.setup()
  const writeText = vi.fn().mockResolvedValue(undefined)
  vi.spyOn(navigator.clipboard, 'writeText').mockImplementation(writeText)

  await renderDocumentation('/docs?article=quickstart')
  await screen.findByRole('heading', { level: 1, name: '快速开始' })
  const addresses = screen.getByRole('region', { name: 'Gateway URL' })
  expect(within(addresses).getByText(window.location.origin)).toBeVisible()

  await user.click(
    within(addresses).getByRole('button', { name: 'Copy Gateway URL' })
  )
  expect(writeText).toHaveBeenCalledWith(window.location.origin)

  cleanup()
  await renderDocumentation('/docs?article=text')
  await screen.findByRole('heading', { level: 1, name: '文本模型' })
  expect(
    screen.queryByRole('region', { name: 'Gateway URL' })
  ).not.toBeInTheDocument()
})

test('code blocks render and support plain-text clipboard copying', async () => {
  const user = userEvent.setup()
  const writeText = vi.fn().mockResolvedValue(undefined)
  vi.spyOn(navigator.clipboard, 'writeText').mockImplementation(writeText)

  await renderDocumentation('/docs?article=quickstart')
  await screen.findByRole('heading', { level: 1, name: '快速开始' })
  await user.click(screen.getByRole('button', { name: 'Copy to clipboard' }))
  expect(writeText).toHaveBeenCalledWith(requestBody)
  expect(await screen.findByRole('button', { name: 'Copied' })).toBeVisible()
})

test('deep link scrolls to anchor heading and supports article traversal', async () => {
  const user = userEvent.setup()
  const scroll = vi.spyOn(HTMLElement.prototype, 'scrollIntoView')
  const router = await renderDocumentation('/docs?article=quickstart#首次调用')

  const heading = await screen.findByRole('heading', {
    level: 2,
    name: '首次调用',
  })
  await waitFor(() => expect(scroll.mock.contexts).toContain(heading))

  const next = screen.getByRole('link', { name: /Next\s*文本模型/ })
  await user.click(next)
  await screen.findByRole('heading', { level: 1, name: '文本模型' })
  expect(router.state.location.search.article).toBe('text')
})

test('enforces access control with authentication redirects and error recovery', async () => {
  const user = userEvent.setup()
  enabled = true
  requireAuth = true
  await renderDocumentation()
  expect(
    await screen.findByRole('heading', { level: 1, name: 'Login' })
  ).toBeVisible()

  cleanup()
  enabled = false
  await renderDocumentation()
  expect(
    await screen.findByRole('heading', { level: 1, name: 'Home' })
  ).toBeVisible()

  cleanup()
  enabled = true
  requireAuth = false
  failDocs = true
  await renderDocumentation()
  expect(
    await screen.findByRole('heading', {
      level: 1,
      name: 'Documentation unavailable',
    })
  ).toBeVisible()

  expect(
    screen.queryByText('The documentation may be disabled or require login.')
  ).not.toBeInTheDocument()
  expect(screen.getByRole('alert')).toHaveTextContent('Request failed')
  expect(screen.getByRole('alert')).toHaveTextContent('404')
  failDocs = false
  await user.click(screen.getByRole('button', { name: 'Retry' }))
  expect(
    await screen.findByRole('heading', { level: 1, name: '快速开始' })
  ).toBeVisible()
})

test('scrolling synchronizes the directory without changing the deep link', async () => {
  const router = await renderDocumentation('/docs#请求体')
  const heading = await screen.findByRole('heading', {
    level: 3,
    name: '请求体',
  })
  const directory = screen.getByRole('navigation', { name: 'On this page' })
  const first = within(directory).getByRole('link', { name: '首次调用' })
  const request = within(directory).getByRole('link', { name: '请求体' })
  let top = 400
  vi.spyOn(HTMLElement.prototype, 'getBoundingClientRect').mockImplementation(
    function (this: HTMLElement) {
      const y = this === heading ? top : 80
      return {
        top: y,
        bottom: y + 32,
        height: 32,
        left: 0,
        right: 600,
        width: 600,
        x: 0,
        y,
        toJSON: () => ({}),
      }
    }
  )
  fireEvent.scroll(window)
  await waitFor(() => expect(first).toHaveAttribute('aria-current', 'page'))
  expect(request).not.toHaveAttribute('aria-current')
  top = 100
  fireEvent.scroll(window)
  await waitFor(() => expect(request).toHaveAttribute('aria-current', 'page'))
  expect(first).not.toHaveAttribute('aria-current')
  top = 400
  fireEvent.scroll(window)
  await waitFor(() => expect(first).toHaveAttribute('aria-current', 'page'))
  expect(decodeURIComponent(router.state.location.hash)).toBe('请求体')
})

test('a new login session retries a failed documentation request for the same user', async () => {
  requireAuth = true
  const bundle = {
    user: { id: 72, username: 'docs-reader', role: 1 },
    access_token: 'test-session-token',
    token_type: 'Bearer',
    access_expires_at: 9999999999,
    session: {
      sid: 'docs-session',
      current: true,
      login_method: 'password',
      ip: '',
      user_agent: '',
      created_at: 1,
      last_active_at: 1,
      expires_at: 9999999999,
    },
  }
  useAuthStore.getState().auth.setBundle(bundle)
  failDocs = true
  await renderDocumentation()
  await screen.findByRole('heading', { name: 'Documentation unavailable' })
  failDocs = false
  act(() =>
    useAuthStore.getState().auth.setBundle({
      ...bundle,
      access_token: 'replacement-session-token',
      session: { ...bundle.session, sid: 'replacement-session' },
    })
  )
  expect(
    await screen.findByRole('heading', { level: 1, name: '快速开始' })
  ).toBeVisible()
})
