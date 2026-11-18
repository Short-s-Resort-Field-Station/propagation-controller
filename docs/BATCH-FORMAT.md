# Propagation batch format, version 1

Two CSV sheets describe propagation work: one row per batch, one row per dated
event. Long Watch owns this format and reads and writes it. Propagation Unit
No. 01 writes it (Batches tab, "Batches CSV" and "Log CSV"). The same two
sheets can be kept by hand in any spreadsheet.

Rules for both sheets:

- UTF-8 CSV with a header row. Column names are fixed and lower case.
- Dates are written `YYYY-MM-DD`.
- `batch_code` joins the two sheets. It is unique and never reused.
- An empty cell means "not recorded". It never means zero.
- A source is a name or a section of a property. It is never coordinates.

## Sheet 1: Batches (`propagation-batches-v1.csv`)

| Column | Required | Meaning |
|---|---|---|
| `batch_code` | yes | The label on the tray, for example `M-07` |
| `species` | yes | Botanical name |
| `species_code` | no | USDA PLANTS symbol, for example `CAAM2` |
| `source` | no | Where the seed or cutting came from |
| `started_on` | yes | Date the batch was started |
| `method` | no | One of the methods below |
| `container` | no | For example `2.5 in pot` |
| `medium` | no | For example `coir and perlite` |
| `count_started` | no | Seeds or cuttings started |
| `count_current` | no | Alive now |
| `status` | yes | One of the statuses below |
| `location` | no | Where it is now, for example `Shelf 2` |
| `notes` | no | Free text |

Methods: `direct_sow`, `flood_tray`, `cold_moist_strat`, `warm_strat`,
`scarification`, `direct_stick`, `perlite_coir`, `water_rooting`, `other`.

Statuses: `active`, `rooted`, `hardening`, `planted_out`, `failed`, `archived`.

## Sheet 2: Log (`propagation-log-v1.csv`)

| Column | Required | Meaning |
|---|---|---|
| `batch_code` | yes | The batch this line belongs to |
| `date` | yes | The day it happened |
| `event` | yes | One of the events below |
| `count` | no | Number alive after this event |
| `location` | no | Where the batch was, or for `moved`, where it went |
| `note` | no | Free text |
| `waterings` | no | Waterings in the period ending on `date` |
| `moisture_low_pct` | no | Lowest media moisture in the period |
| `moisture_high_pct` | no | Highest media moisture in the period |
| `light_hours` | no | Hours of light in the period |
| `conditions_source` | no | `measured` (an instrument) or `manual` (written down by hand) |

Events: `started`, `sown`, `germinated`, `counted`, `potted_up`, `moved`,
`lost`, `planted_out`, `note`, `conditions`.

`count` always means the number alive after the event, for every event, so the
newest count in a batch's log is its current count.

A `conditions` line carries the last five columns and nothing in `count`. The
propagation unit writes one per batch per week it sat on a shelf, dated the last
day of that week, with `conditions_source` set to `measured`. It does not yet
total light hours, so that cell is empty.
