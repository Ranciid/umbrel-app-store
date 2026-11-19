import { useState } from 'react'
import { request } from '../api/core'

/** Admin-only UI. The API additionally enforces RequireAdmin. */
export default function SendToKindleButton({ bookId }: { bookId: number }) {
  const [busy, setBusy] = useState(false)
  const [message, setMessage] = useState('')

  const send = async (force: boolean): Promise<void> => {
    setBusy(true)
    setMessage('')
    try {
      await request('/kindle/send/' + bookId + (force ? '?force=1' : ''), { method: 'POST' })
      setMessage('Email accepted by SMTP. Check Amazon Docs and Kindle Sync.')
    } catch (err) {
      const e = err as { status?: number; message?: string }
      if (e.status === 409 && !force && window.confirm('This book was already sent or attempted. Send it again?')) {
        setBusy(false)
        return send(true)
      }
      setMessage(e.message || 'Unable to send to Kindle')
    } finally { setBusy(false) }
  }

  return (
    <span className="inline-flex flex-wrap items-center gap-2">
      <button type="button" disabled={busy} onClick={() => void send(false)}
        className="rounded bg-emerald-700 px-2.5 py-1 text-xs font-medium text-white hover:bg-emerald-600 disabled:opacity-50">
        {busy ? 'Sending…' : 'Send to Kindle'}
      </button>
      {message && <span role="status" className="text-xs text-fg-muted">{message}</span>}
    </span>
  )
}
