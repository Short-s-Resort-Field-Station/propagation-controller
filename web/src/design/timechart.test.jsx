// Timeline axis (2026-11-11): ticks scale with the window on the local clock.
import { describe, it, expect } from 'vitest';
import { timeTicks, nearest } from './timechart.jsx';

const local = (y, m, d, h = 0) => new Date(y, m - 1, d, h, 0, 0).getTime();
const H = 3600e3, D = 24 * H;

describe('timeTicks', () => {
  it('24 hours: every 3 h, midnight carries the date', () => {
    const from = local(2026, 11, 11, 20) + 25 * 60e3; // 20:25 local
    const ticks = timeTicks(from, from + D);
    expect(ticks.map((t) => t.label)).toEqual(['21:00', '00:00', '03:00', '06:00', '09:00', '12:00', '15:00', '18:00']);
    const mid = ticks.find((t) => t.label === '00:00');
    expect(mid.major).toBe(true);
    expect(mid.sub).toMatch(/^Thu 11\/12$/);
    expect(ticks.filter((t) => t.label !== '00:00').every((t) => t.sub === '')).toBe(true);
  });
  it('7 days: one tick per local midnight with weekday and date', () => {
    const from = local(2026, 11, 5, 9);
    const ticks = timeTicks(from, from + 7 * D);
    expect(ticks).toHaveLength(7);
    expect(ticks[0]).toMatchObject({ label: 'Fri', sub: '11/6' });
    expect(ticks.find((t) => t.label === 'Mon').major).toBe(true);
  });
  it('30 days: every 5 days; season: weekly', () => {
    const from = local(2026, 10, 12, 9);
    const month = timeTicks(from, from + 30 * D);
    expect(month).toHaveLength(6);
    expect(month[0].label).toBe('Oct 13');
    expect(month[1].t - month[0].t).toBe(5 * D);
    const season = timeTicks(from, from + 90 * D);
    expect(season).toHaveLength(13);
    expect(season[1].t - season[0].t).toBe(7 * D);
  });
});

describe('nearest', () => {
  it('returns the closest point by time', () => {
    const pts = [{ t: 0 }, { t: 10 }, { t: 20 }];
    expect(nearest(pts, 4)).toBe(pts[0]);
    expect(nearest(pts, 6)).toBe(pts[1]);
    expect(nearest(pts, 99)).toBe(pts[2]);
  });
});
