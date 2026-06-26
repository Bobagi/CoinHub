import './app.css'
import App from './App.svelte'
import { initAnalyticsConsent } from './lib/analytics'

// Non-essential scripts (analytics) load only after the user opts in via the cookie banner.
initAnalyticsConsent()

const app = new App({
  target: document.getElementById('app') as HTMLElement
})

export default app
