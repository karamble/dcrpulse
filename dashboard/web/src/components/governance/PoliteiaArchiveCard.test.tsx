// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import * as api from '../../services/api';
import * as files from '../../utils/files';
import { PoliteiaArchiveCard } from './PoliteiaArchiveCard';

vi.mock('../../services/api', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../services/api')>()),
  getPoliteiaArchive: vi.fn(),
  exportPoliteiaArchive: vi.fn(),
  importPoliteiaArchive: vi.fn(),
}));
vi.mock('../../utils/files', () => ({ downloadBlob: vi.fn() }));

const pickFile = (file: File) =>
  fireEvent.change(screen.getByLabelText('Archive file to import'), { target: { files: [file] } });

afterEach(() => {
  cleanup();
  vi.mocked(api.getPoliteiaArchive).mockReset();
  vi.mocked(api.exportPoliteiaArchive).mockReset();
  vi.mocked(api.importPoliteiaArchive).mockReset();
  vi.mocked(files.downloadBlob).mockReset();
});

describe('PoliteiaArchiveCard', () => {
  it('imports a file and lists what it refused', async () => {
    vi.mocked(api.getPoliteiaArchive)
      .mockResolvedValueOnce({ proposals: 0 })
      .mockResolvedValue({ proposals: 2 });
    vi.mocked(api.importPoliteiaArchive).mockResolvedValue({
      imported: 2,
      kept: 1,
      refused: [{ token: 'abcdabcdabcdabcd', reason: 'invalid signature' }],
    });
    const onImported = vi.fn();
    render(<PoliteiaArchiveCard onImported={onImported} />);
    await screen.findByText(/^0 finished proposals archived/);
    expect((screen.getByRole('button', { name: /export/i }) as HTMLButtonElement).disabled).toBe(true);

    const file = new File(['{}'], 'archive.json', { type: 'application/json' });
    pickFile(file);
    await screen.findByText('Imported 2, kept 1 the dashboard already holds.');
    expect(api.importPoliteiaArchive).toHaveBeenCalledWith(file);
    expect(screen.getByText('1 refused:')).toBeTruthy();
    expect(screen.getByText('abcdabcdabcdabcd')).toBeTruthy();
    expect(screen.getByText(/: invalid signature/)).toBeTruthy();
    expect(onImported).toHaveBeenCalledTimes(1);
    expect(await screen.findByText(/^2 finished proposals archived/)).toBeTruthy();
  });

  it('says why an import stopped', async () => {
    vi.mocked(api.getPoliteiaArchive).mockResolvedValue({ proposals: 1 });
    const response = { status: 400, data: 'not a dcrpulse Politeia archive' };
    vi.mocked(api.importPoliteiaArchive).mockRejectedValue(
      Object.assign(new Error('failed'), { response }),
    );
    const onImported = vi.fn();
    render(<PoliteiaArchiveCard onImported={onImported} />);
    await screen.findByText(/^1 finished proposal archived/);
    pickFile(new File(['[]'], 'other.json'));
    expect(await screen.findByText('not a dcrpulse Politeia archive')).toBeTruthy();
    expect(onImported).not.toHaveBeenCalled();
  });

  it('downloads the export', async () => {
    vi.mocked(api.getPoliteiaArchive).mockResolvedValue({ proposals: 3 });
    const blob = new Blob(['{}']);
    type Download = Awaited<ReturnType<typeof api.exportPoliteiaArchive>>;
    vi.mocked(api.exportPoliteiaArchive).mockResolvedValue({ data: blob } as Download);
    render(<PoliteiaArchiveCard onImported={() => {}} />);
    await screen.findByText(/^3 finished proposals archived/);
    fireEvent.click(screen.getByRole('button', { name: /export/i }));
    const name = 'dcrpulse-politeia-archive.json';
    await waitFor(() => expect(files.downloadBlob).toHaveBeenCalledWith(blob, name, 'application/json'));
  });
});
