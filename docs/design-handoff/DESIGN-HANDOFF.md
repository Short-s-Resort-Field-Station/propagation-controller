# Propagation Unit 01 — UI Design Handoff

**For:** a design pass by the `claude-design` skill.
**From:** the engineer who owns the backend + integration.
**Goal:** polish the *entire* interface into a cohesive, intuitive, beautiful
instrument — without inventing features or breaking the data contract. Output
HTML/CSS mockups (canvas of variations is welcome); I will translate the chosen
design back into the live React app afterward.

> **Read this first, then look at `screens/`.** Every screen referenced below
> has a real screenshot in `screens/`. These are the source of truth for the
> current state — design *from* them, not from imagination.

---

## 0. What this is

A headless Go daemon (`propagationd`) runs a 4-shelf plant-**propagation** rack
(cuttings, seeds, seedlings for a native-plant nursery / land-restoration
project on the Lampasas River). It controls irrigation (pump + per-shelf
solenoid valves, **ebb-and-flow**: fill, then hold the valve open to drain the
tray back), lighting, and fans, and monitors soil moisture, temperature, and pH.

**Two operator surfaces, one binary, two distinct identities:**

| Surface | Route | Device | Identity |
|---|---|---|---|
| **Control Center** | `/` | desktop/laptop browser | *Institutional paper* — a forestry-department field manual rendered as software |
| **Rack Instrument Face** | `/ui/touch` | Waveshare **5" 800×480** panel on the rack | *Dark instrument* — an engraved apparatus face, needle dials, diamond ornaments |

These identities are intentional and should stay **clearly distinct**. Do not
homogenize them into one "app" look.

**Who uses it:** nursery growers and a steward (A. Short). Calm, unhurried,
data-dense but never alarming. No consumer-app gloss. Think field station,
forestry service, laboratory logbook.

---

## 1. Design system — lift these exact values

Self-hosted fonts (already bundled, no CDN):

- **Zilla Slab** — display / numerals / status. Weight **900** for big values and titles. Slab-serif, institutional.
- **Newsreader** — serif body & *italic hints/notes*. Warm, editorial.
- **JetBrains Mono** — all-caps labels, timestamps, metadata. Letter-spaced `0.14–0.20em`, `9.5–11px`.

Type feel: **BIG slab numerals** (a 42–68px `Zilla Slab 900` reading) paired with
**tiny mono labels** and **italic serif** asides. That contrast is the whole voice.

### Paper palette (Control Center)
```
--paper      #ECE3CE   page background
--paper-2    #E2D7BE   --paper-3 #D6C9AC   (deeper paper)
--bone       #F4ECD8   raised surfaces / footer
--ink        #1B1813   primary text / borders
--char       #2C2620   secondary text
--rule       #1B181333 (hairline)   --rule-soft #1B181318
--sage       #8E9B85   --sage-deep #5F6B58   (healthy / primary action)
--ochre      #B58A3C   --ochre-deep #8E6A28   (attention / maintenance)
--oxblood    #7A2E1E   --oxblood-deep #5A1E12 (fault / destructive)
--river      #5C7A78   --river-deep #3F5957   (water / irrigation)
--dusk       #5E6E8C
```

### Instrument palette (Touch face)
```
--face   #211E18   --face-2 #2A2720          panel body
--lcd    #F0E8D4   --lcd-dim #A79F8B   --lcd-faint #6F6859   (engraved text tiers)
--hair   #F0E8D420  --hair-soft #F0E8D412    (etched hairlines)
--accent #7FA088    operational green
--i-river #6E97A0   --i-ochre #C99A45   --i-oxblood #C25A3E   --i-sage #9FB08C
```

### Motifs already in use (keep / refine, don't discard)
- **Diamond ornaments** (◆ / rotated squares) as bullet accents and dividers.
- **Engraved hairline rules** with a centered diamond on the touch face.
- **Framed corners** (little L-brackets) on the instrument panel.
- **Needle dial gauges** (270° sweep, tick marks, target-band arc) on touch;
  **arc gauges** + **target-band bars** on paper.
- A **forestry seal** logo (`/seal.png`) top-left on the web header.
- Status shown as a **lamp/dot + label**, never a loud badge.

---

## 2. Screens — the inventory (see `screens/`)

### Control Center (`/`) — paper

Header (persistent): seal · "PROPAGATION UNIT 01 / Control Center" · tab nav
(**Dashboard · Shelf Management · Environmental · Controls**) · Alerts count ·
signed-in operator · **Maintenance** toggle chip · rack-status chip.

1. **`01-web-overview.png` — Dashboard / Operations Overview.**
   5-cell stat grid (Rack Status, Reservoir, Active Shelves, Watering Now,
   Active Alerts) · Event Log panel · Reservoir tank panel. *This screen reads
   well already — treat as the reference for the paper voice.*

2. **`02-web-shelf-management.png` — Shelf Management.**
   One card per physical bay (01–04). Card shows specimen, status badge,
   moisture + temperature readings, a moisture-vs-target band bar, lighting /
   last-water / 24h-usage, and actions. **Four card states** (see §3): active,
   dry, **disabled/paused**, **vacant**.

3. **`03-web-environmental.png` — Environmental Monitoring.**
   Period segmented control (24h / 7d / 30d / Season) + stacked chart cards:
   Soil Moisture (per-shelf lines), Soil Temperature (rack mean area),
   Soil pH (per-shelf), Watering minutes (bars). **A "Target Adherence" panel**
   (per-shelf % of time inside its pH/temp happy-band) appears here with live
   data — not visible in this demo shot; described in §3/§4.
   *Known nit:* moisture chart mid-axis label renders clipped ("62.5%"→"2.5%").

4. **`04-web-controls.png` — Controls.**
   Per-shelf irrigation sliders (moisture target/min, duration, cooldown) +
   rack photoperiod + manual light override + Apply bar (scope: this shelf / all).

5. **`05-web-modal-edit.png` — Assign / Edit Shelf modal. ← the priority.**
   Just reworked into labeled sections (**Specimen & Profile · Watering ·
   Monitoring targets · Lighting**) with compact typed number fields.
   `05b-...BEFORE.png` shows the prior version (a wall of +/- steppers that
   overflowed the modal) — **included so you see what "messy" meant and don't
   regress to it.** The current version is *better but still the weakest screen*
   — this is where the most design attention is wanted. It carries the species
   picker, propagation-stage dropdown (Seeds/Seedlings/Cuttings/Trimmings/
   Suckers/Saplings + custom), a **species-profile** picker (saved settings), a
   save-as-profile control, moisture band, drain-back time, pH & temp
   **monitoring** happy-bands, and photoperiod.

   Other modals (not screenshotted, same paper language, lower priority):
   **Sign In** (operator + PIN pad), **Alerts & Escalation**, **Event History**
   (+ CSV export), **Confirm** (generic).

### Rack Instrument Face (`/ui/touch`) — dark, **fixed 880×528 logical → scales to 800×480**

6. **`06-touch-overview.png` — Overview.**
   Masthead ("PROPAGATION UNIT · NO. 01") · date/status/time line · four shelf
   **plates** (specimen, needle dial, reading, target, status) · reservoir
   measure. A **ledger** (table) layout also exists as an alternate. Light
   ("limestone") and dark themes both exist; dark is default.

7. **`07-touch-shelf-detail.png` — Shelf detail (tap a plate).**
   Return / status header · specimen name · three dial cards (Moisture,
   Temperature, **Soil pH** — each shows its happy-band) · Lighting + Fans
   segmented toggles · **Water Now** · **Disable Shelf** + **Maintenance**
   actions. **Constraint risk:** on the 480px panel the Water Now + action row
   must not fall below the fold — verify.

   Also exists: **Lock screen** (operator + dark PIN pad), **offline** + **maintenance** banners.

---

## 3. State vocabulary (design every state, not just the happy one)

**Per-shelf status** (drives badge color + card/plate treatment):

| Status | Meaning | Paper ink | Touch color |
|---|---|---|---|
| Healthy | in band, all good | sage-deep | i-sage / accent |
| Monitoring | drifting toward low band | ochre-deep | i-ochre |
| Watering | pump on, valve open (fill) | river-deep | i-river |
| **Draining** | pump **off**, valve **open**, tray draining back | river-deep | i-river |
| Dry | below target band | ochre-deep | i-ochre |
| Sensor Fault | probe not reporting | oxblood | i-oxblood |
| **Maintenance** | automation suspended (calmed alarms) | ochre-deep | i-ochre |
| **Disabled** | assigned but paused (sensors+valve off) | faint/grey | faint |
| Inactive | vacant bay, no specimen | faint/grey | faint |

**Rack status chip:** Operational · Watering · Draining · **Maintenance** · Fault.

**System modes:**
- **Maintenance mode** (rack-wide): suspends irrigation automation only (lighting
  keeps running); shows an **ochre banner** on both surfaces; calms per-shelf
  moisture/probe alarms; manual watering still allowed *with a confirm*.
- **Offline**: controller unreachable → stale banner + dimmed readings.
- **Per-shelf disable**: paused (assigned) vs vacant (unassigned) are distinct.

---

## 4. What's real — the data contract (keep designs honest)

Design only what the hardware actually provides. Do **not** invent controls for
things the unit can't do.

- **Controls (actuators):** pump, 4 solenoid valves, lighting circuit (rack-wide,
  one relay), fans (rack-wide, single-speed). Irrigation is **one shelf at a
  time**, including the drain-back phase.
- **Sensors (monitor only):** per-shelf soil **moisture**; soil **temperature**
  and **pH** (in v1 the rack shares one temperature probe — *temperature is
  rack-wide until each shelf gets its own probe*; pH is already per-shelf);
  reservoir level.
- **pH & temperature are MONITORING ONLY.** The unit senses but cannot heat, cool,
  or dose. Their "happy-band" ranges drive gauges + the time-in-target tracking,
  nothing more. Never draw a pH/temp *setpoint/control*.
- **Species profiles:** per species (native catalog, e.g. Bald Cypress /
  *Taxodium distichum*), per **stage**, a grower saves settings (moisture control
  + pH/temp bands) and re-applies them to a shelf.
- **Reservoir:** 26.5 L (7 gal) tank; shows %, litres, ~days remaining.
- **No buzzer** exists or ever will — escalation is email/SMS only. Don't design audible alarms.
- Everything is **logged** (event log / history with CSV export).

---

## 5. Design goals for this pass

1. **The Assign/Edit modal is the priority.** Make it genuinely intuitive and
   calm. It's information-dense (specimen, profile, watering, monitoring,
   lighting) — solve density with hierarchy and grouping, not by hiding things
   behind steppers. Explore layouts (sectioned vs. progressive-disclosure vs.
   two-column). The stage/profile/save flow tested well — keep that intent.
2. **Cohesion.** Unify spacing rhythm, section-header treatment, control styling
   (inputs, toggles, segmented controls, buttons) across all web screens.
3. **Hierarchy.** Every screen should have one obvious focal reading, then
   supporting detail — the big-numeral / tiny-label contrast is the tool.
4. **Touch face:** keep the instrument character; verify the shelf-detail fits
   800×480 without scrolling; make Water Now unmistakably primary.
5. **Respect the two identities.** Paper stays paper; instrument stays instrument.
6. **No slop:** no aggressive gradients, no emoji, no rounded-card-left-border
   accent stripes, no fake SVG imagery. (The current design already avoids these
   — hold the line.)

Give options (a canvas of 2–3 variations for the modal + a control-styling
system) rather than a single answer.

---

## 6. Hard constraints

- Touch canvas is **fixed 880×528**, scaled to the physical **800×480**. All
  touch content must fit that ratio with **≥44px tap targets**.
- Web is desktop-first (large viewport); it does not need a mobile layout.
- Keep the **forestry seal** and the institutional voice.
- Don't add features, sections, or copy that aren't in this doc — ask instead.

---

## 7. How your output comes back to code (for my integration)

The live app is React + Vite. Design lives in `web/src/design/` as ES-module
components; visual styling is one stylesheet, `web/src/design/pgui.css`, driven
by the CSS custom properties in §1. A thin `web/src/live/` layer feeds real API
data in.

So: **produce HTML/CSS using the tokens in §1** (reuse the variable names where
you can). Static HTML mockups or a design-canvas of variations are ideal — I'll
port the chosen direction into the React components and wire it to the API. You
do **not** need to touch React, the backend, or data-fetching; focus purely on
the visual/interaction design. Call out any new token or component you introduce
so I can add it cleanly.

---

## 8. Screenshot index (`screens/`)

| File | Screen |
|---|---|
| `01-web-overview.png` | Control Center · Dashboard/Overview |
| `02-web-shelf-management.png` | Control Center · Shelf Management (4 card states) |
| `03-web-environmental.png` | Control Center · Environmental charts |
| `04-web-controls.png` | Control Center · Controls (sliders + photoperiod) |
| `05-web-modal-edit.png` | **Assign/Edit Shelf modal — current (priority)** |
| `05b-web-modal-edit-BEFORE.png` | The modal *before* rework (what "messy" meant — avoid) |
| `06-touch-overview.png` | Rack instrument face · Overview |
| `07-touch-shelf-detail.png` | Rack instrument face · Shelf detail |

*Generated 2026-07-09 from the live design components with demo data.*
