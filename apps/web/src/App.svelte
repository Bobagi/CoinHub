<script lang="ts">
  import { onMount } from 'svelte'
  import { get } from 'svelte/store'
  import { currentUser, route, showModalMessage } from './lib/stores'
  import { api } from './lib/api'
  import { t } from './lib/i18n'
  import Login from './lib/Login.svelte'
  import Dashboard from './lib/Dashboard.svelte'
  import AccountSettings from './lib/AccountSettings.svelte'
  import TopNav from './lib/TopNav.svelte'
  import ResetPassword from './lib/ResetPassword.svelte'
  import VerifyEmail from './lib/VerifyEmail.svelte'
  import VerifyBanner from './lib/VerifyBanner.svelte'
  import AppModal from './lib/AppModal.svelte'

  let loading = true
  let emailEnabled = false

  // The Google step-up flow bounces back here with ?step_up=ok|error|unavailable. Show a notice and
  // strip the param so a refresh does not repeat it.
  function handleStepUpReturn() {
    if (typeof location === 'undefined') return
    const params = new URLSearchParams(location.search)
    const result = params.get('step_up')
    if (!result) return
    params.delete('step_up')
    const remaining = params.toString()
    history.replaceState(null, '', location.pathname + (remaining ? '?' + remaining : '') + location.hash)
    const translate = get(t)
    if (result === 'ok') showModalMessage(translate('stepup.confirmedRetry'))
    else if (result === 'unavailable') showModalMessage(translate('stepup.unavailable'))
    else showModalMessage(translate('stepup.failed'))
  }

  onMount(async () => {
    handleStepUpReturn()
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
</script>

{#if $route === 'reset'}
  <ResetPassword />
{:else if $route === 'verify'}
  <VerifyEmail />
{:else if loading}
  <div class="center muted">{$t('app.loading')}</div>
{:else if $currentUser}
  <TopNav />
  {#if emailEnabled && !$currentUser.email_verified}
    <!-- Unverified accounts can look around, but every save is blocked (API 403 + styled modal). -->
    <VerifyBanner />
  {/if}
  {#if $route === 'account'}
    <AccountSettings />
  {:else}
    <Dashboard />
  {/if}
{:else}
  <Login />
{/if}

<!-- Global styled dialog (confirm-email / locked-screen notices), mounted once. -->
<AppModal />

<style>
  .center { display: grid; place-items: center; min-height: 100vh; }
</style>
