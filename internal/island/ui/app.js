// jbn-player popup UI. Preact + htm (vendored), no build step.
// Go functions are exposed on window by the popup process (see island_windows.go).
const { html, render, useState, useEffect, useRef } = window.UI;

// ---------- Icons: one line style for everything, plus a few filled player glyphs ----------

const FILL = {
  play: 'M8 5v14l11-7z',
  pause: 'M6 5h4v14H6zM14 5h4v14h-4z',
  next: 'M6 18l8.5-6L6 6v12zM16 6v12h2V6h-2z',
  prev: 'M6 6h2v12H6zM9.5 12l8.5 6V6z',
  shuffle: 'M10.59 9.17L5.41 4 4 5.41l5.17 5.17 1.42-1.41zM14.5 4l2.04 2.04L4 18.59 5.41 20 17.96 7.46 20 9.5V4h-5.5zm.33 9.41l-1.41 1.41 3.13 3.13L14.5 20H20v-5.5l-2.04 2.04-3.13-3.13z',
  note: 'M12 3v10.55A4 4 0 1 0 14 17V7h4V3h-6z',
  volume: 'M3 9v6h4l5 5V4L7 9H3zm13.5 3A4.5 4.5 0 0 0 14 8v8a4.5 4.5 0 0 0 2.5-4zM14 3.2v2.1a7 7 0 0 1 0 13.4v2.1a9 9 0 0 0 0-17.6z',
};
const LINE = {
  albums: '<circle cx="12" cy="12" r="9"/><circle cx="12" cy="12" r="2.5"/>',
  songs: '<path d="M9 18V5l11-2v13"/><circle cx="6" cy="18" r="3"/><circle cx="17" cy="16" r="3"/>',
  playlists: '<path d="M4 6h12M4 12h12M4 18h7"/><path d="M19 9v7"/><circle cx="17" cy="17" r="2.2"/>',
  playing: '<path d="M6 20V10M12 20V4M18 20v-7"/>',
  settings: '<path d="M4 7h9M17 7h3M4 17h3M11 17h9"/><circle cx="15" cy="7" r="2"/><circle cx="9" cy="17" r="2"/>',
  playnext: '<path d="M5 5l8 7-8 7z"/><path d="M18 5v14"/>',
  queueadd: '<path d="M4 6h11M4 12h11M4 18h7"/><path d="M18 14v6M15 17h6"/>',
  trash: '<path d="M4 7h16M10 11v6M14 11v6M6 7l1 13h10l1-13M9 7V4h6v3"/>',
  edit: '<path d="M4 20h4L19 9l-4-4L4 16v4z"/>',
  check: '<path d="M5 12l5 5 9-10"/>',
  back: '<path d="M15 5l-7 7 7 7"/>',
  close: '<path d="M6 6l12 12M18 6L6 18"/>',
  sync: '<path d="M20 12a8 8 0 1 0-2.3 5.7M20 5v7h-7"/>', // centered on (12,12) so it turns around its middle
  search: '<circle cx="11" cy="11" r="7"/><path d="M20 20l-4-4"/>',
  download: '<path d="M12 4v11M7 11l5 5 5-5M5 20h14"/>',
  offlineon: '<circle cx="12" cy="12" r="9"/><path d="M8 12l3 3 5-6"/>',
  grip: '<g fill="currentColor" stroke="none"><circle cx="9" cy="6" r="1.5"/><circle cx="15" cy="6" r="1.5"/><circle cx="9" cy="12" r="1.5"/><circle cx="15" cy="12" r="1.5"/><circle cx="9" cy="18" r="1.5"/><circle cx="15" cy="18" r="1.5"/></g>',
};
const Icon = ({ name, size = 20 }) =>
  FILL[name]
    ? html`<svg viewBox="0 0 24 24" width=${size} height=${size} fill="currentColor" aria-hidden="true"><path d=${FILL[name]} /></svg>`
    : html`<svg viewBox="0 0 24 24" width=${size} height=${size} fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true" dangerouslySetInnerHTML=${{ __html: LINE[name] }} />`;

// A button that is only an icon: the tooltip doubles as its accessible name.
const IconBtn = ({ name, title, onClick, cls = '', size = 18, disabled }) =>
  html`<button class=${'icon ghost ' + cls} title=${title} aria-label=${title} disabled=${disabled} onClick=${onClick}><${Icon} name=${name} size=${size} /></button>`;

// ---------- Look: accent color + surface tone ----------

const TONES = {
  graphite: { bg: '#14141a', card: '#1d1d26', 'card-hover': '#262633', line: '#2c2c3a', nav: '#101015', 'nav-hover': '#16161d', 'btn-hover': '#30303f', scroll: '#2f2f3f', 'scroll-hover': '#3d3d52', 'input-hover': '#3a3a4c', toast: '#2a2a38', grip: '#303046', 'grip-hover': '#4a4a66' },
  midnight: { bg: '#0e1320', card: '#172033', 'card-hover': '#1f2b42', line: '#263250', nav: '#0b0f1a', 'nav-hover': '#111827', 'btn-hover': '#2a3855', scroll: '#2a3855', 'scroll-hover': '#3a4b70', 'input-hover': '#364a74', toast: '#1f2b42', grip: '#2a3855', 'grip-hover': '#3a4b70' },
  black: { bg: '#000000', card: '#111114', 'card-hover': '#1a1a1f', line: '#232328', nav: '#000000', 'nav-hover': '#0b0b0e', 'btn-hover': '#26262c', scroll: '#26262c', 'scroll-hover': '#36363e', 'input-hover': '#34343c', toast: '#1a1a1f', grip: '#26262c', 'grip-hover': '#3a3a44' },
  warm: { bg: '#161310', card: '#201c18', 'card-hover': '#2a251f', line: '#35302a', nav: '#110e0b', 'nav-hover': '#181410', 'btn-hover': '#3a342c', scroll: '#3a342c', 'scroll-hover': '#4d463c', 'input-hover': '#4a433a', toast: '#2a251f', grip: '#3a342c', 'grip-hover': '#4d463c' },
};
const TONE_NAMES = { graphite: 'Graphite', midnight: 'Midnight', black: 'Black', warm: 'Warm' };
const ACCENTS = ['#2d7be0', '#8b5cf6', '#ec4899', '#ef4444', '#f59e0b', '#22c55e', '#14b8a6', '#06b6d4'];

function applyAppearance(a) {
  const root = document.documentElement.style;
  root.setProperty('--accent', a.accent);
  for (const [k, v] of Object.entries(TONES[a.surface] || TONES.graphite)) root.setProperty('--' + k, v);
}

// ---------- Helpers ----------

const fmt = (s) => {
  s = Math.max(0, Math.floor(s || 0));
  return Math.floor(s / 60) + ':' + String(s % 60).padStart(2, '0');
};
const ago = (iso) => {
  if (!iso) return 'never';
  const s = (Date.now() - new Date(iso).getTime()) / 1000;
  if (s < 60) return 'just now';
  if (s < 3600) return Math.floor(s / 60) + ' min ago';
  if (s < 86400) return Math.floor(s / 3600) + ' h ago';
  return Math.floor(s / 86400) + ' d ago';
};

const Cover = ({ src, cls }) =>
  src
    ? html`<img class=${'cover ' + cls} src=${src} loading="lazy" alt="" />`
    : html`<div class=${'cover ' + cls}><${Icon} name="note" /></div>`;

const SearchBox = ({ value, onInput, placeholder }) =>
  html`<div class="search"><${Icon} name="search" size=${16} /><input type="search" placeholder=${placeholder} value=${value} onInput=${onInput} /></div>`;

// ---------- Offline copies: the downloads run in the player process; this is a shared view of them ----------

const OFF = { ids: new Set(), status: { count: 0, bytes: 0, pending: 0, failed: 0, lastError: '', current: '' }, subs: new Set(), timer: 0 };
const offNotify = () => OFF.subs.forEach((f) => f());
function useOff() {
  const [, set] = useState(0);
  useEffect(() => { const f = () => set((n) => n + 1); OFF.subs.add(f); return () => OFF.subs.delete(f); }, []);
  return OFF;
}
// Reads the status and the stored songs; keeps polling every 1.5 s only while downloads are pending.
async function offRefresh() {
  clearTimeout(OFF.timer);
  const [st, ids] = await Promise.all([window.offlineStatus(), window.offlineIDs()]);
  if (!st.error) OFF.status = st;
  OFF.ids = new Set(ids.ids || []);
  offNotify();
  if (!st.error && st.pending > 0) OFF.timer = setTimeout(offRefresh, 1500);
}
// op: add | remove | clear. Returns '' or an error message.
async function offDo(op, req) {
  const r = await window.offlineDo(op, req);
  if (r.error) return r.error;
  OFF.status = r;
  offNotify();
  offRefresh();
  return '';
}
const MB = (b) => (b >= 1e9 ? (b / 1e9).toFixed(1) + ' GB' : Math.round(b / 1e6) + ' MB');
const offItems = (ts) => ts.map((t) => ({ id: t.id, title: t.title }));

// ---------- songkey: YouTube links downloaded into the library ----------
// The Go side talks to songkey (the token never reaches this page). Followed
// jobs are polled every few seconds while one is still in progress.

const SK = { cfg: null, jobs: [], rev: 0, subs: new Set(), timer: 0, onFiled: null };
const skNotify = () => SK.subs.forEach((f) => f());
function useSk() {
  const [, set] = useState(0);
  useEffect(() => { const f = () => set((n) => n + 1); SK.subs.add(f); return () => SK.subs.delete(f); }, []);
  return SK;
}
const SK_DONE = new Set(['filed', 'failed', 'duplicate']);
const SK_LABEL = { received: 'Checking', queued: 'Downloading', downloaded: 'Adding to playlist', failed: 'Failed', duplicate: 'Already in library' };
function skTake(r) {
  SK.jobs = r.jobs || [];
  if (r.filed && r.filed.length) {
    if (r.refreshed) SK.rev++; // lists showing the library re-read it
    if (SK.onFiled) SK.onFiled(r.filed);
  }
  skNotify();
  clearTimeout(SK.timer);
  if (SK.jobs.some((j) => !SK_DONE.has(j.state))) SK.timer = setTimeout(skPoll, 4000);
}
async function skPoll() { skTake(await window.songkeyJobs()); }
async function skLoad() {
  SK.cfg = await window.getSongkey();
  skNotify();
  if (SK.cfg.configured) skPoll();
}

function SongkeyAdd({ toast }) {
  const sk = useSk();
  const [link, setLink] = useState('');
  const [busy, setBusy] = useState(false);
  const [reply, setReply] = useState(null); // { kind, text }
  if (!sk.cfg || !sk.cfg.configured) return '';
  const submit = async (e) => {
    e.preventDefault();
    if (!link.trim() || busy) return;
    setBusy(true);
    const r = await window.songkeyAdd(link.trim());
    setBusy(false);
    if (r.error) return setReply({ kind: 'err', text: r.error });
    setLink('');
    setReply({ kind: r.result === 'already_have' ? 'info' : 'ok', text: r.message });
    if (r.job) skPoll();
  };
  return html`<div class="card sk">
    <form class="row" onSubmit=${submit}>
      <input class="grow" type="text" placeholder="Paste a YouTube link (song or playlist)" spellcheck="false" value=${link}
        aria-label="YouTube link" onInput=${(e) => setLink(e.target.value)} />
      <button type="submit" class="primary" disabled=${busy || !link.trim()}>${busy ? 'Sending…' : 'Add'}</button>
    </form>
    ${reply ? html`<div class=${'skmsg ' + reply.kind}>${reply.text}<button class="icon ghost sm" title="Hide" aria-label="Hide" onClick=${() => setReply(null)}><${Icon} name="close" size=${13} /></button></div>` : ''}
    ${sk.jobs.length ? html`<div class="skjobs">${sk.jobs.map((j) => html`
      <div class="skjob" key=${j.id}>
        <div class="grow"><div class="row-title">${j.title || j.id}</div>
          <div class="sub">${[j.artist, j.reason].filter(Boolean).join(' · ')}</div></div>
        <span class=${'skstate ' + j.state}>${SK_DONE.has(j.state) ? '' : html`<span class="ico spin"><${Icon} name="sync" size=${12} /></span>`}${SK_LABEL[j.state] || j.state}</span>
        ${SK_DONE.has(j.state) ? html`<${IconBtn} name="close" title="Dismiss" cls="sm" size=${14} onClick=${async () => skTake(await window.songkeyDismiss(j.id))} />` : ''}
      </div>`)}</div>` : ''}
  </div>`;
}

function SongkeySettings({ toast }) {
  const sk = useSk();
  const cfg = sk.cfg || { url: '', hasToken: false, configured: false };
  const [url, setUrl] = useState(cfg.url);
  const [token, setToken] = useState('');
  const [status, setStatus] = useState({ kind: '', text: '' });
  const [busy, setBusy] = useState(false);
  const failedOnce = useRef(false);
  useEffect(() => { setUrl(cfg.url); }, [cfg.url]);
  const save = async (e, clear) => {
    e.preventDefault();
    setBusy(true);
    setStatus({ kind: 'busy', text: clear ? 'Removing…' : 'Connecting…' });
    const r = await window.saveSongkey(clear ? { url: '', token: '' } : { url, token, force: failedOnce.current });
    setBusy(false);
    if (r.error) { // an unreachable songkey must not block saving: pressing Save again keeps it
      failedOnce.current = true;
      return setStatus({ kind: 'err', text: r.error + ' — press Save again to keep it anyway' });
    }
    failedOnce.current = false;
    setToken('');
    setStatus({ kind: 'ok', text: clear ? 'Removed' : 'Connected' });
    skLoad();
    if (!clear) toast('songkey saved');
  };
  const edit = (set) => (e) => { failedOnce.current = false; set(e.target.value); };
  return html`<div class="card" style="margin-bottom:16px">
    <h2 style="margin-top:0">songkey</h2>
    <div class="sub" style="white-space:normal">Paste YouTube links in Playlists: songkey downloads the new songs into your library playlist.</div>
    <form autocomplete="off" novalidate onSubmit=${(e) => save(e, false)} style="margin-top:8px">
      <label>songkey URL<input placeholder="https://songkey.example.com" spellcheck="false" value=${url} onInput=${edit(setUrl)} /></label>
      <label>API token<input type="password" placeholder=${cfg.hasToken ? 'Unchanged' : ''} value=${token} onInput=${edit(setToken)} /></label>
      <div class=${'status ' + status.kind} role="status" aria-live="polite">${status.text ? html`<span class="dot"></span>${status.text}` : ''}</div>
      <div class="row">
        ${cfg.configured ? html`<button type="button" class="ghost danger" disabled=${busy} onClick=${(e) => save(e, true)}>Remove</button>` : ''}
        <span class="grow"></span>
        <button type="submit" class="primary" disabled=${busy || !url.trim()}>Save</button>
      </div>
    </form>
  </div>`;
}

// ---------- Shared: track rows and track lists ----------

// One big click target: clicking the row plays the song inside the current
// queue (it keeps what was queued and carries on with it afterwards; with an
// empty queue the list becomes the queue). Play next / add to queue / offline
// are icons that appear on hover; the offline mark stays visible.
function TrackRow({ t, n, tracks, start, act, toast, select }) {
  const off = useOff();
  const isOff = off.ids.has(t.id);
  // In select mode a click ticks the song instead of playing it.
  const play = () => (select ? select.toggle(t) : act('playnow', { tracks, start }));
  const add = async (e, op, msg) => { e.stopPropagation(); if (await act(op, { tracks: [t] })) toast(msg); };
  const toggleOff = async (e) => {
    e.stopPropagation();
    const err = isOff ? await offDo('remove', { ids: [t.id] }) : await offDo('add', { items: offItems([t]) });
    if (err) toast(err, 'err'); else toast(isOff ? 'Offline copy removed' : 'Downloading…');
  };
  return html`<div class="track clickable" tabindex="0" onClick=${play} onKeyDown=${(e) => { if (e.key === 'Enter') play(); }}>
    ${select
      ? html`<input type="checkbox" class="pick" checked=${select.checked(t)} tabindex="-1" aria-label=${'Select ' + t.title} />`
      : html`<span class="num"><span class="n">${n}</span><span class="p"><${Icon} name="play" size=${14} /></span></span>`}
    <div class="grow"><div class="row-title">${t.title}${isOff ? html`<span class="offmark" title="Available offline"><${Icon} name="offlineon" size=${13} /></span>` : ''}</div><div class="sub">${t.artist || ''}${t.duration ? ' · ' + fmt(t.duration) : ''}</div></div>
    ${select ? '' : html`<div class="acts">
      <${IconBtn} name=${isOff ? 'offlineon' : 'download'} title=${isOff ? 'Remove offline copy' : 'Make available offline'} cls="sm" onClick=${toggleOff} />
      <${IconBtn} name="playnext" title="Play next" cls="sm" onClick=${(e) => add(e, 'playnext', 'Playing next')} />
      <${IconBtn} name="queueadd" title="Add to queue" cls="sm" onClick=${(e) => add(e, 'enqueue', 'Added to queue')} />
    </div>`}
  </div>`;
}

// An album or a playlist: header + Play / Shuffle + secondary icons + tracks.
function TracksView({ head, load, onBack, act, toast, rev = 0 }) {
  const [res, setRes] = useState(null);
  useEffect(() => { load().then(setRes); }, [rev]);
  const tracks = res ? res.items : [];
  const none = !tracks.length;
  const all = async (op, msg) => { if (await act(op, { tracks })) toast(msg); };
  const off = useOff();
  const allOff = !none && tracks.every((t) => off.ids.has(t.id));
  const toggleAllOff = async () => {
    const err = allOff ? await offDo('remove', { ids: tracks.map((t) => t.id) }) : await offDo('add', { items: offItems(tracks) });
    if (err) toast(err, 'err'); else toast(allOff ? 'Offline copies removed' : 'Downloading ' + tracks.length + ' songs…');
  };
  return html`<main>
    <div class="row" style="margin-bottom:8px"><${IconBtn} name="back" title="Back" onClick=${onBack} /></div>
    <div class="row" style="gap:14px;align-items:flex-start">
      <${Cover} src=${head.cover} cls="m" />
      <div class="grow"><h1>${head.title}</h1><div class="sub">${head.sub}</div></div>
    </div>
    <div class="row" style="margin:14px 0">
      <button class="primary with-icon" disabled=${none} onClick=${() => act('play', { tracks, start: -1 })}><${Icon} name="play" size=${16} /> Play</button>
      <button class="with-icon" disabled=${none} onClick=${async () => { await act('shuffle', { on: true }); act('play', { tracks, start: -1 }); }}><${Icon} name="shuffle" size=${16} /> Shuffle</button>
      <span class="grow"></span>
      <${IconBtn} name=${allOff ? 'offlineon' : 'download'} title=${allOff ? 'Remove offline copies' : 'Make available offline'} disabled=${none} cls=${allOff ? 'on' : ''} onClick=${toggleAllOff} />
      <${IconBtn} name="playnext" title="Play next" disabled=${none} onClick=${() => all('playnext', 'Playing next')} />
      <${IconBtn} name="queueadd" title="Add to queue" disabled=${none} onClick=${() => all('enqueue', 'Added to queue')} />
    </div>
    ${res && res.error ? html`<div class="banner">${res.error}</div>` : ''}
    ${!res ? html`<div class="empty">Loading…</div>` : ''}
    <div class="tracks">${tracks.map((t, i) => html`<${TrackRow} key=${t.id + ':' + i} t=${t} n=${i + 1} tracks=${tracks} start=${i} act=${act} toast=${toast} />`)}</div>
  </main>`;
}

// ---------- Sync ----------

function SyncBar({ sync }) {
  const { info, syncing, run } = sync;
  return html`<div class="syncbar">
    <div class="grow sub">${info && info.synced
      ? info.songs + ' songs · ' + info.playlists + ' playlists · synced ' + ago(info.at)
      : 'Not synced yet'}</div>
    <button class="icon ghost" title=${syncing ? 'Syncing…' : 'Sync library'} aria-label="Sync library" disabled=${syncing} onClick=${run}>
      <span class=${'ico' + (syncing ? ' spin' : '')}><${Icon} name="sync" size=${18} /></span></button>
  </div>`;
}

function NeedsSync({ sync }) {
  return html`<div class="empty">Sync your library to browse every song<br />and your playlists instantly.<br />
    <button class="primary with-icon" disabled=${sync.syncing} onClick=${sync.run}><${Icon} name="sync" size=${16} /> ${sync.syncing ? 'Syncing…' : 'Sync now'}</button></div>`;
}

// ---------- Albums (live from the server) ----------

const KINDS = [['newest', 'Newest'], ['random', 'Random'], ['alphabeticalByName', 'A–Z'], ['recent', 'Recent']];

function Albums({ act, toast, goSettings }) {
  const [kind, setKind] = useState('newest');
  const [albums, setAlbums] = useState([]);
  const [more, setMore] = useState(false);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [q, setQ] = useState('');
  const [results, setResults] = useState(null);
  const [album, setAlbum] = useState(null);
  const searchSeq = useRef(0);

  const load = async (reset) => {
    setLoading(true);
    const r = await window.albums(kind, reset ? 0 : albums.length);
    setLoading(false);
    setError(r.error || '');
    setAlbums(reset ? r.items : albums.concat(r.items));
    setMore(r.items.length >= 30);
  };
  useEffect(() => { load(true); }, [kind]);

  useEffect(() => {
    const term = q.trim();
    if (!term) { setResults(null); return; }
    const seq = ++searchSeq.current;
    const id = setTimeout(async () => {
      const r = await window.searchLibrary(term);
      if (seq === searchSeq.current) setResults(r);
    }, 300);
    return () => clearTimeout(id);
  }, [q]);

  // One click on the round button plays the whole album, without opening it.
  const playAlbum = async (a) => {
    const r = await window.albumTracks(a.id);
    if (r.error) return toast(r.error, 'err');
    act('play', { tracks: r.items, start: -1 });
  };

  if (album) {
    return html`<${TracksView} head=${{ title: album.name, sub: album.artist + (album.year ? ' · ' + album.year : ''), cover: album.cover }}
      load=${() => window.albumTracks(album.id)} onBack=${() => setAlbum(null)} act=${act} toast=${toast} />`;
  }

  const grid = (list) => html`<div class="grid">${list.map((a, i) => html`
    <div class="album" key=${a.id} style=${{ animationDelay: Math.min(i, 12) * 25 + 'ms' }}>
      <button class="open" onClick=${() => setAlbum(a)}>
        <${Cover} src=${a.cover} cls="" />
        <div><div class="name">${a.name}</div><div class="sub">${a.artist}</div></div>
      </button>
      <button class="fab icon" title="Play" aria-label=${'Play ' + a.name} onClick=${() => playAlbum(a)}><${Icon} name="play" size=${20} /></button>
    </div>`)}</div>`;

  const noServer = error === 'no server configured';
  return html`<main>
    <${SearchBox} value=${q} onInput=${(e) => setQ(e.target.value)} placeholder="Search albums and songs" />
    ${noServer
      ? html`<div class="empty">No server configured yet.<br /><button class="primary" onClick=${goSettings}>Add a server</button></div>`
      : results
        ? html`
          ${results.error ? html`<div class="banner">${results.error}</div>` : ''}
          ${results.albums.length ? html`<h2>Albums</h2>${grid(results.albums)}` : ''}
          ${results.songs.length ? html`<h2>Songs</h2><div class="tracks">${results.songs.map((t, i) => html`<${TrackRow} key=${t.id} t=${t} n=${i + 1} tracks=${results.songs} start=${i} act=${act} toast=${toast} />`)}</div>` : ''}
          ${!results.error && !results.albums.length && !results.songs.length ? html`<div class="empty">No results.</div>` : ''}`
        : html`
          <div class="chips">${KINDS.map(([k, label]) => html`<button class=${'chip' + (k === kind ? ' active' : '')} onClick=${() => setKind(k)}>${label}</button>`)}</div>
          ${error ? html`<div class="banner">${error}</div>` : ''}
          ${grid(albums)}
          ${loading ? html`<div class="empty">Loading…</div>` : ''}
          ${!loading && more ? html`<div class="row" style="justify-content:center;margin-top:14px"><button onClick=${() => load(false)}>Load more</button></div>` : ''}`}
  </main>`;
}

// ---------- Songs: every track in one list (from the local copy) ----------

const SONGS_PAGE = 100;

function Songs({ act, toast, sync, playAll, goSettings }) {
  const [q, setQ] = useState('');
  const [items, setItems] = useState([]);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(true);
  const [synced, setSynced] = useState(true);
  const [error, setError] = useState('');
  const seq = useRef(0);
  const sentinel = useRef(null);
  const off = useOff();
  const [selecting, setSelecting] = useState(false);
  const [sel, setSel] = useState(new Map()); // id -> track, for "Download selected"
  const [armed, setArmed] = useState(false);
  const sk = useSk(); // songs songkey filed show up without a full sync

  const select = selecting ? {
    checked: (t) => sel.has(t.id),
    toggle: (t) => setSel((m) => { const n = new Map(m); if (n.has(t.id)) n.delete(t.id); else n.set(t.id, t); return n; }),
  } : null;
  const stopSelecting = () => { setSelecting(false); setSel(new Map()); };
  const downloadSelected = async () => {
    const fresh = [...sel.values()].filter((t) => !off.ids.has(t.id)); // never re-download what is stored
    const skipped = sel.size - fresh.length;
    stopSelecting();
    if (!fresh.length) return toast('Already offline');
    const err = await offDo('add', { items: offItems(fresh) });
    toast(err || 'Downloading ' + fresh.length + ' songs' + (skipped ? ' (' + skipped + ' already offline)' : ''), err ? 'err' : 'ok');
  };
  // One click on "Download all" only arms it (a big library is a lot of disk); the second one starts.
  const downloadAll = async () => {
    if (!armed) { setArmed(true); setTimeout(() => setArmed(false), 3000); return; }
    setArmed(false);
    const r = await window.offlineAll(q.trim());
    if (r.error) return toast(r.error, 'err');
    offRefresh();
    toast(r.queued ? 'Downloading ' + r.queued + ' songs' + (r.skipped ? ' (' + r.skipped + ' already offline)' : '') : 'Everything is already offline');
  };

  const load = async (reset) => {
    const my = ++seq.current;
    setLoading(true);
    const r = await window.songs(q.trim(), reset ? 0 : items.length, SONGS_PAGE);
    if (my !== seq.current) return; // a newer query superseded this one
    setLoading(false);
    setSynced(r.synced);
    setError(r.error || '');
    setTotal(r.total);
    setItems(reset ? r.items : items.concat(r.items));
  };
  // Reload on typing (debounced) and after a sync finishes.
  useEffect(() => {
    const id = setTimeout(() => load(true), q ? 250 : 0);
    return () => clearTimeout(id);
  }, [q, sync.info && sync.info.at, sk.rev]);

  // Infinite scroll: load the next page when the end of the list is visible.
  useEffect(() => {
    if (!sentinel.current) return;
    const io = new IntersectionObserver((e) => {
      if (e[0].isIntersecting && !loading && items.length < total) load(false);
    });
    io.observe(sentinel.current);
    return () => io.disconnect();
  }, [items.length, total, loading]);

  const searching = q.trim() !== '';
  return html`<main>
    <${SyncBar} sync=${sync} />
    ${error === 'no server configured'
      ? html`<div class="empty">No server configured yet.<br /><button class="primary" onClick=${goSettings}>Add a server</button></div>`
      : !synced && !loading
        ? html`<${NeedsSync} sync=${sync} />`
        : html`
          <${SearchBox} value=${q} onInput=${(e) => setQ(e.target.value)} placeholder="Search all songs" />
          ${selecting ? html`<div class="row" style="margin:12px 0">
            <span class="sub grow">${sel.size} selected</span>
            <button class="sm" onClick=${() => setSel(new Map(items.map((t) => [t.id, t])))}>Select shown</button>
            <button class="primary sm with-icon" disabled=${!sel.size} onClick=${downloadSelected}><${Icon} name="download" size=${14} /> Download</button>
            <button class="ghost sm" onClick=${stopSelecting}>Cancel</button>
          </div>` : html`<div class="row" style="margin:12px 0">
            ${!searching ? html`<button class="primary with-icon" disabled=${!total} onClick=${() => playAll(true)}><${Icon} name="shuffle" size=${16} /> Shuffle all</button>` : ''}
            <span class="grow sub">${total} songs</span>
            <button class="sm" disabled=${!total} onClick=${() => setSelecting(true)}>Select</button>
            <button class=${'sm with-icon' + (armed ? ' danger' : '')} disabled=${!total} onClick=${downloadAll}
              title="Downloads the songs that are not offline yet"><${Icon} name="download" size=${14} />
              ${armed ? 'Sure?' : searching ? 'Download ' + total + ' found' : 'Download all'}</button>
          </div>`}
          ${error ? html`<div class="banner">${error}</div>` : ''}
          <div class="tracks">${items.map((t, i) => html`<${TrackRow} key=${t.id} t=${t} n=${i + 1} tracks=${items} start=${i} act=${act} toast=${toast} select=${select} />`)}</div>
          ${!loading && synced && !items.length ? html`<div class="empty">${searching ? 'No songs match.' : 'The library is empty.'}</div>` : ''}
          ${loading ? html`<div class="empty">Loading…</div>` : ''}
          <div ref=${sentinel} style="height:1px"></div>`}
  </main>`;
}

// ---------- Playlists (from the local copy) ----------

function Playlists({ act, toast, sync }) {
  const [res, setRes] = useState(null);
  const [open, setOpen] = useState(null);
  const sk = useSk(); // a song songkey filed changes the playlist: re-read
  useEffect(() => { window.playlists().then(setRes); }, [sync.info && sync.info.at, sk.rev]);

  const playPlaylist = async (e, p) => {
    e.stopPropagation();
    const r = await window.playlistTracks(p.id);
    if (r.error) return toast(r.error, 'err');
    act('play', { tracks: r.items, start: -1 });
  };

  if (open) {
    return html`<${TracksView} head=${{ title: open.name, sub: open.count + ' songs', cover: open.cover }}
      load=${() => window.playlistTracks(open.id)} onBack=${() => setOpen(null)} act=${act} toast=${toast} rev=${sk.rev} />`;
  }
  return html`<main>
    <${SyncBar} sync=${sync} />
    <${SongkeyAdd} toast=${toast} />
    ${res && res.error ? html`<div class="banner">${res.error}</div>` : ''}
    ${res && !res.synced && !res.error ? html`<${NeedsSync} sync=${sync} />` : ''}
    ${res && res.synced && !res.items.length ? html`<div class="empty">This server has no playlists.</div>` : ''}
    <div class="tracks">${(res ? res.items : []).map((p, i) => html`
      <div class="track clickable plitem" key=${p.id} style=${{ animationDelay: Math.min(i, 12) * 30 + 'ms' }} onClick=${() => setOpen(p)}>
        <${Cover} src=${p.cover} cls="s" />
        <div class="grow"><div class="row-title">${p.name}</div><div class="sub">${p.count} songs</div></div>
        <button class="fab icon" title="Play" aria-label=${'Play ' + p.name} onClick=${(e) => playPlaylist(e, p)}><${Icon} name="play" size=${16} /></button>
      </div>`)}</div>
  </main>`;
}

// ---------- Volume: the player's own, the Windows volume is untouched ----------

function Volume({ value, onChange }) {
  return html`<div class="volume">
    <${Icon} name="volume" size=${18} />
    <input type="range" min="0" max="100" step="1" value=${value} aria-label="Volume"
      onInput=${(e) => onChange(+e.target.value, false)} onChange=${(e) => onChange(+e.target.value, true)} />
    <span class="pct">${value}%</span>
  </div>`;
}

// ---------- Seek: drag to move inside the song ----------

// While dragging, the slider shows the dragged spot (the 1 s poll must not
// pull it back); after release it holds that spot until mpv has caught up.
function Seek({ position, duration, act }) {
  const [hold, setHold] = useState(null);
  const timer = useRef(0);
  useEffect(() => () => clearTimeout(timer.current), []);
  const v = hold !== null ? hold : position;
  const seek = (pos) => {
    setHold(pos);
    clearTimeout(timer.current);
    act('seek', { position: pos }).finally(() => { timer.current = setTimeout(() => setHold(null), 1200); });
  };
  return html`
    <input type="range" class="seek" min="0" max=${Math.max(1, Math.floor(duration))} step="1" value=${Math.floor(v)}
      disabled=${!(duration > 0)} aria-label="Seek" aria-valuetext=${fmt(v)}
      onInput=${(e) => { clearTimeout(timer.current); setHold(+e.target.value); }} onChange=${(e) => seek(+e.target.value)} />
    <div class="times"><span>${fmt(v)}</span><span>${fmt(duration)}</span></div>`;
}

// ---------- Now playing + the single queue ----------

const QUEUE_WINDOW = 150;

function NowPlaying({ ps, act, goLibrary, setVolume }) {
  const [page, setPage] = useState({ items: [], from: 0, total: 0 });
  const [over, setOver] = useState(-1); // queue position under the dragged row
  const dragFrom = useRef(-1);
  const from = Math.max(0, ps.index - 3);
  // The queue can hold thousands of tracks: fetch only a window of it, and
  // only when the list changed (version) or the current track moved.
  useEffect(() => {
    let alive = true;
    window.playerQueue(from, QUEUE_WINDOW).then((r) => { if (alive && !r.error) setPage(r); });
    return () => { alive = false; };
  }, [ps.version, from]);

  const cur = ps.current;
  if (!cur) {
    return html`<main><div class="empty">Nothing playing yet.<br /><button class="primary" onClick=${goLibrary}>Browse the library</button></div></main>`;
  }
  const upcoming = ps.queueLen - ps.index - 1;
  const hidden = page.total - (page.from + page.items.length);
  return html`<main>
    ${ps.cover ? html`<img class="cover xl" src=${ps.cover} alt="" />` : html`<div class="cover xl"><${Icon} name="note" size=${64} /></div>`}
    <div class="np-title">${cur.title}</div>
    <div class="np-sub">${cur.artist}${cur.album ? ' · ' + cur.album : ''}</div>
    <${Seek} position=${ps.position} duration=${ps.duration || cur.duration} act=${act} />
    <div class="controls">
      <button class=${'icon' + (ps.shuffle ? ' on' : '')} title=${ps.shuffle ? 'Shuffle: on' : 'Shuffle: off'} aria-pressed=${ps.shuffle} onClick=${() => act('shuffle', { on: !ps.shuffle })}><${Icon} name="shuffle" /></button>
      <button class="icon" title="Previous" onClick=${() => act('prev')}><${Icon} name="prev" size=${26} /></button>
      <button class="icon big" title=${ps.playing ? 'Pause' : 'Play'} onClick=${() => act('toggle')}><${Icon} name=${ps.playing ? 'pause' : 'play'} size=${30} /></button>
      <button class="icon" title="Next" onClick=${() => act('next')}><${Icon} name="next" size=${26} /></button>
      <span style="width:34px"></span>
    </div>
    <${Volume} value=${ps.volume} onChange=${setVolume} />
    <div class="row between"><h2 style="margin:18px 0 8px">Queue · ${ps.queueLen}</h2>
      ${upcoming > 0 ? html`<button class="ghost sm with-icon" onClick=${() => act('clear')}><${Icon} name="trash" size=${14} /> Clear upcoming</button>` : ''}</div>
    <div class="tracks">${page.items.map((t, k) => {
      const i = page.from + k; // absolute position in the queue
      return html`
      <div class=${'track clickable' + (i === ps.index ? ' current' : '') + (over === i ? ' over' : '')} key=${i + ':' + t.id}
        draggable="true"
        onDragStart=${(e) => { dragFrom.current = i; e.dataTransfer.effectAllowed = 'move'; e.dataTransfer.setData('text/plain', String(i)); }}
        onDragOver=${(e) => { e.preventDefault(); if (over !== i) setOver(i); }}
        onDragLeave=${() => setOver(-1)}
        onDrop=${(e) => { e.preventDefault(); const f = dragFrom.current; setOver(-1); dragFrom.current = -1; if (f >= 0 && f !== i) act('move', { from: f, to: i }); }}
        onDragEnd=${() => { setOver(-1); dragFrom.current = -1; }}
        onClick=${() => i !== ps.index && act('jump', { index: i })}>
        <span class="dragi" title="Drag to reorder"><${Icon} name="grip" size=${14} /></span>
        <${Cover} src=${t.cover} cls="s" />
        <div class="grow"><div class="row-title">${t.title}</div><div class="sub">${t.artist}</div></div>
        ${i === ps.index ? html`<span class="badge">${ps.playing ? 'Playing' : 'Paused'}</span>`
          : html`<div class="acts"><${IconBtn} name="close" title="Remove from queue" cls="sm danger" size=${14} onClick=${(e) => { e.stopPropagation(); act('remove', { index: i }); }} /></div>`}
      </div>`;
    })}</div>
    ${hidden > 0 ? html`<div class="sub" style="text-align:center;margin:10px 0">+ ${hidden} more in the queue</div>` : ''}
  </main>`;
}

function MiniPlayer({ ps, act, open }) {
  const cur = ps.current;
  if (!cur) return '';
  const dur = ps.duration || cur.duration;
  const pct = dur > 0 ? Math.min(100, (ps.position / dur) * 100) : 0;
  return html`<div class="mini" onClick=${open}>
    <div class="prog" aria-hidden="true"><div style=${{ width: pct + '%' }}></div></div>
    <${Cover} src=${cur.cover} cls="s" />
    <div class="t"><div class="title row-title">${cur.title}</div><div class="sub">${cur.artist}</div></div>
    <button class="icon" title=${ps.playing ? 'Pause' : 'Play'} onClick=${(e) => { e.stopPropagation(); act('toggle'); }}><${Icon} name=${ps.playing ? 'pause' : 'play'} /></button>
    <button class="icon" title="Next" onClick=${(e) => { e.stopPropagation(); act('next'); }}><${Icon} name="next" /></button>
  </div>`;
}

// ---------- Settings: appearance, audio, servers ----------

function AppearanceSettings({ ap, setAp }) {
  return html`<div class="card" style="margin-bottom:16px">
    <h2 style="margin-top:0">Appearance</h2>
    <div class="sub">Accent color</div>
    <div class="swatches">
      ${ACCENTS.map((c) => html`<button class=${'swatch' + (ap.accent === c ? ' on' : '')} key=${c} style=${{ '--c': c }} title=${c} aria-label=${'Accent ' + c} aria-pressed=${ap.accent === c} onClick=${() => setAp({ ...ap, accent: c })}></button>`)}
      <input type="color" value=${ap.accent} title="Pick any color" aria-label="Custom accent color" onInput=${(e) => setAp({ ...ap, accent: e.target.value })} />
    </div>
    <div class="sub" style="margin-bottom:8px">Background</div>
    <div class="tones">
      ${Object.keys(TONES).map((t) => html`<button class=${'tone' + (ap.surface === t ? ' on' : '')} key=${t} style=${{ '--c': TONES[t].bg }} aria-pressed=${ap.surface === t} onClick=${() => setAp({ ...ap, surface: t })}><span class="dot2"></span>${TONE_NAMES[t]}</button>`)}
    </div>
  </div>`;
}

const BAND_LABELS = ['31', '62', '125', '250', '500', '1k', '2k', '4k', '8k', '16k'];

function AudioSettings({ toast, setVolume, volume }) {
  const [a, setA] = useState(null);
  const [eq, setEq] = useState(null); // local copy while the sliders move
  useEffect(() => { window.audioInfo().then((r) => { setA(r); setEq(r.eq); }); }, []);
  if (!a) return html`<div class="card"><div class="sub">Loading audio…</div></div>`;
  if (a.error) return html`<div class="banner">Audio: ${a.error}</div>`;

  const apply = async (op, req) => {
    const r = await window.audioDo(op, req);
    if (r.error) { toast(r.error, 'err'); return; }
    setA(r); setEq(r.eq);
  };
  const sendEq = (next) => { setEq(next); return apply('eq', { eq: next }); };
  const setGain = (i, v) => { const gains = eq.gains.slice(); gains[i] = v; setEq({ ...eq, gains, preset: 'custom' }); };

  return html`<div class="card" style="margin-bottom:16px">
    <h2 style="margin-top:0">Audio</h2>
    <label>Output device
      <select value=${a.device} onChange=${(e) => apply('device', { name: e.target.value })}>
        ${a.devices.map((d) => html`<option value=${d.name} key=${d.name}>${d.name === 'auto' ? 'System default' : d.description}</option>`)}
      </select>
    </label>
    <div style="margin-top:12px"><${Volume} value=${volume} onChange=${setVolume} /></div>

    <div class="row between" style="margin:16px 0 8px">
      <label class="check"><input type="checkbox" checked=${eq.enabled} onChange=${(e) => sendEq({ ...eq, enabled: e.target.checked })} /> Equalizer</label>
      <select class="auto" disabled=${!eq.enabled} value=${eq.preset}
        onChange=${(e) => { const n = e.target.value; if (a.presets[n]) sendEq({ ...eq, preset: n, gains: a.presets[n] }); }}>
        ${a.presetOrder.map((n) => html`<option value=${n} key=${n}>${n[0].toUpperCase() + n.slice(1)}</option>`)}
        ${eq.preset === 'custom' ? html`<option value="custom">Custom</option>` : ''}
      </select>
    </div>
    <div class=${'eq' + (eq.enabled ? '' : ' off')}>
      ${eq.gains.map((g, i) => html`<div class="band" key=${i}>
        <span class="db">${g > 0 ? '+' : ''}${g}</span>
        <input type="range" class="vslider" min="-12" max="12" step="1" value=${g} disabled=${!eq.enabled} aria-label=${BAND_LABELS[i] + ' Hz'}
          onInput=${(e) => setGain(i, +e.target.value)} onChange=${(e) => { const gains = eq.gains.slice(); gains[i] = +e.target.value; sendEq({ ...eq, gains, preset: 'custom' }); }} />
        <span class="hz">${BAND_LABELS[i]}</span>
      </div>`)}
    </div>
    <div class="sub" style="margin-top:8px">${eq.enabled ? 'Uses a little extra CPU while on.' : 'Off: no filter runs, no extra CPU.'}
      ${eq.enabled ? html`<button class="ghost sm" onClick=${() => sendEq({ ...eq, preset: 'flat', gains: a.presets.flat })}>Reset</button>` : ''}</div>
  </div>`;
}

function ServerForm({ server, canCancel, onDone, onCancel }) {
  const [f, setF] = useState({
    id: server ? server.id : '', type: server ? server.type : 'subsonic', name: server ? server.name : '',
    url: server ? server.url : '', user: server ? server.user : '', password: '',
  });
  const [status, setStatus] = useState({ kind: '', text: '' });
  const [busy, setBusy] = useState(false);
  const failedOnce = useRef(false);
  const set = (k) => (e) => { failedOnce.current = false; setF({ ...f, [k]: e.target.value }); };

  const test = async () => {
    setStatus({ kind: 'busy', text: 'Connecting…' });
    setBusy(true);
    const err = await window.testServer(f);
    setBusy(false);
    setStatus(err ? { kind: 'err', text: err } : { kind: 'ok', text: 'Connected' });
    return err;
  };
  const submit = async (e) => {
    e.preventDefault();
    if (!failedOnce.current) {
      const err = await test();
      if (err) { // an offline server must not block saving: pressing Save again keeps it
        failedOnce.current = true;
        setStatus({ kind: 'err', text: err + ' — press Save again to keep it anyway' });
        return;
      }
    }
    failedOnce.current = false;
    const err = await window.saveServer(f);
    if (err) return setStatus({ kind: 'err', text: err });
    onDone();
  };

  return html`<main>
    <h1>${server ? 'Edit server' : 'Add server'}</h1>
    <p class="hint">${server ? '' : 'Connect jbn-player to your music server to start listening.'}</p>
    <form autocomplete="off" novalidate onSubmit=${submit}>
      <label>Type
        <select value=${f.type} onChange=${set('type')}><option value="subsonic">Subsonic API (Navidrome, Airsonic, ...)</option></select>
      </label>
      <label>Name (optional)<input placeholder="Home server" value=${f.name} onInput=${set('name')} /></label>
      <label>Server URL<input placeholder="https://music.example.com" spellcheck="false" value=${f.url} onInput=${set('url')} /></label>
      <label>Username<input spellcheck="false" value=${f.user} onInput=${set('user')} /></label>
      <label>Password<input type="password" placeholder=${server && server.hasPassword ? 'Unchanged' : ''} value=${f.password} onInput=${set('password')} /></label>
      <div class=${'status ' + status.kind} role="status" aria-live="polite">${status.text ? html`<span class="dot"></span>${status.text}` : ''}</div>
      <div class="row">
        <button type="button" disabled=${busy} onClick=${test}>Test connection</button>
        <span class="grow"></span>
        ${canCancel ? html`<button type="button" class="ghost" onClick=${onCancel}>Cancel</button>` : ''}
        <button type="submit" class="primary" disabled=${busy}>Save</button>
      </div>
    </form>
  </main>`;
}

function OfflineSettings({ toast }) {
  const off = useOff();
  const [armed, setArmed] = useState(false);
  const s = off.status;
  const [dir, setDir] = useState('');
  const setFolder = async (value) => {
    const err = await offDo('setdir', { name: value });
    if (err) return toast(err, 'err');
    setDir('');
    toast(value ? 'Offline folder changed' : 'Using the default folder');
  };
  const clear = async () => {
    if (!armed) { setArmed(true); setTimeout(() => setArmed(false), 3000); return; } // two-step delete
    setArmed(false);
    const err = await offDo('clear', {});
    if (err) toast(err, 'err'); else toast('Offline copies removed');
  };
  return html`<div class="card" style="margin-bottom:16px">
    <div class="row between"><h2 style="margin:0">Offline</h2>
      ${s.count ? (armed
        ? html`<button class="ghost danger sm" onClick=${clear}>Sure?</button>`
        : html`<${IconBtn} name="trash" title="Remove all offline copies" cls="danger" onClick=${clear} />`) : ''}</div>
    <div class="sub">${s.count} songs stored · ${MB(s.bytes)}</div>
    ${s.pending ? html`<div class="row" style="margin-top:6px"><div class="sub grow">Downloading ${s.pending} left${s.current ? ' · ' + s.current : ''}</div>
      <button class="sm" onClick=${() => offDo('cancel', {})}>Stop</button></div>` : ''}
    ${s.failed ? html`<div class="banner" style="margin-top:8px">${s.failed} failed: ${s.lastError}</div>` : ''}
    <div class="sub" style="margin-top:10px">Folder${s.customDir ? '' : ' (default)'}: <span style="word-break:break-all">${s.dir || '—'}</span></div>
    <div class="row" style="margin-top:6px">
      <input class="grow" type="text" placeholder="D:\\Music\\offline" value=${dir} onInput=${(e) => setDir(e.target.value)} aria-label="Offline folder" />
      <button class="sm" disabled=${!dir.trim()} onClick=${() => setFolder(dir.trim())}>Use</button>
      ${s.customDir ? html`<button class="ghost sm" onClick=${() => setFolder('')}>Default</button>` : ''}
    </div>
    <div class="sub" style="margin-top:6px">Songs already downloaded stay in the old folder. Downloaded songs play without the server; removing them never touches the server.</div>
  </div>`;
}

function Settings({ onChanged, toast, setVolume, volume, ap, setAp }) {
  const [list, setList] = useState(null);
  const [form, setForm] = useState(null); // null = list, {} = new, server = edit
  const [armed, setArmed] = useState('');
  const load = async () => {
    const l = await window.getServers();
    setList(l);
    if (l.length === 0) setForm({}); // first run goes straight to the setup form
  };
  useEffect(() => { load(); }, []);
  if (list === null) return html`<main></main>`;

  const changed = async () => { setForm(null); await load(); onChanged(); };
  if (form) {
    return html`<${ServerForm} server=${form.id ? form : null} canCancel=${list.length > 0} onDone=${changed} onCancel=${() => setForm(null)} />`;
  }
  const del = async (s) => {
    if (armed !== s.id) { setArmed(s.id); setTimeout(() => setArmed(''), 3000); return; } // two-step delete
    await window.deleteServer(s.id); setArmed(''); changed();
  };
  return html`<main>
    <${AppearanceSettings} ap=${ap} setAp=${setAp} />
    <${AudioSettings} toast=${toast} setVolume=${setVolume} volume=${volume} />
    <${OfflineSettings} toast=${toast} />
    <${SongkeySettings} toast=${toast} />
    <div class="row between" style="margin-bottom:12px"><h2 style="margin:0">Servers</h2><button class="primary" onClick=${() => setForm({})}>+ Add server</button></div>
    <ul class="servers">${list.map((s, i) => html`
      <li class=${'server' + (s.active ? ' active' : '')} key=${s.id} style=${{ animationDelay: i * 40 + 'ms' }}>
        <div class="row"><span class="name">${s.name}</span>${s.active ? html`<span class="badge">In use</span>` : ''}</div>
        <div class="sub">${s.user} @ ${s.url}</div>
        <div class="actions">
          ${!s.active ? html`<${IconBtn} name="check" title="Use this server" onClick=${async () => { await window.setActive(s.id); changed(); }} />` : ''}
          <${IconBtn} name="edit" title="Edit" onClick=${() => setForm(s)} />
          ${armed === s.id
            ? html`<button class="ghost danger sm" onClick=${() => del(s)}>Sure?</button>`
            : html`<${IconBtn} name="trash" title="Delete" cls="danger" onClick=${() => del(s)} />`}
        </div>
      </li>`)}</ul>
  </main>`;
}

// ---------- App shell ----------

const EMPTY_PS = { queueLen: 0, version: 0, index: 0, current: null, playing: false, shuffle: false, position: 0, duration: 0, volume: 100, cover: '', error: '' };
let initialAppearance = { accent: '#2d7be0', surface: 'graphite' };

function App() {
  const [tab, setTab] = useState('albums');
  const [ps, setPs] = useState(EMPTY_PS);
  const [toastMsg, setToastMsg] = useState(null);
  const [libKey, setLibKey] = useState(0); // bump after server changes
  const [info, setInfo] = useState(null);
  const [syncing, setSyncing] = useState(false);
  const [ap, setApState] = useState(initialAppearance);
  const toastTimer = useRef(0);
  const volTouched = useRef(0); // while the slider is being used, polled volumes must not move it back
  const volTimer = useRef(0);
  const apTimer = useRef(0);

  const toast = (text, kind = 'ok') => {
    setToastMsg({ text, kind });
    clearTimeout(toastTimer.current);
    toastTimer.current = setTimeout(() => setToastMsg(null), 2200);
  };

  // Colors change at once; they are saved a moment after the last change.
  const setAp = (next) => {
    setApState(next);
    applyAppearance(next);
    clearTimeout(apTimer.current);
    apTimer.current = setTimeout(() => window.setAppearance({ accent: next.accent, surface: next.surface }), 400);
  };

  // Poll the light player state while the window is open (nothing runs once it is closed).
  useEffect(() => {
    let alive = true;
    const tick = async () => {
      if (document.visibilityState === 'hidden') return;
      const r = await window.playerState();
      if (alive) setPs((prev) => (Date.now() - volTouched.current < 1500 ? { ...r, volume: prev.volume } : r));
    };
    tick();
    const id = setInterval(tick, 1000);
    return () => { alive = false; clearInterval(id); };
  }, []);

  const act = async (op, req = {}) => {
    const r = await window.playerDo(op, req);
    if (r.error) { toast(r.error, 'err'); return false; }
    setPs(r);
    return true;
  };

  // Plays the whole library: this replaces THE queue, it never makes another one.
  const playAll = async (shuffle) => {
    const r = await window.playAll(shuffle);
    if (r.error) return toast(r.error, 'err');
    setPs(r);
    toast(shuffle ? 'Shuffling all songs' : 'Playing all songs');
  };

  // The slider moves at once; the player is told at most every 100 ms, and at the end.
  const setVolume = (level, final) => {
    volTouched.current = Date.now();
    setPs((p) => ({ ...p, volume: level }));
    clearTimeout(volTimer.current);
    volTimer.current = setTimeout(() => window.audioDo('volume', { level }), final ? 0 : 100);
  };

  useEffect(() => { window.syncInfo().then(setInfo); offRefresh(); }, [libKey]);
  // songkey: follow the jobs while the window is open; say when a song reached the playlist.
  useEffect(() => {
    SK.onFiled = (names) => toast(names.length === 1 ? 'Added to the playlist: ' + names[0] : names.length + ' songs added to the playlist');
    skLoad();
    return () => { SK.onFiled = null; clearTimeout(SK.timer); };
  }, [libKey]);
  const runSync = async () => {
    if (syncing) return;
    setSyncing(true);
    const r = await window.syncLibrary();
    setSyncing(false);
    if (r.error) return toast(r.error, 'err');
    setInfo(r);
    toast('Synced ' + r.songs + ' songs, ' + r.playlists + ' playlists');
  };
  const sync = { info, syncing, run: runSync };

  // First run: no server yet means the setup form comes first.
  useEffect(() => { window.getServers().then((l) => { if (l.length === 0) setTab('settings'); }); }, []);

  const tabs = [['albums', 'Albums', 'albums'], ['songs', 'Songs', 'songs'], ['playlists', 'Playlists', 'playlists'], ['now', 'Playing', 'playing'], ['settings', 'Settings', 'settings']];
  return html`
    <div class="grip" title="Drag to move" onMouseDown=${() => window.startDrag()}></div>
    ${tab === 'albums' ? html`<${Albums} key=${libKey} act=${act} toast=${toast} goSettings=${() => setTab('settings')} />` : ''}
    ${tab === 'songs' ? html`<${Songs} key=${libKey} act=${act} toast=${toast} sync=${sync} playAll=${playAll} goSettings=${() => setTab('settings')} />` : ''}
    ${tab === 'playlists' ? html`<${Playlists} key=${libKey} act=${act} toast=${toast} sync=${sync} />` : ''}
    ${tab === 'now' ? html`<${NowPlaying} ps=${ps} act=${act} goLibrary=${() => setTab('albums')} setVolume=${setVolume} />` : ''}
    ${tab === 'settings' ? html`<${Settings} onChanged=${() => setLibKey(libKey + 1)} toast=${toast} setVolume=${setVolume} volume=${ps.volume} ap=${ap} setAp=${setAp} />` : ''}
    ${ps.error && tab !== 'settings' ? html`<div class="banner" style="margin:0 16px 8px">${ps.error}</div>` : ''}
    ${tab !== 'now' ? html`<${MiniPlayer} ps=${ps} act=${act} open=${() => setTab('now')} />` : ''}
    ${toastMsg ? html`<div class=${'toast ' + toastMsg.kind}>${toastMsg.text}</div>` : ''}
    <nav>${tabs.map(([k, label, icon]) => html`<button class=${tab === k ? 'active' : ''} onClick=${() => setTab(k)}><${Icon} name=${icon} size=${20} /><span>${label}</span></button>`)}</nav>`;
}

// The window has no title bar: Esc closes it (clicking elsewhere does too).
window.addEventListener('keydown', (e) => { if (e.key === 'Escape') window.closeIsland(); });

// Apply the saved colors before the first paint so the window never flashes the default look.
window.getAppearance()
  .then((a) => { if (a && !a.error) { initialAppearance = { accent: a.accent, surface: a.surface }; applyAppearance(initialAppearance); } })
  .catch(() => {})
  .finally(() => render(html`<${App} />`, document.getElementById('app')));
