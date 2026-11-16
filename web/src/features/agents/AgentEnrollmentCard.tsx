import { useState } from "react";
import {
  Alert,
  Button,
  Card,
  CardContent,
  Dialog,
  DialogActions,
  DialogContent,
  DialogContentText,
  DialogTitle,
  Stack,
  Typography,
} from "@mui/material";
import { apiClient } from "../../api/client";
import type { components } from "../../api/generated";
import { useEnrollment } from "./useEnrollment";

type Agent = components["schemas"]["Agent"];

export function AgentEnrollmentCard({
  computeId,
  computeName,
  agent,
  onRevoked,
}: {
  computeId: string;
  computeName: string;
  agent?: Agent;
  onRevoked?: (agent: Agent) => void;
}) {
  const enrollment = useEnrollment(computeId);
  const [confirming, setConfirming] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string>();

  async function revoke() {
    if (!agent) return;
    setBusy(true);
    setError(undefined);
    try {
      const { data, error: requestError } = await apiClient.POST(
        "/agents/{agent_id}/revoke",
        {
          params: {
            path: { agent_id: agent.id },
            header: { "If-Match": `"revision-${agent.revision}"` },
          },
        },
      );
      if (requestError || !data)
        throw requestError ?? new Error("Revoke was not confirmed.");
      onRevoked?.(data);
      enrollment.clear();
      setConfirming(false);
    } catch {
      setError(
        "Could not confirm revocation. Reload agent status before retrying; a revision conflict requires refresh.",
      );
    } finally {
      setBusy(false);
    }
  }

  return (
    <Card>
      <CardContent>
        <Stack spacing={1}>
          <Typography variant="h6">Discovery agent</Typography>
          <Typography>Bound compute: {computeName}</Typography>
          {agent ? (
            <>
              <Typography>
                Status: {agent.status} · client {agent.client_version} ·
                protocol {agent.protocol_version} ({agent.compatibility})
              </Typography>
              <Typography>
                Credential generation {agent.credential_generation} · enrolled{" "}
                {agent.enrolled_at}
              </Typography>
              <Typography>
                Last heartbeat: {agent.last_heartbeat_at ?? "Never"}
              </Typography>
              <Typography>
                Last fully successful snapshot:{" "}
                {agent.last_successful_snapshot_at ?? "Never"}
              </Typography>
              {agent.last_error && (
                <Alert severity="warning">
                  Latest collection error: {agent.last_error}
                </Alert>
              )}
              {agent.status !== "revoked" && (
                <Button
                  color="error"
                  variant="outlined"
                  onClick={() => setConfirming(true)}
                >
                  Revoke credential
                </Button>
              )}
            </>
          ) : (
            <Typography>
              No enrolled agent. Enrollment binds only to this compute.
            </Typography>
          )}
          {!agent || agent.status === "revoked" ? (
            <Button
              disabled={enrollment.pending}
              onClick={() => void enrollment.issue()}
            >
              {enrollment.pending
                ? "Issuing…"
                : "Issue one-time enrollment token"}
            </Button>
          ) : null}
          {enrollment.error && (
            <Alert severity="error">{enrollment.error}</Alert>
          )}
          {error && <Alert severity="error">{error}</Alert>}
          {enrollment.secret && (
            <Alert severity="info" onClose={enrollment.clear}>
              <Typography sx={{ fontWeight: "bold" }}>
                Copy this enrollment token now. It is shown once and will be
                cleared when this view closes or the compute/session changes.
              </Typography>
              <Typography
                component="code"
                sx={{ display: "block", overflowWrap: "anywhere", my: 1 }}
              >
                {enrollment.secret.token}
              </Typography>
              <Typography>
                Expires {enrollment.secret.expires_at}. Deploy via your external
                systemd/Ansible process. If the response was lost, revoke any
                outstanding token and issue a fresh one; do not blindly repeat
                issuance.
              </Typography>
              <Button onClick={enrollment.clear}>Clear token</Button>
            </Alert>
          )}
          <Dialog
            open={confirming}
            onClose={() => !busy && setConfirming(false)}
          >
            <DialogTitle>Revoke this agent credential?</DialogTitle>
            <DialogContent>
              <DialogContentText>
                Revocation takes effect immediately. The agent will stop
                authenticating; re-enrollment requires a new one-time token.
              </DialogContentText>
            </DialogContent>
            <DialogActions>
              <Button disabled={busy} onClick={() => setConfirming(false)}>
                Cancel
              </Button>
              <Button
                color="error"
                disabled={busy}
                onClick={() => void revoke()}
              >
                {busy ? "Revoking…" : "Confirm revoke"}
              </Button>
            </DialogActions>
          </Dialog>
        </Stack>
      </CardContent>
    </Card>
  );
}
