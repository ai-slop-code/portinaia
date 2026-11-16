# Host installation and recovery procedure

This procedure uses the packaged Linux agent service and an independently
managed Portinaia server. Keep the server database, configuration, referenced
secret files, and generated zones on persistent storage. Protect backups and
configuration copies as secrets; an older database backup may contain agent
credentials that have since been revoked.

## Verify and install an agent release

Download the selected release binary and `checksums.txt` from the same release
directory, then verify before installation:

```sh
sha256sum --check checksums.txt
./portinaia-agent version
```

Run the commands as root on the intended Linux host. Install the verified binary
as `/usr/local/bin/portinaia-agent`, owned by `root:root` and mode `0755`. Install
`portinaia-agent.service` under `/etc/systemd/system/`. The unit uses the rootful
Podman socket; activate that socket before enrollment and service start:

```sh
systemctl enable --now podman.socket
systemctl is-active podman.socket
test -S /run/podman/podman.sock
systemctl daemon-reload
```

The supplied unit collects the rootful Podman socket at
`/run/podman/podman.sock`. The service runs as root and keeps credential state
under `/var/lib/portinaia-agent` with systemd-managed private directory
permissions. Do not put enrollment tokens or agent credentials in unit arguments
or journal output. Create the token file from the one-time token delivered by an
administrator using a protected transfer; do not place the token in a command
argument. For example, while reading the token at a root-only prompt:

```sh
umask 077
read -r -s -p 'Enrollment token: ' PORTINAIA_ENROLLMENT_TOKEN
printf '\n'
printf '%s' "$PORTINAIA_ENROLLMENT_TOKEN" > /run/portinaia-enrollment-token
unset PORTINAIA_ENROLLMENT_TOKEN
chmod 0600 /run/portinaia-enrollment-token
/usr/local/bin/portinaia-agent enroll \
  -server https://portinaia.example \
  -token-file /run/portinaia-enrollment-token \
  -state /var/lib/portinaia-agent/credential.json
rm -f /run/portinaia-enrollment-token
systemctl enable --now portinaia-agent.service
systemctl status portinaia-agent.service
journalctl -u portinaia-agent.service
```

Replace the example URL with the deployed HTTPS URL. Enrollment must complete
before enabling/starting the service. The agent's current command contract returns
exit status `0` after SIGTERM and `1` for any returned error; transient network
failures retry internally, while revoked credentials and local/configuration
failures are terminal. The unit prevents automatic restart on status `1`, avoiding
a revocation request loop.

After an administrator revokes the agent, stop the unit and investigate the
revocation. Re-enrollment requires a newly issued token and an explicit reset of
the old credential and its associated pending report; preserve neither across
that identity/generation change. With the service stopped, run:

```sh
systemctl stop portinaia-agent.service
rm -f /var/lib/portinaia-agent/credential.json.pending
rm -f /var/lib/portinaia-agent/credential.json
```

Then repeat token-file enrollment above, and only after it succeeds run
`systemctl enable --now portinaia-agent.service`. Keep the state directory and
lock file; the lock is not pending report data.

## Upgrade

1. Record the current server/agent versions and schema status. Create and verify
   a new database backup before changing server software.
2. Stop the server before replacing server files; keep the previous binary and
   configuration available for rollback.
3. Verify release checksums, install the selected server and agent artifacts,
   and preserve ownership/modes of persistent directories and mounted secrets.
4. Run `portinaia db status -path /var/lib/portinaia/portinaia.db`. If the release
   requires a migration, run `portinaia db migrate -path ...` explicitly before
   starting the server. Do not roll back a binary across an applied schema change
   unless that older version supports the resulting schema.
5. Start the server and verify readiness, login, inventory, and agent contact.
   Upgrade the agent independently and confirm its version in server diagnostics.

## Backup and isolated restore drill

Create an online, consistent SQLite backup to a new destination (the command
refuses to overwrite an existing file):

```sh
portinaia db backup -path /var/lib/portinaia/portinaia.db \
  -output /var/backups/portinaia-YYYYMMDD-HHMMSS.db
portinaia db verify -path /var/backups/portinaia-YYYYMMDD-HHMMSS.db
```

Back up the matching server configuration and referenced secret files using
separately secured tooling. Include generated zone files for service continuity.
Record which secrets were current at the backup point and later credential
revocations/rotations; restoring SQLite does not undo remote Cloudflare changes
or invalidate credentials copied from an older backup.

For a non-destructive restore drill, use a new isolated directory and never
point the test server at production paths:

```sh
install -d -m 0700 /var/tmp/portinaia-restore-drill
install -m 0600 /var/backups/portinaia-YYYYMMDD-HHMMSS.db \
  /var/tmp/portinaia-restore-drill/portinaia.db
portinaia db verify -path /var/tmp/portinaia-restore-drill/portinaia.db
portinaia db status -path /var/tmp/portinaia-restore-drill/portinaia.db
```

Start a separate server instance on a loopback-only unused port with that copied
database, matching non-production configuration, and no production provider
credentials. Confirm readiness, administrator login, and representative inventory
and discovery records, then stop it and remove the isolated directory. This drill
does not prove DNS/provider recovery.

For production recovery, stop Portinaia first and preserve the failed database
and its `-wal`/`-shm` sidecars together for diagnosis. Restore the verified
database into a clean destination with the service account's expected owner and
mode; do not combine the restored database with sidecars from the failed copy.
Restore matching configuration, secret references, and zone backup with
appropriate permissions. Run `db verify` and `db status`, perform any required
explicit migration, then start the server and check readiness and inventory.
Reconcile DNS only when that functionality is deployed, verifying each provider
independently; never infer that restoring SQLite rolled back external changes or
authorizes adopting unknown provider records. Review and revoke credentials that
may have been reintroduced from the backup.
