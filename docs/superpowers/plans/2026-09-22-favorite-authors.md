# Favorite Authors Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Per-browser favorite authors whose notes always appear at the top of search results.

**Architecture:** Pure frontend change to the AlpineJS `searchTab` component. Favorites live in localStorage; a fresh search snapshots them and issues two parallel PostgREST queries (`in.` for favorites, `not.in.` for the rest), concatenating favorites-first. Infinite scroll paginates only the rest stream. An AbortController cancels superseded searches.

**Tech Stack:** Vanilla JS + AlpineJS 3 (CDN), PostgREST behind nginx, no build step.

**Spec:** `docs/superpowers/specs/2026-09-22-favorite-authors-design.md`

**Verified constraints (live stack, do not re-litigate):**
- `noteauthorparticipantid=in.(A,B)` and `not.in.(A,B)` work, combined with `summary_ts=wfts.<q>`
- Encoded parens (`in.%28...%29`) return `[]` — encode each id individually, keep `(`, `)`, `,` raw
- `www/` is volume-mounted (`./www:/home/www`) — edits are live on the running stack at http://localhost:8080, no rebuild needed
- AGENTS.md: no automated tests in this repo — verification is manual (curl + browser). Playwright browser tools are available for browser checks.

**Worktree note:** none — the running compose stack serves this working directory; a worktree would not be served.

---

### Task 1: Favorite toggle (state + star button + CSS)

**Files:**
- Modify: `www/index.html` (searchTab data object ~lines 186-250; note card template ~line 339; stylesheet link line 7)
- Modify: `www/styles.css` (append star styles)

- [ ] **Step 1: Add favorites state and methods to the searchTab data object**

In `www/index.html`, in `Alpine.data('searchTab', () => ({ ... }))`, immediately after the line `hasMore: true,` insert:

```js
                favorites: [],
                isFavorite(authorId) {
                    return this.favorites.includes(authorId);
                },
                toggleFavorite(authorId) {
                    if (!authorId) return;
                    const i = this.favorites.indexOf(authorId);
                    if (i >= 0) this.favorites.splice(i, 1); else this.favorites.push(authorId);
                    localStorage.setItem('favoriteAuthors', JSON.stringify(this.favorites));
                },
```

And replace the existing `init()`:

```js
                init() {
                    this.favorites = JSON.parse(localStorage.getItem('favoriteAuthors') || '[]');
                    this.setupInfiniteScroll();
                },
```

- [ ] **Step 2: Add the star button to the note card template**

In `www/index.html`, in the `x-for` note card template, the `.note-meta` div currently contains `.note-date` then `.author-badge`. Insert the star button between them (before the `<span class="author-badge"` line):

```html
                            <button class="fav-btn" :class="{ 'active': isFavorite(item.noteauthorparticipantid) }" x-show="item.noteauthorparticipantid" @click="toggleFavorite(item.noteauthorparticipantid)" :title="isFavorite(item.noteauthorparticipantid) ? 'Remove from favorite authors' : 'Add to favorite authors'">
                                <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                                    <polygon points="12 2 15.09 8.26 22 9.27 17 14.14 18.18 21.02 12 17.77 5.82 21.02 7 14.14 2 9.27 8.91 8.26 12 2"></polygon>
                                </svg>
                            </button>
```

- [ ] **Step 3: Add CSS and bump the stylesheet version**

Append to `www/styles.css`:

```css
/* Favorite author star */
.fav-btn {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    width: 26px;
    height: 26px;
    padding: 0;
    background: transparent;
    border: none;
    border-radius: var(--radius-sm);
    color: var(--text-muted);
    cursor: pointer;
    transition: all var(--transition);
}

.fav-btn:hover {
    color: var(--warning);
    background: var(--bg-card-hover);
}

.fav-btn.active {
    color: var(--warning);
}

.fav-btn.active svg {
    fill: var(--warning);
}
```

In `www/index.html` line 7, bump the cache-busting version: `href="/styles.css?v=15"` → `href="/styles.css?v=16"`.

- [ ] **Step 4: Verify the toggle in the browser (stack is live at http://localhost:8080)**

Using playwright browser tools (or a manual browser):

1. Navigate to `http://localhost:8080`
2. Type `ukraine` in the search input and press Enter; wait for note cards
3. Click the star on the first note card — it must turn gold/filled
4. Evaluate in page context:
```js
() => ({
    favorites: Alpine.$data(document.querySelector('[x-data="searchTab()"]')).favorites,
    stored: localStorage.getItem('favoriteAuthors'),
    active: document.querySelector('.fav-btn')?.classList.contains('active')
})
```
Expected: `favorites` is a 1-element array with that card's author id, `stored` matches, `active: true`
5. Reload the page, evaluate `localStorage.getItem('favoriteAuthors')` — the favorite persists; search again and the star renders already-active on that author's notes
6. Click the star again — it un-fills, `favorites` is empty

- [ ] **Step 5: Commit**

```bash
git add www/index.html www/styles.css
git commit -m "feat: add favorite-author star toggle with localStorage persistence"
```

---

### Task 2: Favorites-first search (two-query flow + abort handling)

**Files:**
- Modify: `www/index.html` (searchTab data object — add fields/helpers, replace `fetchItems`)

- [ ] **Step 1: Add fields and helpers, replace fetchItems**

In `www/index.html` `Alpine.data('searchTab', ...)`, immediately after the `favorites: [],` line from Task 1 insert:

```js
                searchFavorites: [],
                abortController: null,
                queryUrl(limit, offset, authorFilter) {
                    let url = '/data/note?limit=' + limit + '&offset=' + offset +
                        '&summary_ts=wfts.' + encodeURIComponent(this.search) +
                        '&select=createdatmillis,noteid,tweetid,noteauthorparticipantid,summary&order=createdatmillis.desc';
                    if (authorFilter) url += '&noteauthorparticipantid=' + authorFilter;
                    return url;
                },
                async fetchJson(url, signal) {
                    const response = await fetch(url, { signal });
                    if (!response.ok) throw new Error('Fetch failed with status ' + response.status);
                    return response.json();
                },
```

Replace the entire existing `fetchItems` method with:

```js
                async fetchItems(append = false) {
                    if (!append) {
                        if (this.abortController) this.abortController.abort();
                        this.abortController = new AbortController();
                        this.items = [];
                        this.offset = 0;
                        this.hasMore = true;
                        this.error = '';
                        this.searchFavorites = this.favorites.map(encodeURIComponent);
                    } else {
                        if (!this.hasMore || this.loading) return;
                        if (!this.abortController) this.abortController = new AbortController();
                    }
                    const ctrl = this.abortController;
                    this.loading = true;
                    try {
                        let newItems;
                        if (this.searchFavorites.length > 0) {
                            const favFilter = 'in.(' + this.searchFavorites.join(',') + ')';
                            const restFilter = 'not.in.(' + this.searchFavorites.join(',') + ')';
                            if (!append) {
                                // ponytail: favorites capped at 100 per search; paginate favorites too if it ever matters
                                const [favRes, restRes] = await Promise.allSettled([
                                    this.fetchJson(this.queryUrl(100, 0, favFilter), ctrl.signal),
                                    this.fetchJson(this.queryUrl(50, this.offset, restFilter), ctrl.signal)
                                ]);
                                if (ctrl !== this.abortController) return;
                                if (favRes.status === 'rejected' && restRes.status === 'rejected') throw favRes.reason;
                                if (favRes.status === 'rejected' || restRes.status === 'rejected') {
                                    const reason = favRes.status === 'rejected' ? favRes.reason : restRes.reason;
                                    this.error = 'Partial results, one query failed: ' + reason.message;
                                }
                                const favData = favRes.status === 'fulfilled' ? favRes.value : [];
                                const restData = restRes.status === 'fulfilled' ? restRes.value : [];
                                newItems = [...favData, ...restData];
                                this.offset += restData.length;
                                this.hasMore = restData.length === 50;
                            } else {
                                const restData = await this.fetchJson(this.queryUrl(50, this.offset, restFilter), ctrl.signal);
                                if (ctrl !== this.abortController) return;
                                newItems = restData;
                                this.offset += restData.length;
                                this.hasMore = restData.length === 50;
                            }
                        } else {
                            const data = await this.fetchJson(this.queryUrl(50, this.offset, null), ctrl.signal);
                            if (ctrl !== this.abortController) return;
                            newItems = data;
                            this.offset += data.length;
                            this.hasMore = data.length === 50;
                        }
                        this.items = append ? [...this.items, ...newItems] : newItems;
                    } catch (e) {
                        if (e.name === 'AbortError' || ctrl !== this.abortController) return;
                        this.error = 'Failed to load notes: ' + e.message;
                    } finally {
                        if (ctrl === this.abortController) this.loading = false;
                    }
                },
```

Design notes for the implementer (do not change):
- Fresh searches never check `this.loading` — they supersede: abort the old controller, then proceed. A stale call's `finally` is guarded by `ctrl === this.abortController` so it cannot clobber the new call's `loading`/`error` state.
- `searchFavorites` is a snapshot (encoded) taken at search time; toggling stars mid-results only affects the *next* search — this is the spec's behavior.
- `offset`/`hasMore` track only the `not.in.` (rest) stream when favorites exist; the favorites query runs exactly once per search at `limit=100`.

- [ ] **Step 2: Verify favorites-first ordering**

With one author favorited (from Task 1), in the browser:

1. Search `ukraine`
2. Evaluate:
```js
() => {
    const d = Alpine.$data(document.querySelector('[x-data="searchTab()"]'));
    const favs = d.searchFavorites;
    const k = d.items.filter(i => favs.includes(i.noteauthorparticipantid)).length;
    return {
        count: d.items.length,
        favNotesInResults: k,
        firstKAAllFavorites: d.items.slice(0, k).every(i => favs.includes(i.noteauthorparticipantid)),
        restHasNoFavorites: d.items.slice(k).every(i => !favs.includes(i.noteauthorparticipantid)),
        firstAuthor: d.items[0]?.noteauthorparticipantid
    };
}
```
Expected: `firstKAAllFavorites: true`, `restHasNoFavorites: true`, `firstAuthor` is the favorited author, `count` = favorites-matching + up-to-50 rest

- [ ] **Step 3: Verify infinite scroll appends non-favorite pages**

Evaluate (simulates the sentinel trigger):
```js
() => Alpine.$data(document.querySelector('[x-data="searchTab()"]')).fetchItems(true)
```
Then re-run the Step 2 evaluation. Expected: `count` grew by up to 50, `restHasNoFavorites` still `true`, no favorite-author note in the appended slice.

- [ ] **Step 4: Verify unfavorite returns to single-query behavior**

1. Click the star to unfavorite the author
2. Search `ukraine` again
3. Evaluate: `searchFavorites` is `[]`, `items.length` ≤ 50 and > 0, first item's author is simply the newest matching note (no ordering constraint)

- [ ] **Step 5: Commit**

```bash
git add www/index.html
git commit -m "feat: favorite authors' notes always first in search results"
```

---

### Task 3: Regression and edge pass

**Files:** none expected to change (fix-ups only if verification fails)

- [ ] **Step 1: No-favorites path is byte-identical to the old behavior**

1. Ensure favorites empty (unfavorite all, or evaluate `localStorage.removeItem('favoriteAuthors')` and reload)
2. Search `ukraine`, scroll/append at least one extra page
3. Expected: results paginate exactly as before (50-item batches, `hasMore` flips false when a short batch arrives). The request URL visible in devtools network tab (or via curl) is the single unfiltered query:
```bash
curl -s "http://localhost:8080/data/note?limit=50&offset=0&summary_ts=wfts.ukraine&select=createdatmillis,noteid,tweetid,noteauthorparticipantid,summary&order=createdatmillis.desc" | python3 -c "import sys,json;print(len(json.load(sys.stdin)))"
```
Expected: a number ≤ 50

- [ ] **Step 2: Two-query shape sanity (curl, mirrors what the browser sends)**

```bash
A=<some author id from a search result>
curl -s "http://localhost:8080/data/note?limit=100&offset=0&summary_ts=wfts.ukraine&noteauthorparticipantid=in.($A)&select=noteid&order=createdatmillis.desc" | python3 -c "import sys,json;print('fav:',len(json.load(sys.stdin)))"
curl -s "http://localhost:8080/data/note?limit=50&offset=0&summary_ts=wfts.ukraine&noteauthorparticipantid=not.in.($A)&select=noteid&order=createdatmillis.desc" | python3 -c "import sys,json;print('rest:',len(json.load(sys.stdin)))"
```
Expected: both print numbers; `fav` + displayed-favorites must match the browser

- [ ] **Step 3: Code-review-only edges (no data-dependent check possible)**

Confirm in code, no browser check needed: null-author notes render no star (`x-show` guard + `toggleFavorite` null guard); a superseded search's `AbortError` is swallowed; partial query failure shows the succeeded half plus an error banner.

- [ ] **Step 4: Final state**

```bash
git status
git log --oneline -4
```
Expected: clean tree, 3 feature commits on top of the spec commit (`e097769`).

---

## Self-Review (completed)

- **Spec coverage:** star toggle (Task 1), persistence (Task 1 Step 4), favorites-first across whole result set (Task 2), infinite scroll on rest stream (Task 2 Step 3), empty-favorites = current behavior (Task 3 Step 1), 100-cap (Task 2 code + ponytail comment), partial failure (Task 2 code), abort (Task 2 code), null author (Task 1 template + method guard), no live re-sort (snapshot design note), raw delimiters (verified constraint header). All spec rows have a task.
- **Placeholders:** none — every step carries complete code or exact commands.
- **Consistency:** `searchFavorites` holds *encoded* ids in both tasks; `isFavorite`/`toggleFavorite` operate on the live `favorites` array; `queryUrl`/`fetchJson` signatures match all call sites.
