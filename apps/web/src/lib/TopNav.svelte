<script lang="ts">
  import { onMount, onDestroy } from 'svelte'
  import { currentUser, binanceStatus, systemStatus, navigate } from './stores'
  import { api } from './api'
  import { t } from './i18n'
  import LanguageDropdown from './LanguageDropdown.svelte'

  let open = false
  let container: HTMLDivElement
  let headerEl: HTMLElement

  // Expose the sticky header's height as a CSS var so global fixed overlays (toasts) can sit below it
  // instead of over it. Re-measured on resize; falls back to 1rem when the nav isn't mounted (login).
  function updateTopbarHeight() {
    if (headerEl) document.documentElement.style.setProperty('--topbar-h', `${headerEl.offsetHeight}px`)
  }
  onMount(updateTopbarHeight)

  // Poll operational status so the header light reflects whether the bots can run (red when not). A
  // transient fetch failure keeps the last known status, so the light never flickers on a blip.
  let statusTimer: ReturnType<typeof setInterval> | undefined
  async function refreshSystemStatus() {
    try {
      systemStatus.set(await api.systemStatus())
    } catch {
      /* keep last known status */
    }
  }
  onMount(() => {
    refreshSystemStatus()
    statusTimer = setInterval(refreshSystemStatus, 45000)
  })
  onDestroy(() => {
    if (statusTimer) clearInterval(statusTimer)
  })

  // null status = not loaded yet ⇒ assume operational (never flash a false alarm before the first poll).
  $: operational = !$systemStatus || $systemStatus.operational !== false
  $: statusReasons = $systemStatus?.reasons ?? []
  $: statusTooltip = operational
    ? $t('status.operational')
    : statusReasons.map((reason) => $t('status.' + reason.code)).join(' ')

  $: displayName = $currentUser?.display_name?.trim() || $currentUser?.email || ''

  // Google profile picture (proxied same-origin). Falls back to the initial when absent or it fails
  // to load. Reset the failure flag whenever the URL changes (e.g. a different user signs in).
  let avatarFailed = false
  $: avatarUrl = $currentUser?.avatar_url || ''
  $: if (avatarUrl) avatarFailed = false

  async function logout() {
    open = false
    try {
      await api.logout()
    } catch {
      /* ignore */
    }
    binanceStatus.set(null)
    currentUser.set(null)
    navigate('dashboard')
  }

  function goAccount() {
    open = false
    navigate('account')
  }

  function onWindowClick(event: MouseEvent) {
    if (open && container && !container.contains(event.target as Node)) open = false
  }
  function onKeydown(event: KeyboardEvent) {
    if (event.key === 'Escape') open = false
  }
</script>

<svelte:window on:click={onWindowClick} on:keydown={onKeydown} on:resize={updateTopbarHeight} />

<header class="topbar" bind:this={headerEl}>
  <button class="brand" type="button" on:click={() => navigate('dashboard')}>Coin<span>Hub</span></button>
  <div class="spacer"></div>
  {#if $binanceStatus}
    <span class="pill binance" class:alert={!operational} title={statusTooltip}>
      <span class="dot" class:on={operational && $binanceStatus.has_active_credential} class:alert={!operational}></span>
      {$t('header.binance')}
      {$binanceStatus.active_environment || $t('header.notConnected')}{#if $binanceStatus.active_environment && !$binanceStatus.has_active_credential} ({$t('header.noKey')}){/if}
    </span>
  {/if}
  <LanguageDropdown compact />
  <div class="account" bind:this={container}>
    <button
      type="button"
      class="ghost trigger"
      aria-haspopup="menu"
      aria-expanded={open}
      aria-label={displayName || $t('nav.account')}
      on:click|stopPropagation={() => (open = !open)}
    >
      {#if avatarUrl && !avatarFailed}
        <img class="avatar avatar-img" src={avatarUrl} alt="" aria-hidden="true" on:error={() => (avatarFailed = true)} />
      {:else}
        <span class="avatar">{(displayName[0] || '?').toUpperCase()}</span>
      {/if}
      <span class="who">{displayName}</span>
      <span class="caret" class:up={open}>▾</span>
    </button>
    {#if open}
      <div class="menu" role="menu">
        <button type="button" class="menu-item" role="menuitem" on:click={goAccount}>⚙&nbsp; {$t('nav.account')}</button>
        <div class="menu-divider"></div>
        <button type="button" class="menu-item" role="menuitem" on:click={logout}>⎋&nbsp; {$t('header.signOut')}</button>
      </div>
    {/if}
  </div>
</header>

<style>
  .topbar {
    display: flex; align-items: center; gap: var(--space-3);
    padding: var(--space-3) var(--space-5);
    border-bottom: 1px solid var(--border);
    background: rgba(21, 19, 13, 0.72);
    backdrop-filter: blur(8px);
    position: sticky; top: 0; z-index: 30;
  }
  .brand { gap: 0; background: transparent; border: none; padding: 0; height: auto; min-height: 24px; color: var(--text); font-weight: 800; font-size: var(--text-lg); }
  .brand span { color: var(--brand); }
  .binance { gap: var(--space-2); }
  .binance .dot { width: 8px; height: 8px; border-radius: var(--radius-pill); background: var(--muted); }
  .binance .dot.on { background: var(--green); box-shadow: 0 0 0 3px rgba(43, 214, 106, 0.18); }
  /* Operational alert: the environment light turns red and pulses, the pill border warms, and a hover
     (title) reveals why the bots are paused. */
  .binance.alert { border-color: var(--red); color: var(--red); }
  .binance .dot.alert { background: var(--red); box-shadow: 0 0 0 3px rgba(255, 90, 95, 0.2); animation: status-pulse 1.8s ease-out infinite; }
  @keyframes status-pulse {
    0% { box-shadow: 0 0 0 0 rgba(255, 90, 95, 0.5); }
    70% { box-shadow: 0 0 0 6px rgba(255, 90, 95, 0); }
    100% { box-shadow: 0 0 0 0 rgba(255, 90, 95, 0); }
  }
  @media (prefers-reduced-motion: reduce) { .binance .dot.alert { animation: none; } }
  .account { position: relative; }
  .trigger { gap: var(--space-2); padding-inline: 5px; }
  .avatar { display: grid; place-items: center; width: 32px; height: 32px; border-radius: var(--radius-pill); background: var(--brand); color: var(--on-brand); font-size: var(--text-sm); font-weight: 800; }
  .avatar-img { object-fit: cover; padding: 0; border: 1px solid var(--border); background: var(--surface-2); }
  .who { max-width: 160px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .caret { font-size: 0.7em; transition: transform 0.15s ease; }
  .caret.up { transform: rotate(180deg); }
  .menu { right: 0; top: calc(100% + 6px); }
  /* On phones the pill is hidden to save space — but a red operational alert stays visible (the user
     can't hover, so the Dashboard banner carries the full reason). */
  @media (max-width: 600px) {
    .who { display: none; }
    .binance { display: none; }
    .binance.alert { display: inline-flex; }
  }
</style>
