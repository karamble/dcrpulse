// Copyright (c) 2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { act, cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import { BisonrelayPages } from './BisonrelayPages';
import { fetchBisonrelayPage } from '../../services/bisonrelayApi';

vi.mock('./BisonrelayUserBar', () => ({ BisonrelayUserBar: () => null }));
vi.mock('./BisonrelayStoreMode', () => ({ BisonrelayStoreModePanel: () => null }));
vi.mock('./BisonrelayStoreManager', () => ({ BisonrelayStoreManager: () => null }));
vi.mock('./editor', () => ({ BisonrelayEditor: () => null, composeBRBody: (s: string) => s }));
vi.mock('../../services/bisonrelayStoreBlog', () => ({
  isArticlePath: () => false,
  isBlogManaged: async () => false,
  rebuildBlogIndex: async () => 0,
}));
vi.mock('../../services/bisonrelayApi', async (original) => ({
  ...(await original<typeof import('../../services/bisonrelayApi')>()),
  fetchBisonrelayPage: vi.fn(),
  getBisonrelayContacts: vi.fn(async () => []),
  getBisonrelayIdentity: vi.fn(async () => ({ identity: 'ee'.repeat(32) })),
}));

const A = 'aa'.repeat(32);
const B = 'bb'.repeat(32);
const page = (html: string) => ({
  session_id: 1,
  page_id: 1,
  parent_page: 0,
  status: 200,
  markdown: '',
  segments: [{ kind: 'text', html }],
});

beforeEach(() => {
  window.location.hash = `#pages/visit/${A}/index.md`;
});
afterEach(() => {
  cleanup();
  window.location.hash = '';
});

// FEBR-3: a slow host's reply that arrives after the user went back must not
// replace the page the address bar names.
it('drops a late reply for a page the user has left', async () => {
  let releaseB!: (v: any) => void;
  vi.mocked(fetchBisonrelayPage).mockImplementation(async (req: any) => {
    if (req.uid === A) return page(`<p>PAGE-A <a href="br://${B}/index.md">go to B</a></p>`) as any;
    return new Promise((res) => {
      releaseB = res;
    });
  });
  render(<BisonrelayPages />);
  await screen.findByText(/PAGE-A/);
  fireEvent.click(screen.getByText('go to B'));
  await act(async () => {});
  fireEvent.click(screen.getByTitle('Back'));
  await screen.findByText(/PAGE-A/);

  await act(async () => releaseB(page('<p>PAGE-B-HOSTILE</p>')));
  expect(screen.queryByText(/PAGE-B-HOSTILE/)).toBeNull();
  expect(screen.queryByText(/PAGE-A/)).not.toBeNull();
});
