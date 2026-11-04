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

1. **Plots and drawing.** Time-series plots, pen mode at a finer resolution than one sample per patch, and mouse reporters (`mouse-down?`, `mouse-xcor`) so models can draw on the view. Plots (#12) and pixel-resolution pen strokes are in; the drawing layer keeps at most 8192 marks (#16). Still missing: the mouse reporters (`mouse-down?`, `mouse-xcor`).
2. **NetLogo's RNG.** The Mersenne Twister NetLogo uses, so a seed matches an upstream run. Needed before any experiment tool is meaningful.
3. **The long tail of primitives.** `carefully`, `error`, `uphill`, tie and untie, `layout-spring`, `follow`, `watch`, `inspect`, output-widget commands, and file primitives (`file-open`, `file-read`). Output-widget commands are in (#9): `output-print`, `output-show`, `output-type`, `output-write`, `clear-output`. Still missing: `carefully`, `error`, `uphill`, tie and untie, `layout-spring`, `follow`, `watch`, `inspect`, and the file primitives (`file-open`, `file-read`).
4. **BehaviorSpace.** Parameter sweeps over the existing sliders, writing a table. Depends on the RNG if results are meant to match NetLogo.
5. **Extensions.** Honor the `extensions` declaration and load a small Go extension API. `nw`, `gis`, `table`, `array`, `matrix`, and `bitmap` are the ones models hit first.
6. **The rest of the interface.** A real Info tab, an output widget separate from the command center, a plot widget, the command-center history, and an agent inspector. An output widget (#9), a plot widget (#12), and a World tab on the web UI and the desktop window (#20) are in. The tabs are Interface, World, and Code. Still missing: a real Info tab, command-center history, and an agent inspector.
7. **System Dynamics.** The stock-and-flow diagram and its generated code.
8. **HubNet.** Participatory simulations. A large protocol; not required for single-player models.
9. **3D.** `turtles3d`, `pzcor`, and a second view. The world type would grow a Z axis; the renderer would need a new backend.
10. **LevelSpace, the controlling API, and the profiler.** Embedding one model in another, driving a model from outside the process, and primitive-level timing.

Also not in this milestone: NetLogo 3D worlds, the shape editor, bitmap import, and the full widget geometry of the Interface tab (widgets here are a column, not a free-form canvas).
