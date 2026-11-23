# S16. Library search and chat

## Purpose

Make the library searchable by your own words, filterable, and askable. Full-text search over titles, bodies and
answers with snippets, duplicate detection, and the general chat: questions answered inline and edits that go
through a confirm card, opened deliberately from the Library screen.

## Depends on

S12 (the library), S08 (the agent and `search_library`), S04 (FTS tables). Phase 2.

## Where we start

The Library screen lists and filters by medium; search and the Ask button have reserved places. The tool
`search_library` exists as a minimal read in S08.

## Settled

- SQLite FTS5 over titles, bodies and answers, with sentence snippets and highlighted matches.
- The general chat **may read and edit, with a confirm card each time, and can never delete**. An edit opens a
  draft revision of that entry with the same card and confirm; the previous version is kept.
- Open the chat on purpose from an **Ask button in the Library screen**; its start screen shows example
  questions and the last few chats (`GET /api/sessions?kind=general`). A question typed into the home composer
  is routed there by the `intent` decision.
- `sessions.title` holds a general chat's first question. Past chats are listed newest first.
- Search queries are at most 200 characters; page size 50, max 100.

## Scope

1. **S16.1 FTS.** The index, triggers or rebuild strategy that keep it in step with entries, a safe query parser
   (user text never reaches FTS syntax raw), ranking, snippets, and a rebuild command.
2. **S16.2 Library queries.** Filters (medium, rating, favourite, date), sorts, cursor pagination, and counts.
3. **S16.3 Duplicates and revisits.** Same-work detection used by the revisit flow and by the "this already
   exists" path; ambiguous matches ask, never guess.
4. **S16.4 Chat tools.** `search_library` with filters, an `edit_entry` tool that can only open a draft revision
   (no direct write, no delete tool exists), and the routing from home.
5. **S16.5 UI.** The search field and results (page 03 frames 4 and 5), the Ask button, the chat start screen, the
   answer list inline, and the confirm card for an edit (page 04).
6. **S16.6 Performance.** A seeded library of thousands of entries and a budget for search latency.

## Tests and exit

| Route | Cases |
|---|---|
| Happy | A phrase from a review finds it with the right snippet; "make hades 2 a 4.5" opens a confirm card and, on confirm, writes a revision |
| Bad | FTS syntax characters and SQL-shaped input are treated as plain text; a query over 200 characters is refused; a prompt asking the chat to delete an entry has no tool to do it and says so; an edit with no confirm changes nothing |
| Sad | An index out of step is detected and rebuilt; a slow search returns within budget or shows a skeleton and retries; the model erroring mid-edit leaves the entry untouched |

**Fuzz:** the query parser. **Performance:** search latency and memory on a large seeded library.

## Gates

None.

## Decisions

Answered by Autumn on 2026-11-22, in the batch before the slice started.

1. **Ranking weights: title, then body, then answers.** FTS5's `bm25` is weighted `title=3.0, body=1.0,
   answers=0.5`, so a title hit always outranks a sentence she wrote on the card, and an answer she gave a
   chip for never outranks her own prose. Recorded here, and in `PLAN.md` 5 where the library view is listed.
2. **Two pull requests.** Search (S16.1, S16.2, S16.3, S16.6 and the search UI) lands first and is shippable
   alone; the general chat (S16.4, S16.5) follows on its own branch, so each is a diff a reviewer can hold in
   their head and the library becomes searchable before the chat exists.
3. **A hit carries its entry, and both lists carry a total.** The contract's `SearchHit` gained an optional `entry`
   so a result row draws without a request per hit, and `/api/entries` and `/api/search` gained a required `total`
   so the tool row says how many there are rather than how many fitted on the page. Both are additive and both are in
   `api/openapi.yaml` in the same commit as the code that sends them.
4. **The snippet marks its matches with private-use characters, not HTML.** U+E000 and U+E001 rather than `<mark>`:
   a snippet is her own text, so the browser escapes everything between them and a review of "<script>" stays a
   sentence about a script. Outside the UI, `search_library` strips them and shows the model plain words.

## Open questions

None. Both questions above were answered before the slice started; anything a builder meets afterwards is a
judgement call recorded in this section, or trigger T10.

## Done when

Search finds your own words, the filters work on a phone, the general chat answers questions and edits only
through the confirm card, and no path in the chat can delete.
