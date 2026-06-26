// Consent-gated analytics loader (LGPD).
//
// CoinHub uses a single self-hosted, privacy-friendly Umami instance for product analytics. Even
// though Umami is cookieless, it still processes connection data, so it is treated as a NON-essential
// script: it must NOT run until the visitor has explicitly accepted via the cookie banner. The static
// <script> tag was removed from index.html; this module injects it only when consent === 'accepted'.
//
// Rejecting (or simply not deciding) means the script is never added to the page, so no analytics
// request is ever made. A withdrawn consent takes full effect on the next load (the script is not
// re-injected); see resetCookieConsent() in stores.ts.
import { cookieConsent } from './stores'

const UMAMI_SRC = 'https://analytics.bobagi.space/script.js'
const UMAMI_WEBSITE_ID = '01c69fe8-2417-491a-b4a9-b9b368c6ad8d'

let injected = false

function injectUmami() {
  if (injected || typeof document === 'undefined') return
  // Guard against a double-inject across hot reloads / repeated store emissions.
  if (document.querySelector('script[data-coinhub-analytics]')) {
    injected = true
    return
  }
  const script = document.createElement('script')
  script.async = true
  script.src = UMAMI_SRC
  script.setAttribute('data-website-id', UMAMI_WEBSITE_ID)
  script.setAttribute('data-coinhub-analytics', '')
  document.head.appendChild(script)
  injected = true
}

// Wire analytics to the consent state. Loads Umami once, and only once the user has accepted
// non-essential cookies. Returning visitors who accepted on a previous session have their choice in
// localStorage (read by cookieConsent's initial value), so analytics loads immediately for them too.
export function initAnalyticsConsent() {
  cookieConsent.subscribe((choice) => {
    if (choice === 'accepted') injectUmami()
  })
}
