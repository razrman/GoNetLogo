# Sign-in and admin

Pass 1 is the web UI and the process around it. Accounts live in a local file. Cognito, GitHub, and the infrastructure that would host them are pass 2 and are not in this tree.

`provider: none` is the default. The site behaves as it did before accounts existed: anyone can open and run a model, and the sign-in links stay hidden. Existing deploys and tests keep working when they do not load a config file.

## Config file

The server reads `config/auth.yaml`, or the file named by `GONETLOGO_AUTH_CONFIG`. A set variable that points at a missing or invalid file stops the process. An unset variable and a missing `config/auth.yaml` means anonymous access.

```yaml
provider: local          # none | local
require_login: false     # true rejects anonymous API calls
allow_anonymous: false   # local default; true auto-starts a model with no account
session_lifetime: 168h
cookie:
  name: gonetlogo_auth
  secure: false          # true on HTTPS. http://localhost still stores the cookie
  same_site: lax         # lax | strict | none
  path: /
users_file: data/users.json
default_max_sessions: 4  # open model sessions per account; 1 to 50
initial_admin_emails:
  - you@example.com
providers:
  local:
    min_password_length: 8
```

`provider` must be a registered plugin. `require_login: true` with `provider: none` is rejected. `session_lifetime` is a Go duration and must be positive. `cookie.path` starts with `/`. Admin emails must contain `@` and no spaces. The local password minimum is 8 to 200. `default_max_sessions` is how many open model sessions a new account may have. A missing value is 4. An admin can change it per user. A session that is ending does not count. At the limit, starting another session asks the person to end one first.

`provider: none` ignores accounts: the fire model auto-starts and the sign-in links stay hidden. With any other provider, `allow_anonymous` defaults to false. The page shows sign-in and sign-up and does not start a model until someone signs in. Set `allow_anonymous: true` to keep the anonymous auto-start. `require_login: true` is separate: it rejects anonymous calls to the model API even if the UI would otherwise allow them.

A model opened before sign-in is attached to the account at sign-in, when it has no other owner. If that session belongs to someone else, or it is already ending, sign-in starts a new one. A model started after sign-in is owned immediately. While someone is signed in they have at least one open model session. Ending that last session signs them out. Account shows only that person's sessions, as "Open sessions: N" and "Limit: M". Those numbers are not a total of every session on the server. `cookie.secure: true` marks the cookie Secure only on HTTPS, so `http://localhost` still keeps the sign-in cookie.

`providers` is a block per plugin. The local block is the only one the server reads today. The commented `cognito`, `github`, and `oidc` blocks in `config/auth.yaml` document the issuer, client id, client secret, and redirect URL those plugins will want. Selecting one of those names fails startup with a clear error, because the plugins are not registered yet.

The Docker image sets `GONETLOGO_AUTH_CONFIG=/config/auth.yaml`. That file ships as `provider: none`.

## Local accounts are ephemeral

Users are a JSON file at `users_file`: internal id, provider, provider subject, email, display name, bcrypt password hash, role (`user` or `admin`), disabled flag, created time, and last login. Writes go to a temporary file and then rename, and a mutex covers the read-modify-write.

The file is not a database. Deleting it drops every account. In the container the working directory is `/tmp`, so `data/users.json` is `/tmp/data/users.json` and disappears when the container is replaced. Login sessions are only in memory, so a restart also signs everyone out.

The file store implements `UserStore`. A later database replaces that implementation. The provider does not own the role; role, disabled, and the internal id live in `UserStore`.

## First admin

Put the email in `initial_admin_emails` and start the server. That person signs up with that email and the usual password form. The new account is an admin. The config file does not set a password.

On later startups, any existing user with one of those emails is promoted to admin.

The last enabled admin cannot be demoted or deleted. An admin cannot disable or delete their own account.

## Try it locally

From the repo root, edit `config/auth.yaml`: set `provider: local` and add your email under `initial_admin_emails`. Then:

```bash
./scripts/dev.sh
```

Open http://127.0.0.1:43132, choose Sign up, and use that email. The top bar shows the display name, Account, Admin, and Log out. Account lists this browser and any other signed-in browsers, and the model workspaces opened while signed in. A model that was already open in that browser is attached to the account at sign-in, when it does not already belong to someone else. Admin searches users and can promote, demote, disable, delete, and revoke a user's login sessions.

Set `require_login: true` to keep anonymous visitors on the sign-in page. With it false, they can still open and run models.

`./scripts/dev.sh` proxies `/auth` and `/admin` to the API. The session cookie is HttpOnly. State-changing auth and admin calls send `X-CSRF-Token`, copied from the `gonetlogo_csrf` cookie that `GET /auth/csrf` sets.

To go back to the anonymous site, set `provider: none` and restart.

## Adding a provider plugin

A provider is anything that implements `AuthProvider`:

- `Signup` and `Login` return an `Identity` (provider name, provider subject, email, display name). The local provider also fills the internal user id and the role from `UserStore`.
- `Logout` drops the login session.
- `Verify` turns a session id back into that `Identity`.

Register it from `init` under the config name:

```go
func init() {
    auth.Register("example", func(cfg auth.Config, sessions *auth.SessionManager) (auth.AuthProvider, error) {
        block := cfg.RawProviders["example"]
        return newExample(block, sessions)
    })
}
```

Import that package from `backend/cmd/server` so the registration runs. Put a `example:` block under `providers` in the YAML. HTTP middleware, the cookie, CSRF, and the admin page stay on the `AuthProvider` and `UserStore` interfaces. A redirect provider can return `ErrDisabled` from `Signup` and `Login` until its callback exists.

Roles are not claims from the provider. After the provider identifies the person, this server looks up the user row and uses the role stored there.
