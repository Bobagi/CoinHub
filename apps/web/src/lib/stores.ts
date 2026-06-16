import { writable } from 'svelte/store'
import type { User } from './api'

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

// Minimal hash-based routing — enough for the authenticated views + the email-link pages, without
// pulling in a router. `reset` and `verify` are reached from email links (#/reset?token=…).
export type Route = 'dashboard' | 'account' | 'reset' | 'verify'

function pathFromHash(): string {
  if (typeof location === 'undefined') return ''
  return location.hash.replace(/^#\/?/, '').split('?')[0]
}

function routeFromHash(): Route {
  switch (pathFromHash()) {
    case 'settings':
      return 'account'
    case 'reset':
      return 'reset'
    case 'verify':
      return 'verify'
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

export function navigate(to: Route) {
  const hash = to === 'account' ? '#/settings' : '#/'
  if (typeof location !== 'undefined' && location.hash !== hash) location.hash = hash
  route.set(to)
}

if (typeof window !== 'undefined') {
  window.addEventListener('hashchange', () => route.set(routeFromHash()))
}
