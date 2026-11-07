import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { describe, expect, it } from 'vitest';

/**
 * The mockups are the UI spec, so the tokens are theirs. This test is the only thing stopping the app's
 * copy of the palette from quietly drifting away from the frame a designer or an agent checked by eye.
 */
const repoRoot = new URL('../../../', import.meta.url);
const mockupCSS = fileURLToPath(new URL('mockups/sepia.css', repoRoot));
const appCSS = fileURLToPath(new URL('../app.css', import.meta.url));

function rootTokens(css: string): Set<string> {
  // The mockups declare tokens in more than one :root block (the card states add theirs beside the component),
  // so every block is read. Reading only the first would let a token drift as soon as it is declared later.
  const names = new Set<string>();
  for (const block of css.matchAll(/:root\s*\{([^}]*)\}/g)) {
    for (const token of block[1].matchAll(/(--[a-z0-9-]+)\s*:/g)) {
      names.add(token[1]);
    }
  }
  if (names.size === 0) throw new Error('no :root block');
  return names;
}

describe('design tokens', () => {
  const spec = rootTokens(readFileSync(mockupCSS, 'utf8'));
  const app = rootTokens(readFileSync(appCSS, 'utf8'));

  it('finds tokens in both files', () => {
    expect(spec.size).toBeGreaterThan(10);
    expect(app.size).toBeGreaterThan(10);
  });

  it('defines every token the mockups define', () => {
    const missing = [...spec].filter((name) => !app.has(name));
    expect(missing).toEqual([]);
  });

  it('defines no token the mockups do not have', () => {
    // A token invented in the app but absent from the spec is either a typo or an undiscussed design change.
    const extra = [...app].filter((name) => !spec.has(name));
    expect(extra).toEqual([]);
  });
});
