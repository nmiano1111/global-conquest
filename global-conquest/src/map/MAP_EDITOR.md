# Tracing territory polygons

`territoryShapes.ts` has traced shapes for all 42 territories. All of them
except Madagascar were traced with the automated pixel-analysis method
described below, not by hand — see "Automated tracing" if a territory ever
needs re-tracing (a bad edge found later, art changes, etc.). The fallback
path is still live and still worth knowing about: if a territory's entry is
ever removed or fails to load, `MapScene.buildScene`
(`territoryShapes[name] ? new TerritoryPolygon(...) : new TerritoryNode(...)`)
drops it back to the legacy circular token rather than crashing. This doc
covers both the manual in-app editor and the automated method.

## Opening the editor

1. `npm run dev` (dev server only — the editor is compiled out of production
   builds via `import.meta.env.DEV`).
2. Open a game page with `?mapEditor=true` appended, e.g.
   `http://localhost:5173/app/game/<id>?mapEditor=true`.
3. A panel appears in the top-right corner of the map. While it's open, the
   map's normal click-to-select/attack/reinforce behavior is suspended —
   every click on the map is interpreted as an editor action instead (pan,
   pinch, and wheel-zoom still work normally, so you can zoom in for
   precision).

## Tracing a territory

1. Pick the territory from the dropdown. If it already has traced data,
   its existing polygon(s) and label load in for editing; otherwise you
   start from a blank polygon.
2. Click along the territory's border in risk0.png to lay down vertices in
   order (they don't need to close the loop — the last point connects back
   to the first automatically once there are 3+ points).
3. Click **"Set label position"**, then click once on the map — that's
   where the army-count badge will render for this territory. Pick
   somewhere clearly inside the shape.
4. To fix a misplaced vertex, click and drag it (works for any vertex of
   the polygon currently being edited).
5. **Undo point** removes the most recently placed vertex (or the current
   empty polygon, if you just started a new one and haven't placed a point
   in it yet). **Clear territory** discards all progress on the current
   territory and starts over.

## Islands / disconnected landmasses

If a territory has more than one visually separate landmass (e.g.
Indonesia, or Japan's split islands), finish tracing the first landmass,
click **"New polygon"**, then trace the next one. All polygons you add for
the same selected territory share one entry in `territoryShapes.ts` and
render/select/highlight as a single territory — clicking any one of them
selects the same territory.

## Dateline territories (Alaska / Kamchatka)

The map doesn't visually wrap — Alaska and Kamchatka each sit fully on one
side of the image with no landmass crossing the left/right edge. Trace each
as an ordinary single polygon; do not try to stretch a polygon across the
whole map width to represent their adjacency (that adjacency is drawn as a
connector line by `MAP_EDGES`, not by shape geometry).

## Exporting

Click **"Export all shapes → clipboard"**. This copies a complete, ready-
to-paste `export const territoryShapes: Record<string, TerritoryShape> = {
... }` block covering every territory you've traced this session *plus*
whatever was already in `territoryShapes.ts` when you opened the editor
(the editor loads a working copy, so exporting doesn't lose others' prior
work). Paste it over the existing `territoryShapes` declaration in
`territoryShapes.ts`, keeping the file's imports/types/doc comments as-is.

If clipboard access is denied (some dev setups block it), the export falls
back to a `console.log` of the same text — open devtools and copy it from
there.

A territory is only included in the export once it has **3+ points in at
least one polygon and a label position set** — half-finished territories
are silently omitted so you can't accidentally ship a broken shape.

## Coordinate format

Coordinates are normalized (0–1) against risk0.png's actual pixel
dimensions (`MAP_SOURCE_WIDTH`/`MAP_SOURCE_HEIGHT` in `territoryShapes.ts`,
currently 2048×1367), not against any render size — so shapes stay aligned
regardless of how large the map is drawn on screen.

## Known rough edges

Two territories have a visibly straight (not coastline-shaped) edge:
**Japan** and, to a lesser extent, **Siam**. Both sit at a busy multi-way
adjacency-line junction (Kamchatka/Mongolia/Japan and China/India/Siam/
Indonesia respectively) where the automated method kept leaking through the
connector lines regardless of hub-punching or spike-trimming (see
"Automated tracing" below) — the shapes shipped are the largest
clean, non-leaking regions found, clipped by the bounding box rather than
by real coastline on one side. Both are easy to fix by hand in the editor:
select the territory, drag the flat-edge vertices out to match the true
coastline, no need to retrace the rest of the shape.

## Automated tracing

Clicking every vertex by hand is slow and, for a complex coastline, easy to
get subtly wrong. Great Britain, Iceland, and Indonesia's sample shapes
were instead extracted with a small offline script (Python + numpy/scipy +
Pillow, run against `src/assets/images/risk0.png` directly — not part of
the app or its build). The method, in case it's worth scripting again for
the remaining territories:

1. **Find a land/ocean color discriminant.** risk0.png tints each continent
   a distinct hue over a shared warm tan "parchment" ocean/background. Don't
   assume raw luminance or a fixed RGB threshold works — sample pixel values
   on both sides of a known coastline first. What worked here:
   - Europe (grayish-lavender continent, e.g. Great Britain, Iceland): `R − B`
     is small on land, large (warm) on ocean.
   - Australia (pink-lavender, e.g. Indonesia): `G − B` is small on land,
     large on ocean.
   - Africa and the Americas (tan continents): no channel-difference
     reliably separates them from the ocean's own tan. Edge/ink-line
     detection (below) works instead, with heavier denoising to suppress
     parchment-texture noise — see `segment_region_edges_only` /
     `MAG_DENOISED` in the tracing script. If that still doesn't cleanly
     separate a territory, hand-tracing via the in-app editor (above) is
     the fallback.
2. **Threshold that channel** into a binary land mask within a bounding box
   around the territory.
3. **Punch out adjacency-line hub points** before labeling connected
   components. Territory borders converge on shared "hub" dots (visible as
   small red-tinted blobs where thin connector lines meet); those lines are
   dark/desaturated enough to sometimes cross the same land threshold and
   silently bridge a territory's mask to its neighbor's. Detect hubs as
   local maxima of `R − G` (they're reddish) and zero out a small disk
   (radius ~8–10px) around each before connected-component labeling.
   `scipy.ndimage.binary_opening` with a small (3×3) structuring element
   helps too, but isn't sufficient by itself — the punch is what actually
   breaks the bridge.
4. **Label connected components** (`scipy.ndimage.label`) and select the
   one containing a seed pixel you've manually confirmed sits inside the
   territory (sample a few candidates and check which one reads as "land"
   by your threshold — text glyphs and coastline ink both read as non-land,
   so avoid seeding directly on either).
5. **Trace the boundary** with a Moore-neighbor contour walk, then simplify
   with Douglas-Peucker (epsilon ≈ 2px) to cut a ~1000-point pixel boundary
   down to a few dozen usable vertices.
6. **Always render the result back over risk0.png and inspect it** before
   trusting it — a leaked/bridged mask is usually obvious once overlaid
   (the outline visibly departs from the coastline and runs suspiciously
   straight toward a neighboring territory or the crop edge). Widen or
   narrow the bounding box and re-punch hubs as needed; for a small
   remaining bad segment, it's faster to manually patch just that stretch
   of points than to fight the segmentation further.
7. Normalize the traced pixel coordinates (divide by `MAP_SOURCE_WIDTH`/
   `MAP_SOURCE_HEIGHT`) before pasting into `territoryShapes.ts`.

**A different failure mode**: sometimes a territory comes back *fragmented*
instead of leaked — one real landmass splits into several disconnected
components because a thin neck of land (or a hub-connector line crossing
right through it) is narrow enough that the ink on both sides of the neck
touches, leaving zero actual land pixels to hold it together. This looks
like a genuinely separate island when rendered (a real bug both Kamchatka's
peninsula and Japan hit), but check first — a territory this ragged is more
often one shape with a bit poorly connected than a real archipelago. Fix by
bridging rather than re-tracing: get each fragment's mask (seed + nearest-
label snap), then `binary_dilation` each by a few px, `|` them together,
check they've merged into one connected piece (`ndimage.label`, expect
count 1), then `binary_erosion` by the same amount to restore the original
silhouette before tracing the boundary. Start the bridge radius small (2–3px)
and increase only as needed — too large will swallow real detail elsewhere
in the shape.
