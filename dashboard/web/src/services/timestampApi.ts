// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import api from './api';

export type TimestampStatus = 'submitted' | 'awaiting' | 'pending' | 'anchored' | 'failed';
export type ChainState = 'notfound' | 'awaiting' | 'pending' | 'anchored';

export interface TimestampRecord {
  digest: string;
  filename: string;
  title?: string;
  description?: string;
  fileSize: number;
  mimeType?: string;
  fileMtime?: string;
  tags?: string[];
  status: TimestampStatus;
  submittedAt: string;
  failReason?: string;
  anchorTime?: number;
  merkleRoot?: string;
  merklePath?: unknown;
  txId?: string;
  confirmations?: number;
  minConfirmations?: number;
  createdAt: string;
  updatedAt: string;
}

export interface DigestResult {
  digest: string;
  found: boolean;
  state: ChainState;
  anchorTime: number;
  merkleRoot?: string;
  merklePath?: unknown;
  txId?: string;
  confirmations: number;
  minConfirmations: number;
}

export interface Validation {
  digest: string;
  hasProof: boolean;
  merklePathValid: boolean;
  digestInTree: boolean;
  rootMatches: boolean;
  anchoredOnChain: boolean;
  txId?: string;
  blockHeight?: number;
  blockTime?: number;
  confirmations?: number;
  merkleRoot?: string;
  note?: string;
}

export interface VerifyResponse {
  digest: string;
  inArchive: boolean;
  record?: TimestampRecord;
  dcrtime?: DigestResult;
  dcrtimeError?: string;
  validation?: Validation;
}

export interface TimestampStatusInfo {
  enabled: boolean;
  network: string;
  host: string;
  pending?: number;
  total?: number;
  reachable?: boolean;
  reachableError?: string;
}

export interface CreateTimestampInput {
  digest: string;
  filename: string;
  title?: string;
  description?: string;
  fileSize: number;
  mimeType?: string;
  fileMtime?: string;
  tags?: string[];
}

export interface ListParams {
  q?: string;
  status?: string;
  tag?: string;
  sort?: string;
}

export async function listTimestamps(params?: ListParams): Promise<TimestampRecord[]> {
  const res = await api.get<TimestampRecord[]>('/timestamp/records', { params });
  return res.data || [];
}

export async function createTimestamp(input: CreateTimestampInput): Promise<TimestampRecord> {
  const res = await api.post<TimestampRecord>('/timestamp/records', input);
  return res.data;
}

export async function getTimestamp(digest: string): Promise<TimestampRecord> {
  const res = await api.get<TimestampRecord>(`/timestamp/records/${digest}`);
  return res.data;
}

export async function updateTimestamp(
  digest: string,
  input: { title: string; description: string; tags: string[] },
): Promise<TimestampRecord> {
  const res = await api.patch<TimestampRecord>(`/timestamp/records/${digest}`, input);
  return res.data;
}

export async function deleteTimestamp(digest: string): Promise<void> {
  await api.delete(`/timestamp/records/${digest}`);
}

export async function retryTimestamp(digest: string): Promise<TimestampRecord> {
  const res = await api.post<TimestampRecord>(`/timestamp/records/${digest}/retry`);
  return res.data;
}

export async function verifyTimestamp(digest: string): Promise<VerifyResponse> {
  const res = await api.post<VerifyResponse>('/timestamp/verify', { digest });
  return res.data;
}

export async function validateTimestamp(input: {
  digest?: string;
  merkleRoot?: string;
  merklePath?: unknown;
  txId?: string;
}): Promise<Validation> {
  const res = await api.post<Validation>('/timestamp/validate', input);
  return res.data;
}

export async function refreshTimestamps(sort?: string): Promise<TimestampRecord[]> {
  const res = await api.post<TimestampRecord[]>('/timestamp/refresh', null, {
    params: sort ? { sort } : undefined,
  });
  return res.data || [];
}

export async function getTimestampStatus(ping = false): Promise<TimestampStatusInfo> {
  const res = await api.get<TimestampStatusInfo>('/timestamp/status', {
    params: ping ? { ping: 1 } : undefined,
  });
  return res.data;
}

// Direct-download endpoints (opened in a new tab / via an anchor element).
export function proofDownloadUrl(digest: string): string {
  return `/api/timestamp/records/${digest}/proof`;
}

export function exportUrl(): string {
  return '/api/timestamp/export';
}
