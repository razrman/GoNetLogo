import Markdown from 'react-markdown'
import type { Components, UrlTransform } from 'react-markdown'
import rehypeRaw from 'rehype-raw'
import rehypeSanitize, { defaultSchema } from 'rehype-sanitize'
import remarkGfm from 'remark-gfm'
import type { PluggableList } from 'unified'

// NetLogo's Info tab is Markdown, including a little HTML for subscripts
// and images. rehype-sanitize drops scripts, event handlers, and unsafe
// URL schemes before anything is rendered. react-markdown does not run
// the source as code.
const infoSchema = {
  ...defaultSchema,
  protocols: {
    ...defaultSchema.protocols,
    src: withProtocol(defaultSchema.protocols?.src, 'file'),
    href: withProtocol(defaultSchema.protocols?.href, 'file'),
  },
}

const remarkPlugins: PluggableList = [remarkGfm]
const rehypePlugins: PluggableList = [rehypeRaw, [rehypeSanitize, infoSchema]]

const imageExt = /\.(png|jpe?g|gif|webp|bmp)$/i

function withProtocol(list: readonly string[] | null | undefined, protocol: string) {
  const next = [...(list ?? [])]
  if (!next.includes(protocol)) next.push(protocol)
  return next
}

// infoAssetURL turns a Markdown image address into a URL the browser can
// load. Remote http(s) images stay as written. A relative path or a
// NetLogo file: URL is served from the model folder. Anything else,
// including script URLs, is dropped.
function infoAssetURL(url: string, modelDir: string | null) {
  const value = url.trim()
  if (!value) return ''
  if (/^https?:\/\//i.test(value)) return value
  if (modelDir == null) return ''
  let rel = value.replace(/^file:/i, '')
  try {
    rel = decodeURIComponent(rel)
  } catch {
    return ''
  }
  if (rel.includes('\0') || /^[a-z][a-z0-9+.-]*:/i.test(rel)) return ''
  rel = rel.replace(/\\/g, '/').replace(/^\/+/, '')
  if (!imageExt.test(rel)) return ''
  const parts = rel.split('/')
  if (parts.some((part) => part === '' || part === '.' || part === '..')) return ''
  const base = modelDir.replace(/\\/g, '/').replace(/^\/+|\/+$/g, '')
  const segments = base ? base.split('/').concat(parts) : parts
  return '/model-asset/' + segments.map((part) => encodeURIComponent(part)).join('/')
}

function infoLinkURL(url: string, modelDir: string | null) {
  const value = url.trim()
  if (!value) return ''
  if (/^(https?:\/\/|mailto:)/i.test(value)) return value
  if (value.startsWith('#')) return value
  return infoAssetURL(value, modelDir)
}

function urlTransformFor(modelDir: string | null): UrlTransform {
  return (url, key) => {
    if (key === 'src') return infoAssetURL(url, modelDir)
    if (key === 'href') return infoLinkURL(url, modelDir)
    return ''
  }
}

export function InfoView({
  markdown,
  modelDir,
  loading,
}: {
  markdown: string
  modelDir: string | null
  loading?: boolean
}) {
  const text = markdown.trim()
  const components: Components = {
    a: ({ href, children }) => {
      if (!href) return <span>{children}</span>
      const here = href.startsWith('#')
      return (
        <a href={href} {...(here ? {} : { target: '_blank', rel: 'noopener noreferrer' })}>
          {children}
        </a>
      )
    },
    img: ({ src, alt, title }) => {
      const url = typeof src === 'string' ? src : ''
      if (!url) return alt ? <span className="info-missing">{alt}</span> : null
      return <img src={url} alt={alt ?? ''} title={title} />
    },
  }
  return (
    <div className="info-doc">
      {loading ? (
        <p className="empty">Loading info…</p>
      ) : text ? (
        <Markdown
          remarkPlugins={remarkPlugins}
          rehypePlugins={rehypePlugins}
          urlTransform={urlTransformFor(modelDir)}
          components={components}
        >
          {text}
        </Markdown>
      ) : (
        <p className="empty">This model has no info.</p>
      )}
    </div>
  )
}
