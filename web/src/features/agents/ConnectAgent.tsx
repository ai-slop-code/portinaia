import { useState } from "react";
import {
  Alert,
  Box,
  Button,
  IconButton,
  Stack,
  Tooltip,
  Typography,
} from "@mui/material";

import { sendJSON, useInventoryMutation } from "../../app/data";
import { formatDateTime } from "../../app/format";
import { fonts, tokens } from "../../theme/theme";
import { CopyIcon } from "../../ui/icons";
import { StatusLight } from "../../ui/primitives";
import { useEnrollment } from "./useEnrollment";

const TOKEN_FILE = "/root/portinaia-token";
const STATE_FILE = "/var/lib/portinaia-agent/credential.json";

function installCommands() {
  const origin = window.location.origin;
  const insecure =
    window.location.protocol === "http:" ? " \\\n  -allow-insecure-http" : "";
  return [
    `read -rsp 'Token: ' T && install -m 600 /dev/null ${TOKEN_FILE} && printf '%s' "$T" > ${TOKEN_FILE}`,
    `portinaia-agent enroll \\\n  -server ${origin} \\\n  -token-file ${TOKEN_FILE} \\\n  -state ${STATE_FILE}${insecure}`,
    `systemctl enable --now portinaia-agent && rm ${TOKEN_FILE}`,
  ].join("\n");
}

export function CodeBlock({ code, label }: { code: string; label: string }) {
  const [copied, setCopied] = useState(false);
  return (
    <Box
      sx={{
        display: "flex",
        alignItems: "flex-start",
        gap: 1.5,
        p: 2,
        pl: 2.5,
        borderRadius: "14px",
        background: "#06080C",
        border: `1px solid ${tokens.rope}`,
      }}
    >
      <Box
        component="pre"
        aria-label={label}
        sx={{
          flex: 1,
          minWidth: 0,
          m: 0,
          fontFamily: fonts.mono,
          fontSize: 13.5,
          lineHeight: 1.7,
          color: tokens.mist,
          whiteSpace: "pre-wrap",
          wordBreak: "break-all",
        }}
      >
        {code}
      </Box>
      <Tooltip title={copied ? "Copied" : "Copy"}>
        <IconButton
          aria-label={`Copy ${label.toLowerCase()}`}
          onClick={() => {
            void navigator.clipboard?.writeText(code).then(() => {
              setCopied(true);
              window.setTimeout(() => setCopied(false), 1500);
            });
          }}
          sx={{ border: `1px solid ${tokens.rope}`, borderRadius: "10px" }}
        >
          <CopyIcon size={18} />
        </IconButton>
      </Tooltip>
    </Box>
  );
}

/**
 * Issues a one-time enrollment token for a host and shows how to install the
 * read-only agent with it. The secret lives only in component memory.
 */
export function ConnectAgent({
  computeID,
  agentStatus,
}: {
  computeID: string;
  agentStatus: "unenrolled" | "online" | "offline" | "revoked";
}) {
  const { secret, pending, error, issue, clear } = useEnrollment(computeID);
  const revokeToken = useInventoryMutation((id: string) =>
    sendJSON<void>("DELETE", `/enrollment-tokens/${id}`),
  );
  const canIssue = agentStatus === "unenrolled" || agentStatus === "revoked";

  if (agentStatus === "online" && !secret)
    return (
      <Stack
        direction="row"
        spacing={1.5}
        sx={{
          alignItems: "center",
          p: 2,
          borderRadius: "14px",
          background: "#0F1A1C",
          border: "1px solid #1B3A37",
        }}
      >
        <StatusLight light="live" />
        <Typography>The agent is connected and reporting.</Typography>
      </Stack>
    );

  return (
    <Stack spacing={2.5}>
      {!secret && canIssue && (
        <Stack spacing={1.5} sx={{ alignItems: "flex-start" }}>
          <Typography color="text.secondary">
            The agent is read-only: it reports host facts and Podman containers
            and sends a heartbeat. Start by issuing a one-time token, valid for
            one hour.
          </Typography>
          <Button
            variant="contained"
            disabled={pending}
            onClick={() => void issue(3600)}
          >
            {pending ? "Issuing…" : "Create one-time enrollment token"}
          </Button>
        </Stack>
      )}
      {!secret && agentStatus === "offline" && (
        <Typography color="text.secondary">
          An agent is enrolled but hasn’t checked in recently. Check that the{" "}
          <Box component="code" sx={{ fontFamily: fonts.mono }}>
            portinaia-agent
          </Box>{" "}
          service is running on the host.
        </Typography>
      )}
      {error && <Alert severity="warning">{error}</Alert>}
      {secret && (
        <>
          <Alert severity="warning">
            Copy this token now. It is shown only once and expires{" "}
            {formatDateTime(secret.expires_at)}.
          </Alert>
          <CodeBlock code={secret.token} label="One-time enrollment token" />
          <Typography color="text.secondary">
            On the host, as root (the token is read from the prompt so it stays
            out of shell history):
          </Typography>
          <CodeBlock code={installCommands()} label="Install commands" />
          <Stack
            direction="row"
            spacing={1.75}
            sx={{
              alignItems: "center",
              p: 2,
              borderRadius: "14px",
              background: "#0F1A1C",
              border: "1px solid #1B3A37",
            }}
            role="status"
          >
            <StatusLight light={agentStatus === "online" ? "live" : "stale"} />
            <Typography>
              {agentStatus === "online"
                ? "Connected — the first heartbeat arrived."
                : "Listening for the first heartbeat…"}
            </Typography>
          </Stack>
          {agentStatus !== "online" && (
            <Button
              variant="outlined"
              color="error"
              sx={{ alignSelf: "flex-start" }}
              disabled={revokeToken.isPending}
              onClick={() =>
                revokeToken.mutate(secret.id, { onSuccess: () => clear() })
              }
            >
              Revoke unused token
            </Button>
          )}
          {revokeToken.isError && (
            <Alert severity="error">Could not revoke the unused token.</Alert>
          )}
        </>
      )}
    </Stack>
  );
}
