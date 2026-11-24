# Backend Flows

How the backend behaves for the three flows the frontend has to integrate with:
**token refresh**, **account lockout**, and **API keys**.

This document is about observable behaviour — what the client sends, what comes back,
and what the user should see. Implementation details live in the referenced files.

| Topic | Source of truth |
| --- | --- |
| Auth service | `internal/auth/service.go` |
| Lockout policy | `internal/auth/lockout.go` |
| Refresh rotation | `internal/auth/refresh_repo.go` |
| API keys | `internal/apikey/service.go`, `internal/apikey/policy.go` |
| HTTP contract | `contracts/openapi.yaml` |

All defaults below are read from `.env` at startup. Every value is fail-fast: an
unparseable or non-positive setting stops the server instead of silently falling back.

---

## 0. Two credential types — never mix them

| Caller | Credential | Header | How it is verified | Revocable |
| --- | --- | --- | --- | --- |
| Web app user | HS256 JWT | `Authorization: Bearer <token>` | Signature + issuer + expiry, stateless | No — it expires |
| External plugin | API key `mk_…` | `X-API-Key: mk_…` | SHA-256 lookup in `api_keys` | Yes — immediately |

Rules that follow from this:

- A route accepts **exactly one** of the two. There is no route that accepts both.
- `Authorization` is **never** used for API keys, and `X-API-Key` is **never** used
  by the web app.
- API-key management (`/api/v1/api-keys`) is itself protected by the **JWT** gate.
  An API key cannot create, list, or revoke keys.
- Neither gate re-checks `users.is_active`. A deactivated account keeps working
  until its access token expires, or until its API key is revoked/expires.

---

## 1. Token Refresh Flow

### 1.1 Lifetimes

| Credential | Default | Configured by | Notes |
| --- | --- | --- | --- |
| Access token (JWT) | 24 hours | `JWT_EXPIRY_HOURS` | Fixed at issue; not extended by use |
| Refresh token | 168 hours (7 days) | `REFRESH_TOKEN_TTL_HOURS` | Restarts on every rotation |

The access token is an HS256 JWT. Its claims are `sub` (the user ID), `username`,
`role`, `iss: "music-backend"`, `iat`, `exp`, and a random `jti`. The `jti` only
guarantees that two tokens minted in the same second differ — it is **not** a
revocation handle, and the backend keeps no list of valid tokens.

The refresh token is 32 random bytes encoded as 43 base64url characters. It has
**no prefix** and no structure to parse; treat it as an opaque string.

### 1.2 What happens when the JWT expires

The middleware rejects the request with:

```http
HTTP/1.1 401 Unauthorized
Content-Type: application/json

{"error":{"code":"unauthorized","message":"authentication required"}}
```

Two things worth knowing:

- There is **no** `token_expired` code. Expiry, a bad signature, a missing header
  and a malformed token all produce the same `401 unauthorized`. The client's only
  correct reaction is "try to refresh once, then log out".
- The request never reaches the handler, so no partial work is done.

### 1.3 The refresh request

```http
POST /api/v1/auth/refresh
Content-Type: application/json

{"refresh_token":"<the stored refresh token>"}
```

- **No `Authorization` header is needed** — and sending the expired access token
  has no effect, because this route is outside the JWT gate. The refresh token is
  the only credential.
- On success: `200 OK`

```json
{
  "token": "<new access token>",
  "expires_at": "2026-11-26T04:02:23Z",
  "refresh_token": "<new refresh token>",
  "refresh_token_expires_at": "2026-12-03T04:02:23Z"
}
```

- The response deliberately has **no `user` object**. Refreshing rotates
  credentials; it does not re-serialize the account. Keep the user you already
  have, or call a profile endpoint if one exists later.

### 1.4 How rotation works

Every refresh spends the token it was given and issues a replacement:

1. The server hashes the presented token (SHA-256) and looks up the row. Only the
   hash is ever stored — the raw value exists only in the client's hands.
2. No matching row → `401 invalid_refresh_token`.
3. Row found but past `expires_at` → the row is deleted and the client gets
   `401 invalid_refresh_token`. Expired sessions are cleaned up opportunistically.
4. The owning user is re-read. If the account was deleted or deactivated since the
   token was issued → `401 invalid_refresh_token`.
5. A new access token is minted from the user's **current** role/username.
6. A brand-new refresh token is generated and, in a single database transaction,
   the old row is `DELETE`d and the new one `INSERT`ed.

Consequences you must design around:

- **Refresh tokens are single-use.** Replaying one always fails with
  `401 invalid_refresh_token`.
- **Concurrent refreshes with the same token are a race with one winner.** The
  `DELETE` only matches a row that still exists and is unexpired, so exactly one
  caller rotates it. The loser sees `401 invalid_refresh_token` — which looks
  exactly like a real expiry.
- **The 7-day clock restarts on each rotation.** An actively used session never
  hits the limit; a session idle for 7 days does.
- **Each login is its own session.** Logging in on a second device does not
  invalidate the first device's refresh token.

### 1.5 Client rules

1. Store both tokens. Overwrite the refresh token with the one from the **latest**
   successful response — login *and* refresh. Keeping the old copy guarantees a
   logout on the next attempt.
2. Refresh **once**, no matter how many requests got a 401. Use a single shared
   promise/mutex as the lock (snippet below).
3. Pause the failing requests, refresh, then replay them with the new access token.
   Retry each request **once**; a second 401 means the session is over.
4. If `/auth/refresh` itself returns `401 invalid_refresh_token`, the session is
   gone: clear stored tokens, cancel pending work, and send the user to login. Do
   not loop, and do not retry the refresh call.
5. Never refresh pre-emptively on a timer unless you also keep the single-flight
   lock — a timer plus a 401-triggered refresh is the classic double-spend bug.
6. A `400 invalid_input` from `/refresh` means `refresh_token` was empty or
   whitespace. That is a client bug, not an expired session.

### 1.6 Single-flight refresh with Axios

The interceptor holds one shared promise. The first 401 starts the refresh; every
other 401 awaits the same promise, then replays its own request.

```js
import axios from 'axios';

const BASE_URL = 'http://localhost:8080/api/v1';

// A bare client for the refresh call: no interceptors, so a 401 here cannot
// recurse back into the retry logic.
const raw = axios.create({ baseURL: BASE_URL });

// The client every feature uses.
const api = axios.create({ baseURL: BASE_URL });

const tokens = {
  access: () => localStorage.getItem('access_token'),
  refresh: () => localStorage.getItem('refresh_token'),
  save: ({ token, refresh_token }) => {
    localStorage.setItem('access_token', token);
    localStorage.setItem('refresh_token', refresh_token);
  },
  clear: () => {
    localStorage.removeItem('access_token');
    localStorage.removeItem('refresh_token');
  },
};

api.interceptors.request.use((config) => {
  const access = tokens.access();
  if (access) config.headers.Authorization = `Bearer ${access}`;
  return config;
});

// The lock. While a refresh is in flight, every caller shares this promise.
let refreshPromise = null;

function refreshOnce() {
  if (!refreshPromise) {
    refreshPromise = raw
      .post('/auth/refresh', { refresh_token: tokens.refresh() })
      .then(({ data }) => {
        tokens.save(data); // overwrite BOTH tokens
        return data.token;
      })
      .finally(() => {
        refreshPromise = null; // release the lock on success and on failure
      });
  }
  return refreshPromise;
}

api.interceptors.response.use(
  (response) => response,
  async (error) => {
    const original = error.config;
    const is401 = error.response?.status === 401;

    // Retry only 401s, only once, and never the refresh call itself.
    if (!is401 || !original || original._retried || original.url?.includes('/auth/refresh')) {
      return Promise.reject(error);
    }
    original._retried = true;

    try {
      const token = await refreshOnce(); // all concurrent 401s wait here
      original.headers.Authorization = `Bearer ${token}`;
      return api(original); // replay the paused request
    } catch (refreshError) {
      tokens.clear();
      window.location.assign('/login');
      return Promise.reject(refreshError);
    }
  },
);

export default api;
```

Using `fetch` instead? Keep the same shape: a module-level `let refreshPromise = null`
that stores the in-flight `fetch('/auth/refresh', …)` promise, have every 401 handler
`await` it, then re-issue the original request with the new token. The lock — not the
HTTP library — is what prevents concurrent refreshes.

### 1.7 Logout

```http
POST /api/v1/auth/logout
Content-Type: application/json

{"refresh_token":"<the stored refresh token>"}
```

- No `Authorization` header required; the refresh token identifies the session.
- Always `204 No Content` on success, including when the token was already gone or
  forged. Logout is idempotent and reveals nothing.
- It deletes **only that session**. Other devices stay logged in.
- Already-issued access tokens stay valid until they expire, because the JWT gate is
  stateless. Revoking them early is not currently possible.
- Clear local storage even if the request fails — the user asked to log out.

### 1.8 Refresh quick reference

| Situation | Status | `code` | What the client does |
| --- | --- | --- | --- |
| Access token expired | 401 | `unauthorized` | Refresh once, replay |
| Refresh succeeded | 200 | — | Save both new tokens, replay |
| Refresh token unknown/expired/rotated | 401 | `invalid_refresh_token` | Clear storage, go to login |
| Refresh token replayed or lost a race | 401 | `invalid_refresh_token` | Clear storage, go to login |
| Empty `refresh_token` | 400 | `invalid_input` | Fix the client bug |
| Account deleted/deactivated | 401 | `invalid_refresh_token` | Clear storage, go to login |

---

## 2. Account Lockout Flow

### 2.1 Policy

| Setting | Value | Where it lives |
| --- | --- | --- |
| Failed attempts before lock | **5** | `internal/auth/lockout.go` |
| Lock duration | **15 minutes** | `internal/auth/lockout.go` |
| Scope | Per **account** (username), not per IP or device | `users.failed_logins`, `users.locked_until` |

These are compile-time constants, not `.env` settings. Changing them requires a code
change and a redeploy.

### 2.2 Attempt by attempt

Assuming an existing, active account and a wrong password every time:

| Attempt | Response | What actually happened |
| --- | --- | --- |
| 1 | `401 invalid_credentials` | `failed_logins = 1` |
| 2 | `401 invalid_credentials` | `failed_logins = 2` |
| 3 | `401 invalid_credentials` | `failed_logins = 3` |
| 4 | `401 invalid_credentials` | `failed_logins = 4` |
| 5 | `401 invalid_credentials` | Threshold reached: account locks for 15 min, `failed_logins` resets to 0 |
| 6 (during the window) | **`423 account_locked`** | Rejected before the password is judged |
| 7, 8, … (during the window) | **`423 account_locked`** | Same; the window does not move |
| First attempt after the window | Normal `401` / `200` | Counter starts from zero |

**The 5th attempt still returns `401`, not `423`.** This is deliberate: announcing
"you are now locked" would confirm the threshold and help an attacker calibrate.
The lock is already active at that moment, so the *next* attempt is the first one to
see `423`.

A correct password during the window also returns `423`. The lock is checked before
the password is verified, so a locked account cannot be used to test guesses.

### 2.3 What `423` means

```http
HTTP/1.1 423 Locked
Content-Type: application/json

{"error":{"code":"account_locked","message":"account is temporarily locked, try again later"}}
```

- The account is inside its 15-minute window. Nothing is wrong with the request.
- There is **no attempt count**, **no unlock timestamp**, and **no `Retry-After`
  header**. The backend intentionally withholds all three so the response cannot be
  used to time the window.
- `423` is the one deliberate exception to "all auth failures look alike". Every
  other failure is a generic `401`; `423` does reveal that the username exists.
- Only a *login* returns `423`. Refresh and API-key failures never do.

### 2.4 When the lock resets

1. **Automatically, 15 minutes after the lock was set.** No admin action, no unlock
   endpoint. Expiry is evaluated by PostgreSQL's `now()`, not the application clock.
2. **After expiry, the counter is treated as fresh.** Waiting out a lock and then
   mistyping once does not immediately re-lock you — you get the full 5 again.
3. **A successful login clears everything:** `failed_logins = 0`,
   `locked_until = NULL`, `last_login_at` is stamped. So 4 failures + 1 success +
   4 failures is not a lock.
4. **Attempts made during an active lock do not extend it.** The window ends 15
   minutes after the lock was set, regardless of how many times the user retries.
5. **Unknown usernames never lock** — there is no row to count against. They always
   return `401 invalid_credentials`.
6. **Inactive accounts** (`is_active = false`) return `401 invalid_credentials` even
   with the correct password; their failed attempts still increment the counter,
   which is harmless and keeps timing uniform.

### 2.5 Frontend UX guidance

- On `423`, show a message like "Too many failed attempts. This account is
  temporarily locked — try again in about 15 minutes." Do **not** invent a countdown
  from a number the API never sent.
- Disable the submit button and stop automatic retries. Retrying cannot shorten the
  window and only produces more `423`s.
- Do not clear the password field on `423`; the user may want to retry elsewhere.
- Do not tell the user how many attempts remain. The backend never exposes it, and
  guessing creates false expectations.
- On `401 invalid_credentials`, use one message for every cause ("invalid username or
  password"). The API deliberately cannot distinguish unknown user, wrong password
  and inactive account.
- If you build an admin panel later: there is no unlock endpoint, so an admin's only
  lever today is waiting 15 minutes.

### 2.6 Why the API is this vague

| Cause | Response | Reason |
| --- | --- | --- |
| Unknown username | `401 invalid_credentials` | No account enumeration |
| Wrong password | `401 invalid_credentials` | Same body, same timing |
| Inactive account | `401 invalid_credentials` | Same body |
| Account locked | `423 account_locked` | Deliberate exception, worth the disclosure |

Timing is equalised on purpose: an unknown username still costs one bcrypt
comparison against a dummy hash, and the locked path still costs one bcrypt
comparison against the real hash. Response time therefore leaks neither existence
nor lock state.

---

## 3. API Key Flow

### 3.1 Key anatomy

A raw key is **46 characters**, always:

```
mk_9Xr3kQ1oZ2mW8pY4vB6nC0sD5fG7hJ1lK3nM5pR7tV
└┬┘└──────────────────┬───────────────────────────┘
 │                  43 base64url characters
 │                  (32 random bytes, 256 bits)
 └── "mk_" scheme
```

- The 46-character length and the `mk_` prefix are **hardcoded** in
  `internal/apikey/policy.go` (`apiKeyRawLen`, `apiKeyScheme`). They are part of the
  contract, not a configuration knob.
- `key_prefix` is the first **12** characters (`mk_` + 9 random characters). It is
  safe to display and is all that listings ever show.
- The stored `key_hash` is the SHA-256 hex digest (64 characters). The raw key is
  never stored and never logged.
- The key contains no user ID, no scope, and no expiry. Everything is looked up from
  the hash.

### 3.2 Creating a key

```http
POST /api/v1/api-keys
Authorization: Bearer <access token>
Content-Type: application/json

{
  "name": "CI pipeline",
  "scopes": ["read", "write"],
  "expires_at": "2027-01-01T00:00:00Z"
}
```

| Field | Required | Rules |
| --- | --- | --- |
| `name` | Yes | Trimmed; 1–100 characters; must not be blank |
| `scopes` | No | Defaults to `["read"]`. Allowed: `read`, `write`. Case-insensitive, trimmed, deduplicated, order preserved |
| `expires_at` | No | RFC 3339 timestamp, must be in the future, stored as UTC. Omitted/`null` = never expires |

Response: `201 Created`, with `Cache-Control: no-store` and a `Location` header.

```json
{
  "id": 1,
  "name": "CI pipeline",
  "key": "mk_9Xr3kQ1oZ2mW8pY4vB6nC0sD5fG7hJ1lK3nM5pR7tV",
  "key_prefix": "mk_9Xr3kQ1o",
  "scopes": ["read", "write"],
  "expires_at": null,
  "last_used_at": null,
  "created_at": "2026-11-17T05:00:00Z"
}
```

Validation failures are `400 invalid_input` with a specific message, for example
`unsupported scope "admin"` or `expires_at must be in the future`.

### 3.3 Why the key is shown only once

The server keeps only the SHA-256 digest. A hash cannot be reversed, so the raw value
**cannot be retrieved later by anyone** — not by the user, not by an admin, not by
the backend team. That is the property that makes a database leak non-exploitable.

The trade-off is the UX rule: **if the response is lost, the key is lost.** There is
no "show key again" endpoint and no email recovery.

Frontend requirements:

- Treat the `201` body as the only copy. Put it in a modal the user must
  acknowledge, with a copy-to-clipboard button.
- Do not log the `key` field, send it to analytics, or put it in a URL.
- Never re-render it from a cached response. The response is `no-store`, and any
  client-side cache holding it is a liability.
- If the user loses it: revoke the key and create a new one. Say so in the UI.

### 3.4 Using the key

External plugins and scripts send the raw key on every request:

```http
GET /api/v1/some-plugin-endpoint
X-API-Key: mk_9Xr3kQ1oZ2mW8pY4vB6nC0sD5fG7hJ1lK3nM5pR7tV
```

How the backend evaluates it:

1. Header is trimmed. Empty, or longer than 512 bytes → immediate `401`.
2. Shape pre-check: must be exactly 46 characters and start with `mk_`. Obvious
   garbage is rejected **before** hashing and before any database round trip.
3. The value is SHA-256 hashed and looked up by `key_hash` — one indexed query.
4. Unknown hash, revoked (`revoked_at` set), or expired → `401 unauthorized` with the
   same body as every other auth failure.
5. On success the owning user ID and the key's scopes are placed in the request
   context, and the request proceeds.
6. `last_used_at` is stamped in the background, at most once per minute per key.

Key facts for integrators:

- **`X-API-Key` only.** Never `Authorization: Bearer`. Sending a key in
  `Authorization` is a `401`.
- **Every request costs one database lookup**, by design. There is no cache, so
  revocation and expiry take effect on the very next request.
- **`last_used_at` is best-effort telemetry.** It can lag by up to a minute, and
  stamps are dropped under heavy concurrency. Never use it as an audit log.
- **Scopes are stored but not enforced yet.** A `read`-only key currently has the
  same reach as a `write` key. Do not advertise scope-based restrictions in the UI
  until enforcement ships.
- **The key is not tied to the account's active flag.** A key on a deactivated
  account keeps working until it is revoked or expires.

### 3.5 Listing and revoking

`GET /api/v1/api-keys` (JWT required):

```json
{
  "api_keys": [
    {
      "id": 1,
      "name": "CI pipeline",
      "key_prefix": "mk_9Xr3kQ1o",
      "scopes": ["read", "write"],
      "expires_at": null,
      "last_used_at": "2026-11-20T09:14:02Z",
      "created_at": "2026-11-17T05:00:00Z"
    }
  ]
}
```

- Prefix only. No `key`, no hash, no owner ID.
- Revoked keys are omitted; the list is newest first.
- An account with no keys gets `{"api_keys":[]}` — an empty array, never `null`.
- `last_used_at` may be `null` for a key that has never been used.

`DELETE /api/v1/api-keys/{id}` (JWT required) → `204 No Content`, no body.

- This is a **soft delete**: `revoked_at` is stamped and the row is kept for audit.
- Unknown ID, another user's key, an already-revoked key, or a non-numeric ID all
  return the same `404 not_found` — key IDs cannot be enumerated across accounts.
- Revocation is immediate for authentication. The raw key itself is never needed to
  revoke it, which is exactly why losing the key is recoverable by deleting it.

### 3.6 Failure modes

| Situation | Status | `code` | Notes |
| --- | --- | --- | --- |
| Header missing or empty | 401 | `unauthorized` | Same body as every other failure |
| Header > 512 bytes | 401 | `unauthorized` | Rejected before hashing |
| Wrong length or no `mk_` prefix | 401 | `unauthorized` | Shape pre-check, no DB hit |
| Unknown key | 401 | `unauthorized` | Indistinguishable from revoked/expired |
| Revoked key | 401 | `unauthorized` | Immediate |
| Expired key | 401 | `unauthorized` | Compared against server time |
| Database unavailable | 500 | `internal_error` | Never disguised as a bad key |
| Create: bad name/scope/expiry | 400 | `invalid_input` | Message names the problem |
| Create/list without a JWT | 401 | `unauthorized` | API keys cannot manage keys |
| Revoke unknown/foreign/revoked key | 404 | `not_found` | Deliberately indistinguishable |

---

## 4. Error reference

The body shape is always the same:

```json
{"error":{"code":"...","message":"..."}}
```

| Code | Status | Typical cause |
| --- | --- | --- |
| `invalid_body` | 400 | Not one JSON object, unknown field, wrong type, > 1 MiB |
| `invalid_input` | 400 | Field validation failed |
| `invalid_credentials` | 401 | Wrong password, unknown user, or inactive account |
| `invalid_refresh_token` | 401 | Unknown, expired, rotated, replayed, or orphaned token |
| `unauthorized` | 401 | Missing/invalid JWT or API key |
| `account_locked` | 423 | Login attempted inside the 15-minute lockout window |
| `not_found` | 404 | Unknown route, or unknown/foreign/revoked key |
| `method_not_allowed` | 405 | Route exists, method does not |
| `username_taken` | 409 | Registration conflict |
| `email_taken` | 409 | Registration conflict |
| `payload_too_large` | 413 | Body over the size limit |
| `internal_error` | 500 | Unexpected server/database failure; details are logged only |

---

## 5. Invariants for backend developers

- Handlers map domain errors to status codes. Services return sentinels; they never
  write HTTP.
- Lockout policy lives in `internal/auth/lockout.go` and is passed to the repository
  as parameters. The SQL must not hardcode 5 or 15 minutes.
- Refresh rotation is `DELETE` + `INSERT` inside one transaction, and the `DELETE`
  only matches an unexpired row. That is the entire single-use guarantee — do not
  split it.
- Only hashes are persisted: `refresh_tokens.token_hash` and `api_keys.key_hash`,
  both SHA-256 hex via `platform/secret`. Raw credentials must never be logged,
  stored, or returned by any endpoint other than their create/login response.
- `auth.RequireAuth` and `apikey.RequireAPIKey` are mounted per subtree in
  `internal/app/router.go`, never globally. A route accepts exactly one credential.
- The two middlewares put the authenticated user ID in the context through
  `identity.WithUserID`, so handlers stay credential-agnostic.
- Neither middleware re-checks `users.is_active`. If that changes, it is a behaviour
  change for both credential types and must be reflected here and in the contract.
- Keep `contracts/openapi.yaml` in sync. This document explains behaviour; the
  OpenAPI file is the wire contract.
