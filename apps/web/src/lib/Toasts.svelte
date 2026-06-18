<script lang="ts">
  import { fly, fade } from 'svelte/transition'
  import { flip } from 'svelte/animate'
  import { toasts, dismissToast, type Toast } from './stores'

  const iconFor: Record<Toast['kind'], string> = { success: '✓', error: '!', info: 'i' }
</script>

<div class="toast-stack" aria-live="polite" aria-atomic="false">
  {#each $toasts as toast (toast.id)}
    <div
      class="toast {toast.kind}"
      role="status"
      animate:flip={{ duration: 200 }}
      in:fly={{ y: -14, duration: 220 }}
      out:fade={{ duration: 180 }}
    >
      <span class="toast-icon" aria-hidden="true">{iconFor[toast.kind]}</span>
      <span class="toast-text">{toast.text}</span>
      <button class="toast-close" aria-label="Dismiss" on:click={() => dismissToast(toast.id)}>×</button>
    </div>
  {/each}
</div>

<style>
  .toast-stack {
    position: fixed;
    top: 1rem;
    right: 1rem;
    z-index: 1000;
    display: flex;
    flex-direction: column;
    gap: 0.6rem;
    width: min(92vw, 30rem);
    pointer-events: none;
  }
  .toast {
    pointer-events: auto;
    display: flex;
    align-items: flex-start;
    gap: 0.65rem;
    padding: 0.8rem 0.85rem;
    border-radius: var(--radius-md);
    background: var(--surface-2);
    color: var(--text);
    border: 1px solid var(--border-strong);
    border-left: 4px solid var(--brand);
    box-shadow: var(--shadow-pop);
    font-size: var(--text-sm);
    line-height: 1.45;
  }
  .toast.success { border-left-color: var(--green); }
  .toast.error { border-left-color: var(--red); }
  .toast.info { border-left-color: var(--brand); }
  .toast-icon {
    flex: 0 0 auto;
    width: 1.3rem;
    height: 1.3rem;
    display: grid;
    place-items: center;
    border-radius: var(--radius-pill);
    font-weight: 800;
    font-size: 0.8rem;
    background: var(--brand);
    color: var(--on-brand);
  }
  .toast.success .toast-icon { background: var(--green); color: #04210f; }
  .toast.error .toast-icon { background: var(--red); color: #2a0608; }
  .toast-text { flex: 1 1 auto; word-break: break-word; padding-top: 0.05rem; }
  .toast-close {
    flex: 0 0 auto;
    background: none;
    border: none;
    color: var(--muted);
    font-size: 1.15rem;
    line-height: 1;
    padding: 0 0.15rem;
    cursor: pointer;
  }
  .toast-close:hover { color: var(--text); }
</style>
