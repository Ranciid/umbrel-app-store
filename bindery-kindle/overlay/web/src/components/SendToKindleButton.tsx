import { useEffect, useRef, useState } from 'react'
import { Link } from 'react-router'
import { request } from '../api/core'

type KindleRecipientsResponse = { recipients: string[] }
type KindleRecipientOutcome = {
  recipient: string
  status: 'smtp_accepted' | 'already_attempted' | 'failed'
  error?: string
}
type KindleDeliveryResponse = {
  results: KindleRecipientOutcome[]
  message: string
}

/**
 * Manual delivery targets only the explicitly selected configured Kindle
 * addresses. The server independently checks the selection against its stored
 * allowlist. Automatic delivery is intentionally unaffected.
 */
export default function SendToKindleButton({ bookId }: { bookId: number }) {
  const [open, setOpen] = useState(false)
  const [loading, setLoading] = useState(false)
  const [busy, setBusy] = useState(false)
  const [destinations, setDestinations] = useState<string[]>([])
  const [selected, setSelected] = useState<string[]>([])
  const [results, setResults] = useState<KindleRecipientOutcome[]>([])
  const [message, setMessage] = useState('')
  const closeRef = useRef<HTMLButtonElement>(null)

  useEffect(() => {
    if (!open) return
    const escape = (event: KeyboardEvent) => {
      if (event.key === 'Escape' && !busy) setOpen(false)
    }
    document.addEventListener('keydown', escape)
    return () => document.removeEventListener('keydown', escape)
  }, [open, busy])

  useEffect(() => {
    if (open && !loading) closeRef.current?.focus()
  }, [open, loading])

  const openPicker = async () => {
    setOpen(true)
    setLoading(true)
    setDestinations([])
    setSelected([])
    setResults([])
    setMessage('')
    try {
      const data = await request<KindleRecipientsResponse>('/kindle/recipients')
      setDestinations(data.recipients)
    } catch (error) {
      setMessage(error instanceof Error ? error.message : 'Unable to load configured Kindle devices')
    } finally {
      setLoading(false)
    }
  }

  const toggle = (email: string) => {
    setSelected(current => current.includes(email) ?
      current.filter(x => x !== email) : [...current, email])
  }

  const send = async (recipients: string[], force = false) => {
    if (recipients.length === 0) return
    setBusy(true)
    setMessage('')
    try {
      const response = await request<KindleDeliveryResponse>(
        '/kindle/send/' + bookId + (force ? '?force=1' : ''),
        { method: 'POST', body: JSON.stringify({ recipients }) },
      )
      setResults(previous => [
        ...previous.filter(item => !recipients.includes(item.recipient)),
        ...response.results,
      ])
      setMessage(response.message)
    } catch (error) {
      setMessage(error instanceof Error ? error.message : 'Unable to send to Kindle')
    } finally {
      setBusy(false)
    }
  }

  const retryTargets = results
    .filter(item => item.status === 'already_attempted' || item.status === 'failed')
    .map(item => item.recipient)

  const retry = () => {
    if (retryTargets.length === 0) return
    if (!window.confirm(`Re-send this book to ${retryTargets.join(', ')}? This may create duplicate Kindle documents.`)) return
    void send(retryTargets, true)
  }

  return (
    <div className="inline-flex items-center gap-2">
      <button type="button" onClick={() => void openPicker()}
        className="rounded bg-emerald-700 px-2.5 py-1 text-xs font-medium text-white hover:bg-emerald-600">
        Send to Kindle
      </button>

      {open && (
        <div className="fixed inset-0 z-[100] flex items-center justify-center bg-black/60 p-4">
          <section role="dialog" aria-modal="true" aria-labelledby="kindle-device-picker-title"
            className="w-full max-w-lg max-h-[85vh] overflow-y-auto rounded-lg bg-white p-5 shadow-xl text-slate-900 dark:bg-zinc-900 dark:text-zinc-100">
            <div className="flex items-start justify-between gap-3">
              <div>
                <h2 id="kindle-device-picker-title" className="text-lg font-semibold">Send to Kindle</h2>
                <p className="mt-1 text-sm text-fg-muted">Choose which configured Kindle addresses receive this book.</p>
              </div>
              <button ref={closeRef} type="button" aria-label="Close Kindle device picker"
                disabled={busy} onClick={() => setOpen(false)}
                className="rounded border border-slate-300 px-2 py-1 text-sm dark:border-zinc-700 disabled:opacity-50">
                Close
              </button>
            </div>

            {loading ? (
              <p className="mt-5 text-sm text-fg-muted">Loading your Kindle addresses…</p>
            ) : destinations.length === 0 ? (
              <div className="mt-5 text-sm">
                <p>No Kindle addresses are configured yet.</p>
                <Link to="/settings?tab=kindle" className="mt-2 inline-block text-emerald-700 dark:text-emerald-400 underline"
                  onClick={() => setOpen(false)}>
                  Configure Kindle recipients
                </Link>
              </div>
            ) : (
              <>
                <fieldset className="mt-5 space-y-2" disabled={busy}>
                  <legend className="mb-2 text-sm font-semibold">Choose one or more recipients</legend>
                  {destinations.map(email => (
                    <label key={email} className="flex cursor-pointer items-center gap-3 rounded border border-slate-200 p-3 hover:bg-slate-50 dark:border-zinc-700 dark:hover:bg-zinc-800">
                      <input type="checkbox" checked={selected.includes(email)} onChange={() => toggle(email)} />
                      <span className="min-w-0">
                        <span className="block font-medium">{email.split('@')[0]}</span>
                        <span className="block text-xs text-fg-muted break-all">{email}</span>
                      </span>
                    </label>
                  ))}
                </fieldset>
                <div className="mt-4 flex flex-wrap items-center gap-2">
                  <button type="button" disabled={busy || selected.length === 0}
                    onClick={() => void send(selected)}
                    className="rounded bg-emerald-700 px-4 py-2 text-sm font-medium text-white hover:bg-emerald-600 disabled:opacity-50">
                    {busy ? 'Sending…' : `Send to ${selected.length} Kindle${selected.length === 1 ? '' : 's'}`}
                  </button>
                  {retryTargets.length > 0 && (
                    <button type="button" disabled={busy} onClick={retry}
                      className="rounded border border-amber-600 px-3 py-2 text-sm text-amber-700 dark:text-amber-400 disabled:opacity-50">
                      Confirm resend ({retryTargets.length})
                    </button>
                  )}
                </div>
                {results.length > 0 && (
                  <div className="mt-4 space-y-1" aria-live="polite">
                    <h3 className="text-sm font-semibold">Delivery attempts</h3>
                    {results.map(result => (
                      <p key={result.recipient} className="text-sm break-words">
                        <span className="font-medium">{result.recipient}:</span>{' '}
                        {result.status === 'smtp_accepted' ? 'Accepted by SMTP (Amazon processing not confirmed)'
                          : result.status === 'already_attempted' ? 'Already attempted — skipped to prevent duplicates'
                          : `Failed — ${result.error || 'check the Bindery logs'}`}
                      </p>
                    ))}
                  </div>
                )}
              </>
            )}
            {message && <p role="status" className="mt-4 text-xs text-fg-muted">{message}</p>}
          </section>
        </div>
      )}
    </div>
  )
}
