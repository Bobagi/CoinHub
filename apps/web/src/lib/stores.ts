import { writable, get } from 'svelte/store'
import type { User } from './api'
import { t, translateError } from './i18n'

export const currentUser = writable<User | null>(null)

// A single global, styled modal (replaces window.alert): 'verify' renders the confirm-your-email
// dialog (with a resend button), 'message' renders an arbitrary text (e.g. a locked-screen notice),
// 'stepUp' renders the re-confirm-identity dialog used before money-related actions.
export type AppModal = { type: 'verify' } | { type: 'message'; text: string } | { type: 'stepUp' }
export const appModal = writable<AppModal | null>(null)
export function showVerifyModal() {
  appModal.set({ type: 'verify' })
}
export function showModalMessage(text: string) {
  appModal.set({ type: 'message', text })
}
export function closeModal() {
  appModal.set(null)
}

// Toasts — small self-dismissing "popcorn" notifications (success / error / info) stacked in a
// corner. Used for transient action feedback (a sale placed, a trade rejected) instead of inline
// text that's easy to miss. Rendered once globally by Toasts.svelte.
export type ToastKind = 'success' | 'error' | 'info'
export interface Toast {
  id: number
  kind: ToastKind
  text: string
}
export const toasts = writable<Toast[]>([])
let toastSequence = 0

export function dismissToast(id: number) {
  toasts.update((list) => list.filter((toast) => toast.id !== id))
}

// pushToast shows a toast and auto-dismisses it after durationMs (0 = stay until clicked). Errors
// linger a bit longer by default so they can be read. Returns the toast id.
export function pushToast(text: string, kind: ToastKind = 'info', durationMs = kind === 'error' ? 7000 : 4500): number {
  const id = ++toastSequence
  toasts.update((list) => [...list, { id, kind, text }])
  if (durationMs > 0 && typeof setTimeout !== 'undefined') {
    setTimeout(() => dismissToast(id), durationMs)
  }
  return id
}

// notifyError shows any caught error as a toast — the single place catch blocks should route errors
// to, instead of inline text. It localizes via translateError (so coded backend errors are
// translated), shows a gentle info toast when the user simply cancelled a step-up re-auth, and
// silently ignores the internal "superseded" rejection.
export function notifyError(error: unknown) {
  const candidate = error as { code?: string; message?: string } | null
  const message = candidate?.message ?? ''
  if (candidate?.code === 'step_up_superseded' || message === 'step-up superseded') return
  const translate = get(t)
  if (candidate?.code === 'step_up_cancelled' || message === 'step-up cancelled') {
    pushToast(translate('toast.actionCancelled'), 'info')
    return
  }
  pushToast(translateError(translate, error), 'error')
}

// Step-up ("sudo") re-authentication. requestStepUp() opens the step-up modal and returns a promise
// that resolves once the user re-confirms with their password (transparent retry), or rejects if
// they cancel. The Google path navigates away, so that promise simply never resolves in this page
// load — the user lands back with ?step_up=ok and redoes the action.
let stepUpResolve: (() => void) | null = null
let stepUpReject: ((reason?: unknown) => void) | null = null

export function requestStepUp(): Promise<void> {
  // If one is already pending, cancel it before starting a fresh one.
  if (stepUpReject) stepUpReject(new Error('step-up superseded'))
  return new Promise<void>((resolve, reject) => {
    stepUpResolve = resolve
    stepUpReject = reject
    appModal.set({ type: 'stepUp' })
  })
}

export function completeStepUp() {
  const resolve = stepUpResolve
  stepUpResolve = null
  stepUpReject = null
  appModal.set(null)
  if (resolve) resolve()
}

export function cancelStepUp() {
  const reject = stepUpReject
  stepUpResolve = null
  stepUpReject = null
  appModal.set(null)
  if (reject) reject(new Error('step-up cancelled'))
}

// Binance connection status, surfaced in the top nav. The Dashboard populates it after loading
// credentials so the header can show the active environment from anywhere.
export const binanceStatus = writable<{ has_active_credential: boolean; active_environment: string } | null>(null)

// Operational status (worker liveness + Binance rate-limit gate), polled by TopNav. When
// `operational` is false the header indicator turns red and explains why the bots are paused. Null =
// not loaded yet (treated as operational, so a slow first fetch never flashes a false alarm).
export const systemStatus = writable<{ operational: boolean; reasons: { code: string; retry_seconds?: number }[] } | null>(null)

// Minimal hash-based routing — enough for the authenticated views + the email-link pages, without
// pulling in a router. `reset` and `verify` are reached from email links (#/reset?token=…).
export type Route = 'dashboard' | 'account' | 'reset' | 'verify' | 'terms' | 'privacy' | 'login'

// Which auth form the public Login page opens on. Set by the landing-page CTAs before navigating
// to #/login ("Sign in" → login tab, "Create account" → signup tab).
export const authMode = writable<'login' | 'signup'>('login')

function pathFromHash(): string {
  if (typeof location === 'undefined') return ''
  return location.hash.replace(/^#\/?/, '').split('?')[0]
}

function routeFromHash(): Route {
  switch (pathFromHash()) {
    case 'settings':
      return 'account'
    case 'login':
      return 'login'
    case 'reset':
      return 'reset'
    case 'verify':
      return 'verify'
    case 'terms':
      return 'terms'
    case 'privacy':
      return 'privacy'
    default:
      return 'dashboard'
  }
}

// hashToken extracts ?token=… from the current hash (used by the reset/verify pages).
export function hashToken(): string {
  if (typeof location === 'undefined') return ''
  const questionMarkIndex = location.hash.indexOf('?')
  if (questionMarkIndex < 0) return ''
  return new URLSearchParams(location.hash.slice(questionMarkIndex + 1)).get('token') ?? ''
}

export const route = writable<Route>(routeFromHash())

const routeHashes: Partial<Record<Route, string>> = {
  account: '#/settings',
  terms: '#/terms',
  privacy: '#/privacy',
  login: '#/login'
}

export function navigate(to: Route) {
  const hash = routeHashes[to] ?? '#/'
  if (typeof location !== 'undefined' && location.hash !== hash) location.hash = hash
  route.set(to)
  scrollToTop()
}

// Each route is a distinct top-level page (terms, privacy, account, …); always start it at the top
// rather than inheriting the previous page's scroll position.
function scrollToTop() {
  if (typeof window !== 'undefined') window.scrollTo(0, 0)
}

if (typeof window !== 'undefined') {
  // Covers in-app navigate() AND browser back/forward / direct hash edits.
  window.addEventListener('hashchange', () => { route.set(routeFromHash()); scrollToTop() })
}

// --- Display currency ----------------------------------------------------------------------------
// The one currency all AGGREGATE money (profitability cards, allocation totals) is converted into at
// current market rates. '' = auto: the Dashboard resolves it to the user's dominant quote currency
// (per-row amounts always stay in the pair's own quote currency). Persisted so it survives reloads.
const DISPLAY_CURRENCY_KEY = 'coinhub_display_currency'

function readDisplayCurrency(): string {
  if (typeof localStorage === 'undefined') return ''
  return (localStorage.getItem(DISPLAY_CURRENCY_KEY) || '').toUpperCase()
}

export const displayCurrency = writable<string>(readDisplayCurrency())

export function setDisplayCurrency(code: string) {
  const normalized = (code || '').toUpperCase()
  if (typeof localStorage !== 'undefined') {
    if (normalized) localStorage.setItem(DISPLAY_CURRENCY_KEY, normalized)
    else localStorage.removeItem(DISPLAY_CURRENCY_KEY)
  }
  displayCurrency.set(normalized)
}

// --- Cookie / non-essential script consent (LGPD) ----------------------------------------------
// CoinHub sets only one strictly-necessary cookie (the session), which doesn't require consent.
// Everything NON-essential is opt-in and must be gated on `cookieConsent === 'accepted'`:
//   - analyticsEnabled: self-hosted Umami product analytics (cookieless, but still opt-in). Loaded
//     by lib/analytics.ts only after acceptance.
//   - adsEnabled: third-party advertising/tracking (not turned on yet).
// The banner (CookieConsent.svelte) is shown whenever a non-essential script needs consent and the
// user hasn't decided. The user's choice persists in localStorage; it can be withdrawn/changed via
// resetCookieConsent() ("Manage cookies").
export const analyticsEnabled = true
export const adsEnabled = false
export const consentRequired = analyticsEnabled || adsEnabled

export type CookieChoice = 'accepted' | 'rejected'
const COOKIE_CONSENT_KEY = 'coinhub_cookie_consent'

function readCookieConsent(): CookieChoice | null {
  if (typeof localStorage === 'undefined') return null
  const stored = localStorage.getItem(COOKIE_CONSENT_KEY)
  return stored === 'accepted' || stored === 'rejected' ? stored : null
}

export const cookieConsent = writable<CookieChoice | null>(readCookieConsent())

export function setCookieConsent(choice: CookieChoice) {
  if (typeof localStorage !== 'undefined') localStorage.setItem(COOKIE_CONSENT_KEY, choice)
  cookieConsent.set(choice)
}

// Withdraw/change a previous choice (LGPD: revoking consent must be as easy as giving it). Clears the
// stored decision so the banner reappears. Callers reload the page afterwards so any already-loaded
// non-essential script (e.g. Umami injected earlier this session) is dropped until the user re-decides.
export function resetCookieConsent() {
  if (typeof localStorage !== 'undefined') localStorage.removeItem(COOKIE_CONSENT_KEY)
  cookieConsent.set(null)
}
