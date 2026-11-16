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
  useInventoryMutation,
  useLocations,
  useProviders,
  type Location,
} from "../app/data";
import { computeLight } from "../app/format";
import { LocationDetails } from "../features/inventory/LocationDetails";
import { PlaceIcon, PlusIcon } from "../ui/icons";
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

function nestPlaces(places: Location[]) {
  const ids = new Set(places.map((place) => place.id));
  const children = new Map<string, Location[]>();
  const roots: Location[] = [];
  for (const place of places) {
    if (place.parent_id && ids.has(place.parent_id))
      children.set(place.parent_id, [
        ...(children.get(place.parent_id) ?? []),
        place,
      ]);
    else roots.push(place);
  }
  const ordered: { place: Location; depth: number }[] = [];
  const byName = (a: Location, b: Location) => a.name.localeCompare(b.name);
  const visit = (place: Location, depth: number) => {
    ordered.push({ place, depth });
    (children.get(place.id) ?? [])
      .sort(byName)
      .forEach((child) => visit(child, depth + 1));
  };
  roots.sort(byName).forEach((root) => visit(root, 0));
  return ordered;
}

export function PlacesPage() {
  const { id } = useParams();
  const navigate = useNavigate();
  const [archived, setArchived] = useState(false);
  const locations = useLocations(archived);
  const allLocations = useLocations(true);
  const providers = useProviders(true);
  const computes = useComputes();
  const [editing, setEditing] = useState(false);

  const rows = useMemo(
    () => nestPlaces(locations.data ?? []),
    [locations.data],
  );
  const selected = locations.data?.find((item) => item.id === id);
  const providerName = useMemo(
    () => new Map((providers.data ?? []).map((item) => [item.id, item.name])),
    [providers.data],
  );
  const hostsIn = (placeID: string) =>
    (computes.data ?? []).filter((host) => host.location_id === placeID);

  const update = useInventoryMutation(
    ({ place, body }: { place: Location; body: unknown }) =>
      sendJSON<Location>(
        "PUT",
        `/locations/${place.id}`,
        body,
        revisionTag(place),
      ),
  );
  const lifecycle = useInventoryMutation(
    ({ place, action }: { place: Location; action: "archive" | "restore" }) =>
      sendJSON<Location>(
        "POST",
        `/locations/${place.id}/${action}`,
        undefined,
        revisionTag(place),
      ),
  );

  return (
    <>
      <PageHeader
        title="Places"
        subtitle="Provider regions and your own sites — where your hosts live."
        actions={
          <Button
            component={RouterLink}
            to="/add?step=place"
            variant="contained"
            startIcon={<PlusIcon size={16} />}
          >
            Add place
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
      {locations.isError && (
        <Alert severity="error">Could not load places.</Alert>
      )}
      {locations.isSuccess && rows.length === 0 ? (
        <EmptyHint
          title="No places yet"
          body="A provider region like a datacenter, or your own site like a home rack."
          action={
            <Button
              component={RouterLink}
              to="/add?step=place"
              variant="contained"
            >
              Add place
            </Button>
          }
        />
      ) : (
        <Stack component="ul" sx={{ listStyle: "none", m: 0, p: 0, gap: 0.5 }}>
          {rows.map(({ place, depth }) => {
            const hosts = hostsIn(place.id);
            return (
              <li key={place.id} style={{ paddingLeft: depth * 32 }}>
                <RowLink
                  to={`/places/${place.id}`}
                  ariaLabel={`Open ${place.name}`}
                  icon={<PlaceIcon size={20} />}
                  title={
                    <Stack
                      direction="row"
                      spacing={1}
                      sx={{ alignItems: "center", flexWrap: "wrap" }}
                    >
                      <span>{place.name}</span>
                      {place.archived_at && <Pill>Archived</Pill>}
                    </Stack>
                  }
                  subtitle={[
                    place.type === "provider_region"
                      ? "Provider region"
                      : "Own site",
                    place.provider_id && providerName.get(place.provider_id),
                    `${hosts.length} host${hosts.length === 1 ? "" : "s"}`,
                  ]
                    .filter(Boolean)
                    .join(" · ")}
                />
              </li>
            );
          })}
        </Stack>
      )}

      <SidePanel
        open={Boolean(id)}
        onClose={() => {
          setEditing(false);
          void navigate("/places");
        }}
        title={selected?.name ?? "Place"}
      >
        {!selected && locations.isSuccess && (
          <Typography color="text.secondary">
            This place isn’t in the current list. It may be archived — turn on
            “Show archived”.
          </Typography>
        )}
        {selected && (
          <Stack spacing={3}>
            <LocationDetails
              location={selected}
              providers={providers.data ?? []}
              locations={allLocations.data ?? []}
              editing={editing}
              onToggleEdit={() => setEditing((value) => !value)}
              onLifecycle={() => {
                const action = selected.archived_at ? "restore" : "archive";
                setEditing(false);
                lifecycle.mutate(
                  { place: selected, action },
                  {
                    onSuccess: () => action === "archive" && setArchived(true),
                  },
                );
              }}
              onUpdate={(event: FormEvent<HTMLFormElement>) => {
                event.preventDefault();
                const form = new FormData(event.currentTarget);
                update.mutate(
                  {
                    place: selected,
                    body: {
                      type: form.get("type"),
                      name: form.get("name"),
                      provider_id: form.get("provider_id") || undefined,
                      parent_id: form.get("parent_id") || undefined,
                      description: form.get("description") || "",
                      notes: form.get("notes") || "",
                      tag_ids: selected.tag_ids,
                    },
                  },
                  { onSuccess: () => setEditing(false) },
                );
              }}
            />
            {update.isError && (
              <Alert severity="error">
                {update.error instanceof ApiError && update.error.status === 412
                  ? "Location changed elsewhere. Reload its details before editing."
                  : "Could not update this place."}
              </Alert>
            )}
            {lifecycle.isError && (
              <Alert severity="error">Could not change this place.</Alert>
            )}
            <Box component="section" aria-labelledby="hosts-h">
              <SectionTitle id="hosts-h">Hosts here</SectionTitle>
              {hostsIn(selected.id).length === 0 ? (
                <Typography color="text.secondary">
                  No hosts in this place yet.
                </Typography>
              ) : (
                <Stack direction="row" sx={{ flexWrap: "wrap", gap: 1 }}>
                  {hostsIn(selected.id).map((host) => (
                    <Pill key={host.id} to={`/compute/${host.id}`}>
                      <StatusLight light={computeLight(host)} size={8} />
                      <Mono>{host.display_name}</Mono>
                    </Pill>
                  ))}
                </Stack>
              )}
            </Box>
          </Stack>
        )}
      </SidePanel>
    </>
  );
}
