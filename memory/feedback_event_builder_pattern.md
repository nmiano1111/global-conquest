---
name: feedback-event-builder-pattern
description: User deferred injected domain event builder pattern; revisit if event building grows complex
metadata:
  type: feedback
---

Keep event construction directly in the engine (current approach: engine builds typed payload, service persists it). Do not introduce an injected "domain event builder" interface preemptively.

**Why:** The builder adds indirection without changing the data flow — the engine has the same facts either way. The boundary (engine produces typed data, service persists) is already correct.

**Revisit if:** Event payload construction becomes complex — conditional fields, normalization, multiple subtypes per action. At that point, extracting a constructor function (not an injected interface) may help.
