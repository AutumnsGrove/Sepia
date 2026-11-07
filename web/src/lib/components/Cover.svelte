<script lang="ts">
  /**
   * A work's cover, or the block that stands in for one until a real image is cached. The stand-in is derived
   * from the medium rather than from a raw colour, so there is still one palette (STANDARDS 5). The image is
   * decorative: the title is always beside it, and an alt repeating it would read the title twice.
   */
  type Variant = 'default' | 'sand' | 'rust' | 'slate' | 'green' | 'plum' | 'teal' | 'ink';
  interface Props {
    title: string;
    url?: string | null;
    variant?: Variant;
    size?: 'sm' | 'lg';
  }
  let { title, url = null, variant = 'default', size = 'sm' }: Props = $props();

  const initials = $derived(
    title
      .split(/\s+/)
      .filter((word) => /[\p{L}\p{N}]/u.test(word))
      .slice(0, 3)
      .map((word) => word[0]?.toUpperCase() ?? '')
      .join(''),
  );
</script>

<span class="cover {variant}" class:lg={size === 'lg'} aria-hidden="true">
  {#if url}
    <img src={url} alt="" loading="lazy" decoding="async" />
  {:else}
    <span class="tt">{initials || title.slice(0, 2)}</span>
  {/if}
</span>

<style>
  .cover {
    width: 44px;
    aspect-ratio: 2 / 3;
    border-radius: 3px;
    background: color-mix(in oklch, var(--accent) 16%, var(--panel));
    display: grid;
    align-items: end;
    padding: 3px;
    font:
      600 0.40625rem/1.1 'Lexend',
      system-ui,
      sans-serif;
    letter-spacing: 0;
    color: color-mix(in oklch, var(--text) 85%, var(--accent));
    overflow: hidden;
    flex: none;
  }
  .cover img {
    width: 100%;
    height: 100%;
    object-fit: cover;
  }
  .cover.sand {
    background: color-mix(in oklch, var(--accent) 30%, var(--panel));
  }
  .cover.rust {
    background: color-mix(in oklch, var(--danger) 25%, var(--panel));
  }
  .cover.slate {
    background: color-mix(in oklch, var(--imageread) 18%, var(--panel));
  }
  .cover.green {
    background: color-mix(in oklch, var(--accent) 40%, var(--panel));
  }
  .cover.plum {
    background: color-mix(in oklch, var(--danger) 14%, var(--imageread) 12%, var(--panel));
  }
  .cover.teal {
    background: color-mix(in oklch, var(--imageread) 30%, var(--panel));
  }
  .cover.ink {
    background: color-mix(in oklch, var(--text) 12%, var(--bg));
  }
  .cover.lg {
    width: 84px;
    padding: 6px;
    font-size: 0.5625rem;
    border-radius: 4px;
  }
</style>
