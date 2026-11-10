# Backlog

Ideas parked for later. Not scheduled for work yet.

Web UI is the focus for new UI work. The Fyne desktop app is no longer maintained and will be removed later (not scheduled). A possible future desktop wrapper via Wails or Electron is undecided.

## HubNet

- HubNet Not Implemented. See Roadmap item 8.

## User & Session Management

- **Simple login.** A simple login page so a user signs in to GoNetLogo with a login id. Keep it deliberately simple for now (for example just a username, or a basic local user list). Real authentication is complex and out of scope until later. Each session records the login id of the user who started it and the loaded .nlogo model name. In the session picker, users only see sessions they started.
- **User types.** user, power-user, and admin. A user can have only one session; a power-user can have up to 10 sessions; an admin can access the `/admin` screen.
- **Default admin.** A default user named `admin` with admin capability.
- **Session limit.** The backend session store (`backend/session/session.go`) has no maximum and never removes sessions until the server exits; every page load or new tab creates a session whose loop keeps running. Cap open sessions by user type (one for a user, 10 for a power-user). Sessions can be long lived.
- **Session picker on startup.** The user logs in, sees their own running sessions, then selects one to reconnect or starts a new one when their type's cap allows it. Each running session is shown with its login id and loaded .nlogo model name so the user can tell sessions apart. On reconnect, redraw the view with the model's current state.
- **Ask on close.** When the user closes the page, ask whether to end the session or keep it running in the background.
- **Admin page.** An admin page at `/admin` lists users with their sessions. It shows each user's type, which sessions are connected, and how long each session has been running, and has a button to quit a session that is not connected.
- **Change user types.** Admins can change user types.

## Roadmap

This is a first milestone of the language, plus a GraphQL client. The rest of NetLogo is large; this is the order to take it:

1. ~~**Plots and drawing.** Time-series plots, pen mode at a finer resolution than one sample per patch~~ (done: plots #12, pixel-resolution pen strokes; the drawing layer keeps at most 8192 marks, #16). Still missing: mouse reporters. `mouse-down?` and `mouse-inside?` always return false and `mouse-xcor`/`mouse-ycor` always return 0 (stubs), so models cannot draw on the view yet.
2. ~~**NetLogo's RNG.** The Mersenne Twister NetLogo uses, so a seed matches an upstream run.~~ (done, #39: `pkg/nl/mersenne.go` port of NetLogo's `MersenneTwisterFast`; World tab RNG Seed box on the web UI, #41). ~~Unsigned seed 0..4294967295, a Seed override checkbox on the World tab, and greyed-out seed controls when override is off~~ (done, #43). Needed before any experiment tool is meaningful — BehaviorSpace still waits on that tool, not on the RNG itself.
3. **The long tail of primitives.** ~~`carefully`, `error`, `uphill`, tie and untie, `layout-spring`, `follow`, `watch`~~ (done, #31), ~~output-widget commands~~ (done, #9: `output-print`, `output-show`, `output-type`, `output-write`, `clear-output`), and ~~file primitives (`file-open`, `file-read`)~~ (done, #31). ~~`towardsxyz` and 3D `layout-spring` / `layout-sphere`~~ (done, #45). Still missing: `inspect` and `stop-inspecting` are accepted but do nothing. Known bug: in a chain of fixed ties, grandchildren do not orbit with the root. Setup failures still hit missing `link-length` (2D; the 3D reporter exists), `out-active-link-neighbors`, `out-cursor-drag-neighbors`, `urn-with`, and `plot-x-min`.
4. **BehaviorSpace.** Parameter sweeps over the existing sliders, writing a table. Depends on the RNG if results are meant to match NetLogo. Only the `behaviorspace-run-number` reporter exists today. RNG seed-for-seed parity is in (#39).
5. **Extensions.** Honor the `extensions` declaration and load a small Go extension API. ~~`sr` and `palette`~~ (done, #40; also fixed `approximate-rgb` rmean). The declaration is parsed; installed extensions are compiled into `pkg/nl` (switch in `extensions.go`, lexer `ext:prim` tokens, VM dispatch by prefix). No `plugin.Open`. Latest modelcheck on main (616 models): compile 561 ok / 55 fail, setup 510 ok, go 485 ok (incl. stopped) with 82 absent. Of the 55 compile failures, 52 are unimplemented extensions: `resource` 12, `sound` 9, `import-a` 8, `ls` 7, `time` 5, `csv` 4, `gis` 4, `bitmap` 3, `vid` 3, plus `gogo`, `nw`, `py`, `rnd`, `table`, `view2.5d`, `arduino`, `fetch`, `matrix`, `profiler`. The other 3 are Image Import Example (`import-drawing`) and two HubNet models. Agreed direction: language-shaped extensions (`csv`, `table`, `sr`, `matrix`, `time`, `rnd`, `palette`) stay in-process Go via a compiled-in registry plus a small host API; OS-bound extras may use an optional platform-specific RPC daemon; `sound` as engine events played by the web client via Web Audio.
6. **The rest of the interface.** ~~A real Info tab~~ (done, #35), ~~an output widget separate from the command center~~ (done, #9), ~~a plot widget~~ (done, #12), the command-center history, and an agent inspector. A World tab is in on the web UI and the desktop window (#20). ~~Model name in the Interface header~~ (done, #34). ~~RNG Seed on the World tab; Open Model remembers the last selection and the file list is resizable; ordinary widget edits (slider/switch/chooser/input) while go runs; Setup button disabled when the model has no `setup`~~ (done, #41). ~~Remaining "model is running" errors on once-buttons / seed / setup during go~~ (done, #42). ~~Unsigned seed 0..4294967295, Seed override checkbox on the World tab, and greyed-out seed controls when override is off~~ (done, #43). ~~Dynamics tab (code-only, shown only for SDM models); dt on the World tab for SDM only; uncapped speed 100; plot decimation past 8192 points; World heading removed~~ (done, #44). ~~FIFO once-button queue during go~~ (done, #45; the queue has no size cap). The tabs are Interface, World, Info, and Code, plus Dynamics when an SDM model is loaded. Still missing: command-center history and an agent inspector. During go, the command center, a second forever button, world resize, load model, and code edit stay locked (by design, #42).
7. ~~**System Dynamics.** The stock-and-flow diagram and its generated code.~~ (done, #38: runtime compiles the `<systemDynamics>` diagram into globals/procedures with Euler integration). ~~Dynamics tab is code-only and shown only for SDM models; dt lives on the World tab for SDM only~~ (done, #44). No visual diagram editor; library models that ship a diagram run from the file.
8. **HubNet.** Participatory simulations. A large protocol; not required for single-player models.
9. ~~**3D.** `turtles3d`, `pzcor`, and a second view.~~ (done, #32: Z axis, 3D primitives, .nlogo3d/.nlogox3d loading, three.js orbit view in the web UI; `pzcor` on the World page, #33; wrapped links in 2D and 3D, #36). ~~3D pen trails (`pen-down` / `pen-up` / `pen-erase`, `clear-drawing` / `clear-all`); turtles default pen-up; 3D camera for `follow` / `ride` / `watch` / `reset-perspective`~~ (done, #42). There is no `clear-trace` primitive — NetLogo's name is `clear-drawing`. ~~3D turtle shapes (circle/sphere, square/cube, default dart; vector shapes extruded or billboarded, instanced); translucent patches with RGBA alpha; billboarded turtle, patch, and link labels; 2D view `follow` / `ride` / `watch`; watch-of-patch moves the 3D camera; 3D stamps and `stamp-erase`; FIFO once-button queue; `towardsxyz`; 3D `layout-spring` / `layout-sphere`; modelcheck runs a forever button when there is no `go` (3D go 31→34, overall go ok 452→485)~~ (done, #45). ~~2D patch and drawing layers vertically flipped after #45 (Fire burn scars did not line up with the flames)~~ (fixed, #46). Remaining 3D gaps: the car shape is a stand-in, not NetLogo's mesh; opaque patch colors are drawn at 45% opacity; labels are not occluded by geometry; the once-button queue has no size cap. Neighborhoods Example 3D, Uniform Distribution on a Sphere Example 3D, and Maze 3D have no go loop (once-buttons only).
10. **LevelSpace, the controlling API, and the profiler.** Embedding one model in another, driving a model from outside the process, and primitive-level timing. `ls` and `profiler` are still among the unimplemented extensions above.

Also not in this milestone: ~~NetLogo 3D worlds~~ (done, #32), the shape editor, bitmap import (see Import / Export below), and the full widget geometry of the Interface tab (widgets here are a column, not a free-form canvas).

## Import / Export

Core NetLogo primitives still missing or stubbed (verified on main after #42):

- `export-world` / `import-world` — accepted, return "not supported yet".
- `import-pcolors` / `import-pcolors-rgb` — accepted, return "not supported yet".
- `export-interface` — accepted, no-op (same stub path as `inspect`).
- `import-drawing` — not recognized (Image Import Example fails to compile).
- `export-view`, `export-output`, `export-plot`, `export-all-plots` — not recognized.

Extension import/export (see census above): `import-a`, `bitmap`, `vid`, `csv`, `table`, `gis` (and `resource` for bundled assets).

Agreed design for the web UI: image / video / sound import and export go through the browser file picker, drag-and-drop, and download, with per-session backend storage (size limits, cleanup, no local disk paths, never execute uploads). Sound is engine events played by the web client via Web Audio. Language extensions stay in Go as above.

Suggested first batch when resumed: `export-view`, `export-interface`, `export-plot`, `export-output` as downloads; then `import-drawing` / `import-pcolors` as uploads; then `export-world` / `import-world`.

## Leftovers / known bugs

- gofmt still dirty on `pkg/nl/palette_schemes.go` and `pkg/nl/plot_test.go`.
- "Wolf Sheep Predation - Micro-Sims Cognitive Model" fails in `go` with "line 1: expected a reporter".
- Mouse reporters stubbed (`mouse-down?` / `mouse-inside?` → false; `mouse-xcor` / `mouse-ycor` → 0).
- `inspect` / `stop-inspecting` / `stop-inspecting-dead-agents` are no-ops.
- Fixed-ties grandchildren do not orbit with the root.
- Session limit (see User & Session Management).
- Setup failures still include missing `link-length`, `out-active-link-neighbors`, `out-cursor-drag-neighbors`, `urn-with`, `plot-x-min`.
- Lenia, Cyclic CA, and GenEvo 1 setup timeouts at the 4s modelcheck limit.
- Nine models skipped for worlds over 256 patches per side (`world_skipped`).
- Remaining 3D gaps after #45/#46: the car shape is a stand-in; opaque patch colors are drawn at 45% opacity; labels are not occluded by geometry; the once-button queue has no cap.
- Neighborhoods Example 3D, Uniform Distribution on a Sphere Example 3D, and Maze 3D have no go loop (once-buttons only).

## Suggested next

1. Import / export batch for the web UI (`export-view` / `export-interface` / `export-plot` / `export-output`, then `import-drawing` / `import-pcolors`, then world import/export), using browser pickers and per-session storage as above.
2. More extensions from the census (`sound` via Web Audio, `csv` / `table` / `time` / `rnd` / `matrix` in Go; later `ls`, `import-a`, `gis`, `bitmap`, `vid`).
3. One batched small-fixes PR: mouse reporters, a basic inspector, `link-length` in 2D, the Micro-Sims error, and gofmt of `pkg/nl/palette_schemes.go` and `pkg/nl/plot_test.go`.
4. Session limit (sessions never end; each tab leaks a running session), before the login and admin work.
5. BehaviorSpace, now that the RNG matches NetLogo.
6. Remaining 3D polish: replace the stand-in car shape, don't force 45% opacity on opaque patches, occlude labels behind geometry, and cap the once-button queue.
