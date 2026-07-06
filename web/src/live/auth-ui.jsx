// Live auth surfaces — sign-in, first-boot setup, and operator management —
// in the handoff's paper modal language (fx-* classes from pgui.css).
import React from 'react';
import { api } from './api.js';

const inputStyle = {
  width: '100%', boxSizing: 'border-box',
  border: '1.5px solid var(--ink)', borderRadius: '4px',
  background: 'var(--bone)', color: 'var(--ink)',
  fontFamily: '"JetBrains Mono", monospace', fontSize: '14px',
  padding: '11px 12px', outline: 'none',
};

function Field({ label, type = 'text', value, onChange, autoFocus, placeholder }) {
  return (
    <div className="fx-field">
      <div className="fx-k">{label}</div>
      <input style={inputStyle} type={type} value={value} autoFocus={autoFocus}
        placeholder={placeholder || ''} onChange={(e) => onChange(e.target.value)} />
    </div>
  );
}

const ROLE_NAMES = { admin: 'Steward', operator: 'Grower', viewer: 'Viewer' };
export const toDesignSession = (u) =>
  u ? { name: u.username, role: ROLE_NAMES[u.role] || 'Viewer', backendRole: u.role } : null;

/* ---- Sign in (username + password against the real backend) ---- */
export function LiveSignInModal({ onSignIn, onClose }) {
  const [username, setUsername] = React.useState('');
  const [password, setPassword] = React.useState('');
  const [err, setErr] = React.useState('');
  const [busy, setBusy] = React.useState(false);
  const submit = () => {
    if (busy) return;
    setBusy(true); setErr('');
    api.login(username, password)
      .then((r) => onSignIn(toDesignSession(r.user)))
      .catch((e) => { setErr(e.message); setBusy(false); });
  };
  return (
    <div className="fx-back" onClick={onClose}>
      <div className="fx-modal" style={{ maxWidth: '400px' }} onClick={(e) => e.stopPropagation()}>
        <div className="fx-head"><span className="ttl">Sign In</span><span className="x" onClick={onClose}>&times;</span></div>
        <div className="fx-body">
          <Field label="Operator" value={username} onChange={setUsername} autoFocus />
          <div onKeyDown={(e) => { if (e.key === 'Enter') submit(); }}>
            <Field label="Password" type="password" value={password} onChange={setPassword} />
          </div>
          {err && <div className="fx-warn">{err}</div>}
        </div>
        <div className="fx-foot">
          <span className="note">Signed actions are logged.</span>
          <button className="fx-btn" onClick={onClose}>Cancel</button>
          <button className="fx-btn primary" onClick={submit} disabled={busy || !username || !password}>
            {busy ? 'Signing in…' : 'Sign In'}
          </button>
        </div>
      </div>
    </div>
  );
}

/* ---- First boot: create the steward account ---- */
export function SetupModal({ onDone }) {
  const [username, setUsername] = React.useState('');
  const [password, setPassword] = React.useState('');
  const [confirm, setConfirm] = React.useState('');
  const [err, setErr] = React.useState('');
  const [busy, setBusy] = React.useState(false);
  const valid = username.trim().length >= 2 && password.length >= 8 && password === confirm;
  const submit = () => {
    if (!valid || busy) return;
    setBusy(true); setErr('');
    api.setup(username.trim(), password)
      .then((r) => onDone(toDesignSession(r.user)))
      .catch((e) => { setErr(e.message); setBusy(false); });
  };
  return (
    <div className="fx-back">
      <div className="fx-modal" style={{ maxWidth: '440px' }}>
        <div className="fx-head"><span className="ttl">Unit Setup</span><span className="tag">First Boot</span></div>
        <div className="fx-body">
          <div style={{ fontFamily: 'Newsreader', fontSize: '15px', lineHeight: 1.5, marginBottom: '16px' }}>
            Welcome to Propagation Unit 01. Create the steward account — it holds
            full control of the unit and manages all other operators.
          </div>
          <Field label="Steward name" value={username} onChange={setUsername} autoFocus placeholder="A. Short" />
          <Field label="Password · 8+ characters" type="password" value={password} onChange={setPassword} />
          <div onKeyDown={(e) => { if (e.key === 'Enter') submit(); }}>
            <Field label="Confirm password" type="password" value={confirm} onChange={setConfirm} />
          </div>
          {password && confirm && password !== confirm && <div className="fx-warn">Passwords do not match</div>}
          {err && <div className="fx-warn">{err}</div>}
        </div>
        <div className="fx-foot">
          <span className="note">The rack touchscreen needs no sign-in.</span>
          <button className="fx-btn primary" onClick={submit} disabled={!valid || busy}>
            {busy ? 'Creating…' : 'Create & Sign In'}
          </button>
        </div>
      </div>
    </div>
  );
}

/* ---- Operator management (admin) ---- */
export function OperatorsModal({ onClose, selfName }) {
  const [users, setUsers] = React.useState(null);
  const [err, setErr] = React.useState('');
  const [nu, setNu] = React.useState({ username: '', password: '', role: 'operator' });
  const load = React.useCallback(() => {
    api.listUsers().then((r) => setUsers(r.users)).catch((e) => setErr(e.message));
  }, []);
  React.useEffect(load, [load]);

  const act = (fn) => fn.then(load).catch((e) => setErr(e.message));
  const add = () => {
    setErr('');
    act(api.createUser(nu.username.trim(), nu.password, nu.role)
      .then(() => setNu({ username: '', password: '', role: 'operator' })));
  };

  return (
    <div className="fx-back" onClick={onClose}>
      <div className="fx-modal" style={{ maxWidth: '620px' }} onClick={(e) => e.stopPropagation()}>
        <div className="fx-head">
          <span className="ttl">Operators</span>
          <span className="tag">{users ? users.length + ' accounts' : '…'}</span>
          <span className="x" onClick={onClose}>&times;</span>
        </div>
        <div className="fx-body">
          {users && users.map((u) => (
            <div key={u.username} style={{
              display: 'flex', alignItems: 'center', gap: 12,
              padding: '10px 2px', borderBottom: '1px solid var(--rule-soft)',
              opacity: u.enabled ? 1 : 0.5,
            }}>
              <div style={{ flex: 1 }}>
                <div style={{ fontFamily: 'Newsreader', fontSize: 16 }}>{u.username}
                  {u.username === selfName && <span className="fx-k" style={{ display: 'inline', marginLeft: 8 }}>· you</span>}
                </div>
                <div className="fx-k" style={{ margin: '2px 0 0' }}>{ROLE_NAMES[u.role]}{u.enabled ? '' : ' · disabled'}</div>
              </div>
              <select className="fx-select" style={{ width: 'auto', fontSize: 12 }} value={u.role}
                onChange={(e) => act(api.updateUser(u.username, { role: e.target.value }))}>
                <option value="admin">Steward</option>
                <option value="operator">Grower</option>
                <option value="viewer">Viewer</option>
              </select>
              <button className="fx-btn" onClick={() => act(api.updateUser(u.username, { enabled: !u.enabled }))}>
                {u.enabled ? 'Disable' : 'Enable'}
              </button>
              <button className="fx-btn" onClick={() => {
                const pw = window.prompt(`New password for ${u.username} (8+ chars):`);
                if (pw) act(api.updateUser(u.username, { password: pw }));
              }}>Reset PW</button>
            </div>
          ))}

          <div className="fx-k" style={{ margin: '20px 0 8px' }}>Add operator</div>
          <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr auto auto', gap: 10, alignItems: 'end' }}>
            <input style={inputStyle} placeholder="Name" value={nu.username}
              onChange={(e) => setNu({ ...nu, username: e.target.value })} />
            <input style={inputStyle} type="password" placeholder="Password (8+)" value={nu.password}
              onChange={(e) => setNu({ ...nu, password: e.target.value })} />
            <select className="fx-select" style={{ width: 'auto' }} value={nu.role}
              onChange={(e) => setNu({ ...nu, role: e.target.value })}>
              <option value="admin">Steward</option>
              <option value="operator">Grower</option>
              <option value="viewer">Viewer</option>
            </select>
            <button className="fx-btn primary" onClick={add}
              disabled={nu.username.trim().length < 2 || nu.password.length < 8}>Add</button>
          </div>
          {err && <div className="fx-warn" style={{ marginTop: 10 }}>{err}</div>}
        </div>
        <div className="fx-foot">
          <span className="note">Stewards configure · Growers operate · Viewers observe. The public sees the read-only view without an account.</span>
          <button className="fx-btn" onClick={onClose}>Close</button>
        </div>
      </div>
    </div>
  );
}
