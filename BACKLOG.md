# Backlog

Ideas parked for later. Not scheduled for work yet.

## HubNet

- HubNet Not Implemented.

## Web UI sessions

- **Simple login.** A simple login page so a user signs in to GoNetLogo with a login id. Keep it deliberately simple for now (for example just a username, or a basic local user list). Real authentication is complex and out of scope until later. Each session records the login id of the user who started it, and users only see sessions they started.
- **Session limit and cleanup.** The backend session store (`backend/session/session.go`) has no maximum and never removes sessions until the server exits; every page load or new tab creates a session whose loop keeps running. Add a configurable maximum and drop sessions that have had no connected client for a while.
- **Session picker on startup.** The user logs in, sees their own running sessions, then selects one to reconnect or starts a new one. On reconnect, redraw the view with the model's current state.
- **Ask on close.** When the user closes the page, ask whether to end the session or keep it running in the background.
