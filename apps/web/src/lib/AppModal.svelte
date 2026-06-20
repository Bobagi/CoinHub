<script lang="ts">
  // A single, styled modal mounted once at the app root. Driven by the `appModal` store, it replaces
  // window.alert: 'verify' shows the confirm-your-email dialog with a resend button; 'message' shows
  // an arbitrary notice; 'stepUp' re-confirms identity (password and/or Google) before money actions.
  import { appModal, closeModal, completeStepUp, cancelStepUp } from './stores'
  import { api, type StepUpStatus } from './api'
  import { t } from './i18n'

  let busy = false
  let resendMessage = ''

  // Step-up state.
  let stepUpStatus: StepUpStatus | null = null
  let stepUpLoaded = false
  let stepUpPassword = ''
  let stepUpError = ''

  // Load the available step-up methods (password / Google) when the dialog opens; reset when it closes.
  $: if ($appModal && $appModal.type === 'stepUp') {
    if (!stepUpLoaded) {
      stepUpLoaded = true
      loadStepUpStatus()
    }
  } else {
    stepUpLoaded = false
    stepUpStatus = null
    stepUpPassword = ''
    stepUpError = ''
  }

  async function loadStepUpStatus() {
    try {
      stepUpStatus = await api.stepUpStatus()
    } catch {
      stepUpStatus = { fresh: false, window_seconds: 0, password_method: true, google_method: false }
    }
  }

  async function confirmWithPassword() {
    if (!stepUpPassword) return
    busy = true
    stepUpError = ''
    try {
      await api.stepUpPassword(stepUpPassword)
      stepUpPassword = ''
      completeStepUp()
    } catch (e) {
      stepUpError = (e as Error).message
    } finally {
      busy = false
    }
  }

  function confirmWithGoogle() {
    // Open the Google re-confirm (prompt=login) in a POPUP, so this page — and the form the user
    // already filled in — stays alive. When the popup finishes it postMessages the opener, which
    // resolves the pending step-up and retries the original action transparently (nothing to re-type,
    // and the API secret never touches storage). If the popup is blocked we fall back to a full-page
    // redirect (the user then redoes the action on return).
    const popup = window.open('/auth/step-up/google/start', 'coinhub-stepup', 'width=480,height=680')
    if (!popup) {
      window.location.href = '/auth/step-up/google/start'
    }
  }

  async function resend() {
    busy = true
    resendMessage = ''
    try {
      resendMessage = (await api.resendVerification()).message
    } catch (e) {
      resendMessage = (e as Error).message
    } finally {
      busy = false
    }
  }

  function close() {
    if ($appModal && $appModal.type === 'stepUp') {
      cancelStepUp()
      return
    }
    resendMessage = ''
    closeModal()
  }

  // Esc closes the dialog (a11y). The card stops keydown propagation, so it handles Esc itself;
  // the window handler covers the case where focus is outside the card (e.g. on the backdrop/body).
  function onCardKeydown(event: KeyboardEvent) {
    event.stopPropagation()
    if (event.key === 'Escape') close()
  }
  function onWindowKeydown(event: KeyboardEvent) {
    if (event.key === 'Escape' && $appModal) close()
  }
</script>

<svelte:window on:keydown={onWindowKeydown} />

{#if $appModal}
  <div class="backdrop" role="presentation" on:click={close}>
    <!-- svelte-ignore a11y-no-noninteractive-element-interactions -->
    <div class="modal-card" role="dialog" aria-modal="true" on:click|stopPropagation on:keydown={onCardKeydown}>
      {#if $appModal.type === 'verify'}
        <span class="micon" aria-hidden="true">✉️</span>
        <h2 class="mtitle">{$t('wall.title')}</h2>
        <p class="mtext">{$t('verify.bannerText')}</p>
        {#if resendMessage}<p class="success">{resendMessage}</p>{/if}
        <div class="mactions">
          <button class="ghost btn-sm" disabled={busy} on:click={resend}>{busy ? $t('common.saving') : $t('verify.resend')}</button>
          <button class="btn-sm btn-primary" on:click={close}>{$t('modal.ok')}</button>
        </div>
      {:else if $appModal.type === 'stepUp'}
        <span class="micon" aria-hidden="true">🔐</span>
        <h2 class="mtitle">{$t('stepup.title')}</h2>
        <p class="mtext">{$t('stepup.intro')}</p>
        {#if stepUpStatus?.password_method}
          <form class="stepup-form" on:submit|preventDefault={confirmWithPassword}>
            <label class="stepup-label" for="stepup-password">{$t('stepup.passwordLabel')}</label>
            <!-- svelte-ignore a11y-autofocus -->
            <input
              id="stepup-password"
              type="password"
              autocomplete="current-password"
              autofocus
              bind:value={stepUpPassword}
              placeholder={$t('stepup.passwordPlaceholder')}
            />
            <button class="btn-sm btn-primary" type="submit" disabled={busy || !stepUpPassword}>
              {busy ? $t('common.saving') : $t('stepup.confirm')}
            </button>
          </form>
        {/if}
        {#if stepUpStatus?.password_method && stepUpStatus?.google_method}
          <div class="stepup-or">{$t('stepup.or')}</div>
        {/if}
        {#if stepUpStatus?.google_method}
          {#if !stepUpStatus?.password_method}
            <p class="mtext muted-hint">{$t('stepup.googleOnlyHint')}</p>
          {/if}
          <button class="ghost btn-sm" on:click={confirmWithGoogle}>{$t('stepup.googleButton')}</button>
        {/if}
        {#if stepUpError}<p class="error">{stepUpError}</p>{/if}
        <div class="mactions"><button class="ghost btn-sm" on:click={close}>{$t('stepup.cancel')}</button></div>
      {:else}
        <span class="micon" aria-hidden="true">🔒</span>
        <p class="mtext">{$appModal.text}</p>
        <div class="mactions"><button class="btn-sm btn-primary" on:click={close}>{$t('modal.ok')}</button></div>
      {/if}
    </div>
  </div>
{/if}

<style>
  .backdrop { position: fixed; inset: 0; z-index: 100; background: rgba(0, 0, 0, 0.55); display: grid; place-items: center; padding: var(--space-5); }
  .modal-card {
    background: var(--surface); border: 1px solid var(--border-strong); border-radius: var(--radius-md);
    padding: var(--space-5); max-width: 30rem; width: 100%; text-align: center;
    box-shadow: 0 16px 48px rgba(0, 0, 0, 0.5); display: flex; flex-direction: column; gap: var(--space-3); align-items: center;
  }
  .micon { font-size: 2rem; }
  .mtitle { font-size: var(--text-lg); margin: 0; }
  .mtext { color: var(--text); line-height: 1.5; margin: 0; }
  .mactions { display: flex; gap: var(--space-2); justify-content: center; flex-wrap: wrap; margin-top: var(--space-2); }
  .stepup-form { display: flex; flex-direction: column; gap: var(--space-2); width: 100%; }
  .stepup-label { font-size: var(--text-sm); color: var(--muted); text-align: left; }
  .stepup-form input { width: 100%; }
  .stepup-or { color: var(--muted); font-size: var(--text-sm); }
  .muted-hint { color: var(--muted); font-size: var(--text-sm); }
  .error { color: var(--red); font-size: var(--text-sm); margin: 0; }
  .success { color: var(--green); font-size: var(--text-sm); margin: 0; }
</style>
