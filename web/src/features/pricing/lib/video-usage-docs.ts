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

/**
 * 视频模型的调用文档：可用端点、参数表、请求示例与使用备注。
 *
 * 计费维度由插件声明（billing_usage_schema），但请求字段属于各厂商协议，
 * 插件没有声明，所以这里按模型名维护。新增视频模型时在此追加一份即可，
 * 没有条目的模型只显示通用示例，不会显示错误的参数表。
 */
export type UsageParameter = {
  name: string
  type: 'string' | 'integer' | 'enum' | 'array' | 'object' | 'boolean'
  required?: boolean
  defaultValue?: string | number
  range?: string
  enumValues?: string[]
  /** i18n key, or an already-localized sentence when the text is model-specific. */
  description: string
}

export type VideoApiOperationId =
  | 'minimax-create'
  | 'minimax-query'
  | 'video-create'
  | 'video-query'
  | 'responses-create'
  | 'responses-query'

export type VideoApiEndpoint = {
  id: VideoApiOperationId
  method: 'POST' | 'GET'
  /** Path on this gateway, with a leading slash. */
  path: string
  /** Protocol group the operation belongs to. */
  protocol: string
  summary: string
}

export type VideoUsageDoc = {
  endpoints: VideoApiEndpoint[]
  parameters: UsageParameter[]
  /** Short notes rendered under the parameter table. */
  notes: string[]
}

export type VideoSampleContext = {
  baseUrl: string
  apiKeyEnv: string
  modelName: string
}

export type VideoSampleLang = 'curl' | 'python' | 'typescript'

const H3_DOC: VideoUsageDoc = {
  endpoints: [
    {
      id: 'minimax-create',
      method: 'POST',
      path: '/v2/video_generation',
      protocol: 'MiniMax v2',
      summary: 'Create a video generation task and return its task_id',
    },
    {
      id: 'minimax-query',
      method: 'GET',
      path: '/v2/query/video_generation/{task_id}',
      protocol: 'MiniMax v2',
      summary: 'Query a task by task_id; the video URL appears once it succeeds',
    },
    {
      id: 'video-create',
      method: 'POST',
      path: '/v1/videos',
      protocol: 'OpenAI Video',
      summary: 'Same model over the OpenAI video protocol',
    },
    {
      id: 'video-query',
      method: 'GET',
      path: '/v1/videos/{video_id}',
      protocol: 'OpenAI Video',
      summary: 'Retrieve an OpenAI video job',
    },
    {
      id: 'responses-create',
      method: 'POST',
      path: '/v1/responses',
      protocol: 'OpenAI Responses',
      summary: 'Create a background response that renders the generated video',
    },
    {
      id: 'responses-query',
      method: 'GET',
      path: '/v1/responses/{response_id}',
      protocol: 'OpenAI Responses',
      summary: 'Poll the background response until it completes',
    },
  ],
  parameters: [
    {
      name: 'model',
      type: 'string',
      required: true,
      defaultValue: 'MiniMax-H3',
      description: 'Model identifier for the request',
    },
    {
      name: 'content',
      type: 'array',
      required: true,
      description:
        'MiniMax v2 message list. A text item is required; image/video/audio items attach reference media.',
    },
    {
      name: 'content[].type',
      type: 'enum',
      enumValues: ['text', 'image_url', 'video_url', 'audio_url'],
      description: 'Content item kind, one per media reference',
    },
    {
      name: 'content[].role',
      type: 'enum',
      enumValues: ['first_frame', 'last_frame', 'reference_image'],
      description:
        'Image role. first_frame/last_frame must be supplied as an exact pair and only at 768P; a single image is treated as reference_image.',
    },
    {
      name: 'resolution',
      type: 'enum',
      enumValues: ['768P', '1080P', '2K'],
      defaultValue: '768P',
      description: 'Output resolution tier',
    },
    {
      name: 'duration',
      type: 'integer',
      range: '4 ~ 15',
      defaultValue: 5,
      description: 'Output video duration in seconds',
    },
    {
      name: 'ratio',
      type: 'enum',
      enumValues: ['16:9', '9:16', '1:1'],
      defaultValue: '16:9',
      description:
        'Output aspect ratio. 768P accepts 16:9, 9:16 and 1:1; 1080P and 2K accept 16:9 and 9:16 only.',
    },
    {
      name: 'callback_url',
      type: 'string',
      description: 'Optional. Webhook notified when the task finishes',
    },
    {
      name: 'aigc_watermark',
      type: 'boolean',
      description: 'Optional. Adds the AIGC watermark to the output',
    },
    {
      name: 'prompt / seconds / size',
      type: 'string',
      description:
        'OpenAI protocol only. prompt maps to the text item, seconds to duration and size to resolution.',
    },
  ],
  notes: [
    'Every endpoint is asynchronous: creating a task returns an id, and the video URL appears once the task status becomes succeeded.',
    'Billing is per second of output video, at the unit price of the selected resolution. The first 5 input images in a task are free; each additional image is charged separately.',
    'Input video seconds are billed at the same per-second rate as the selected resolution. Audio input is free.',
    'Duration is 4 to 15 seconds. A request without a duration is billed as 5 seconds.',
    'Large media should be passed as a public URL: the request body limit is 64 MB and Base64 encoding adds about 33%.',
    'x-api-key is accepted in place of Authorization: Bearer on the Anthropic-formatted surface.',
  ],
}

const VIDEO_USAGE_DOCS: Record<string, VideoUsageDoc> = {
  'minimax-h3': H3_DOC,
}

export function videoUsageDoc(modelName: string): VideoUsageDoc | null {
  if (!modelName) return null
  return VIDEO_USAGE_DOCS[modelName.trim().toLowerCase()] ?? null
}

// ---------------------------------------------------------------------------
// Request samples
// ---------------------------------------------------------------------------

const H3_PROMPT = 'A cat playing piano on a rainy neon street.'

function minimaxBody(ctx: VideoSampleContext): string {
  return JSON.stringify(
    {
      model: ctx.modelName,
      content: [{ type: 'text', text: H3_PROMPT }],
      resolution: '768P',
      duration: 5,
      ratio: '16:9',
    },
    null,
    2
  )
}

function buildMinimaxCreate(lang: VideoSampleLang, ctx: VideoSampleContext) {
  const url = `${ctx.baseUrl}/v2/video_generation`
  if (lang === 'curl') {
    return [
      `curl ${url} \\`,
      `  -H "Authorization: Bearer $${ctx.apiKeyEnv}" \\`,
      `  -H "Content-Type: application/json" \\`,
      `  -d '${minimaxBody(ctx).replace(/\n/g, '\n     ')}'`,
      '',
      `# => {"task_id":"task_xxxxxxxx"}`,
    ].join('\n')
  }
  if (lang === 'python') {
    return [
      'import requests',
      '',
      `BASE = "${ctx.baseUrl}"`,
      `HEADERS = {"Authorization": "Bearer <YOUR_API_KEY>"}`,
      '',
      'created = requests.post(',
      `    f"{BASE}/v2/video_generation",`,
      '    headers=HEADERS,',
      '    json={',
      `        "model": "${ctx.modelName}",`,
      `        "content": [{"type": "text", "text": "${H3_PROMPT}"}],`,
      '        "resolution": "768P",',
      '        "duration": 5,',
      '        "ratio": "16:9",',
      '    },',
      ')',
      'created.raise_for_status()',
      'task_id = created.json()["task_id"]',
      'print(task_id)',
    ].join('\n')
  }
  return [
    `const created = await fetch('${ctx.baseUrl}/v2/video_generation', {`,
    `  method: 'POST',`,
    `  headers: {`,
    `    Authorization: \`Bearer \${process.env.${ctx.apiKeyEnv}}\`,`,
    `    'Content-Type': 'application/json',`,
    `  },`,
    `  body: JSON.stringify(${minimaxBody(ctx).replace(/\n/g, '\n  ')}),`,
    `})`,
    '',
    `const { task_id: taskId } = await created.json()`,
    `console.log(taskId)`,
  ].join('\n')
}

function buildMinimaxQuery(lang: VideoSampleLang, ctx: VideoSampleContext) {
  const url = `${ctx.baseUrl}/v2/query/video_generation`
  if (lang === 'curl') {
    return [
      `curl ${url}/<TASK_ID> \\`,
      `  -H "Authorization: Bearer $${ctx.apiKeyEnv}"`,
      '',
      '# => {"task":{"id":"task_xxxxxxxx","status":"succeeded","content":{"url":"https://..."}}}',
      '# status is one of queued, running, succeeded, failed',
    ].join('\n')
  }
  if (lang === 'python') {
    return [
      'import time',
      '',
      'while True:',
      `    task = requests.get(`,
      `        f"{BASE}/v2/query/video_generation/{task_id}", headers=HEADERS`,
      '    ).json()["task"]',
      '    if task["status"] in ("succeeded", "failed"):',
      '        break',
      '    time.sleep(5)',
      '',
      'if task["status"] == "failed":',
      '    raise SystemExit(task.get("error", {}).get("message", "task failed"))',
      '',
      'print(task["content"]["url"])',
    ].join('\n')
  }
  return [
    'let task: any',
    'while (true) {',
    `  task = await fetch(`,
    `    \`${url}/\${taskId}\`,`,
    `    { headers: { Authorization: \`Bearer \${process.env.${ctx.apiKeyEnv}}\` } }`,
    '  ).then((res) => res.json()).then((body) => body.task)',
    `  if (task.status === 'succeeded' || task.status === 'failed') break`,
    '  await new Promise((resolve) => setTimeout(resolve, 5000))',
    '}',
    '',
    `console.log(task.content.url)`,
  ].join('\n')
}

function buildVideoCreate(lang: VideoSampleLang, ctx: VideoSampleContext) {
  const url = `${ctx.baseUrl}/v1/videos`
  const prompt = H3_PROMPT.replace(/'/g, "\\'")
  if (lang === 'curl') {
    const body = JSON.stringify(
      { model: ctx.modelName, prompt: H3_PROMPT, seconds: 5, size: '768P' },
      null,
      2
    )
    return [
      `curl ${url} \\`,
      `  -H "Authorization: Bearer $${ctx.apiKeyEnv}" \\`,
      `  -H "Content-Type: application/json" \\`,
      `  -d '${body.replace(/\n/g, '\n     ')}'`,
      '',
      `# => {"id":"task_xxxxxxxx","object":"video","status":"queued"}`,
    ].join('\n')
  }
  if (lang === 'python') {
    return [
      'from openai import OpenAI',
      '',
      `client = OpenAI(base_url="${ctx.baseUrl}/v1", api_key="<YOUR_API_KEY>")`,
      '',
      'job = client.videos.create(',
      `    model="${ctx.modelName}",`,
      `    prompt="${prompt}",`,
      '    seconds=5,',
      '    size="768P",',
      ')',
      'print(job.id)',
    ].join('\n')
  }
  return [
    `const created = await fetch('${url}', {`,
    `  method: 'POST',`,
    `  headers: {`,
    `    Authorization: \`Bearer \${process.env.${ctx.apiKeyEnv}}\`,`,
    `    'Content-Type': 'application/json',`,
    `  },`,
    `  body: JSON.stringify({`,
    `    model: '${ctx.modelName}',`,
    `    prompt: '${prompt}',`,
    `    seconds: 5,`,
    `    size: '768P',`,
    `  }),`,
    `})`,
    '',
    `const job = await created.json()`,
    `console.log(job.id)`,
  ].join('\n')
}

function buildVideoQuery(lang: VideoSampleLang, ctx: VideoSampleContext) {
  const url = `${ctx.baseUrl}/v1/videos`
  if (lang === 'curl') {
    return [
      `curl ${url}/<VIDEO_ID> \\`,
      `  -H "Authorization: Bearer $${ctx.apiKeyEnv}"`,
      '',
      '# => {"id":"task_xxxxxxxx","status":"completed","url":"https://..."}',
    ].join('\n')
  }
  if (lang === 'python') {
    return [
      'import time',
      '',
      'while job.status not in ("completed", "failed"):',
      '    time.sleep(5)',
      '    job = client.videos.retrieve(job.id)',
      '',
      'print(job.url)',
    ].join('\n')
  }
  return [
    'let detail = job',
    `while (detail.status !== 'completed' && detail.status !== 'failed') {`,
    '  await new Promise((resolve) => setTimeout(resolve, 5000))',
    `  detail = await fetch(\`${url}/\${job.id}\`, {`,
    `    headers: { Authorization: \`Bearer \${process.env.${ctx.apiKeyEnv}}\` },`,
    '  }).then((res) => res.json())',
    '}',
    '',
    'console.log(detail.url)',
  ].join('\n')
}

function buildResponsesCreate(lang: VideoSampleLang, ctx: VideoSampleContext) {
  const url = `${ctx.baseUrl}/v1/responses`
  if (lang === 'curl') {
    const body = JSON.stringify(
      { model: ctx.modelName, input: H3_PROMPT, background: true },
      null,
      2
    )
    return [
      `curl ${url} \\`,
      `  -H "Authorization: Bearer $${ctx.apiKeyEnv}" \\`,
      `  -H "Content-Type: application/json" \\`,
      `  -d '${body.replace(/\n/g, '\n     ')}'`,
      '',
      `# => {"id":"resp_xxxxxxxx","status":"queued","metadata":{"task_id":"task_xxxxxxxx"}}`,
    ].join('\n')
  }
  if (lang === 'python') {
    return [
      'from openai import OpenAI',
      '',
      `client = OpenAI(base_url="${ctx.baseUrl}/v1", api_key="<YOUR_API_KEY>")`,
      '',
      'response = client.responses.create(',
      `    model="${ctx.modelName}",`,
      `    input="${H3_PROMPT}",`,
      '    background=True,',
      ')',
      'print(response.id)',
    ].join('\n')
  }
  return [
    `const created = await fetch('${url}', {`,
    `  method: 'POST',`,
    `  headers: {`,
    `    Authorization: \`Bearer \${process.env.${ctx.apiKeyEnv}}\`,`,
    `    'Content-Type': 'application/json',`,
    `  },`,
    `  body: JSON.stringify({`,
    `    model: '${ctx.modelName}',`,
    `    input: '${H3_PROMPT}',`,
    `    background: true,`,
    `  }),`,
    `})`,
    '',
    `const response = await created.json()`,
    `console.log(response.id)`,
  ].join('\n')
}

function buildResponsesQuery(lang: VideoSampleLang, ctx: VideoSampleContext) {
  const url = `${ctx.baseUrl}/v1/responses`
  if (lang === 'curl') {
    return [
      `curl ${url}/<RESPONSE_ID> \\`,
      `  -H "Authorization: Bearer $${ctx.apiKeyEnv}"`,
      '',
      '# => {"id":"resp_xxxxxxxx","status":"completed","output":[{"content":[{"text":"https://..."}]}]}',
    ].join('\n')
  }
  if (lang === 'python') {
    return [
      'import time',
      '',
      'while response.status in ("queued", "in_progress"):',
      '    time.sleep(5)',
      '    response = client.responses.retrieve(response.id)',
      '',
      'print(response.output_text)',
    ].join('\n')
  }
  return [
    'let detail = response',
    `while (detail.status === 'queued' || detail.status === 'in_progress') {`,
    '  await new Promise((resolve) => setTimeout(resolve, 5000))',
    `  detail = await fetch(\`${url}/\${response.id}\`, {`,
    `    headers: { Authorization: \`Bearer \${process.env.${ctx.apiKeyEnv}}\` },`,
    '  }).then((res) => res.json())',
    '}',
    '',
    'console.log(detail.output_text)',
  ].join('\n')
}

const SAMPLE_BUILDERS: Record<
  VideoApiOperationId,
  (lang: VideoSampleLang, ctx: VideoSampleContext) => string
> = {
  'minimax-create': buildMinimaxCreate,
  'minimax-query': buildMinimaxQuery,
  'video-create': buildVideoCreate,
  'video-query': buildVideoQuery,
  'responses-create': buildResponsesCreate,
  'responses-query': buildResponsesQuery,
}

export function buildVideoApiSample(
  operation: VideoApiOperationId,
  lang: VideoSampleLang,
  ctx: VideoSampleContext
): string {
  return SAMPLE_BUILDERS[operation](lang, ctx)
}
