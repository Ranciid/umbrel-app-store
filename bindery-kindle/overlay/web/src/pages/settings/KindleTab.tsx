import { useEffect, useState } from 'react'
import { request } from '../../api/core'

type KindleConfig = {
  host: string
  port: number
  login: string
  from: string
  recipients: string
  auto: boolean
  passwordSet: boolean
}
type Delivery = {
  bookId: number
  title: string
  recipient: string
  status: string
  error: string
  updatedAt: string
}

const inputClass = 'w-full rounded border border-slate-300 bg-white px-3 py-2 text-sm text-slate-900 dark:bg-zinc-900 dark:border-zinc-700 dark:text-zinc-100'
const buttonClass = 'rounded bg-emerald-700 hover:bg-emerald-600 px-4 py-2 text-sm font-medium text-white disabled:opacity-50'

export default function KindleTab() {
  const [config, setConfig] = useState<KindleConfig>({
    host: 'mail.smtp2go.com', port: 587, login: '', from: '', recipients: '', auto: false, passwordSet: false,
  })
  const [password, setPassword] = useState('')
  const [loading, setLoading] = useState(true)
  const [busy, setBusy] = useState(false)
  const [status, setStatus] = useState('')
  const [deliveries, setDeliveries] = useState<Delivery[]>([])

  const reload = () => {
    request<Delivery[]>('/kindle/history').then(setDeliveries).catch(() => {})
  }
  useEffect(() => {
    request<KindleConfig>('/kindle/config').then(setConfig)
      .catch((e: unknown) => setStatus(e instanceof Error ? e.message : 'Could not load Kindle settings'))
      .finally(() => setLoading(false))
    reload()
  }, [])

  const update = <K extends keyof KindleConfig>(key: K, value: KindleConfig[K]) =>
    setConfig(prev => ({ ...prev, [key]: value }))

  async function save() {
    setBusy(true)
    setStatus('')
    try {
      await request('/kindle/config', {
        method: 'PUT',
        body: JSON.stringify({
          host: config.host.trim(), port: config.port, login: config.login.trim(),
          password, from: config.from.trim(), recipients: config.recipients.trim(), auto: config.auto,
        }),
      })
      setConfig(await request<KindleConfig>('/kindle/config'))
      setPassword('')
      setStatus('Settings saved. Auto-send only applies to newly imported files from this point forward.')
    } catch (e) {
      setStatus(e instanceof Error ? e.message : 'Unable to save Kindle settings')
    } finally { setBusy(false) }
  }

  async function test() {
    setBusy(true)
    setStatus('')
    try {
      const r = await request<{message: string}>('/kindle/test', { method: 'POST' })
      setStatus(r.message)
    } catch (e) {
      setStatus(e instanceof Error ? e.message : 'Test failed')
    } finally { setBusy(false) }
  }

  if (loading) return <div className="text-fg-muted">Loading Kindle settings…</div>
  return (
    <section className="space-y-6 max-w-2xl">
      <div>
        <h3 className="text-lg font-semibold">Send to Kindle</h3>
        <p className="text-sm text-fg-muted mt-1">
          Send EPUB or PDF ebooks directly from Bindery through your SMTP provider.
          This runs inside Bindery; no Calibre, external companion app or extra container required.
        </p>
      </div>
      <div className="rounded-lg border border-amber-300 bg-amber-50 p-3 text-sm text-amber-900 dark:border-amber-900 dark:bg-amber-950/20 dark:text-amber-200">
        To avoid duplicates, disable the old Calibre/CWA handoff and change General → File Naming → Ebook
        import mode to Auto. Auto-send starts with newly imported files; old books can be sent manually.
        An SMTP acceptance means the email was accepted, not that Amazon delivered it to the device.
      </div>
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
        <label className="text-sm">SMTP hostname
          <input className={inputClass} value={config.host} onChange={e => update('host', e.target.value)} placeholder="mail.smtp2go.com" />
        </label>
        <label className="text-sm">SMTP port / encryption
          <select className={inputClass} value={config.port} onChange={e => update('port', Number(e.target.value))}>
            <option value={587}>587 — STARTTLS</option>
            <option value={465}>465 — TLS</option>
          </select>
        </label>
        <label className="text-sm">SMTP username
          <input className={inputClass} autoComplete="off" value={config.login} onChange={e => update('login', e.target.value)} />
        </label>
        <label className="text-sm">SMTP password
          <input className={inputClass} type="password" autoComplete="new-password"
            value={password} onChange={e => setPassword(e.target.value)}
            placeholder={config.passwordSet ? 'Configured — leave blank to keep' : 'Your provider SMTP password'} />
        </label>
        <label className="text-sm">Verified From address
          <input className={inputClass} type="email" value={config.from} onChange={e => update('from', e.target.value)}
            placeholder="books@example.com" />
        </label>
        <label className="text-sm">Kindle delivery address(es)
          <input className={inputClass} value={config.recipients} onChange={e => update('recipients', e.target.value)}
            placeholder="mykindle@kindle.com" />
          <span className="text-xs text-fg-muted">Up to 8 addresses, separated with commas. Approve the From address in Amazon Personal Document Settings.</span>
        </label>
      </div>
      <label className="flex items-center gap-3 text-sm">
        <input type="checkbox" checked={config.auto} onChange={e => update('auto', e.target.checked)} />
        Automatically send newly imported EPUB/PDF books
      </label>
      <div className="flex flex-wrap gap-2">
        <button type="button" className={buttonClass} disabled={busy} onClick={save}>Save Kindle settings</button>
        <button type="button" className="rounded border border-slate-400 px-4 py-2 text-sm disabled:opacity-50"
          disabled={busy} onClick={test}>Send SMTP test to From address</button>
        <button type="button" className="rounded border border-slate-400 px-4 py-2 text-sm" onClick={reload}>Refresh history</button>
      </div>
      {status && <p className="text-sm rounded border border-slate-300 dark:border-zinc-700 p-3" role="status">{status}</p>}
      <section className="space-y-3">
        <h4 className="font-semibold">Delivery history</h4>
        <p className="text-xs text-fg-muted">Failed and uncertain sends require manual retry to prevent repeated emails.
          Use Send to Kindle on a book page, and explicitly confirm a resend.</p>
        {deliveries.length===0 ? <p className="text-sm text-fg-muted">No delivery attempts yet.</p> :
          <div className="divide-y divide-slate-200 dark:divide-zinc-800">
            {deliveries.map((d, i) => <div key={i} className="py-2 text-sm">
              <div className="font-medium">{d.title}</div>
              <div className="text-xs text-fg-muted break-all">
                {d.recipient} · {d.status==='smtp_accepted' ? 'Accepted by SMTP (not Kindle-confirmed)' : d.status} · {d.updatedAt}
              </div>
              {d.error && <div className="text-xs text-amber-700 dark:text-amber-300">{d.error}</div>}
            </div>)}
          </div>}
      </section>
    </section>
  )
}
