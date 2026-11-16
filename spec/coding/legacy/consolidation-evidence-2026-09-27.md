# Consolidation pilot evidence — 2026-09-27

This records the local implementation and the Linux/Podman pilot performed for
consolidation. It is not M2 or M5 release sign-off.

## Linux/Podman environment

- Host environment: Podman machine running Fedora CoreOS
  `44.20260817.3.1`, Linux kernel `7.1.8-200.fc44.aarch64`, `linux/arm64`.
- Podman Engine: `6.1.1`; rootless socket owner UID `501`; rootful socket at
  `/run/podman/podman.sock`.
- Agent: `portinaia-agent version=dev commit=unknown date=unknown`; arm64 binary
  SHA-256: `f2658f968ed1fd2e40c14d384c497ae87c8b3d0b0d116ab6efb7b2b63f077e10`.
- Server image: `localhost/portinaia:dev-arm64`, image ID
  `040937516946ae35ba0642cafd468899e6153011f8f27d0c9f9bf19a606c18d9`, running
  as UID/GID `65532:65532` with a named persistent SQLite volume.
- The application database measured `970,752` bytes with nine discovery reports
  and two current containers. This is a small pilot observation, not a
  per-report-storage or capacity claim.

## Executed evidence

1. Built the `linux/arm64` server image with `make image-arm64`, ran the image's
   version command, explicitly migrated the mounted database, and started the
   server as non-root. `/api/v1/health/ready` returned `{"status":"ok"}`.
2. Bootstrapped an isolated pilot administrator, created a compute and enrollment
   token, copied the Linux arm64 agent to the machine, enrolled it, and collected
   against the actual rootless socket. Server reads showed successful host,
   disk, interface, filesystem and Podman collectors. The UI displayed hostname
   `localhost.localdomain`, arm64, two interfaces, one disk, and the discovered
   containers.
3. Created a temporary Podman workload, collected it, removed/recreated it under
   the same logical name, and collected again. The Portinaia container ID stayed
   `01a0e28a-609b-70bf-8fb2-15c385120cf0`; its runtime ID changed from
   `95383674d9c3ed86fdc80b4fbf528b3faa77498e2f533744e0b66fc09c9e6cd0` to
   `a09d78dbcb9f8d956ea2e825c0f0111b5703011ba4ee300e0551425c35a8545a`.
4. An invalid configured socket produced a failed collector; the subsequent
   successful rootless scan recovered. A rootful agent invocation against the
   actual rootful socket returned a successful empty inventory while the
   rootless inventory remained populated.
5. Stopped the server during an agent heartbeat: connection was refused. Restarted
   the server with the same database; the same agent credential heartbeat and
   snapshot were accepted. Revoked the enrolled agent through the authenticated
   API; its subsequent heartbeat returned 401.
6. Created an online SQLite backup after discovery, verified integrity and
   foreign keys, and checked migration status. Started a second non-root server
   against an isolated database copy; readiness passed and the pilot compute was
   readable. The original pilot server and database remained intact during this
   isolated restore check.
7. Ran a browser against the Linux-hosted server, signed in, selected the enrolled
   compute, and observed host facts and the workload in the UI.
8. Installed `deploy/portinaia-agent.service` on Fedora CoreOS, with a root-owned
   0600 credential under `/var/lib/portinaia-agent`. `systemctl is-active` returned
   `active`; the rootful collector returned a successful empty inventory. The
   final systemd run used server image
   `bec03c2046ba133266127198aec8e8ea016ed38b25f564e9e8050ff1101fa112` and agent
   binary SHA-256 `275aab9b5abae4fee549e53c4add616dba3dd4bf546113b8aa28e67c6ec45dec`.
   The
   server showed the agent online and accepted its reports. `systemctl restart`
   restarted the agent with the persisted credential and subsequent reports were
   accepted. The first sandboxed run showed interfaces failed because the unit
   restricted address families without `AF_NETLINK`; after adding it, host, disk,
   interface, filesystem and Podman collectors all reported success. Readiness
   returned `ready` with schema/database checks.

The Podman 6.1.1 daemon initially failed collection because the agent rejected
anything beyond API 5.x. The supported API range was extended to 4.x–6.x with
version-prefix tests; actual 6.1.1 rootless and rootful collection then passed.
Successful reports with no diagnostics also initially serialized `errors: null`;
the empty successful payload now serializes an empty array and passed ingestion.

## Native systemd follow-up — 2026-09-30

The documented unit was subsequently installed and exercised on the same isolated
Fedora CoreOS 44 arm64 Podman machine:

- `deploy/portinaia-agent.service` was installed at
  `/etc/systemd/system/portinaia-agent.service`, enabled, started and restarted.
  Final `systemctl` state was `active/running`.
- Agent state directory/file modes were `0700 root:root` and `0600 root:root`;
  rootful Podman socket was `0660 root:root` and rootless socket `0660 core:core`.
- The systemd-managed rootful agent reported host, disks, interfaces, filesystems,
  and Podman collectors as successful. Rootful Podman had no containers, so the
  UI showed the successful empty inventory distinctly. An authenticated browser
  displayed the host facts, last contact, and empty container state.
- The first systemd sandbox attempt failed interface collection with
  `netlinkrib: address family not supported by protocol`. Adding `AF_NETLINK` to
  `RestrictAddressFamilies` fixed it; the unit and evidence record include this
  required address family.
- Systemd-pilot server image ID:
  `bec03c2046ba133266127198aec8e8ea016ed38b25f564e9e8050ff1101fa112`.
  Linux arm64 agent SHA-256:
  `275aab9b5abae4fee549e53c4add616dba3dd4bf546113b8aa28e67c6ec45dec`.
- The isolated DB measured `880,640` bytes with 11 accepted reports and a
  successful empty rootful inventory. An online backup measured `978,944` bytes,
  passed `db verify` and `db status`, and an isolated server restart against it
  returned ready. This small database-size difference is not a per-report or
  capacity estimate.

## Reproduction outline

From the repository root, with Podman machine available:

```sh
make build-agent
make image-arm64
podman volume create portinaia-c06-evidence
podman run --rm --entrypoint /portinaia \
  -v portinaia-c06-evidence:/data:U localhost/portinaia:dev-arm64 \
  db migrate -path /data/portinaia.db
podman run -d --name portinaia-server \
  -p 127.0.0.1:18080:8080 \
  -e PORTINAIA_PUBLIC_URL=http://127.0.0.1:18080 \
  -v portinaia-c06-evidence:/data localhost/portinaia:dev-arm64 \
  server -listen 0.0.0.0:8080 -db /data/portinaia.db
podman machine cp dist/portinaia-agent-linux-arm64 \
  podman-machine-default:/tmp/portinaia-agent
podman machine ssh chmod 0755 /tmp/portinaia-agent
```

Bootstrap an isolated administrator with the documented CLI; create the compute
and single-use enrollment token through the authenticated UI; transfer the token
to the Linux machine in a 0600 file; enroll with `http://127.0.0.1:18080`; then
run `snapshot` using `rootless=501:/run/user/501/podman/podman.sock`. For the
systemd rootful procedure, install the binary and unit, enroll as root into
`/var/lib/portinaia-agent/credential.json`, and run `systemctl enable --now
portinaia-agent.service`. Verify the rootful socket and retain `AF_NETLINK` in
the unit's restricted address families. Do not reuse loopback HTTP outside this
isolated machine; deployed agents require HTTPS.

## Remaining limits

- This was a virtualized Fedora CoreOS Linux host, not the approved Pi 4 reference
  hardware. It does not establish performance, thermal behavior, or native
  arm64 deployment on the owner's target host.
- The browser process-restart test covers the local isolated SQLite workflow;
  the Linux pilot separately exercised a real containerized server restart and
  agent retry. The systemd unit was tested on this virtualized host, not across
  multiple physical Linux distributions.
- Discovery retention was covered by local SQLite tests, not a long-duration
  retained-report workload. The measured database contains only a small pilot
  fixture and does not establish long-term growth.
- No DNS behavior or DNS recovery was tested or implemented.
