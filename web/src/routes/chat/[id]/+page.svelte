<script lang="ts">
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { page } from '$app/state';
  import { copyFor } from '../../../lib/contract/codes';
  import type { ServerEvent } from '../../../lib/contract/events';
  import { createClient } from '../../../lib/api/client';
  import ChatScreen from '../../../lib/components/ChatScreen.svelte';
  import TopBar from '../../../lib/components/TopBar.svelte';
  import type { PendingMessage, ScreenHandlers } from '../../../lib/session/screen';
  import {
    SessionSocket,
    type ConnectionState,
    type OutgoingMessage,
  } from '../../../lib/socket/machine';
  import { takeFirstMessage } from '../../../lib/state/handoff';
  import {
    applyEvent,
    applyHello,
    emptySession,
    type SessionState,
  } from '../../../lib/state/session';
  import { isWorking } from '../../../lib/state/timeline';

  /**
   * One library chat: the socket, the reducer and the screen. It is the same machinery the logging screen uses, with a
   * smaller surface: a chat has no card, so it sends messages and answers the one interaction it can be asked (a
   * proposed change), and nothing else.
   */
  const client = createClient();

  let session = $state<SessionState>(emptySession());
  let connection = $state<ConnectionState>('idle');
  let pending = $state<PendingMessage[]>([]);
  let problem = $state('');

  let socket: SessionSocket | null = null;
  let restore = $state({ seq: 0, text: '' });

  const sessionID = $derived(page.params.id ?? '');

  function today(): string {
    return new Date().toLocaleDateString('en-CA');
  }

  function zone(): string {
    return Intl.DateTimeFormat().resolvedOptions().timeZone || 'UTC';
  }

  function send(message: OutgoingMessage): string {
    if (!socket) return '';
    return socket.send(message);
  }

  $effect(() => {
    const id = sessionID;
    if (!id) return;
    session = emptySession(id);
    pending = [];
    problem = '';

    const machine = new SessionSocket({
      url: `/ws/sessions/${encodeURIComponent(id)}`,
      onStateChange: (next) => (connection = next),
      onHello: (hello) => {
        session = applyHello(session, hello);
        // The question she typed on the start screen arrives here, once, the way a sentence typed on Home does.
        const first = takeFirstMessage();
        if (first !== '') {
          send({ type: 'message', data: { text: first, client_date: today(), tz: zone() } });
        }
      },
      onEvent: (event: ServerEvent) => {
        session = applyEvent(session, event).state;
        if (event.type === 'user_message') {
          const clientMsgID = (event.data as { client_msg_id?: string })?.client_msg_id;
          if (clientMsgID) pending = pending.filter((message) => message.id !== clientMsgID);
        }
      },
      onAck: (ack) => {
        if (ack.status !== 'rejected' || !ack.client_msg_id) return;
        const held = pending.find((message) => message.id === ack.client_msg_id);
        if (held) {
          restore = { seq: restore.seq + 1, text: held.text };
          pending = pending.filter((message) => message.id !== ack.client_msg_id);
        }
        problem = ack.reason ? copyFor(ack.reason) : 'Sepia could not take that.';
      },
      onReload: () => window.location.reload(),
      onIgnored: (reason, detail) => console.warn('socket ignored a frame:', reason, detail),
    });
    socket = machine;
    machine.start();
    return () => {
      machine.stop();
      socket = null;
    };
  });

  const handlers: ScreenHandlers = {
    onsend: (text) => {
      problem = '';
      const working = isWorking(session);
      const offline = connection !== 'open';
      const id = send({ type: 'message', data: { text, client_date: today(), tz: zone() } });
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
    onstop: () => send({ type: 'stop', data: {} }),
    onretry: () => send({ type: 'retry', data: {} }),
    onback: () => {
      if (window.history.length > 1) window.history.back();
      else void goto(resolve('/chat'));
    },
    onopenEntry: (entryID) => void goto(resolve('/entry/[id]', { id: entryID })),
    // A chat has no card, so the card's handlers are not reachable from this screen.
    onedit: () => undefined,
    oninclude: () => undefined,
    onaccept: () => undefined,
    onacceptall: () => undefined,
    onconfirm: () => undefined,
    ondiscard: () => undefined,
    oncancel: () => undefined,
  };
  void client;
</script>

<svelte:head>
  <title>Ask your library · Sepia</title>
</svelte:head>

<main class="screen">
  <TopBar title="Ask your library" back />

  <div class="scroll">
    <ChatScreen {session} {connection} {handlers} {pending} {problem} />
  </div>
</main>
