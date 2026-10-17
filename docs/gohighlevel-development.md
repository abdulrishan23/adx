# GoHighLevel development connection

ADX uses a Private Integration Token for the first connection to Reportage.
A workspace admin generates this scoped token in the CRM.
The connection does not require a Marketplace listing, OAuth callback, public domain, or tunnel.

ADX saves each workspace separately.
A reconnect replaces credentials only for the same workspace.
The core encrypts tokens in SQLite with a key outside the database.
Connection responses contain metadata and exclude the token.

## Create the test workspace and token

1. In the working CRM agency, create a blank sub-account named **ADX Test**.
2. At the business map, select **Add account manually**.
3. Use Dubai, UAE, for the test address and **Asia/Dubai** for the timezone.
4. Open the workspace.
5. Copy its location ID from the URL after `/location/`.
6. Open **Settings → Private Integrations → Create new integration**.
7. Name the integration **ADX development**.
8. Select these read scopes:

   - `contacts.readonly`
   - `opportunities.readonly`
   - `locations.readonly`
   - `locations/customFields.readonly`

9. Create the integration.
10. Copy its token.

The test workspace contains synthetic data.
Its exact address does not affect the connection test.
If the workspace or Private Integrations menu is unavailable, ask the agency owner to grant access.

## Prepare the encryption key

1. Create a private directory outside the repository.

   ```sh
   mkdir -p ~/.config/adx
   chmod 700 ~/.config/adx
   ```

2. If no connection key exists, create it once.

   ```sh
   python3 - <<'PY'
   import os
   from pathlib import Path

   path = Path.home() / ".config/adx/connections.key"
   fd = os.open(path, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
   with os.fdopen(fd, "wb") as key_file:
       key_file.write(os.urandom(32))
   PY
   ```

The command refuses to replace an existing key.
Loss of the key requires a reconnect.
The key stays outside Git and database backups.
Production key recovery belongs to the backup plan.

## Connect and run the real reads

1. From the repository, open the `server` directory.
2. Start the development server.

   ```sh
   ADX_ENV=dev \
   ADX_ADDR=localhost:8091 \
   ADX_DB_PATH=data/ghl-dev.db \
   ADX_CONNECTION_KEY_FILE="$HOME/.config/adx/connections.key" \
   go run ./cmd/adx
   ```

3. Open `http://localhost:8091/api/connections/crm/connect`.
4. Enter the workspace ID.
5. Paste the private integration token.
6. Select **Connect and test**.
7. Make sure that the response contains the expected workspace ID and name.

Before saving the connection, ADX reads the workspace, a contact sample, custom fields, and pipelines.
A failed read preserves the previous credentials.
ADX records no OAuth expiry for a private token.
Its `tokenExpiresAt` value is null, and its `authMethod` is `private_token`.

## Inspect the saved connection

1. List the connected workspaces.

   ```sh
   curl http://localhost:8091/api/connections/crm
   ```

2. Replace `LOCATION_ID` with the saved workspace ID.
3. Repeat the real reads with the saved token.

   ```sh
   curl -X POST http://localhost:8091/api/connections/crm/LOCATION_ID/check
   ```

A successful response updates `verifiedAt`.
The check uses the encrypted token from SQLite and does not return it.
Private tokens use manual rotation in GHL, rather than an OAuth refresh request.
After rotation, enter the replacement token through the same connection form.

## Optional OAuth support

The code also supports a browser-bound OAuth flow for future customer authorization.
This flow requires Marketplace client credentials, allowed redirect URLs, and a published app or a draft Test Link.
The optional `ADX_GHL_CREDENTIALS_FILE` points to a file with `client_id`, `client_secret`, `redirect_uri`, and `version_id`.
The file must permit access only to its owner.

The start endpoint is `POST /api/connections/crm/start`.
The callback is `GET /api/connections/crm/callback`.
OAuth tokens use automatic refresh and rotation.
The agency exchange accepts one explicitly approved workspace per flow.
A live OAuth installation remains unverified.
The first working connection uses the private token.

## Tests and current scope

From the `server` directory, run the focused tests:

```sh
go test -race ./core/connections ./providers/gohighlevel ./api
```

Core tests use temporary SQLite databases and fake providers.
They cover separate workspaces, credential replacement, permissions, encrypted storage, private tokens, OAuth replay, and refresh rotation.
HTTP tests include the real core.
Provider tests cover request headers, read permissions, token exchange, and errors.

The development server uses the existing development admin actor.
Production requests remain unauthorized until the separate authentication work provides an actor.
This change connects workspaces and proves read access.
Lead imports and the production connection screen belong to separate tickets.

GHL documents [Private Integration Tokens](https://marketplace.gohighlevel.com/docs/Authorization/PrivateIntegrationsToken/index.html)
and the [OAuth flow](https://marketplace.gohighlevel.com/docs/Authorization/OAuth2.0/index.html).
