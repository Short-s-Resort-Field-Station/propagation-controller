# Instructions for claude-design — seamless round-trip

**Read this before designing.** Last pass rebuilt the UI from screenshots and
(unintentionally) **dropped built features and changed the file format**, which
made integration slow. This time you're editing the **real source** in place.
Follow these rules and your output drops straight into the live app.

---

## 1. Prime directive

You are **refining the real production components in place** — not building a new
prototype. The files to edit are in `source-current/`:

| File | Surface |
|---|---|
| `pgui-data.jsx` | shared data, status vocabulary, gauges/charts |
| `pgui-dashboard.jsx` | web Control Center (4 tabs) |
| `pgui-flows.jsx` | modals & flows (Assign/Edit, Sign-In, Alerts, History, Confirm, banners) |
| `pgui-rack2.jsx` | touch instrument face (plates, ledger, shelf detail) |
| `pgui.css` | the single stylesheet — **all styling lives here** |
| `REFERENCE-adapter.js.txt` | **read-only.** Shows what live fields each component receives. Do **not** edit or design around changing it. |

**Return these same files, edited.** Do not reconstruct from the screenshots; the
screenshots are stale (they predate several features). The code is the truth.

## 2. Keep it a Vite ES-module app (this is what broke last time)

- **Keep every `import` / `export` statement.** Do **NOT** convert to CDN
  `<script type="text/babel">` + `Object.assign(window, …)` globals.
- **CSS stays in `pgui.css`** (external). Do **NOT** embed a `<style>` block in an
  HTML file. There is no `index.html` to ship — the app is built by Vite.
- No new CDNs, no new npm deps, no new fonts (Zilla Slab / Newsreader /
  JetBrains Mono are already self-hosted).

## 3. Do not break the public API (the app passes these in)

Keep exported component **names and props exactly**. Selected contracts:

- `Dashboard({ shelves, reservoir, rackStatus, log, session, alerts, connection,
  onAssign, onSetEnabled, onUnassign, onSetMaintenance, maintenance, profiles,
  getSeries, controls, auth, … })`
- `AssignModal({ shelfN, initial, onAssign, onClose, onClear, onDisable, profiles })`
  — `profiles = { list(speciesId), save(body), update(id, body) }`.
- `RackDisplay2({ shelves, reservoir, rackStatus, layout, clock, day, theme,
  maintenance, onWater, onLight, onFan, onSetMaintenance, onSetEnabled,
  onThemeToggle, initialOpen, connection, locked, onUnlock })`
- Also exported & used: `Stepper, NumField, RangeField, PinPad, PhotoRail,
  SignInModal, LockScreen, OfflineBanner, MaintenanceBanner, AlertBar,
  AlertsModal, HistoryModal, Confirm`.

Renaming or dropping any prop/export **silently breaks the live app.** If you must
add one, list it in your changelog.

## 4. Do not drop these shelf-object fields (they come from live sensors)

Each `shelf` object the components render includes (see `REFERENCE-adapter.js.txt`):
`n, zoneKey, active, assigned, species, tag, status, moisture, temp, ph, lo, hi,
phBand, tempBand, profileName, speciesId, stage, light, on, off, fan, lastWater,
lastWaterDate, usage, stripe, raw{ species1, species2, durMin, drainMin, phLo,
phHi, tempLo, tempHi, coolMin, maxCycles, profileId, stage }`.

Render them; never invent or rename them.

## 5. Preserve ALL features & states — do not remove any (this is what got lost)

- **Ebb-and-flow:** a distinct **`Draining`** state (pump off, valve open) alongside `Watering`.
- **Maintenance mode:** `MaintenanceBanner` (both surfaces), header toggle chip,
  Confirm both directions, calmed alarms. Touch: the `Maintenance` plaque button + water-in-maintenance confirm.
- **Per-shelf disable:** distinct **`Disabled`** (assigned but paused → Enable/Clear)
  vs **`Inactive`** (vacant bay → Assign). Touch: `Disable Shelf` / `Enable Shelf` plaque + "enable to water".
- **Species profiles (in `AssignModal`):** propagation-**stage** dropdown
  (Seeds/Seedlings/Cuttings/Trimmings/Suckers/Saplings + custom), **profile picker**
  (fills settings), **save-as-profile** (name → Save new / Update existing). Keep this — it works well.
- **Monitoring happy-bands:** pH & temperature ranges in the modal and drawn on the touch dials.
- **Environmental "Target Adherence" panel** (per-shelf % time-in-band).
- **Full status vocabulary** (must exist in every color map — `PG.STATUS`,
  `PG.RACK_STATUS`, and the touch `SC2`/`RS2` maps): `Healthy, Monitoring,
  Watering, Draining, Dry, Sensor Fault, Inactive, Maintenance, Disabled`.
  **A missing key crashes the touch view** — if you add a status, add it to all maps.

## 6. Data contract & units (correctness)

- **Soil moisture = `%VWC`** (volumetric water content). Never `%RH`.
- **Temperature (°F) and pH (unitless) are MONITORED, not controlled** — show a
  target/happy **band**, never an operator-drivable setpoint. Moisture target/min
  *are* control setpoints (the pump acts on them).
- Reservoir in `%` and litres (26.5 L / 7 gal tank).
- **No audible alarms** — escalation is email/SMS only.
- Don't add features, tabs, sensors, or controls that aren't already present. Ask instead.

## 7. Touch face constraints

- Fixed logical canvas **880×528**, scaled to the physical **800×480** panel.
- Everything must fit with **no scrolling**; primary actions (e.g. **Water Now**)
  must stay above the fold. ≥44px tap targets. Light + dark themes both supported
  via the existing tokens.
- Your last dial treatment (target-band arc + reading marker + centered value)
  was excellent — keep that direction; just bind it to the real `phBand`/`tempBand`.

## 8. CSS tokens — restyle values, don't rename tokens

Keep the custom-property **names** (`--paper, --ink, --char, --sage[-deep],
--ochre[-deep], --oxblood[-deep], --river[-deep], --bone, --rule[-soft]`;
instrument: `--face, --lcd[-dim/-faint], --hair, --accent, --i-river/-ochre/
-oxblood/-sage`, plus the `rk2-*` / `--sline`/`--sink`/`--sfaint` instrument set).
Change their **values** or the rules that use them; don't rename them, or the JSX
that references them goes unstyled.

## 9. Output format

- Return the **same files, edited** (drop-in). Keep diffs surgical; **prefer
  CSS-only changes** where a look can be achieved in `pgui.css` alone (lowest risk).
- Include a short **CHANGELOG.md**: what changed per file, and a list of any **new**
  token / class / prop / status you introduced (so it can be wired cleanly).
- Do not touch backend, data-fetching, or `web/src/live/*`.

## 10. Priorities for this pass

1. **The Assign/Edit modal** — make it genuinely beautiful and intuitive **without
   dropping** the stage / profile / save-as-profile / happy-band controls.
2. **Cross-screen cohesion** — unify control styling (inputs, toggles, segmented
   controls, buttons), spacing rhythm, and section-header treatment across all web tabs.
3. **Hold the two identities** — paper Control Center vs dark instrument face stay distinct.
4. Give **options** (a canvas / 2–3 variations) for the modal + the control system,
   not a single answer.
