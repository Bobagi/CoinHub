<script lang="ts">
  // Single reusable disclosure ("spoiler") for the whole app: one marker (a gold ▸ that
  // rotates on open), one set of design tokens. Replaces the three former ad-hoc patterns
  // (.start/.alloc-card caret card, the .help "+/−" box, and a bare <details>).
  /** Heading text shown next to the caret. */
  export let title = ''
  /** 'section' = a full collapsible card with a large heading; 'help' = a compact "how it works" box. */
  export let variant: 'section' | 'help' = 'help'
  /** Render expanded on first paint. Toggling afterwards is native <details> behaviour. */
  export let open = false
</script>

<details class="collapsible {variant}" class:card={variant === 'section'} open={open || undefined}>
  <summary>
    <span class="cl-caret" aria-hidden="true">▸</span>
    <span class="cl-title">{title}</span>
  </summary>
  <slot />
</details>

<style>
  .collapsible > summary {
    cursor: pointer;
    list-style: none;
    display: flex;
    align-items: center;
    gap: var(--space-2);
  }
  .collapsible > summary::-webkit-details-marker { display: none; }

  .cl-caret {
    flex: none;
    color: var(--brand);
    display: inline-block;
    transition: transform 0.15s ease;
  }
  .collapsible[open] > summary .cl-caret { transform: rotate(90deg); }

  /* Section: a full card with a large heading (e.g. "First steps", "Positions & performance"),
     matching the non-collapsible card titles (var(--text-lg)). Scope to the section's OWN
     summary (direct child) — a plain ">" descendant selector would leak into a nested help
     Collapsible's title (same component scope) and blow it up to section size. */
  .section > summary > .cl-title { font-size: var(--text-lg); font-weight: 800; }

  /* Help: the compact "how it works" disclosure used throughout the app. */
  .collapsible.help {
    margin-top: var(--space-3);
    border: 1px solid var(--border);
    border-radius: var(--radius-md);
    background: var(--surface-2);
    padding: 0 var(--space-3);
  }
  .collapsible.help > summary {
    padding: var(--space-3) 0;
    font-weight: 700;
    font-size: var(--text-xs);
    color: var(--brand);
  }
  .collapsible.help .cl-caret { font-size: var(--text-xs); }
  /* Body paragraphs are slotted (rendered in the parent's scope), so reach them with :global. */
  .collapsible.help > :global(p) {
    margin: 0 0 var(--space-3);
    color: var(--muted);
    font-size: var(--text-sm);
    line-height: 1.55;
  }
</style>
