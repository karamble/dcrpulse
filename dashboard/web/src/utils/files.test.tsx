import { afterEach, describe, expect, it, vi } from 'vitest';
import { downloadBlob } from './files';

afterEach(() => {
  vi.useRealTimers();
  vi.restoreAllMocks();
});

describe('downloadBlob', () => {
  it('clicks an attached anchor and revokes the URL only after the click', () => {
    vi.useFakeTimers();
    const create = vi.fn(() => 'blob:x');
    const revoke = vi.fn();
    Object.assign(URL, { createObjectURL: create, revokeObjectURL: revoke });
    let attached = false;
    const click = vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(function (this: HTMLAnchorElement) {
      attached = document.body.contains(this);
      expect(this.download).toBe('a.json');
    });
    downloadBlob('{}', 'a.json', 'application/json');
    expect(click).toHaveBeenCalledTimes(1);
    expect(attached).toBe(true);
    expect((create.mock.calls[0] as unknown as [Blob])[0].type).toBe('application/json');
    expect(revoke).not.toHaveBeenCalled();
    vi.runAllTimers();
    expect(revoke).toHaveBeenCalledWith('blob:x');
  });
});
