import { useMemo, useState, type FormEvent } from "react";
import { Link as RouterLink, useNavigate, useParams } from "react-router-dom";
import {
  Alert,
  Box,
  Button,
  FormControlLabel,
  Stack,
  Switch,
  Typography,
} from "@mui/material";

import { ApiError } from "../api/client";
import {
  revisionTag,
  sendJSON,
  useComputes,
  useDomains,
  useInventoryMutation,
  useLocations,
  useProviders,
  type Compute,
  type Domain,
  type Location,
  type Provider,
} from "../app/data";
import { computeLight } from "../app/format";
import { ProviderDetails } from "../features/providers/ProviderDetails";
import { PlusIcon, ProviderIcon } from "../ui/icons";
import {
  EmptyHint,
  Mono,
  PageHeader,
  Pill,
  RowLink,
  SectionTitle,
  StatusLight,
} from "../ui/primitives";
import { SidePanel } from "../ui/SidePanel";

const roleLabel: Record<Provider["roles"][number], string> = {
  hosting_provider: "Hosting",
  registrar: "Registrar",
  hardware_vendor: "Hardware",
  other: "Other",
};

function providerPayload(form: FormData, existing: Provider) {
  return {
    name: String(form.get("name") ?? ""),
    website: String(form.get("website") ?? "") || undefined,
    external_account_reference:
      String(form.get("external_account_reference") ?? "") || undefined,
    description: String(form.get("description") ?? ""),
    notes: String(form.get("notes") ?? ""),
    roles: (Object.keys(roleLabel) as Provider["roles"]).filter(
      (role) => form.get(role) === "on",
    ),
    tag_ids: existing.tag_ids,
  };
}

export function ProvidersPage() {
  const { id } = useParams();
  const navigate = useNavigate();
  const [archived, setArchived] = useState(false);
  const providers = useProviders(archived);
  const computes = useComputes();
  const domains = useDomains();
  const locations = useLocations();
  const [editing, setEditing] = useState(false);

  const list = useMemo(
    () =>
      [...(providers.data ?? [])].sort((a, b) => a.name.localeCompare(b.name)),
    [providers.data],
  );
  const selected = list.find((item) => item.id === id);
  const usage = (providerID: string) => ({
    hosts: (computes.data ?? []).filter(
      (item) => item.provider_id === providerID,
    ),
    domains: (domains.data ?? []).filter(
      (item) => item.registrar_provider_id === providerID,
    ),
    places: (locations.data ?? []).filter(
      (item) => item.provider_id === providerID,
    ),
  });

  const update = useInventoryMutation(
    ({ provider, body }: { provider: Provider; body: unknown }) =>
      sendJSON<Provider>(
        "PUT",
        `/providers/${provider.id}`,
        body,
        revisionTag(provider),
      ),
  );
  const lifecycle = useInventoryMutation(
    ({
      provider,
      action,
    }: {
      provider: Provider;
      action: "archive" | "restore";
    }) =>
      sendJSON<Provider>(
        "POST",
        `/providers/${provider.id}/${action}`,
        undefined,
        revisionTag(provider),
      ),
  );

  return (
    <>
      <PageHeader
        title="Providers"
        subtitle="Who you rent from, register with or buy hardware from."
        actions={
          <Button
            component={RouterLink}
            to="/add?step=provider"
            variant="contained"
            startIcon={<PlusIcon size={16} />}
          >
            Add provider
          </Button>
        }
      />
      <Stack direction="row" sx={{ justifyContent: "flex-end" }}>
        <FormControlLabel
          control={
            <Switch
              checked={archived}
              onChange={(event) => setArchived(event.target.checked)}
            />
          }
          label="Show archived"
        />
      </Stack>
      {providers.isError && (
        <Alert severity="error">Could not load providers.</Alert>
      )}
      {providers.isSuccess && list.length === 0 ? (
        <EmptyHint
          title="No providers yet"
          body="Add your hosting company or registrar so hosts and domains can point to it."
          action={
            <Button
              component={RouterLink}
              to="/add?step=provider"
              variant="contained"
            >
              Add provider
            </Button>
          }
        />
      ) : (
        <Stack spacing={0.5}>
          {list.map((provider) => {
            const used = usage(provider.id);
            const parts = [
              used.hosts.length &&
                `${used.hosts.length} host${used.hosts.length === 1 ? "" : "s"}`,
              used.domains.length &&
                `${used.domains.length} domain${used.domains.length === 1 ? "" : "s"}`,
              used.places.length &&
                `${used.places.length} place${used.places.length === 1 ? "" : "s"}`,
            ].filter(Boolean);
            return (
              <RowLink
                key={provider.id}
                to={`/providers/${provider.id}`}
                ariaLabel={`Open ${provider.name}`}
                icon={<ProviderIcon size={20} />}
                title={
                  <Stack
                    direction="row"
                    spacing={1}
                    sx={{ alignItems: "center", flexWrap: "wrap" }}
                  >
                    <span>{provider.name}</span>
                    {provider.roles.map((role) => (
                      <Pill key={role}>{roleLabel[role]}</Pill>
                    ))}
                    {provider.archived_at && <Pill>Archived</Pill>}
                  </Stack>
                }
                subtitle={parts.length ? parts.join(" · ") : "Not used yet"}
              />
            );
          })}
        </Stack>
      )}

      <SidePanel
        open={Boolean(id)}
        onClose={() => {
          setEditing(false);
          void navigate("/providers");
        }}
        title={selected?.name ?? "Provider"}
      >
        {!selected && providers.isSuccess && (
          <Typography color="text.secondary">
            This provider isn’t in the current list. It may be archived — turn
            on “Show archived”.
          </Typography>
        )}
        {selected && (
          <Stack spacing={3}>
            <Box data-testid="provider-workspace">
              <ProviderDetails
                provider={selected}
                editing={editing}
                onToggleEdit={() => setEditing((value) => !value)}
                onLifecycle={() => {
                  const action = selected.archived_at ? "restore" : "archive";
                  setEditing(false);
                  lifecycle.mutate(
                    { provider: selected, action },
                    {
                      onSuccess: () =>
                        action === "archive" && setArchived(true),
                    },
                  );
                }}
                onUpdate={(event: FormEvent<HTMLFormElement>) => {
                  event.preventDefault();
                  update.mutate(
                    {
                      provider: selected,
                      body: providerPayload(
                        new FormData(event.currentTarget),
                        selected,
                      ),
                    },
                    { onSuccess: () => setEditing(false) },
                  );
                }}
              />
            </Box>
            {update.isError && (
              <Alert severity="error">
                {update.error instanceof ApiError && update.error.status === 412
                  ? "Provider changed elsewhere. Reload its details before editing."
                  : update.error instanceof ApiError &&
                      update.error.status === 409
                    ? "A provider with that name already exists."
                    : "Could not update provider."}
              </Alert>
            )}
            {lifecycle.isError && (
              <Alert severity="error">Could not change this provider.</Alert>
            )}
            <UsedBy usage={usage(selected.id)} />
          </Stack>
        )}
      </SidePanel>
    </>
  );
}

function UsedBy({
  usage,
}: {
  usage: { hosts: Compute[]; domains: Domain[]; places: Location[] };
}) {
  if (!usage.hosts.length && !usage.domains.length && !usage.places.length)
    return null;
  return (
    <Box component="section" aria-labelledby="used-h">
      <SectionTitle id="used-h">Used by</SectionTitle>
      <Stack direction="row" sx={{ flexWrap: "wrap", gap: 1 }}>
        {usage.hosts.map((host) => (
          <Pill key={host.id} to={`/compute/${host.id}`}>
            <StatusLight light={computeLight(host)} size={8} />
            <Mono>{host.display_name}</Mono>
          </Pill>
        ))}
        {usage.domains.map((domain) => (
          <Pill key={domain.id} to={`/domains/${domain.id}`}>
            <Mono>{domain.name}</Mono>
          </Pill>
        ))}
        {usage.places.map((place) => (
          <Pill key={place.id} to={`/places/${place.id}`}>
            {place.name}
          </Pill>
        ))}
      </Stack>
    </Box>
  );
}
