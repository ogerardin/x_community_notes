# Favorite Authors — Design

Date: 2026-09-22
Status: Approved

## Overview

Users can flag authors as favorites (per-browser, stored in localStorage). Search results always show notes from favorite authors first, followed by all other matching results — across the entire result set, not just per-batch. Pure frontend feature: no backend, database, or nginx changes.

## Requirements

- Star toggle on each note card (next to the author badge) marks/unmarks that author as a favorite
- Favorites persist per-browser in localStorage
- On every search, notes from favorite authors appear above all other results
- Infinite scroll continues to paginate non-favorite results after favorites are shown
- With no favorites, search behaves exactly as today

## Design

### Storage

- localStorage key `favoriteAuthors`: JSON array of `noteauthorparticipantid` strings
- Loaded on component init; written on every toggle
- The live array drives star state only; queries use a snapshot (`searchFavorites`, captured at search time) so a result list always reflects the favorites set from when the search ran

### UI

- Star button in `.note-meta` before the author badge, styled like existing controls
- Outline star = not favorited; filled star (`var(--warning)`) = favorited; single SVG, fill toggled via CSS class
- Click toggles instantly (no confirmation); not rendered on notes with null author id (column is nullable)
- No manage-favorites UI; unfavorite via the star on any of that author's notes
- Live re-sorting of already-displayed results is out of scope: results reflect the favorites set at search time; re-run the search to re-sort

### Search flow (two-query)

With a non-empty favorites list, a fresh search runs two parallel PostgREST queries:

1. Favorites (once per search): `GET /data/note?limit=100&summary_ts=wfts.<q>&noteauthorparticipantid=in.(<favs>)&select=createdatmillis,noteid,tweetid,noteauthorparticipantid,summary&order=createdatmillis.desc`
2. Rest (paginated): `GET /data/note?limit=50&offset=<n>&summary_ts=wfts.<q>&noteauthorparticipantid=not.in.(<favs>)&select=<same>&order=createdatmillis.desc`

Displayed items = favorites results then rest results. Favorites are capped at 100 (most recent first); infinite scroll paginates only the rest query (batches of 50, `hasMore` = last batch was full).

With an empty favorites list: single unfiltered query — identical to current behavior.

Each author id is individually `encodeURIComponent`-ed; list delimiters (commas, parentheses) stay raw — PostgREST returns empty results for encoded parens in `in.()` filters (verified against the live stack).

## Edge cases

| Case | Behavior |
|------|----------|
| No favorites | Single query path, current behavior unchanged |
| >100 matching favorite notes | Capped at 100 most recent (ponytail ceiling; paginate favorites if it ever matters) |
| One of the two queries fails | Show whichever succeeded + error message; both fail → error only |
| Loading state | Overlay stays visible until both queries settle (success or error) |
| New search while queries in flight | AbortController cancels stale requests; AbortError silently ignored |
| Note with null author id | No star rendered; toggle guards against null |
| Toggling a star mid-results | Star updates instantly; list order unchanged until next search |
| Multiple tabs | No live sync (no `storage` listener); each tab reads favorites at its own search time |
| Duplicate notes between queries | Impossible: `in.` / `not.in.` partition the table |
| Nginx proxy cache | Two query URLs → distinct cache keys; no interaction |
| Very large favorites list | URL grows ~20 chars/id; fine within nginx 8k header default for any realistic list |

## Verification

No automated tests exist in this repo (per AGENTS.md). Manual checks:

1. `curl` the running stack with `in.` / `not.in.` filters to confirm PostgREST accepts them — DONE at design time: both filters verified, including combined with `wfts.`, and the encoded-paren failure documented above
2. Browser: favorite an author → star fills; search → their notes on top; reload → favorite persists; unfavorite → single-query behavior returns
3. Infinite scroll still loads additional non-favorite pages after favorites

## Files changed

- `www/index.html` — favorites state, star button, two-query fetch with AbortController
- `www/styles.css` — star button styles
