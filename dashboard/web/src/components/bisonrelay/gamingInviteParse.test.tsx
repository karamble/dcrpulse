import { describe, expect, it } from 'vitest';
import { splitGamingInvites, termsComplete, type GamingInvite } from './gamingInviteParse';

const full = 'gaming://poker/table?fv=2&sid=a1&buyin=100000&seats=2&csv=288&until=900100&bond=30000&bondcsv=4032&tablebond=0&tablebondcsv=0';

const parse = (link: string): GamingInvite => {
  const part = splitGamingInvites(`join ${link}`).find((p) => p.kind === 'invite');
  if (!part || part.kind !== 'invite') throw new Error('no invite');
  return part.invite;
};

describe('gaming invite terms', () => {
  it('reads every financial field from the link', () => {
    const i = parse(full.replace('tablebond=0&tablebondcsv=0', 'tablebond=70000&tablebondcsv=8064'));
    expect([i.fv, i.buyinAtoms, i.seats, i.csv, i.until, i.bondAtoms, i.bondCsv, i.tableBondAtoms, i.tableBondCsv]).toEqual(
      ['2', 100000, 2, 288, 900100, 30000, 4032, 70000, 8064],
    );
    expect(termsComplete(i)).toBe(true);
  });

  it('treats missing or old terms as incomplete', () => {
    expect(termsComplete(parse(full))).toBe(true);
    expect(termsComplete(parse(full.replace('fv=2', 'fv=1')))).toBe(false);
    expect(termsComplete(parse(full.replace('&bond=30000', '')))).toBe(false);
    expect(termsComplete(parse(full.replace('&bondcsv=4032', '')))).toBe(false);
    expect(termsComplete(parse(full.replace('&until=900100', '')))).toBe(false);
    expect(termsComplete(parse(full.replace('&tablebondcsv=0', '')))).toBe(false);
    expect(termsComplete(parse(full.replace('tablebond=0&tablebondcsv=0', 'tablebond=70000&tablebondcsv=0')))).toBe(false);
  });
});
