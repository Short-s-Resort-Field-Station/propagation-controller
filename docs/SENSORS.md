# SENSORS — verified hardware reference

The soil probes for Propagation Unit 01 were reverse-engineered against the
physical device on 2026-07-10 (full 64K register dump over RS485, plus live
grip / tap-water / salt-water response tests). Everything below is verified
against real hardware, not a seller datasheet.

## The probe

- **Model:** ZTS-3000-TR-ECWS-N01 · 3-prong soil temperature / moisture / EC
- **OEM:** Jinan Zhaotaisheng Electronic Technology (兆泰盛), Jinan, Shandong
- **Supply:** 4.5–30 VDC (runs fine off the FTDI cable's 5 V wire on the bench;
  on the rack it shares the 12 V rail)
- **Bus:** Modbus RTU, **4800 baud, 8N1, factory slave address 1**
  (`-modbus-baud` defaults to 4800 to match)
- **No pH channel.** Register 3 is empty; sellers advertising pH on this model
  are inventing it. The product therefore surfaces **substrate EC** instead.

### Wiring

| Probe wire | Meaning | FTDI USB-RS485-WE bench cable |
| --- | --- | --- |
| Brown | V+ (4.5–30 V) | Red (+5 V) |
| Black | GND | Black |
| Yellow | RS485 A | **Orange** (Data+) |
| Blue | RS485 B | Yellow (Data−) |

⚠️ The probe's *yellow* wire does **not** go to the cable's *yellow* wire —
that mismatch reads as a completely silent bus.

## Register map (function 0x03; 0x04 mirrors it)

| Register | Meaning | Encoding |
| --- | --- | --- |
| 0x0000 | Soil moisture | uint16 ×10 → %VWC |
| 0x0001 | Temperature | int16 ×10 → °C |
| 0x0002 | EC | uint16, µS/cm, **pins at 20000** (driver marks such readings quality `clipped`) |
| 0x0003 | — | always 0 (the pH that never was) |
| 0x0010 | Raw dielectric channel | float32 (2 regs), feeds the moisture conversion |
| 0x0012 | Raw NTC thermistor | float32 (2 regs), inverse to temperature |
| 0x07D0 | **Slave address** | uint16, writable — `probe-address` tool uses this |
| 0x07D1 | Baud code | 1 = 4800 |
| 0x07D3–4 | Serial number / UID (probable) | uint32 |
| 0x0834–0x084B | Per-channel config blocks (enable, float gain 1.0, range limits ×10) | undocumented, writable-looking |
| 0x530E–0x531B | Factory calibration table (six float32) | do not touch |

The firmware serves the entire 64K address space without bounds checks;
everything not listed reads as zero.

## Measured behaviour (bench, 2026-07-10)

- **Salt-water test passed:** EC 246 → 20000+ µS/cm moved the moisture reading
  only 92.0 → 88.2 %. The moisture channel is a genuine dielectric (FDR-class)
  measurement, largely immune to salinity/fertigation — not a cheap
  low-frequency capacitive sensor.
- Noise floor in a uniform medium: ±0.2 %VWC, ±1 µS/cm. Response to contact
  change is sub-250 ms.
- In air: moisture 0.0 %, EC 0 — useful as a pulled-probe signature.

## Multi-probe bus (shelves 02–04)

Every probe ships at address 1, so two fresh probes on one bus collide.
Before connecting an additional probe, readdress it **alone on the bus**:

```sh
bin/probe-address -to 2        # dry-run: reports the probe it found
bin/probe-address -to 2 -yes   # writes register 0x07D0, verifies, done
```

Then add a `soil_zone_N` device row pointing at the new address and label the
cable. Suggested convention: shelf number = slave address.

## Logging the raw dielectric channel (optional)

For building a substrate-specific calibration curve (pot-weight experiment),
add the raw channel to the soil sensor's `config_json` — no code needed:

```json
{"name": "raw_dielectric", "register": 16, "function_code": 3,
 "data_type": "float32", "scale": 1, "unit": "", "sensor_key": "soil_zone_1_raw"}
```

Log it alongside `soil_zone_1_moisture` while a planted pot dries on a scale;
the pair gives you a conversion curve better than the factory's.

## Reservoir level transmitter

Second Modbus device, address 2, register 0 = level uint16 ×10 → % of water
column. Not yet verified against real hardware — treat its map as provisional
until bring-up.
