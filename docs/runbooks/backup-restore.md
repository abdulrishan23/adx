# Backup and restore

## What a backup is

`adx backup DEST` writes `DEST/<UTC stamp>/adx.db` and `DEST/<UTC stamp>/assets/`. The database copy comes from SQLite's `VACUUM INTO`, which is consistent while the server keeps running. The copy contains the sealed provider credentials but not the key that opens them. The key file (`ADX_CONNECTION_KEY_FILE`) is backed up separately and kept apart from the database backups, so a leaked backup holds nothing anyone can use.

`deploy/adx-backup.timer.example` runs it daily at 02:30 server time. Ship each new directory off the host (rsync or object storage) and prune old ones there.

## Restore, rehearsed in isolation

Do this on a machine or directory that is not the production data directory, until the result is checked.

1. Stop nothing in production. Create an empty directory, say `/tmp/adx-restore/data`, with an `assets/` subdirectory.
2. Copy `adx.db` from the chosen backup into it and the backup's `assets/` tree beside it.
3. Put the key file in place from its own backup.
4. Start a server against it in dev on a spare port:

   ```sh
   ADX_ENV=dev ADX_ADDR=localhost:8099 \
   ADX_DB_PATH=/tmp/adx-restore/data/adx.db ADX_ASSETS_DIR=/tmp/adx-restore/data/assets \
   ADX_CONNECTION_KEY_FILE=/tmp/adx-restore/connections.key \
   ./adx
   ```

   Starting applies any migrations the running code has that the backup predates.
5. Check `GET http://localhost:8099/api/ready` answers 200.
6. Check records: `GET /api/projects` lists the expected projects; `GET /api/connections/crm` lists the connections; `POST /api/connections/crm/{locationID}/check` must succeed, which proves the key opens the sealed credentials. With the wrong key the check fails with "cannot decrypt connection credentials" and nothing else is damaged.
7. Only then move the restored files into `/var/lib/adx/data` on the real host, with the server stopped, and start it.

`server/cmd/adx/backup_test.go` runs steps 2 to 6 in a temporary directory on every test run: it seals a credential, backs up, restores, and checks that the right key opens it and a wrong key doesn't. A second test restores a person with an enquiry, the opportunity on it, its evidence and the asset file, checks every link still holds, and lets run recovery interrupt the run a dead host held when the backup was taken.

## What is not covered yet

- River's job tables live in the same file and are included. In-flight jobs at backup time restart from their last state.
- Retention: `ADX_BACKUP_RETENTION_DAYS` prunes stamped directories older than that many days, only after a new backup succeeded, and never touches a directory whose name is not a stamp. The default 0 keeps everything until Reportage names a retention period (an unresolved business rule, `docs/build/decisions.md`).
