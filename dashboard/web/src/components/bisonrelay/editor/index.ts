// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// Public surface of the BR editor package. Import from this module rather
// than the individual files so the internal layout can change without
// disrupting callers.

export { BisonrelayEditor, isEditorOverHardCap } from './BisonrelayEditor';
export type { EditorFeatures } from './BisonrelayEditor';
export {
  composeBRBody,
  estimatedWireBytes,
  newEmbedId,
  placeholderFor,
} from './brEmbedBuilder';
export type { EditorEmbed, EditorEmbedMap } from './brEmbedBuilder';
export { SharedFilePickerModal } from './SharedFilePickerModal';
export { ImageAttachModal, isCompressibleImage } from './ImageAttachModal';
export type { ImageAttachResult } from './ImageAttachModal';
export { blobToDataB64 } from './imageCompress';
