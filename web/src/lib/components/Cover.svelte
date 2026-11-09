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
    /** The work's medium, which picks the stand-in's colour when no variant is named. */
    medium?: string;
    size?: 'sm' | 'lg';
  }
  let { title, url = null, variant, medium = '', size = 'sm' }: Props = $props();

  // One palette, and the medium is what varies inside it. A named variant always wins, so the gallery's states are
  // unchanged.
  const byMedium: Record<string, Variant> = {
    book: 'sand',
    film: 'slate',
    tv: 'ink',
    game: 'plum',
    album: 'teal',
    theatre: 'rust',
    podcast: 'green',
  };
  const tone = $derived(variant ?? byMedium[medium] ?? 'default');

  const initials = $derived(
    title
      .split(/\s+/)
      .filter((word) => /[\p{L}\p{N}]/u.test(word))
      .slice(0, 3)
      .map((word) => word[0]?.toUpperCase() ?? '')
      .join(''),
  );
</script>

<span class="cover {tone}" class:lg={size === 'lg'} aria-hidden="true">
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
  /* A nested mix, because color-mix takes one pair: red and blue make the plum, and the panel darkens it to the
   * rest of the palette. Three colours in one declaration is invalid, and that is how this cover once came out
   * transparent. */
  .cover.plum {
    background: color-mix(
      in oklch,
      color-mix(in oklch, var(--danger) 30%, var(--imageread)) 45%,
      var(--panel)
    );
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
