# Bison Relay quote embeds (draft v1)

A post or comment quotes another post by reference, using the standard
embed container:

    --embed[type=quote,from=<uid>,post=<pid>,alt=<text>]--

## Keys

- `type` MUST be the literal string `quote`.
- `from` MUST be the quoted post author's identity: 64 lowercase hex
  characters.
- `post` MUST be the quoted post id: 64 lowercase hex characters.
- `alt` SHOULD be a human-readable fallback such as
  `Quoted post from <nick>`, URL-path-escaped. Existing clients already
  parse and display alt, so clients that do not understand quotes degrade
  to meaningful text instead of an empty attachment.

## Rules

- The embed carries no quoted content. Renderers MUST resolve
  (from, post) against locally stored, author-signed posts and render
  from that copy. An unresolvable reference MUST NOT invent content:
  render a placeholder (the alt text) and, at most, offer a
  user-initiated fetch. Renderers MUST NOT fetch automatically.
- Renderers MUST validate both ids as 64-character hex; otherwise treat
  the embed as an unknown attachment.
- Quote depth is one: inside a rendered quote card, nested quote embeds
  are shown as plain references and are not resolved further.
- Emitters SHOULD pair the quote with a native relay of the quoted post
  ("quote relay"), so the emitter's subscribers receive the quoted
  content and the reference resolves for them.
- Unknown additional keys follow the container's rule: ignored.

## Authenticity

The quoting post's signature covers the reference; the quoted content's
authenticity comes from the quoted post's own signature. Nothing in this
scheme lets anyone put words in the quoted author's mouth.
