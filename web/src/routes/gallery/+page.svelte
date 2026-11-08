<script lang="ts">
  import { resolve } from '$app/paths';
  import { page } from '$app/state';
  import { createClient } from '../../lib/api/client';
  import { SessionSocket } from '../../lib/socket/machine';
  import Banner from '../../lib/components/Banner.svelte';
  import Button from '../../lib/components/Button.svelte';
  import Chip from '../../lib/components/Chip.svelte';
  import Cloud from '../../lib/components/Cloud.svelte';
  import Cover from '../../lib/components/Cover.svelte';
  import Errbox from '../../lib/components/Errbox.svelte';
  import InteractionCard from '../../lib/components/InteractionCard.svelte';
  import ListRow from '../../lib/components/ListRow.svelte';
  import Pill from '../../lib/components/Pill.svelte';
  import Rating from '../../lib/components/Rating.svelte';
  import SessionScreen from '../../lib/components/SessionScreen.svelte';
  import Statusbar from '../../lib/components/Statusbar.svelte';
  import Switch from '../../lib/components/Switch.svelte';
  import Tabs from '../../lib/components/Tabs.svelte';
  import { fixtureHandlers, screenFixture, screenNames } from '../../lib/session/fixtures';

  /**
   * The gallery: every base component in every state the mockups draw, in one column, addressable one state at
   * a time with `?state=name`. It is the unit of visual regression (S10.3): Playwright opens each state, checks
   * axe and takes one baseline, and a change that moves a pixel has to be looked at.
   *
   * It is linked from nowhere and carries no data of its own. SvelteKit reserves a leading `_`, so the plan's
   * `/_gallery` is `/gallery` here.
   */

  const states = [
    'buttons',
    'pills',
    'chips',
    'switch',
    'tabs',
    'status',
    'banner',
    'errbox',
    'covers',
    'cloud',
    'list',
    'rating',
    'ix-medium',
    'ix-candidate',
    'ix-describe',
    'ix-form',
    'ix-revisit',
    'ix-gap',
    'ix-lookup',
    'socket',
    ...screenNames,
  ] as const;
  type State = (typeof states)[number];

  const requested = page.url.searchParams.get('state');
  const shown: State | null = states.includes(requested as State) ? (requested as State) : null;
  const only = (name: State) => shown === null || shown === name;

  let rating = $state(18);
  let favourite = $state(false);
  let connected = $state(true);
  let tab = $state('characters');
  let answer = $state('');

  const mediumPayload = {
    options: [
      { value: 'book', label: 'The book', hint: 'novel' },
      { value: 'film', label: 'A film', hint: 'movie' },
      { value: 'other', label: 'Something else', hint: 'none of these' },
    ],
  };
  const candidatePayload = {
    candidates: [
      { id: 'cand_1', title: 'Dune', year: 1965, creator: 'Frank Herbert', medium: 'book' },
      {
        id: 'cand_2',
        title: 'Dune Messiah',
        year: 1969,
        creator: 'Frank Herbert',
        medium: 'book',
      },
      {
        id: 'cand_3',
        title: 'Dune: The Graphic Novel',
        year: 2020,
        creator: 'Brian Herbert',
        medium: 'book',
      },
    ],
    none_label: 'None of these',
  };
  const describePayload = {
    categories: [
      {
        id: 'characters',
        label: 'Characters',
        covered: true,
        tags: [
          { value: 'believable', label: 'believable', tone: 'positive' },
          { value: 'flat', label: 'flat', tone: 'negative' },
          { value: 'morally-grey', label: 'morally grey', tone: 'mixed' },
          { value: 'memorable', label: 'memorable', tone: 'positive' },
        ],
        hints: ['believable'],
      },
      {
        id: 'setting',
        label: 'Setting',
        covered: false,
        tags: [
          { value: 'immersive', label: 'immersive', tone: 'positive' },
          { value: 'lived-in', label: 'lived in', tone: 'positive' },
          { value: 'thin', label: 'thin', tone: 'negative' },
        ],
        hints: ['immersive'],
      },
      {
        id: 'writing',
        label: 'Writing',
        covered: false,
        tags: [
          { value: 'spare', label: 'spare', tone: 'mixed' },
          { value: 'dense', label: 'dense', tone: 'mixed' },
        ],
        hints: [],
      },
    ],
  };
  const formPayload = {
    questions: [
      {
        id: 'ending',
        prompt: 'How did the ending land?',
        type: 'choice',
        options: ['stuck the landing', 'faded', 'rushed'],
        hint: 'stuck the landing',
      },
      {
        id: 'length',
        prompt: 'Too long, too short, or right?',
        type: 'choice',
        options: ['too long', 'right', 'too short'],
      },
      {
        id: 'recommend',
        prompt: 'One line for a friend?',
        type: 'short_text',
      },
    ],
  };
  const revisitPayload = {
    previous: {
      entry_id: 'e_12',
      rating_q: 14,
      finished_on: '2024-03-02',
      verdict: 'good, not great',
    },
    options: [
      { value: 'new_entry', label: 'A new entry' },
      { value: 'edit_existing', label: 'Edit the old one' },
    ],
  };
  const gapPayload = { field: 'rating', prompt: 'How many stars would you give it?' };
  const lookupPayload = {
    service: 'openlibrary',
    reason: 'It did not answer in time.',
    can_ask_polaris: true,
  };

  function answered(value: unknown) {
    answer = JSON.stringify(value);
  }

  // A live harness for the socket machine, used by the end-to-end suite to prove the machine against a real
  // socket and the real service, not only against the fake in Vitest. It only runs on the `socket` state.
  let live = $state({ state: 'idle', events: 0, notice: '', subscribed: false, opens: 0 });
  let socket: SessionSocket | null = null;
  let sessionID = $state('');

  $effect(() => {
    if (shown !== 'socket') return;
    let cancelled = false;
    const client = createClient();
    const machine = new SessionSocket({
      url: '',
      onStateChange: (state) => {
        live.state = state;
        if (state === 'open') live.opens += 1;
      },
      onEvent: (event) => {
        live.events += 1;
        live.notice = event.type;
      },
      onIgnored: (reason) => (live.notice = reason),
    });
    socket = machine;
    void (async () => {
      const session = await client.createSession({ kind: 'entry' });
      if (cancelled) return;
      sessionID = session.id;
      // The machine was built before the id existed, so hand it the URL it needs and open it.
      socket = new SessionSocket({
        url: `/ws/sessions/${session.id}`,
        onStateChange: (state) => {
          live.state = state;
          if (state === 'open') live.opens += 1;
        },
        onEvent: (event) => {
          live.events += 1;
          live.notice = event.type;
        },
        onIgnored: (reason) => (live.notice = reason),
      });
      machine.stop();
      socket.start();
      live.subscribed = true;
    })().catch((error: unknown) => {
      live.notice = String(error);
    });
    return () => {
      cancelled = true;
      socket?.stop();
      machine.stop();
    };
  });

  function reconnect() {
    socket?.stop();
    socket?.start();
  }

  function sendOne() {
    socket?.send({
      type: 'message',
      client_msg_id: `gallery-${Date.now()}`,
      data: {
        text: 'finished dune last night',
        client_date: '2026-11-08',
        tz: 'Europe/London',
      },
    } as Parameters<SessionSocket['send']>[0]);
  }
</script>

<svelte:head>
  <title>Gallery · Sepia</title>
  <meta name="robots" content="noindex" />
</svelte:head>

<main>
  <header>
    <h1>Gallery</h1>
    {#if shown === null}
      <p class="lede">
        Every base component in every state. Open one at a time with <code>?state=name</code>; each
        is a Playwright baseline.
      </p>
      <nav class="index">
        {#each states as name (name)}
          <a href="{resolve('/gallery')}?state={name}">{name}</a>
        {/each}
      </nav>
    {:else}
      <p class="lede"><a href={resolve('/gallery')}>All states</a> · {shown}</p>
    {/if}
  </header>

  {#if only('buttons')}
    <section>
      <h2>Buttons</h2>
      <div class="row">
        <Button onclick={() => {}}>Plain</Button>
        <Button variant="primary" onclick={() => {}}>Primary</Button>
        <Button variant="ghost" onclick={() => {}}>Ghost</Button>
      </div>
      <div class="row">
        <Button variant="danger" onclick={() => {}}>Delete it</Button>
        <Button size="sm" onclick={() => {}}>Small</Button>
        <Button disabled onclick={() => {}}>Disabled</Button>
      </div>
      <div class="row">
        <Button variant="primary" grow disabled onclick={() => {}}>Confirm</Button>
      </div>
    </section>
  {/if}

  {#if only('pills')}
    <section>
      <h2>Pills</h2>
      <div class="row wrap">
        <Pill>paperback</Pill>
        <Pill>1965</Pill>
        <Pill>Frank Herbert</Pill>
        <Pill>18/20</Pill>
      </div>
    </section>
  {/if}

  {#if only('chips')}
    <section>
      <h2>Chips</h2>
      <p class="label">Plain, selected</p>
      <div class="row wrap">
        <Chip>books</Chip>
        <Chip pressed>films</Chip>
      </div>
      <p class="label">Pick</p>
      <div class="row wrap">
        <Chip look="pick">believable</Chip>
        <Chip look="pick" pressed>memorable</Chip>
        <Chip look="pick" disabled>disabled</Chip>
      </div>
      <p class="label">Hint, dashed until tapped</p>
      <div class="row wrap">
        <Chip look="hint">immersive</Chip>
        <Cloud label="All">
          <Chip look="pick">flat</Chip>
          <Chip look="pick">morally grey</Chip>
        </Cloud>
      </div>
    </section>
  {/if}

  {#if only('switch')}
    <section>
      <h2>Switch</h2>
      <div class="srow">
        <span>Show spoilers</span>
        <Switch checked={connected} label="Show spoilers" onchange={(v) => (connected = v)} />
      </div>
      <div class="srow">
        <span>Dark only</span>
        <Switch checked={true} label="Dark only" onchange={() => {}} />
      </div>
      <div class="srow">
        <span>Unavailable</span>
        <Switch checked={false} label="Unavailable" disabled onchange={() => {}} />
      </div>
    </section>
  {/if}

  {#if only('tabs')}
    <section>
      <h2>Tabs</h2>
      <Tabs
        tabs={[
          { id: 'characters', label: 'Characters', count: 2 },
          { id: 'plot', label: 'Plot' },
          { id: 'setting', label: 'Setting', count: 1 },
          { id: 'writing', label: 'Writing', count: 1 },
          { id: 'feel', label: 'Feel' },
          { id: 'warnings', label: 'Warnings' },
        ]}
        selected={tab}
        label="Categories"
        onselect={(id) => (tab = id)}
      />
    </section>
  {/if}

  {#if only('status')}
    <section>
      <h2>Statusbar</h2>
      <Statusbar state="ok" />
      <div class="spacer"></div>
      <Statusbar state="offline" />
      <div class="spacer"></div>
      <Statusbar state="working" />
      <div class="spacer"></div>
      <Statusbar state="saved" />
    </section>
  {/if}

  {#if only('banner')}
    <section>
      <h2>Banner</h2>
      <Banner>Two of the facts came from the web. Nothing is saved until you confirm.</Banner>
    </section>
  {/if}

  {#if only('errbox')}
    <section>
      <h2>Errbox</h2>
      <Errbox message="Something went wrong. Your words are kept." retryable onretry={() => {}} />
      <div class="spacer"></div>
      <Errbox message="The service is updating. Try again in a moment." />
    </section>
  {/if}

  {#if only('covers')}
    <section>
      <h2>Covers</h2>
      <p class="label">Stand-ins by medium, and one large</p>
      <div class="row wrap">
        <Cover title="The Lantern Keeper" variant="default" />
        <Cover title="Dune" variant="sand" />
        <Cover title="Solaris" variant="rust" />
        <Cover title="Piranesi" variant="slate" />
        <Cover title="The Dispossessed" variant="green" />
        <Cover title="Roadside Picnic" variant="plum" />
        <Cover title="Annihilation" variant="teal" />
        <Cover title="Blindsight" variant="ink" />
        <Cover title="Dune" variant="sand" size="lg" />
      </div>
    </section>
  {/if}

  {#if only('cloud')}
    <section>
      <h2>Cloud</h2>
      <p class="label">A cloud of describing chips</p>
      <Cloud label="Describe the characters">
        <Chip look="pick">believable</Chip>
        <Chip look="pick" pressed>memorable</Chip>
        <Chip look="hint">morally grey</Chip>
        <Chip look="pick">flat</Chip>
      </Cloud>
    </section>
  {/if}

  {#if only('list')}
    <section>
      <h2>List rows</h2>
      <ul class="list">
        <ListRow
          title="The Lantern Keeper"
          meta="Kestrel Voss · 2024"
          quarters={18}
          note="the ending landed but it was too long, in a good way"
          age="2 days ago"
          onclick={() => {}}
        />
        <ListRow
          title="Piranesi"
          meta="Susanna Clarke · 2020"
          quarters={20}
          note="I keep thinking about the statues."
          age="last month"
          onclick={() => {}}
        />
        <ListRow
          title="Solaris"
          meta="Stanisław Lem · 1961"
          age="3 weeks ago"
          waiting="Waiting for you to pick which edition"
          onclick={() => {}}
        />
        <ListRow
          title="Untitled draft"
          age="just now"
          waiting="No work picked yet"
          onclick={() => {}}
        />
      </ul>
    </section>
  {/if}

  {#if only('rating')}
    <section>
      <h2>Rating</h2>
      <p class="label">Empty, then a drag, then full, then read-only</p>
      <Rating quarters={0} onchange={(q) => (rating = q)} />
      <div class="spacer"></div>
      <Rating
        quarters={rating}
        onchange={(q) => (rating = q)}
        {favourite}
        onfavourite={(v) => (favourite = v)}
        showFavourite
      />
      <div class="spacer"></div>
      <Rating quarters={20} onchange={() => {}} />
      <div class="spacer"></div>
      <Rating quarters={14} readonly label="Saved rating" />
      <p class="label">Value: {rating} quarters</p>
    </section>
  {/if}

  {#if only('ix-medium')}
    <section>
      <h2>Interaction · choose a medium</h2>
      <InteractionCard kind="choose_medium" payload={mediumPayload} onanswer={answered} />
    </section>
  {/if}
  {#if only('ix-candidate')}
    <section>
      <h2>Interaction · choose a candidate</h2>
      <InteractionCard kind="choose_candidate" payload={candidatePayload} onanswer={answered} />
    </section>
  {/if}
  {#if only('ix-describe')}
    <section>
      <h2>Interaction · describe</h2>
      <InteractionCard kind="describe" payload={describePayload} onanswer={answered} />
    </section>
  {/if}
  {#if only('ix-form')}
    <section>
      <h2>Interaction · questions</h2>
      <InteractionCard kind="question_form" payload={formPayload} onanswer={answered} />
    </section>
  {/if}
  {#if only('ix-revisit')}
    <section>
      <h2>Interaction · logged before</h2>
      <InteractionCard kind="revisit_choice" payload={revisitPayload} onanswer={answered} />
    </section>
  {/if}
  {#if only('ix-gap')}
    <section>
      <h2>Interaction · one thing missing</h2>
      <InteractionCard kind="fill_gap" payload={gapPayload} onanswer={answered} />
    </section>
  {/if}
  {#if only('socket')}
    <section>
      <h2>Socket machine</h2>
      <p class="label">Connected state, events seen, and the last event type</p>
      <dl class="live">
        <dt>state</dt>
        <dd data-testid="socket-state">{live.state}</dd>
        <dt>events</dt>
        <dd data-testid="socket-events">{live.events}</dd>
        <dt>last</dt>
        <dd data-testid="socket-last">{live.notice || 'none yet'}</dd>
        <dt>session</dt>
        <dd class="mono">{sessionID || 'creating'}</dd>
        <dt>opens</dt>
        <dd data-testid="socket-opens">{live.opens}</dd>
      </dl>
      <div class="row">
        <Button variant="primary" disabled={live.state !== 'open'} onclick={sendOne}>
          Send a message
        </Button>
        <Button disabled={!live.subscribed} onclick={reconnect}>Reconnect</Button>
      </div>
    </section>
  {/if}

  {#if only('ix-lookup')}
    <section>
      <h2>Interaction · lookup failed</h2>
      <InteractionCard kind="lookup_failed" payload={lookupPayload} onanswer={answered} />
    </section>
  {/if}

  {#each screenNames as name (name)}
    {#if shown === name}
      <section class="screenstate">
        <h2>{name.replace('screen-', '')}</h2>
        <SessionScreen {...screenFixture(name)} handlers={fixtureHandlers} />
      </section>
    {/if}
  {/each}

  {#if answer}
    <p class="answer" role="status">Answer: {answer}</p>
  {/if}
</main>

<style>
  main {
    max-width: 430px;
    margin: 0 auto;
    padding: 1.25rem 1rem 3rem;
    min-height: 100dvh;
  }
  header h1 {
    font:
      400 2rem/1.1 'Fraunces',
      Georgia,
      serif;
    font-variation-settings: 'opsz' 72;
    margin: 0 0 0.35rem;
  }
  .lede {
    margin: 0 0 1.25rem;
    color: var(--text-2);
    font-size: 0.875rem;
    line-height: 1.5;
  }
  .lede a {
    color: var(--accent);
    /* A link inside a sentence is underlined: colour alone is not allowed to carry it. */
    text-decoration: underline;
    text-underline-offset: 3px;
  }
  .index {
    display: flex;
    flex-wrap: wrap;
    gap: 0.5rem;
    margin-bottom: 1rem;
  }
  .index a {
    min-height: 44px;
    display: inline-flex;
    align-items: center;
    padding: 0 0.9rem;
    border: 1px solid var(--line);
    border-radius: 999px;
    color: var(--text-2);
    text-decoration: none;
    font-size: 0.875rem;
  }
  section {
    margin-bottom: 2rem;
  }
  h2 {
    font:
      500 0.6875rem/1 'Lexend',
      system-ui,
      sans-serif;
    letter-spacing: 0.09em;
    text-transform: uppercase;
    color: var(--text-3);
    margin: 0 0 0.75rem;
  }
  .label {
    font-size: 0.75rem;
    color: var(--text-3);
    margin: 0.9rem 0 0.4rem;
  }
  .row {
    display: flex;
    gap: 0.6rem;
    align-items: center;
    margin-bottom: 0.6rem;
  }
  .row.wrap {
    flex-wrap: wrap;
  }
  .spacer {
    height: 0.9rem;
  }
  .list {
    margin: 0;
    padding: 0;
    list-style: none;
  }
  .srow {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 1rem;
    min-height: 56px;
    border-bottom: 1px solid var(--line);
    font-size: 1rem;
  }
  .live {
    display: grid;
    grid-template-columns: 5rem 1fr;
    gap: 0.35rem 1rem;
    margin: 0 0 1rem;
    font-size: 0.875rem;
  }
  .live dt {
    color: var(--text-3);
  }
  .live dd {
    margin: 0;
    color: var(--text);
    overflow-wrap: anywhere;
  }
  .mono {
    font-family: ui-monospace, Menlo, monospace;
    font-size: 0.75rem;
  }
  .answer {
    position: sticky;
    bottom: 0.5rem;
    padding: 0.6rem 0.8rem;
    border-radius: var(--r-md);
    background: var(--raised);
    color: var(--text-2);
    font-family: ui-monospace, Menlo, monospace;
    font-size: 0.75rem;
    overflow-wrap: anywhere;
  }
</style>
