// Batches tab (2026-11-17): what is on the rack and what has happened to it.
// One row per batch, a dated log under each, and the two CSV sheets of the
// shared batch format (docs/BATCH-FORMAT.md). Growers keep the records;
// anyone signed in can export and print.
//
// `store` is the live API: { list, get, create, patch, log, exportUrls,
// shelves:[{key,name}] }. Without it (design preview) the tab says so.
import React from 'react';
import { PG } from './data.jsx';

const METHODS = [['', '—'], ['direct_sow', 'Direct sow'], ['flood_tray', 'Flood tray'], ['cold_moist_strat', 'Cold moist stratification'],
  ['warm_strat', 'Warm stratification'], ['scarification', 'Scarification'], ['direct_stick', 'Direct stick'],
  ['perlite_coir', 'Perlite and coir'], ['water_rooting', 'Water rooting'], ['other', 'Other']];
const STATUSES = [['active', 'Active'], ['rooted', 'Rooted'], ['hardening', 'Hardening'], ['planted_out', 'Planted out'], ['failed', 'Failed'], ['archived', 'Archived']];
const EVENTS = [['counted', 'Counted'], ['sown', 'Sown'], ['germinated', 'Germinated'], ['potted_up', 'Potted up'],
  ['moved', 'Moved'], ['lost', 'Losses'], ['planted_out', 'Planted out'], ['note', 'Note']];
const EVENT_WORDS = Object.assign({ started: 'Started' }, Object.fromEntries(EVENTS));
const label = (pairs, v) => (pairs.find((p) => p[0] === v) || [v, v || '—'])[1];
const today = () => { const d = new Date(); return d.getFullYear() + '-' + String(d.getMonth() + 1).padStart(2, '0') + '-' + String(d.getDate()).padStart(2, '0'); };
const daysSince = (iso) => { const d = new Date(iso + 'T00:00:00'); return isNaN(d) ? null : Math.max(0, Math.floor((Date.now() - d.getTime()) / 86400e3)); };
const numOrNull = (v) => (v === '' || v == null ? null : Math.max(0, parseInt(v, 10) || 0));

function Field({ k, children, wide }){
  return <label className={'bt-field' + (wide ? ' wide' : '')}><span className="fx-k">{k}</span>{children}</label>;
}

function BatchForm({ initial, shelves, onSave, onCancel, busy, error }){
  const [v, setV] = React.useState(() => ({
    batch_code: '', species: '', species_code: '', source: '', started_on: today(), method: '', container: '', medium: '',
    count_started: '', zone_key: '', notes: '', status: 'active', ...(initial || {}),
    ...(initial ? { count_started: initial.count_started == null ? '' : initial.count_started } : {}),
  }));
  const set = (k) => (e) => setV((p) => ({ ...p, [k]: e.target.value }));
  const ok = v.batch_code.trim() && v.species.trim() && /^\d{4}-\d{2}-\d{2}$/.test(v.started_on);
  const submit = () => onSave({ ...v, count_started: numOrNull(v.count_started) });
  return (
    <div className="fx-back" onClick={onCancel}><div className="fx-modal" style={{ maxWidth: '640px' }} onClick={(e) => e.stopPropagation()}>
      <div className="fx-head"><span className="ttl">{initial ? 'Edit batch' : 'New batch'}</span><span className="x" onClick={onCancel}>&times;</span></div>
      <div className="fx-body bt-form">
        <Field k="Batch code"><input value={v.batch_code} onChange={set('batch_code')} placeholder="M-07" autoFocus={!initial}/></Field>
        <Field k="Date started"><input type="date" value={v.started_on} onChange={set('started_on')}/></Field>
        <Field k="Species" wide>
          <input value={v.species} onChange={set('species')} list="bt-species" placeholder="Botanical name"/>
          <datalist id="bt-species">{(PG.SPECIES || []).map((s) => <option key={s.tag} value={s.tag.charAt(0) + s.tag.slice(1).toLowerCase()}>{s.species}</option>)}</datalist>
        </Field>
        <Field k="Seed or cutting source" wide><input value={v.source} onChange={set('source')} placeholder="A name or a section, not coordinates"/></Field>
        <Field k="Method"><select value={v.method} onChange={set('method')}>{METHODS.map(([k, t]) => <option key={k} value={k}>{t}</option>)}</select></Field>
        <Field k="Count started"><input type="number" min="0" inputMode="numeric" value={v.count_started} onChange={set('count_started')}/></Field>
        <Field k="Container"><input value={v.container} onChange={set('container')} placeholder="2.5 in pot"/></Field>
        <Field k="Medium"><input value={v.medium} onChange={set('medium')} placeholder="Coir and perlite"/></Field>
        <Field k="On shelf"><select value={v.zone_key} onChange={set('zone_key')}><option value="">Not on the rack</option>{shelves.map((s) => <option key={s.key} value={s.key}>{s.name}</option>)}</select></Field>
        {initial && <Field k="Status"><select value={v.status} onChange={set('status')}>{STATUSES.map(([k, t]) => <option key={k} value={k}>{t}</option>)}</select></Field>}
        <Field k="Notes" wide><input value={v.notes} onChange={set('notes')}/></Field>
      </div>
      <div className="fx-foot"><span className="note">{error || (initial ? 'Moving it to another shelf is written to the log.' : 'The first log line is written for you.')}</span>
        <button className="fx-btn" onClick={onCancel}>Cancel</button>
        <button className="fx-btn primary" disabled={!ok || busy} onClick={submit}>{busy ? 'Saving…' : 'Save'}</button></div>
    </div></div>
  );
}

function LogForm({ batch, shelves, onAdd, busy, error }){
  const [v, setV] = React.useState({ date: today(), event: 'counted', count: '', note: '', zone_key: batch.zone_key || '' });
  const set = (k) => (e) => setV((p) => ({ ...p, [k]: e.target.value }));
  const ok = /^\d{4}-\d{2}-\d{2}$/.test(v.date) && (v.event !== 'note' || v.note.trim()) && (v.event !== 'counted' || v.count !== '');
  const add = () => {
    const body = { date: v.date, event: v.event, count: numOrNull(v.count), note: v.note.trim() };
    if (v.event === 'moved') body.zone_key = v.zone_key;
    onAdd(body).then((done) => { if (done) setV((p) => ({ ...p, count: '', note: '' })); });
  };
  return (
    <div className="bt-logform">
      <Field k="Date"><input type="date" value={v.date} onChange={set('date')}/></Field>
      <Field k="What happened"><select value={v.event} onChange={set('event')}>{EVENTS.map(([k, t]) => <option key={k} value={k}>{t}</option>)}</select></Field>
      {v.event === 'moved'
        ? <Field k="Moved to"><select value={v.zone_key} onChange={set('zone_key')}><option value="">Off the rack</option>{shelves.map((s) => <option key={s.key} value={s.key}>{s.name}</option>)}</select></Field>
        : <Field k="Alive after"><input type="number" min="0" inputMode="numeric" value={v.count} onChange={set('count')} placeholder="count"/></Field>}
      <Field k="Note" wide><input value={v.note} onChange={set('note')}/></Field>
      <button className="fx-btn primary" disabled={!ok || busy} onClick={add}>Add line</button>
      {error && <div className="bt-err">{error}</div>}
    </div>
  );
}

// The printable bench sheet: one block per batch, with ruled blank lines so it
// can be carried to the bench and written on.
function BenchSheet({ batches, shelfName }){
  return (
    <div className="bt-print">
      <h1>Propagation Unit No. 01 · Bench sheet</h1>
      <p className="when">Short’s Field Station · printed {new Date().toLocaleDateString(undefined, { year: 'numeric', month: 'long', day: 'numeric' })}</p>
      {batches.map((b) => (
        <section key={b.id}>
          <h2>{b.batch_code} · <i>{b.species}</i></h2>
          <table><tbody>
            <tr><th>Shelf</th><td>{shelfName(b.zone_key) || 'Not on the rack'}</td><th>Started</th><td>{b.started_on}</td></tr>
            <tr><th>Source</th><td>{b.source || '—'}</td><th>Method</th><td>{label(METHODS, b.method)}</td></tr>
            <tr><th>Container</th><td>{b.container || '—'}</td><th>Medium</th><td>{b.medium || '—'}</td></tr>
            <tr><th>Count</th><td>{b.count_current == null ? '—' : b.count_current} of {b.count_started == null ? '—' : b.count_started}</td><th>Status</th><td>{label(STATUSES, b.status)}</td></tr>
          </tbody></table>
          <table className="lines"><thead><tr><th>Date</th><th>What happened</th><th>Alive</th><th>Note</th></tr></thead>
            <tbody>{[0, 1, 2, 3, 4].map((i) => <tr key={i}><td>&nbsp;</td><td></td><td></td><td></td></tr>)}</tbody></table>
        </section>
      ))}
    </div>
  );
}

function BatchesTab({ store, role }){
  const [list, setList] = React.useState(null);
  const [err, setErr] = React.useState('');
  const [open, setOpen] = React.useState(null);       // batch with its log
  const [form, setForm] = React.useState(null);        // null | {} (new) | batch (edit)
  const [busy, setBusy] = React.useState(false);
  const [formErr, setFormErr] = React.useState('');
  const [logErr, setLogErr] = React.useState('');
  const [showDone, setShowDone] = React.useState(false);
  const canWrite = role === 'Grower' || role === 'Steward';
  const shelves = (store && store.shelves) || [];
  const shelfName = (key) => (shelves.find((s) => s.key === key) || {}).name || '';

  const reload = React.useCallback(() => {
    if (!store) return Promise.resolve();
    return store.list().then((r) => { setList(r.batches || []); setErr(''); })
      .catch((e) => setErr((e && e.message) || 'Could not read the batches.'));
  }, [store]);
  React.useEffect(() => { reload(); }, [reload]);

  if (!store) return <div className="db-h"><div className="ttl">Batches</div><div className="sub">Available on a running unit</div></div>;

  const openBatch = (id) => store.get(id).then(setOpen).catch((e) => setErr(e.message));
  const save = (body) => {
    setBusy(true); setFormErr('');
    const editing = form && form.id;
    (editing ? store.patch(form.id, body) : store.create(body))
      .then((b) => { setForm(null); setOpen(b); return reload(); })
      .catch((e) => setFormErr((e && e.message) || 'Could not save.'))
      .finally(() => setBusy(false));
  };
  const addLine = (body) => {
    setBusy(true); setLogErr('');
    return store.log(open.id, body)
      .then((b) => { setOpen(b); reload(); return true; })
      .catch((e) => { setLogErr((e && e.message) || 'Could not add the line.'); return false; })
      .finally(() => setBusy(false));
  };

  const done = (b) => ['planted_out', 'failed', 'archived'].includes(b.status);
  const rows = (list || []).filter((b) => showDone || !done(b));
  const hidden = (list || []).length - rows.length;

  return (
    <div className="bt">
      <div className="db-h">
        <div className="ttl">Batches</div>
        <div className="sub">What is on the rack · where it came from · what has happened to it</div>
      </div>
      {!role && <div className="ro-note">Read-only. Sign in to export or print; sign in as a Grower to keep the records.</div>}
      <div className="bt-bar">
        {canWrite && <button className="fx-btn primary" onClick={() => { setFormErr(''); setForm({}); }}>New batch</button>}
        {role && <a className="fx-btn" href={store.exportUrls.batches} download>Batches CSV</a>}
        {role && <a className="fx-btn" href={store.exportUrls.log} download>Log CSV</a>}
        {role && <button className="fx-btn" disabled={!rows.length} onClick={() => window.print()}>Print bench sheet</button>}
      </div>
      {err && <div className="ro-note">{err}</div>}
      <div className="panel">
        <div className="ph"><div className="pt">{showDone ? 'All batches' : 'Growing now'}</div><div className="pm">{list == null ? 'Reading…' : rows.length + (rows.length === 1 ? ' batch' : ' batches')}</div></div>
        <div className="pb">
          {list != null && rows.length === 0 && <div className="ctrl-note">{canWrite ? 'No batches yet. Start one with New batch.' : 'No batches recorded yet.'}</div>}
          {rows.map((b) => {
            const d = daysSince(b.started_on);
            return (
              <div key={b.id} className={'ctrl-row bt-row' + (open && open.id === b.id ? ' on' : '')} onClick={() => (open && open.id === b.id ? setOpen(null) : openBatch(b.id))}>
                <div className="cl"><b>{b.batch_code}</b> · <i>{b.species}</i>
                  <small>{[shelfName(b.zone_key) || 'Not on the rack', label(STATUSES, b.status), d == null ? '' : 'day ' + d, b.source].filter(Boolean).join(' · ')}</small></div>
                <div className="cv">{b.count_current == null ? '—' : b.count_current}<span>{b.count_started == null ? '' : ' / ' + b.count_started}</span></div>
              </div>
            );
          })}
          {hidden > 0 && <div className="ctrl-note bt-more" onClick={() => setShowDone(true)}>Show {hidden} finished {hidden === 1 ? 'batch' : 'batches'}</div>}
          {showDone && <div className="ctrl-note bt-more" onClick={() => setShowDone(false)}>Hide finished batches</div>}
        </div>
      </div>

      {open && (
        <div className="panel bt-detail">
          <div className="ph"><div className="pt">{open.batch_code} · log</div>
            <div className="pm">{canWrite && <span className="bt-link" onClick={() => { setFormErr(''); setForm(open); }}>Edit batch</span>}</div></div>
          <div className="pb">
            <div className="bt-facts">
              {[['Source', open.source], ['Method', open.method && label(METHODS, open.method)], ['Container', open.container], ['Medium', open.medium], ['Notes', open.notes]]
                .filter((f) => f[1]).map(([k, val]) => <span key={k}><b>{k}</b> {val}</span>)}
            </div>
            {(open.log || []).map((e) => (
              <div key={e.id} className="bt-line">
                <span className="d">{e.date}</span>
                <span className="e">{EVENT_WORDS[e.event] || e.event}{e.event === 'moved' || e.event === 'started' ? ' · ' + (shelfName(e.zone_key) || 'off the rack') : ''}</span>
                <span className="c">{e.count == null ? '' : e.count + ' alive'}</span>
                <span className="n">{e.note}</span>
              </div>
            ))}
            {canWrite && <LogForm key={open.id} batch={open} shelves={shelves} onAdd={addLine} busy={busy} error={logErr}/>}
          </div>
        </div>
      )}

      {form && <BatchForm initial={form.id ? form : null} shelves={shelves} onSave={save} onCancel={() => setForm(null)} busy={busy} error={formErr}/>}
      <BenchSheet batches={rows} shelfName={shelfName}/>
    </div>
  );
}

export { BatchesTab };
