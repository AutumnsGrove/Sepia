<script lang="ts">
  import '../app.css';
  import ScreenBoundary from '../lib/components/ScreenBoundary.svelte';

  let { children } = $props();
</script>

<!-- Every screen is drawn inside the boundary, so a render error costs the screen and not the app (issue #40). -->
<ScreenBoundary>
  {@render children()}
</ScreenBoundary>
