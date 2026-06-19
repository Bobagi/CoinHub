<script lang="ts">
  import { api, type AccessEvent } from './api'
  import { currentUser, navigate, notifyError } from './stores'
  import { t, intlLocale } from './i18n'
  import LanguageDropdown from './LanguageDropdown.svelte'
  import Pagination from './Pagination.svelte'
  import CountryFlag from './CountryFlag.svelte'

  let accessEvents: AccessEvent[] = []
  let accessTotal = 0
  let accessPage = 1
  let accessPageSize = 10
  let accessBusy = false
  let accessRequestToken = 0

  // Server-side paging: reload whenever the page or page size changes (and once on mount).
  $: loadAccessHistory(accessPage, accessPageSize)

  async function loadAccessHistory(page: number, pageSize: number) {
    const token = ++accessRequestToken
    accessBusy = true
    try {
      const result = await api.getAccessHistory(page, pageSize)
      if (token !== accessRequestToken) return // a newer request superseded this one
      accessEvents = result.events
      accessTotal = result.total
    } catch (error) {
      if (token === accessRequestToken) notifyError(error)
    } finally {
      if (token === accessRequestToken) accessBusy = false
    }
  }

  function formatAccessTime(timestamp: string): string {
    return new Date(timestamp).toLocaleString($intlLocale, {
      year: 'numeric', month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit'
    })
  }

  // Best-effort friendly label from the raw user agent (browser · OS); falls back to the raw string.
  function deviceLabel(userAgent: string): string {
    if (!userAgent) return '—'
    const os = /Windows/i.test(userAgent) ? 'Windows'
      : /Android/i.test(userAgent) ? 'Android'
      : /iPhone|iPad|iPod|iOS/i.test(userAgent) ? 'iOS'
      : /Mac OS X|Macintosh/i.test(userAgent) ? 'macOS'
      : /Linux/i.test(userAgent) ? 'Linux' : ''
    const browser = /Edg\//i.test(userAgent) ? 'Edge'
      : /OPR\/|Opera/i.test(userAgent) ? 'Opera'
      : /Chrome\//i.test(userAgent) ? 'Chrome'
      : /Firefox\//i.test(userAgent) ? 'Firefox'
      : /Safari\//i.test(userAgent) ? 'Safari' : ''
    const parts = [browser, os].filter(Boolean)
    return parts.length ? parts.join(' · ') : userAgent
  }

  // "City, Region, Country" from the resolved geo fields (dropping empty parts); '—' when unknown.
  function locationLabel(event: AccessEvent): string {
    const parts = [event.city, event.region, event.country_name].filter((part) => part && part.trim())
    return parts.length ? parts.join(', ') : '—'
  }

  function methodLabel(method: string): string {
    switch (method) {
      case 'PASSWORD': return $t('account.access.methodPassword')
      case 'GOOGLE': return $t('account.access.methodGoogle')
      case 'SIGNUP': return $t('account.access.methodSignup')
      default: return method
    }
  }

  let name = $currentUser?.display_name ?? ''
  let profileMsg = ''
  let profileErr = ''
  let profileBusy = false

  let currentPassword = ''
  let newPassword = ''
  let confirmPassword = ''
  let passwordMsg = ''
  let passwordErr = ''
  let passwordBusy = false

  let deletePassword = ''
  let deleteConfirm = ''
  let deleteErr = ''
  let deleteBusy = false

  $: user = $currentUser
  $: hasPassword = user?.has_password ?? false
  $: email = user?.email ?? ''
  $: memberSince = user?.created_at
    ? new Date(user.created_at).toLocaleDateString($intlLocale, { year: 'numeric', month: 'long', day: 'numeric' })
    : ''
  // Google-only accounts confirm deletion by retyping their email; password accounts re-enter it.
  $: canDelete = hasPassword ? deletePassword.length > 0 : deleteConfirm.trim().toLowerCase() === email.toLowerCase()

  async function saveProfile() {
    profileBusy = true
    profileMsg = ''
    profileErr = ''
    try {
      currentUser.set(await api.updateProfile(name))
      profileMsg = $t('account.saved')
    } catch (error) {
      notifyError(error)
    } finally {
      profileBusy = false
    }
  }

  async function savePassword() {
    passwordMsg = ''
    passwordErr = ''
    if (newPassword !== confirmPassword) {
      passwordErr = $t('account.password.mismatch')
      return
    }
    passwordBusy = true
    try {
      await api.changePassword(currentPassword, newPassword)
      currentPassword = ''
      newPassword = ''
      confirmPassword = ''
      passwordMsg = $t('account.password.saved')
      // Refresh so a Google-only account flips to "has password" (change vs set).
      currentUser.set(await api.me())
    } catch (error) {
      notifyError(error)
    } finally {
      passwordBusy = false
    }
  }

  async function deleteAccount() {
    deleteBusy = true
    deleteErr = ''
    try {
      await api.deleteAccount(deletePassword)
      currentUser.set(null)
      navigate('dashboard')
    } catch (error) {
      notifyError(error)
      deleteBusy = false
    }
  }
</script>

<main class="page stack-lg">
  <div class="head">
    <button class="ghost btn-sm" type="button" on:click={() => navigate('dashboard')}>← {$t('nav.backToDashboard')}</button>
    <h1>{$t('account.title')}</h1>
    <p class="muted">{$t('account.subtitle')}{memberSince ? ` · ${$t('account.memberSince', { date: memberSince })}` : ''}</p>
  </div>

  <section class="card">
    <div class="card-header">
      <span class="card-title">{$t('account.profile.title')}</span>
      <span class="card-subtitle">{$t('account.profile.subtitle')}</span>
    </div>
    <div class="field">
      <span class="field-label">{$t('account.email')}</span>
      <input value={email} disabled />
      <span class="muted">{$t('account.emailLocked')}</span>
    </div>
    <div class="field">
      <label for="display-name">{$t('account.name')}</label>
      <input id="display-name" bind:value={name} placeholder={$t('account.namePlaceholder')} maxlength="120" />
    </div>
    {#if user?.google_connected}
      <div class="pill mt-4">✓ {$t('account.googleConnected')}</div>
    {/if}
    <button class="btn-block mt-5" disabled={profileBusy} on:click={saveProfile}>
      {profileBusy ? $t('common.saving') : $t('account.save')}
    </button>
    {#if profileMsg}<p class="success mt-3">{profileMsg}</p>{/if}
    {#if profileErr}<p class="error mt-3">{profileErr}</p>{/if}
  </section>

  <section class="card">
    <div class="card-header">
      <span class="card-title">{hasPassword ? $t('account.password.titleChange') : $t('account.password.titleSet')}</span>
      <span class="card-subtitle">{hasPassword ? $t('account.password.subtitleChange') : $t('account.password.subtitleSet')}</span>
    </div>
    {#if hasPassword}
      <div class="field">
        <label for="current-password">{$t('account.password.current')}</label>
        <input id="current-password" type="password" bind:value={currentPassword} autocomplete="current-password" />
      </div>
    {/if}
    <div class="grid-2 mt-4">
      <div class="field" style="margin-top:0">
        <label for="new-password">{$t('account.password.new')}</label>
        <input id="new-password" type="password" bind:value={newPassword} autocomplete="new-password" placeholder={$t('login.passwordPlaceholder')} />
      </div>
      <div class="field" style="margin-top:0">
        <label for="confirm-password">{$t('account.password.confirm')}</label>
        <input id="confirm-password" type="password" bind:value={confirmPassword} autocomplete="new-password" />
      </div>
    </div>
    <button class="btn-block mt-5" disabled={passwordBusy || !newPassword} on:click={savePassword}>
      {passwordBusy ? $t('common.saving') : hasPassword ? $t('account.password.saveChange') : $t('account.password.saveSet')}
    </button>
    {#if passwordMsg}<p class="success mt-3">{passwordMsg}</p>{/if}
    {#if passwordErr}<p class="error mt-3">{passwordErr}</p>{/if}
  </section>

  <section class="card">
    <div class="card-header">
      <span class="card-title">{$t('account.language.title')}</span>
      <span class="card-subtitle">{$t('account.language.subtitle')}</span>
    </div>
    <LanguageDropdown />
  </section>

  <section class="card access-card">
    <div class="card-header">
      <span class="card-title">{$t('account.access.title')}</span>
      <span class="card-subtitle">{$t('account.access.subtitle')}</span>
    </div>
    {#if accessEvents.length === 0}
      <p class="muted mt-2">{accessBusy ? $t('account.access.loading') : $t('account.access.empty')}</p>
    {:else}
      <div class="access-scroll mt-2">
        <table class="access-table">
          <thead>
            <tr>
              <th>{$t('account.access.when')}</th>
              <th>{$t('account.access.location')}</th>
              <th>{$t('account.access.device')}</th>
              <th>{$t('account.access.ip')}</th>
              <th>{$t('account.access.method')}</th>
            </tr>
          </thead>
          <tbody>
            {#each accessEvents as event (event.id)}
              <tr>
                <td>{formatAccessTime(event.created_at)}</td>
                <td>
                  <span class="loc">
                    {#if event.country_code}<CountryFlag code={event.country_code} />{/if}
                    <span>{locationLabel(event)}</span>
                  </span>
                </td>
                <td>
                  <span title={event.user_agent || ''}>{deviceLabel(event.user_agent)}</span>
                  {#if event.is_new_device}<span class="new-badge">{$t('account.access.new')}</span>{/if}
                </td>
                <td class="mono">{event.ip_address || '—'}</td>
                <td>{methodLabel(event.auth_method)}</td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
      <Pagination total={accessTotal} bind:page={accessPage} bind:pageSize={accessPageSize} />
      <p class="muted mt-2">{$t('account.access.ipNote')}</p>
    {/if}
  </section>

  <section class="card danger">
    <div class="card-header">
      <span class="card-title danger-title">{$t('account.danger.title')}</span>
      <span class="card-subtitle">{$t('account.danger.subtitle')}</span>
    </div>
    <p class="warning">{$t('account.danger.warning')}</p>
    {#if hasPassword}
      <div class="field">
        <label for="delete-password">{$t('account.danger.password')}</label>
        <input id="delete-password" type="password" bind:value={deletePassword} autocomplete="off" />
      </div>
    {:else}
      <div class="field">
        <label for="delete-confirm">{$t('account.danger.confirmType', { word: email })}</label>
        <input id="delete-confirm" bind:value={deleteConfirm} autocomplete="off" placeholder={email} />
      </div>
    {/if}
    <button class="danger btn-block mt-4" disabled={deleteBusy || !canDelete} on:click={deleteAccount}>
      {deleteBusy ? $t('account.danger.deleting') : $t('account.danger.button')}
    </button>
    {#if deleteErr}<p class="error mt-3">{deleteErr}</p>{/if}
  </section>
</main>

<style>
  .head { display: flex; flex-direction: column; gap: var(--space-2); }
  .head .btn-sm { align-self: flex-start; }
  .card { max-width: 640px; width: 100%; }
  .danger { border-color: rgba(255, 90, 95, 0.4); }
  .danger-title { color: var(--red); }
  .warning { color: var(--muted); font-size: var(--text-sm); line-height: 1.55; background: rgba(255, 90, 95, 0.08); border: 1px solid rgba(255, 90, 95, 0.25); border-radius: var(--radius-sm); padding: var(--space-3); }
  /* The access log is a wide table, so it gets more room than the 640px form cards. */
  .card.access-card { max-width: 920px; }
  .access-scroll { overflow-x: auto; }
  .access-table { width: 100%; border-collapse: collapse; font-size: var(--text-sm); }
  /* Cells don't wrap — they use the available width (the card scrolls horizontally only if needed). */
  .access-table th, .access-table td { white-space: nowrap; padding: var(--space-3); border-bottom: 1px solid var(--border); text-align: left; }
  .access-table th { color: var(--muted); font-weight: 600; }
  .access-table td { vertical-align: middle; }
  .access-table tbody tr:last-child td { border-bottom: none; }
  .access-table .mono { font-variant-numeric: tabular-nums; font-family: ui-monospace, monospace; }
  .loc { display: inline-flex; align-items: center; gap: var(--space-2); }
  .new-badge { display: inline-block; margin-left: var(--space-2); padding: 1px var(--space-2); border-radius: 999px; font-size: var(--text-xs); font-weight: 700; white-space: nowrap; color: #1a1714; background: var(--brand); }
</style>
