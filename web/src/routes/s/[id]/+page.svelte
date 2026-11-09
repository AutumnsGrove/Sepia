<script lang="ts">
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { page } from '$app/state';
  import { ApiError, createClient, type Entry } from '../../../lib/api/client';
  import SessionScreen from '../../../lib/components/SessionScreen.svelte';
  import type { ServerEvent } from '../../../lib/contract/events';
  import { copyFor } from '../../../lib/contract/codes';
  import type { PendingMessage, ScreenHandlers } from '../../../lib/session/screen';
  import {
    SessionSocket,
    type ConnectionState,
    type OutgoingMessage,
  } from '../../../lib/socket/machine';
  import {
    applyEvent,
    applyHello,
    emptySession,
    type SessionState,
  } from '../../../lib/state/session';
  import { isWorking } from '../../../lib/state/timeline';

  /**
   * One entry session: the socket, the reducer and the screen. This is the only place the logging screen talks to
   * the service. Everything it draws comes from events, and every action leaves as one client message with a
   * client_msg_id, so a reconnect cannot apply it twice.
   */
  const client = createClient();

  let session = $state<SessionState>(emptySession());
  let connection = $state<ConnectionState>('idle');
  let pending = $state<PendingMessage[]>([]);
  let savedEntry = $state<Entry | null>(null);
  let versions = $state(0);
  let caughtUp = $state(false);
  let sendProblem = $state('');
  let restore = $state({ seq: 0, text: '' });
  let online = $state(true);

  let socket: SessionSocket | null = null;
  let wasOffline = false;
  let reopenSent = false;
  let caughtUpTimer: ReturnType<typeof setTimeout> | null = null;

  const sessionID = $derived(page.params.id ?? '');
  // The browser's own view of the network is what she sees: a phone that knows it is offline says so at once,
  // rather than waiting out the socket's 45 second silence.
  const shownConnection = $derived(online ? connection : 'reconnecting');
  const reopenID = $derived(page.url.searchParams.get('reopen') ?? '');

  function today(): string {
    // en-CA gives YYYY-MM-DD, and the phone's own zone is what "last night" has to resolve against.
    return new Date().toLocaleDateString('en-CA');
  }

  function zone(): string {
    return Intl.DateTimeFormat().resolvedOptions().timeZone || 'UTC';
  }

  function reset(id: string) {
    session = emptySession(id);
    pending = [];
    savedEntry = null;
    versions = 0;
    caughtUp = false;
    sendProblem = '';
    wasOffline = false;
    reopenSent = false;
  }

  async function loadSaved(entryID: string) {
    try {
      savedEntry = await client.getEntry(entryID);
      const pageOfRevisions = await client.listRevisions(entryID);
      versions = pageOfRevisions.items.length;
    } catch (error) {
      // The card still works without the diff; a failed read must not stop the revision.
      if (error instanceof ApiError) sendProblem = error.copy;
    }
  }

  function send(message: OutgoingMessage): string {
    if (!socket) return '';
    return socket.send(message);
  }

  $effect(() => {
    let wasOnline = navigator.onLine;
    const update = () => {
      online = navigator.onLine;
      if (online && !wasOnline) {
        caughtUp = true;
        if (caughtUpTimer) clearTimeout(caughtUpTimer);
        caughtUpTimer = setTimeout(() => (caughtUp = false), 4000);
      }
      wasOnline = online;
    };
    update();
    window.addEventListener('online', update);
    window.addEventListener('offline', update);
    return () => {
      window.removeEventListener('online', update);
      window.removeEventListener('offline', update);
    };
  });

  $effect(() => {
    const id = sessionID;
    const reopen = reopenID;
    if (!id) return;

    reset(id);
    const machine = new SessionSocket({
      url: `/ws/sessions/${encodeURIComponent(id)}`,
      onStateChange: (next) => {
        if (next === 'reconnecting' || next === 'closed') wasOffline = true;
        if (next === 'open' && wasOffline) {
          wasOffline = false;
          caughtUp = true;
          if (caughtUpTimer) clearTimeout(caughtUpTimer);
          caughtUpTimer = setTimeout(() => (caughtUp = false), 4000);
        }
        connection = next;
      },
      onHello: (hello) => {
        session = applyHello(session, hello);
        if (reopen && !reopenSent && session.draft === null) {
          reopenSent = true;
          void loadSaved(reopen);
          send({ type: 'reopen', data: { entry_id: reopen } });
        }
      },
      onEvent: (event: ServerEvent) => {
        session = applyEvent(session, event).state;
        if (event.type === 'user_message') {
          const clientMsgID = (event.data as { client_msg_id?: string })?.client_msg_id;
          if (clientMsgID) pending = pending.filter((message) => message.id !== clientMsgID);
        }
        if (event.type === 'saved') {
          caughtUp = false;
        }
      },
      onAck: (ack) => {
        if (ack.status !== 'rejected' || !ack.client_msg_id) return;
        const held = pending.find((message) => message.id === ack.client_msg_id);
        if (held) {
          // The text goes back where she typed it, so a refusal never loses it.
          restore = { seq: restore.seq + 1, text: held.text };
          pending = pending.filter((message) => message.id !== ack.client_msg_id);
        }
        sendProblem = ack.reason ? copyFor(ack.reason) : 'Sepia could not take that.';
      },
      onReload: () => window.location.reload(),
      onIgnored: (reason, detail) => {
        // Never her text: only the reason and the shape the client could not use.
        console.warn('socket ignored a frame:', reason, detail);
      },
    });
    socket = machine;
    machine.start();

    return () => {
      machine.stop();
      socket = null;
      if (caughtUpTimer) clearTimeout(caughtUpTimer);
    };
  });

  const handlers: ScreenHandlers = {
    onsend: (text) => {
      sendProblem = '';
      const working = isWorking(session);
      const offline = !online || connection !== 'open';
      const id = send({
        type: 'message',
        data: { text, client_date: today(), tz: zone() },
      });
      // Only a message the service has not echoed yet needs a row. The server's own `user_message` removes it.
      if (id && (offline || working)) {
        pending = [...pending, { id, text, queued: working && !offline }];
      }
    },
    onanswer: (interactionID, value) => {
      send({
        type: 'interaction_response',
        data: { interaction_id: interactionID, value: value as Record<string, unknown> },
      });
    },
    onedit: (set) => {
      if (!session.draft) return;
      send({ type: 'draft_edit', data: { base_version: session.draft.version, set } });
    },
    oninclude: (segmentID) => {
      if (!session.draft) return;
      send({
        type: 'include_left_out',
        data: { base_version: session.draft.version, segment_id: segmentID },
      });
    },
    onaccept: (fieldID) => {
      if (!session.draft) return;
      send({
        type: 'accept_unverified',
        data: { base_version: session.draft.version, fields: [fieldID] },
      });
    },
    onacceptall: () => {
      if (!session.draft) return;
      send({ type: 'accept_unverified', data: { base_version: session.draft.version, all: true } });
    },
    onconfirm: () => {
      if (!session.draft) return;
      send({ type: 'confirm', data: { base_version: session.draft.version } });
    },
    ondiscard: () => send({ type: 'discard', data: {} }),
    onstop: () => send({ type: 'stop', data: {} }),
    onretry: () => send({ type: 'retry', data: {} }),
    oncancel: (id: string) => {
      socket?.cancel(id);
      pending = pending.filter((message) => message.id !== id);
    },
    onback: () => {
      if (window.history.length > 1) window.history.back();
      else void goto(resolve('/'));
    },
    oneditSaved: () => {
      const entryID = session.savedEntryID;
      // resolve() builds the path and the base; the one-shot query is added to it.
      // eslint-disable-next-line svelte/no-navigation-without-resolve
      if (entryID) void goto(`${resolve('/new')}?reopen=${encodeURIComponent(entryID)}`);
    },
    onlogAnother: () => void goto(resolve('/new')),
  };
</script>

<svelte:head>
  <title>{session.draft?.work.title ?? 'New entry'} · Sepia</title>
</svelte:head>

<SessionScreen
  {session}
  connection={shownConnection}
  {handlers}
  {pending}
  {savedEntry}
  {versions}
  {caughtUp}
  problem={sendProblem}
  {restore}
/>
