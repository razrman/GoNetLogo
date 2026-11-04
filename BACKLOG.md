# Backlog

Ideas parked for later. Not scheduled for work yet.

## HubNet

- HubNet Not Implemented.

## Web UI sessions

- **Session limit and cleanup.** The backend session store (`backend/session/session.go`) has no maximum and never removes sessions until the server exits; every page load or new tab creates a session whose loop keeps running. Add a configurable maximum and drop sessions that have had no connected client for a while.
- **Session picker on startup.** When the web UI opens, show a popup listing the running sessions and ask whether to reconnect to one or start a new one. On reconnect, redraw the view with the model's current state.
- **Ask on close.** When the user closes the page, ask whether to end the session or keep it running in the background.
