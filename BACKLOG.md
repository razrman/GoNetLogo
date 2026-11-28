# Backlog

Ideas parked for later. Not scheduled for work yet.

## Priorities (Nov 17, 2026)

In order:

1. ~~**Widget text edit tab.** A tab next to Code that shows the model's interface section (buttons, sliders, switches, choosers, monitors, plots) as editable text. Applying an edit rebuilds the side panel, and the change is saved with the model.~~ Done in #65 and #66: the Widgets tab edits every widget as one YAML list, with an optional `description` per widget. Plot pens are kept in the model file and are not editable there.
2. **Import/export.** NetLogo's import and export items (scope to be decided), and possibly the file primitives (`file-open`, `file-read`, and the rest) working for real.
3. **User signup, authorization, and admin page.** Real accounts with signup, replacing the "simple login" idea below. Covers the User & Session Management section: user types, session caps, session picker, ask on close, and the `/admin` page. Pass 1 (web UI, local ephemeral user file, `provider: none` or `local`) is complete; pass 2 (Cognito and GitHub providers, plus the infra to host them) is still pending. This item is not done.
4. **A subset of impactful extensions.** Honor the `extensions` declaration with a small Go extension API. Candidates: `sound` (plays in the browser), `table`, `array`, then `nw`, `gis`, `matrix`, `bitmap`.
5. **Interface mouse actions.** Wire the view's mouse into the session so `mouse-down?`, `mouse-xcor`, and `mouse-ycor` work in the web UI; let the user draw turtles and patches with the mouse; and show a turtle's or patch's properties (an agent inspector, so `inspect` does something).

## HubNet

- HubNet Not Implemented. See Roadmap item 8.

## User & Session Management

- **Simple login.** (Superseded by priority 3: real signup and authorization.) A simple login page so a user signs in to GoNetLogo with a login id. Keep it deliberately simple for now (for example just a username, or a basic local user list). Real authentication is complex and out of scope until later. Each session records the login id of the user who started it and the loaded .nlogo model name. In the session picker, users only see sessions they started.
- **User types.** user, power-user, and admin. A user can have only one session; a power-user can have up to 10 sessions; an admin can access the `/admin` screen.
- **Default admin.** A default user named `admin` with admin capability.
- **Session limit and cleanup.** The backend session store (`backend/session/session.go`) has no maximum and never removes sessions until the server exits; every page load or new tab creates a session whose loop keeps running. Done so far: a server-wide session cap, and a closed tab ends its session after a short grace period. Still to do: cap open sessions by user type (one for a user, 10 for a power-user) and drop sessions that have had no connected client for a while.
- **Session picker on startup.** The user logs in, sees their own running sessions, then selects one to reconnect or starts a new one when their type's cap allows it. Each running session is shown with its login id and loaded .nlogo model name so the user can tell sessions apart. On reconnect, redraw the view with the model's current state.
- **Ask on close.** When the user closes the page, ask whether to end the session or keep it running in the background.
- **Admin page.** An admin page at `/admin` lists users with their sessions. It shows each user's type, which sessions are connected, and how long each session has been running, and has a button to quit a session that is not connected.
- **Change user types.** Admins can change user types.

## Roadmap

This is a first milestone of the language, plus a GraphQL client. The rest of NetLogo is large; this is the order to take it:

1. **Plots and drawing.** (Mostly done.) Time-series plots, pen mode at a finer resolution than one sample per patch, and mouse reporters (`mouse-down?`, `mouse-xcor`) so models can draw on the view. Plots (#12) and pixel-resolution pen strokes are in; the drawing layer keeps at most 8192 marks (#16). The mouse reporters are in the language, but the web view does not send mouse input yet (see priority 5).
2. **NetLogo's RNG.** (Done: Mersenne Twister and seeding are in.) The Mersenne Twister NetLogo uses, so a seed matches an upstream run. Needed before any experiment tool is meaningful.
3. **The long tail of primitives.** `carefully`, `error`, `uphill`, tie and untie, `layout-spring`, `follow`, `watch`, `inspect`, output-widget commands, and file primitives (`file-open`, `file-read`). Output-widget commands are in (#9): `output-print`, `output-show`, `output-type`, `output-write`, `clear-output`. Most of these are now recognized by the compiler. Still to verify or finish: `inspect` (no inspector yet, see priority 5) and real file I/O for the file primitives (see priority 2).
4. **BehaviorSpace.** Parameter sweeps over the existing sliders, writing a table. Depends on the RNG if results are meant to match NetLogo.
5. **Extensions.** (Priority 4.) Honor the `extensions` declaration and load a small Go extension API. `nw`, `gis`, `table`, `array`, `matrix`, and `bitmap` are the ones models hit first.
6. **The rest of the interface.** A real Info tab, an output widget separate from the command center, a plot widget, the command-center history, and an agent inspector. An output widget (#9), a plot widget (#12), and a World tab on the web UI (#20) are in. The tabs are Interface, World, Info, and Code. An Info tab is in. Still missing: command-center history and an agent inspector (priority 5).
7. **System Dynamics.** (Done: stock-and-flow models load and run.) The stock-and-flow diagram and its generated code.
8. **HubNet.** Participatory simulations. A large protocol; not required for single-player models.
9. **3D.** (Done: NetLogo 3D models and a 3D view are in.) `turtles3d`, `pzcor`, and a second view. The world type would grow a Z axis; the renderer would need a new backend.
10. **LevelSpace, the controlling API, and the profiler.** Embedding one model in another, driving a model from outside the process, and primitive-level timing.

Also not in this milestone: the shape editor and bitmap import. The Interface tab places the view and the widgets on one canvas at the model's positions when that geometry is usable, and scales the canvas with the pane. Below 860px, or when the positions are unusable, they stay in a column. The Layout control (Auto, NetLogo, Column) overrides the automatic choice.
