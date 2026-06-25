<script lang="ts">
  import { onMount, onDestroy } from 'svelte'
  import { get } from 'svelte/store'
  import { currentUser, route, showModalMessage, completeStepUp, cancelStepUp } from './lib/stores'
  import { api } from './lib/api'
  import { t } from './lib/i18n'
  import Login from './lib/Login.svelte'
  import Landing from './lib/Landing.svelte'
  import Dashboard from './lib/Dashboard.svelte'
  import AccountSettings from './lib/AccountSettings.svelte'
  import TopNav from './lib/TopNav.svelte'
  import ResetPassword from './lib/ResetPassword.svelte'
  import VerifyEmail from './lib/VerifyEmail.svelte'
  import AgreementGate from './lib/AgreementGate.svelte'
  import LegalDocument from './lib/LegalDocument.svelte'
  import CookieConsent from './lib/CookieConsent.svelte'
  import VerifyBanner from './lib/VerifyBanner.svelte'
  import StatusBanner from './lib/StatusBanner.svelte'
  import AppModal from './lib/AppModal.svelte'
  import Toasts from './lib/Toasts.svelte'

  let loading = true
  let emailEnabled = false

  // The Google step-up flow bounces back with ?step_up=ok|error|unavailable. Normally we are the
  // popup opened by the step-up modal: report the result to the opener (which retries the original
  // action) and close. If we are NOT a popup (popup was blocked → full-page redirect), fall back to
  // just showing a notice.
  function handleStepUpReturn() {
    if (typeof location === 'undefined') return
    const params = new URLSearchParams(location.search)
    const result = params.get('step_up')
    if (!result) return
    params.delete('step_up')
    const remaining = params.toString()
    history.replaceState(null, '', location.pathname + (remaining ? '?' + remaining : '') + location.hash)

    if (window.opener && window.opener !== window) {
      try {
        window.opener.postMessage({ type: 'coinhub-stepup', result }, location.origin)
      } catch {
        /* ignore */
      }
      window.close()
      return
    }

    const translate = get(t)
    if (result === 'ok') showModalMessage(translate('stepup.confirmedRetry'))
    else if (result === 'unavailable') showModalMessage(translate('stepup.unavailable'))
    else showModalMessage(translate('stepup.failed'))
  }

  // In the opener window: receive the popup's result. 'ok' resolves the pending step-up so the
  // original request is retried transparently; anything else cancels it and shows why.
  function onStepUpMessage(event: MessageEvent) {
    if (event.origin !== location.origin) return
    const data = event.data
    if (!data || data.type !== 'coinhub-stepup') return
    if (data.result === 'ok') {
      completeStepUp()
    } else {
      cancelStepUp()
      const translate = get(t)
      showModalMessage(translate(data.result === 'unavailable' ? 'stepup.unavailable' : 'stepup.failed'))
    }
  }

  onMount(async () => {
    handleStepUpReturn()
    window.addEventListener('message', onStepUpMessage)
    try {
      emailEnabled = (await api.getAuthProviders()).email
    } catch {
      emailEnabled = false
    }
    try {
      currentUser.set(await api.me())
    } catch {
      currentUser.set(null)
    } finally {
      loading = false
    }
  })

  onDestroy(() => {
    if (typeof window !== 'undefined') window.removeEventListener('message', onStepUpMessage)
  })
</script>

{#if $route === 'reset'}
  <ResetPassword />
{:else if $route === 'verify'}
  <VerifyEmail />
{:else if $route === 'terms'}
  <LegalDocument doc="terms" />
{:else if $route === 'privacy'}
  <LegalDocument doc="privacy" />
{:else if loading}
  <div class="center muted">{$t('app.loading')}</div>
{:else if $currentUser && !$currentUser.terms_accepted}
  <!-- Hard consent gate: no signed-in user reaches the app without an on-record acceptance of the
       current Terms of Use + Privacy Policy (email, Google and existing users alike). -->
  <AgreementGate />
{:else if $currentUser}
  <TopNav />
  <StatusBanner />
  {#if emailEnabled && !$currentUser.email_verified}
    <!-- Unverified accounts can look around, but every save is blocked (API 403 + styled modal). -->
    <VerifyBanner />
  {/if}
  {#if $route === 'account'}
    <AccountSettings />
  {:else}
    <Dashboard />
  {/if}
{:else if $route === 'login'}
  <Login />
{:else}
  <!-- Logged-out default: the public product landing. #/login shows the auth form. -->
  <Landing />
{/if}

<!-- Global styled dialog (confirm-email / locked-screen notices), mounted once. -->
<AppModal />

<!-- Global "popcorn" toast notifications (transient success/error feedback), mounted once. -->
<Toasts />

<!-- Cookie/advertising consent banner (LGPD). Dormant until ads are enabled (see stores.adsEnabled). -->
<CookieConsent />

<style>
  .center { display: grid; place-items: center; min-height: 100vh; }
</style>
