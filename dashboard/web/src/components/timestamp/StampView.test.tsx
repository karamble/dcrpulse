// Copyright (c) 2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { act, cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, expect, it, vi } from 'vitest';

const resolvers: Record<string, (d: string) => void> = {};
vi.mock('../../utils/hashFile', () => ({
  hashFile: (f: File) =>
    new Promise<string>((res) => {
      resolvers[f.name] = res;
    }),
}));
vi.mock('../../services/timestampApi', () => ({
  createTimestamp: vi.fn(async (b: any) => ({ ...b, status: 'submitted' })),
}));

import * as api from '../../services/timestampApi';
import { StampView } from './StampView';

afterEach(cleanup);

// FESHELL-2: replacing a file while the first still hashes must stamp the
// replacement's digest, even when the first hash finishes last.
it('stamps the digest of the file picked last', async () => {
  const { container } = render(<StampView />);
  const input = container.querySelector('input[type=file]') as HTMLInputElement;
  fireEvent.change(input, { target: { files: [new File(['a'.repeat(10)], 'big-A.iso')] } });
  fireEvent.change(input, { target: { files: [new File(['b'], 'small-B.txt')] } });
  await act(async () => resolvers['small-B.txt']('d'.repeat(64)));
  await act(async () => resolvers['big-A.iso']('a'.repeat(64)));

  fireEvent.click(await screen.findByText('Timestamp this file'));
  await waitFor(() => expect(api.createTimestamp).toHaveBeenCalled());
  const sent = vi.mocked(api.createTimestamp).mock.calls[0][0];
  expect(sent.filename).toBe('small-B.txt');
  expect(sent.digest).toBe('d'.repeat(64));
});
