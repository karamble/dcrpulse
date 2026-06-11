# BR Editor

Self-contained React component for composing Bison Relay post bodies (and
any other surface that benefits from BR's markdown + embed lingo). Designed
as a portable package: a single import path through `./editor` exposes
the component, helpers, and types.

## Import

```ts
import {
  BisonrelayEditor,
  composeBRBody,
  isEditorOverHardCap,
} from './editor';
import type { EditorEmbedMap, EditorFeatures } from './editor';
```

## Minimal usage (controlled)

```tsx
const [body, setBody] = useState('');
const [embeds, setEmbeds] = useState<EditorEmbedMap>({});

return (
  <>
    <BisonrelayEditor
      value={body}
      onChange={setBody}
      embeds={embeds}
      onEmbedsChange={setEmbeds}
      placeholder="Write your post…"
    />
    <button
      disabled={isEditorOverHardCap(body, embeds)}
      onClick={() => createBisonrelayPost(composeBRBody(body, embeds))}
    >
      Publish
    </button>
  </>
);
```

The editor is controlled — it never owns the body string or the embed map;
the host manages both. `composeBRBody(body, embeds)` substitutes the
in-textarea `--embed[id=X]--` placeholders with the full BR wire-format
tags right before submission.

## Toggling toolbar groups

Pass a `features` prop to disable groups that don't fit the host surface:

```tsx
<BisonrelayEditor
  ...
  features={{ linkContent: false, preview: false }}
/>
```

Defaults: all groups enabled. Recognised keys: `attach`, `linkContent`,
`markdownHelpers`, `preview`, `sizeFooter`.

## Server-side dependencies

The editor consumes two dashboard endpoints. They are part of the editor's
contract — moving the editor to another deployment means moving these too:

| Endpoint                  | Purpose                                                      | Backed by                                |
| ------------------------- | ------------------------------------------------------------ | ---------------------------------------- |
| `GET  /api/br/shared-files` | List the local user's `c.ListLocalSharedFiles()` for the picker | `BrclientdSharedFiles` → brclientd `/shared-files` |
| `POST /api/br/posts/render` | Server-side render the draft so Preview matches Feed detail  | `services.SplitAndRenderBRPostBody`      |

The render endpoint runs purely in the dashboard process (no brclientd
hop) using the shared markdown + sanitization helper extracted from
`services/markdown.go`. That same helper renders Politeia proposals and
published BR posts, so the editor's Preview is byte-identical to the
published Feed view.

## Wire format

Inline `--embed[k=v,...]--` is the only BR wire tag. Three practical
shapes that this editor emits (canonical Go reference:
`github.com/companyzero/bisonrelay/internal/mdembeds`):

- **Inline data** — `name`, `type`, `data` (+ optional `alt`). Bytes ride
  inside the post.
- **Free download** — `download` (FID), `filename`, `type`, `size`. Bytes
  fetched separately over BR's file-transfer subsystem.
- **Paid download** — same as free + `cost=<milliatoms>`. Reader pays
  over Lightning before BR releases the bytes. This is BR's "pay to read
  more" mechanic.
