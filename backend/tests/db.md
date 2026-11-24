# Database Command Cheat Sheet

Useful commands for viewing, modifying, and clearing data in the local Postgres database during development and testing.

## Important Rules

1. **Where to run:** Run all commands from the **root of the project** (where `docker-compose.yml` lives), *not* inside the `backend/` folder.
2. **Docker must be running:** Start the database with `docker compose up -d db`.
3. **Safety first:** NEVER run a blanket `DELETE FROM users;`. Always use scoped deletes (e.g. `WHERE username = 'testuser'`) to avoid deleting real data.

---

## 1. Viewing Data (Read-Only)

**See all users (safe columns only):**

```powershell
docker compose exec db psql -U devuser -d music_db -c "SELECT id, username, email, role, is_active, created_at FROM users;"
```

**See all API keys (safe columns only):**

```powershell
docker compose exec db psql -U devuser -d music_db -c "SELECT id, user_id, name, key_prefix, scopes, revoked_at FROM api_keys;"
```

**See all active refresh tokens (shows if a user is logged in):**

```powershell
docker compose exec db psql -U devuser -d music_db -c "SELECT id, user_id, expires_at, created_at FROM refresh_tokens;"
```

**Check the lockout status of a specific user:**

```powershell
docker compose exec db psql -U devuser -d music_db -c "SELECT username, failed_logins, locked_until FROM users WHERE username = 'testuser';"
```

**Verify a password is actually hashed (should look like `$2a$10$...`):**

```powershell
docker compose exec db psql -U devuser -d music_db -c "SELECT username, password_hash FROM users;"
```

---

## 2. Modifying Data (For Testing)

When testing features like lockouts or expirations, you don't want to wait 15 minutes or 7 days. Use these queries to speed up time.

**Clear a user's lockout (so you can test login again immediately):**

```powershell
docker compose exec db psql -U devuser -d music_db -c "UPDATE users SET failed_logins = 0, locked_until = NULL WHERE username = 'testuser';"
```

**Force an API key to expire right now (to test the 401 error):**

```powershell
docker compose exec db psql -U devuser -d music_db -c "UPDATE api_keys SET expires_at = NOW() - INTERVAL '1 day' WHERE id = 1;"
```

**Force a refresh token to expire right now (to test the refresh failure):**

```powershell
docker compose exec db psql -U devuser -d music_db -c "UPDATE refresh_tokens SET expires_at = NOW() - INTERVAL '1 day' WHERE user_id = 1;"
```

---

## 3. Clearing Data (The Safe Way)

**Delete a specific test user** (this cascades and deletes their API keys and refresh tokens):

```powershell
docker compose exec db psql -U devuser -d music_db -c "DELETE FROM users WHERE username = 'testuser';"
```

**Delete all test users (scoped by prefix):**

```powershell
docker compose exec db psql -U devuser -d music_db -c "DELETE FROM users WHERE username LIKE 'test_%';"
```

**Wipe the ENTIRE database (Danger Zone 🚨):**

To start over from scratch, you must delete the Docker volume:

```powershell
docker compose down -v
docker compose up -d db
```

> **Note:** You will need to re-run all your SQL migrations after doing this.

---

## 4. Database Structure Tools

**List all tables:**

```powershell
docker compose exec db psql -U devuser -d music_db -c "\dt"
```

**See the structure (columns, types) of a specific table:**

```powershell
docker compose exec db psql -U devuser -d music_db -c "\d users"
```

**See all indexes (useful to verify your migrations worked):**

```powershell
docker compose exec db psql -U devuser -d music_db -c "\di"
```

---

## 5. Pro-Tip: Open an Interactive Shell

If you find yourself typing these commands over and over, drop into the Postgres shell and stay there:

```powershell
docker compose exec db psql -U devuser -d music_db
```

You will see a prompt like `music_db=#`. From there you can run any of the SQL queries above (without the `docker compose exec` wrapper) and press Enter. For example:

```sql
SELECT * FROM users;
```

Type `\q` to exit and return to PowerShell.