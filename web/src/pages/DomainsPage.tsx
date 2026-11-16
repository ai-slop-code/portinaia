import { useMemo, useState, type FormEvent } from "react";
import { useNavigate, useParams } from "react-router-dom";
import {
  Alert,
  Box,
  Button,
  FormControlLabel,
  InputBase,
  Stack,
  Switch,
  Tooltip,
  Typography,
} from "@mui/material";

import { ApiError } from "../api/client";
import {
  revisionTag,
  sendJSON,
  useDomains,
  useInventoryMutation,
  useProviders,
  type Domain,
} from "../app/data";
import { domainExpiryText, domainLight, formatDate } from "../app/format";
import { DomainDetails } from "../features/domains/DomainDetails";
import { domainWriteFromForm } from "../features/domains/domainAdapter";
import { fonts, tokens } from "../theme/theme";
import {
  EmptyHint,
  Mono,
  PageHeader,
  Pill,
  RowLink,
  SectionTitle,
  StatusLight,
  Surface,
} from "../ui/primitives";
import { SidePanel } from "../ui/SidePanel";

function monthKey(date: Date) {
  return `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, "0")}`;
}

export function DomainsPage() {
  const { id } = useParams();
  const navigate = useNavigate();
  const [archived, setArchived] = useState(false);
  const domains = useDomains(archived);
  const providers = useProviders(true);
  const [draft, setDraft] = useState("");
  const [editing, setEditing] = useState(false);

  const list = useMemo(
    () =>
      [...(domains.data ?? [])].sort((a, b) => a.name.localeCompare(b.name)),
    [domains.data],
  );
  const selected = list.find((item) => item.id === id);
  const registrarName = useMemo(
    () => new Map((providers.data ?? []).map((item) => [item.id, item.name])),
    [providers.data],
  );

  const quickAdd = useInventoryMutation((name: string) =>
    sendJSON<Domain>("POST", "/domains", {
      name,
      nameservers: [],
      tag_ids: [],
    }),
  );
  const update = useInventoryMutation(
    ({ domain, body, etag }: { domain: Domain; body: unknown; etag: string }) =>
      sendJSON<Domain>("PUT", `/domains/${domain.id}`, body, etag),
  );
  const lifecycle = useInventoryMutation(
    ({ domain, action }: { domain: Domain; action: "archive" | "restore" }) =>
      sendJSON<Domain>(
        "POST",
        `/domains/${domain.id}/${action}`,
        undefined,
        revisionTag(domain),
      ),
  );

  const months = useMemo(() => {
    const now = new Date();
    return Array.from({ length: 12 }, (_, offset) => {
      const date = new Date(now.getFullYear(), now.getMonth() + offset, 1);
      const key = monthKey(date);
      return {
        key,
        label: date.toLocaleDateString(undefined, { month: "short" }),
        domains: list.filter((domain) => domain.expires_on?.startsWith(key)),
      };
    });
  }, [list]);
  const upcoming = months.reduce((sum, month) => sum + month.domains.length, 0);
  const expiringSoon = list.filter(
    (domain) =>
      domain.expiry_state === "expiring" || domain.expiry_state === "expired",
  ).length;

  const submit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const name = draft.trim().toLowerCase();
    if (!name) return;
    quickAdd.mutate(name, {
      onSuccess: (domain) => {
        setDraft("");
        void navigate(`/domains/${domain.id}`);
      },
    });
  };

  const close = () => {
    setEditing(false);
    void navigate("/domains");
  };

  return (
    <Stack spacing={4.5} sx={{ maxWidth: 1000, width: "100%", mx: "auto" }}>
      <PageHeader
        title="Domains"
        subtitle={
          domains.data
            ? `${list.length} domain${list.length === 1 ? "" : "s"} · ${expiringSoon} expiring or expired`
            : "Loading…"
        }
      />

      <Box
        component="form"
        onSubmit={submit}
        data-testid="domain-create-form"
        sx={{
          display: "flex",
          alignItems: "center",
          gap: 1.25,
          minHeight: 64,
          pl: 2.75,
          pr: 1,
          borderRadius: 999,
          background: tokens.hull,
          border: `1px solid ${tokens.rope}`,
          "&:focus-within": { borderColor: tokens.signal },
        }}
      >
        <InputBase
          value={draft}
          onChange={(event) => setDraft(event.target.value)}
          placeholder="Add a domain — just the name, details later"
          inputProps={{ "aria-label": "Domain name", autoComplete: "off" }}
          sx={{ flex: 1, fontFamily: fonts.mono, fontSize: 17 }}
        />
        <Button
          type="submit"
          variant="contained"
          size="large"
          disabled={quickAdd.isPending || !draft.trim()}
        >
          Add domain
        </Button>
      </Box>
      {quickAdd.isError && (
        <Alert severity="error">
          {quickAdd.error instanceof ApiError && quickAdd.error.status === 409
            ? "That domain is already in your inventory."
            : quickAdd.error instanceof ApiError
              ? quickAdd.error.message
              : "Could not add the domain."}
        </Alert>
      )}

      <Box component="section" aria-labelledby="renew-h">
        <SectionTitle
          id="renew-h"
          action={
            <Typography color="text.secondary" sx={{ fontSize: 14 }}>
              Next 12 months
            </Typography>
          }
        >
          Renewals ahead
        </SectionTitle>
        <Surface>
          <Box
            sx={{
              display: "grid",
              gridTemplateColumns: {
                xs: "repeat(6, minmax(0, 1fr))",
                sm: "repeat(12, minmax(0, 1fr))",
              },
              gap: 0.75,
            }}
          >
            {months.map((month) => {
              const count = month.domains.length;
              return (
                <Tooltip
                  key={month.key}
                  title={
                    count
                      ? month.domains.map((domain) => domain.name).join(", ")
                      : "Nothing renews"
                  }
                >
                  <Stack spacing={1} sx={{ alignItems: "center" }}>
                    <Box
                      sx={{
                        width: "100%",
                        height: 40,
                        borderRadius: "8px",
                        display: "flex",
                        alignItems: "center",
                        justifyContent: "center",
                        fontFamily: fonts.display,
                        fontWeight: 600,
                        background: count
                          ? month === months[0] || month === months[1]
                            ? tokens.beaconWash
                            : tokens.signalWash
                          : tokens.deck,
                        color: count
                          ? month === months[0] || month === months[1]
                            ? tokens.beacon
                            : tokens.signal
                          : "transparent",
                        border: count
                          ? `1px solid ${month === months[0] || month === months[1] ? tokens.beacon : tokens.signal}`
                          : "1px solid transparent",
                      }}
                    >
                      {count || ""}
                    </Box>
                    <Typography sx={{ fontSize: 12, color: tokens.fog }}>
                      {month.label}
                    </Typography>
                  </Stack>
                </Tooltip>
              );
            })}
          </Box>
          {upcoming === 0 && (
            <Typography color="text.secondary" sx={{ mt: 2, fontSize: 14 }}>
              Nothing on the calendar yet — add expiry dates and renewals land
              here.
            </Typography>
          )}
        </Surface>
      </Box>

      <Box component="section" aria-labelledby="owned-h">
        <SectionTitle
          id="owned-h"
          action={
            <FormControlLabel
              control={
                <Switch
                  checked={archived}
                  onChange={(event) => setArchived(event.target.checked)}
                />
              }
              label="Show archived"
            />
          }
        >
          Owned
        </SectionTitle>
        {domains.isError && (
          <Alert severity="error">Could not load your domains.</Alert>
        )}
        {domains.isSuccess && list.length === 0 ? (
          <EmptyHint
            title="No domains yet"
            body="Type a name above. Registrar, expiry and nameservers can come later."
          />
        ) : (
          <Stack spacing={0.75}>
            {list.map((domain) => (
              <Box
                key={domain.id}
                sx={{
                  borderRadius: "18px",
                  border: `1px solid ${domain.id === id ? tokens.rope : tokens.line}`,
                }}
              >
                <RowLink
                  to={`/domains/${domain.id}`}
                  ariaLabel={`Open ${domain.name}`}
                  title={
                    <Stack
                      direction="row"
                      spacing={1.5}
                      sx={{ alignItems: "center", flexWrap: "wrap" }}
                    >
                      <Mono sx={{ fontSize: 17 }}>{domain.name}</Mono>
                      {domain.archived_at && <Pill>Archived</Pill>}
                    </Stack>
                  }
                  subtitle={
                    domain.registrar_provider_id
                      ? (registrarName.get(domain.registrar_provider_id) ??
                        "Registrar not loaded")
                      : "No registrar"
                  }
                  trailing={
                    <Box
                      component="span"
                      sx={{
                        color: tokens.fog,
                        fontSize: 14,
                        display: { xs: "none", sm: "inline-flex" },
                      }}
                    >
                      <StatusLight
                        light={domainLight(domain)}
                        label={domainExpiryText(domain)}
                      />
                    </Box>
                  }
                />
              </Box>
            ))}
          </Stack>
        )}
      </Box>

      <SidePanel
        open={Boolean(id)}
        onClose={close}
        title={selected?.name ?? "Domain"}
        mono
      >
        {!selected && domains.isSuccess && (
          <Typography color="text.secondary">
            This domain isn’t in the current list. It may be archived — turn on
            “Show archived”.
          </Typography>
        )}
        {selected && (
          <Stack spacing={3} data-testid="domain-workspace">
            <Stack direction="row" sx={{ gap: 1, flexWrap: "wrap" }}>
              <Pill>
                <StatusLight
                  light={domainLight(selected)}
                  label={domainExpiryText(selected)}
                />
              </Pill>
              {selected.expires_on && (
                <Pill>{formatDate(selected.expires_on)}</Pill>
              )}
            </Stack>
            <DomainDetails
              key={`${selected.id}-${selected.revision}`}
              domain={selected}
              providers={providers.data ?? []}
              editing={editing}
              onToggleEdit={() => setEditing((value) => !value)}
              onLifecycle={() => {
                const action = selected.archived_at ? "restore" : "archive";
                setEditing(false);
                lifecycle.mutate(
                  { domain: selected, action },
                  // Keep the archived domain in view so it can be restored.
                  {
                    onSuccess: () => action === "archive" && setArchived(true),
                  },
                );
              }}
              onUpdate={(event, snapshot) => {
                if (snapshot.id !== selected.id) return;
                update.mutate(
                  {
                    domain: selected,
                    etag: snapshot.etag,
                    body: domainWriteFromForm(
                      new FormData(event.currentTarget),
                      { tag_ids: [...snapshot.preservedTagIDs] },
                    ),
                  },
                  { onSuccess: () => setEditing(false) },
                );
              }}
            />
            {update.isError && (
              <Alert severity="error">
                {update.error instanceof ApiError && update.error.status === 412
                  ? "Domain changed elsewhere; reload before editing."
                  : "Could not update the domain."}
              </Alert>
            )}
            {lifecycle.isError && (
              <Alert severity="error">Could not change this domain.</Alert>
            )}
          </Stack>
        )}
      </SidePanel>
    </Stack>
  );
}
