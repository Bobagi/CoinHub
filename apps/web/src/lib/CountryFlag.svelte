<script lang="ts">
  // Country flag by ISO 3166-1 alpha-2 code (any case), served same-origin from /flags/<iso2>.svg
  // (copied from flag-icons at build time). Same-origin keeps it within our CSP (img-src 'self') and,
  // unlike emoji flags, renders identically on Windows. Hides itself if the code has no flag.
  export let code = ''
  export let size = 22

  $: normalized = (code || '').trim().toLowerCase()
  // flag-icons 4x3 aspect ratio.
  $: height = Math.round((size * 3) / 4)
  let failed = false
  // Reset the error state whenever the code changes so a new (valid) flag can render after a bad one.
  $: if (normalized) failed = false
</script>

{#if normalized && !failed}
  <img
    class="cflag"
    src={`/flags/${normalized}.svg`}
    width={size}
    height={height}
    alt={(code || '').toUpperCase()}
    title={(code || '').toUpperCase()}
    loading="lazy"
    on:error={() => (failed = true)}
  />
{/if}

<style>
  .cflag {
    display: inline-block;
    border-radius: 3px;
    object-fit: cover;
    vertical-align: middle;
    flex: 0 0 auto;
    box-shadow: 0 0 0 1px rgba(0, 0, 0, 0.3) inset;
  }
</style>
