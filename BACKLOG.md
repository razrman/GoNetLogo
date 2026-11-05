# Backlog

Ideas parked for later. Not scheduled for work yet.

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
2. **NetLogo's RNG.** The Mersenne Twister NetLogo uses, so a seed matches an upstream run. Today the engine uses Go `math/rand`. Needed before any experiment tool is meaningful.
3. **The long tail of primitives.** ~~`carefully`, `error`, `uphill`, tie and untie, `layout-spring`, `follow`, `watch`~~ (done, #31), ~~output-widget commands~~ (done, #9: `output-print`, `output-show`, `output-type`, `output-write`, `clear-output`), and ~~file primitives (`file-open`, `file-read`)~~ (done, #31). Still missing: `inspect` and `stop-inspecting` are accepted but do nothing. Known bug: in a chain of fixed ties, grandchildren do not orbit with the root.
4. **BehaviorSpace.** Parameter sweeps over the existing sliders, writing a table. Depends on the RNG if results are meant to match NetLogo. Only the `behaviorspace-run-number` reporter exists today.
5. **Extensions.** Honor the `extensions` declaration and load a small Go extension API. The declaration is parsed but nothing is loaded. Census on main (7e42f44): 74 of the 83 models that fail to compile declare an extension. Most used by failing models: `sr` 15 (15 use only `sr`), `resource` 12, `sound` 9 (8 sole), `import-a` 8, `palette` 7 (7 sole), `ls` 7 (6 sole), `time` 5, `csv` 4, `gis` 4.
6. **The rest of the interface.** ~~A real Info tab~~ (done, #35), ~~an output widget separate from the command center~~ (done, #9), ~~a plot widget~~ (done, #12), the command-center history, and an agent inspector. A World tab is in on the web UI and the desktop window (#20). The tabs are now Interface, World, Info, and Code. Still missing: command-center history and an agent inspector.
7. **System Dynamics.** The stock-and-flow diagram and its generated code.
8. **HubNet.** Participatory simulations. A large protocol; not required for single-player models.
9. ~~**3D.** `turtles3d`, `pzcor`, and a second view. The world type would grow a Z axis; the renderer would need a new backend.~~ (done, #32: Z axis, 3D primitives, .nlogo3d/.nlogox3d loading, three.js orbit view in the web UI; `pzcor` on the World page, #33). Still missing in 3D: `follow`, `ride`, and `watch` do not move the 3D camera; patches are drawn as opaque cubes; no turtle shapes, labels, or pen trails.
10. **LevelSpace, the controlling API, and the profiler.** Embedding one model in another, driving a model from outside the process, and primitive-level timing.

Also not in this milestone: ~~NetLogo 3D worlds~~ (done, #32), the shape editor, bitmap import, and the full widget geometry of the Interface tab (widgets here are a column, not a free-form canvas).

Other known bugs: "Wolf Sheep Predation - Micro-Sims Cognitive Model" fails in `go` with "line 1: expected a reporter".

## Suggested next

1. NetLogo's RNG (Mersenne Twister). Enables seed-for-seed comparison with real NetLogo and makes BehaviorSpace results meaningful.
2. Extensions, starting with the top ones from the census (`sr`, `sound`, `palette`, `ls`).
3. One batched small-fixes PR: mouse reporters, a basic inspector, 3D camera for `follow`/`ride`/`watch`, the Micro-Sims error, and gofmt of `pkg/nl/plot_test.go`.
4. Session limit (sessions never end; each tab leaks a running session), before the login and admin work.
