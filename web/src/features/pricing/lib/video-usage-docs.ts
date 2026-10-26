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
 * 视频模型的调用文档：参数表 + 使用备注。
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

export type VideoUsageDoc = {
  /** Where the request goes, relative to the site root. */
  endpoint: string
  parameters: UsageParameter[]
  /** Short notes rendered under the parameter table. */
  notes: string[]
}

const H3_DOC: VideoUsageDoc = {
  endpoint: '/v1/videos',
  parameters: [
    {
      name: 'model',
      type: 'string',
      required: true,
      defaultValue: 'MiniMax-H3',
      description: 'Model identifier for the request',
    },
    {
      name: 'prompt',
      type: 'string',
      required: true,
      description: 'Text prompt describing the video to generate',
    },
    {
      name: 'seconds',
      type: 'integer',
      range: '4 ~ 15',
      defaultValue: 5,
      description: 'Output video duration in seconds',
    },
    {
      name: 'size',
      type: 'enum',
      enumValues: ['768P', '1080P', '2K'],
      defaultValue: '768P',
      description: 'Output resolution tier',
    },
    {
      name: 'input_reference',
      type: 'string',
      description:
        'Optional. Image URL used as the first frame; when an image is supplied the output aspect ratio follows the image.',
    },
    {
      name: 'metadata.ratio',
      type: 'enum',
      enumValues: ['adaptive', '21:9', '16:9', '4:3', '1:1', '3:4', '9:16'],
      defaultValue: 'adaptive',
      description:
        'Optional. Output aspect ratio. Text-to-video needs an explicit ratio; image-to-video ignores it and uses the image.',
    },
    {
      name: 'metadata.reference_video',
      type: 'array',
      description:
        'Optional. Reference video URLs for multimodal reference-to-video; mutually exclusive with first/last frame images.',
    },
    {
      name: 'metadata.reference_audio',
      type: 'array',
      description:
        'Optional. Reference audio URLs (WAV or MP3, up to 15 seconds each); audio input is free.',
    },
  ],
  notes: [
    'The endpoint is asynchronous: creating a task returns an id, and the video URL appears once the task status becomes completed.',
    'Billing is per second of output video, at the unit price of the selected resolution. The first 5 input images in a task are free; each additional image is charged separately.',
    'Input video seconds are billed at the same per-second rate as the selected resolution. Audio input is free.',
    'Duration is 4 to 15 seconds. A request without a duration is billed as 5 seconds.',
    'Large media should be passed as a public URL: the request body limit is 64 MB and Base64 encoding adds about 33%.',
  ],
}

const VIDEO_USAGE_DOCS: Record<string, VideoUsageDoc> = {
  'minimax-h3': H3_DOC,
}

export function videoUsageDoc(modelName: string): VideoUsageDoc | null {
  if (!modelName) return null
  return VIDEO_USAGE_DOCS[modelName.trim().toLowerCase()] ?? null
}
