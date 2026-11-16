import { useState, type FormEvent, type ReactNode } from "react";
import { Link as RouterLink, useSearchParams } from "react-router-dom";
import {
  Alert,
  Box,
  Button,
  Checkbox,
  FormControlLabel,
  MenuItem,
  Stack,
  TextField,
  Typography,
} from "@mui/material";

import { ApiError } from "../api/client";
import {
  revisionTag,
  sendJSON,
  useCompute,
  useComputes,
  useInventoryMutation,
  useLocations,
  useProviders,
  type Compute,
  type Domain,
  type Location,
  type Provider,
} from "../app/data";
import { ConnectAgent } from "../features/agents/ConnectAgent";
import { fonts, tokens } from "../theme/theme";
import { ChoiceCards, ChoicePills, TogglePills } from "../ui/Choice";
import { Mono, PageHeader } from "../ui/primitives";

const steps = [
  { id: "domain", label: "Domain", hint: "What you own" },
  { id: "provider", label: "Provider", hint: "Who you rent from" },
  { id: "place", label: "Place", hint: "Where it lives" },
  { id: "compute", label: "Compute", hint: "The machine itself" },
  { id: "connect", label: "Connect", hint: "Agent reports in" },
] as const;
type StepID = (typeof steps)[number]["id"];

type Created = {
  domain?: Domain;
  provider?: Provider;
  place?: Location;
  compute?: Compute;
};

function problemMessage(error: unknown, conflict: string, fallback: string) {
  if (error instanceof ApiError) {
    if (error.status === 409) return conflict;
    if (error.status === 422 || error.status === 400) return error.message;
  }
  return fallback;
}

export function AddFlowPage() {
  const [params, setParams] = useSearchParams();
  const requested = params.get("step");
  const index = Math.max(
    0,
    steps.findIndex((step) => step.id === requested),
  );
  const [created, setCreated] = useState<Created>({});
  const [visited, setVisited] = useState<Set<StepID>>(new Set());

  const go = (target: number) => {
    setVisited((current) => new Set(current).add(steps[index].id));
    const next = new URLSearchParams(params);
    next.set("step", steps[Math.min(steps.length - 1, Math.max(0, target))].id);
    setParams(next);
  };

  const step = steps[index];
  return (
    <>
      <PageHeader
        title="Add to your estate"
        subtitle="One question at a time. Skip anything you don’t know yet — you can fill it in later."
      />
      <Stack
        direction="row"
        sx={{ flexWrap: "wrap", gap: 4, alignItems: "flex-start" }}
      >
        <Box
          component="nav"
          aria-label="Steps"
          sx={{ flex: "1 1 240px", maxWidth: { md: 300 } }}
        >
          <Box
            component="ol"
            sx={{
              listStyle: "none",
              m: 0,
              p: 0,
              display: "flex",
              flexDirection: "column",
              gap: 0.5,
            }}
          >
            {steps.map((item, itemIndex) => {
              const current = itemIndex === index;
              const done =
                Boolean(
                  (item.id === "domain" && created.domain) ||
                  (item.id === "provider" && created.provider) ||
                  (item.id === "place" && created.place) ||
                  (item.id === "compute" && created.compute),
                ) ||
                (visited.has(item.id) && itemIndex < index);
              return (
                <li key={item.id}>
                  <Box
                    component="button"
                    type="button"
                    onClick={() => go(itemIndex)}
                    aria-current={current ? "step" : undefined}
                    sx={{
                      width: "100%",
                      display: "flex",
                      alignItems: "center",
                      gap: 1.75,
                      px: 1.75,
                      py: 1.5,
                      minHeight: 56,
                      border: 0,
                      borderRadius: "14px",
                      background: current ? tokens.deck : "transparent",
                      color: current || done ? tokens.foam : tokens.fog,
                      font: "inherit",
                      textAlign: "left",
                      cursor: "pointer",
                      "&:hover": { background: tokens.deck },
                    }}
                  >
                    <Box
                      component="span"
                      aria-hidden
                      sx={{
                        flex: "0 0 auto",
                        width: 30,
                        height: 30,
                        borderRadius: "50%",
                        display: "flex",
                        alignItems: "center",
                        justifyContent: "center",
                        fontFamily: fonts.display,
                        fontWeight: 600,
                        fontSize: 13,
                        background: done
                          ? tokens.signal
                          : current
                            ? tokens.night
                            : "transparent",
                        color: done
                          ? tokens.night
                          : current
                            ? tokens.signal
                            : tokens.fog,
                        border: `1px solid ${done || current ? tokens.signal : tokens.ghost}`,
                      }}
                    >
                      {done ? "✓" : itemIndex + 1}
                    </Box>
                    <Box
                      component="span"
                      sx={{ display: "flex", flexDirection: "column" }}
                    >
                      <Box
                        component="span"
                        sx={{ fontWeight: 600, fontSize: 15 }}
                      >
                        {item.label}
                      </Box>
                      <Box
                        component="span"
                        sx={{ fontSize: 12, color: tokens.fog }}
                      >
                        {item.hint}
                      </Box>
                    </Box>
                  </Box>
                </li>
              );
            })}
          </Box>
        </Box>

        <Box
          component="section"
          aria-live="polite"
          sx={{
            flex: "999 1 520px",
            minWidth: 0,
            p: { xs: 3, md: 4.5 },
            borderRadius: "24px",
            background: tokens.hull,
            border: `1px solid ${tokens.line}`,
          }}
        >
          {step.id === "domain" && (
            <DomainStep
              key="domain"
              created={created.domain}
              onDone={(domain) => {
                if (domain) setCreated((c) => ({ ...c, domain }));
                go(1);
              }}
            />
          )}
          {step.id === "provider" && (
            <ProviderStep
              key="provider"
              created={created}
              onDone={(update) => {
                setCreated((c) => ({ ...c, ...update }));
                go(2);
              }}
              onBack={() => go(0)}
            />
          )}
          {step.id === "place" && (
            <PlaceStep
              key="place"
              created={created}
              onDone={(place) => {
                if (place) setCreated((c) => ({ ...c, place }));
                go(3);
              }}
              onBack={() => go(1)}
            />
          )}
          {step.id === "compute" && (
            <ComputeStep
              key="compute"
              created={created}
              onDone={(compute) => {
                setCreated((c) => ({ ...c, compute }));
                go(4);
              }}
              onBack={() => go(2)}
            />
          )}
          {step.id === "connect" && (
            <ConnectStep created={created} onBack={() => go(3)} />
          )}
        </Box>
      </Stack>
    </>
  );
}

function StepHeading({
  index,
  required,
  title,
  body,
}: {
  index: number;
  required?: boolean;
  title: string;
  body?: ReactNode;
}) {
  return (
    <Stack spacing={0.75}>
      <Typography sx={{ color: tokens.fog, fontSize: 13, fontWeight: 500 }}>
        Step {index} of 5 · {required ? "required" : "optional"}
      </Typography>
      <Typography
        component="h2"
        sx={{ fontFamily: fonts.display, fontWeight: 600, fontSize: 28 }}
      >
        {title}
      </Typography>
      {body && <Typography color="text.secondary">{body}</Typography>}
    </Stack>
  );
}

function StepActions({
  onBack,
  onSkip,
  submitLabel,
  pending,
}: {
  onBack?: () => void;
  onSkip?: () => void;
  submitLabel: string;
  pending?: boolean;
}) {
  return (
    <Stack
      direction="row"
      sx={{
        alignItems: "center",
        gap: 1.5,
        flexWrap: "wrap",
        pt: 2.5,
        borderTop: `1px solid ${tokens.line}`,
      }}
    >
      {onBack && (
        <Button variant="outlined" size="large" onClick={onBack}>
          Back
        </Button>
      )}
      <Box sx={{ flex: 1 }} />
      {onSkip && (
        <Button size="large" onClick={onSkip} sx={{ color: tokens.fog }}>
          Skip
        </Button>
      )}
      <Button type="submit" variant="contained" size="large" disabled={pending}>
        {pending ? "Saving…" : submitLabel}
      </Button>
    </Stack>
  );
}

function MoreDetails({ children }: { children: ReactNode }) {
  const [open, setOpen] = useState(false);
  return (
    <>
      <Button
        onClick={() => setOpen((value) => !value)}
        aria-expanded={open}
        sx={{ alignSelf: "flex-start", px: 0.5 }}
      >
        {open ? "− Fewer details" : "+ More details"}
      </Button>
      <Stack spacing={2.25} sx={{ display: open ? "flex" : "none" }}>
        {children}
      </Stack>
    </>
  );
}

const bigInput = { "& .MuiInputBase-input": { fontSize: 18, py: 2 } };
const monoInput = {
  ...bigInput,
  "& .MuiInputBase-input": { fontSize: 18, py: 2, fontFamily: fonts.mono },
};

function Done({ children }: { children: ReactNode }) {
  return (
    <Alert severity="success" variant="outlined">
      {children}
    </Alert>
  );
}

function DomainStep({
  created,
  onDone,
}: {
  created?: Domain;
  onDone: (domain?: Domain) => void;
}) {
  const providers = useProviders();
  const registrars = (providers.data ?? []).filter((item) =>
    item.roles.includes("registrar"),
  );
  const [registrar, setRegistrar] = useState("");
  const [autoRenew, setAutoRenew] = useState<"unknown" | "true" | "false">(
    "unknown",
  );
  const create = useInventoryMutation((body: unknown) =>
    sendJSON<Domain>("POST", "/domains", body),
  );

  const submit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const form = new FormData(event.currentTarget);
    const name = String(form.get("name") ?? "")
      .trim()
      .toLowerCase();
    if (!name) return onDone();
    create.mutate(
      {
        name,
        registrar_provider_id: registrar || undefined,
        registered_on: form.get("registered_on") || undefined,
        expires_on: form.get("expires_on") || undefined,
        auto_renew: autoRenew === "unknown" ? null : autoRenew === "true",
        nameservers: String(form.get("nameservers") ?? "")
          .split(/[\s,]+/)
          .filter(Boolean),
        notes: String(form.get("notes") ?? ""),
        tag_ids: [],
      },
      { onSuccess: (domain) => onDone(domain) },
    );
  };

  return (
    <Stack component="form" spacing={3} onSubmit={submit}>
      <StepHeading index={1} title="Which domain do you own?" />
      {created && (
        <Done>
          Added <Mono>{created.name}</Mono>. Add another or continue.
        </Done>
      )}
      <TextField
        name="name"
        label="Domain name"
        placeholder="example.com"
        autoFocus
        sx={monoInput}
      />
      <Stack direction="row" sx={{ flexWrap: "wrap", gap: 2 }}>
        <TextField
          name="expires_on"
          label="Expires on"
          type="date"
          sx={{ flex: "1 1 220px" }}
          slotProps={{ inputLabel: { shrink: true } }}
        />
        <TextField
          select
          label="Registrar"
          value={registrar}
          onChange={(event) => setRegistrar(event.target.value)}
          sx={{ flex: "1 1 220px" }}
          helperText={
            registrars.length === 0
              ? "No registrars yet — add one in the next step"
              : undefined
          }
        >
          <MenuItem value="">Not tracked</MenuItem>
          {registrars.map((item) => (
            <MenuItem key={item.id} value={item.id}>
              {item.name}
            </MenuItem>
          ))}
        </TextField>
      </Stack>
      <MoreDetails>
        <ChoicePills
          legend="Auto-renew"
          name="auto_renew"
          value={autoRenew}
          onChange={setAutoRenew}
          options={[
            ["unknown", "Unknown"],
            ["true", "Yes"],
            ["false", "No"],
          ]}
        />
        <TextField
          name="registered_on"
          label="Registered on"
          type="date"
          slotProps={{ inputLabel: { shrink: true } }}
        />
        <TextField
          name="nameservers"
          label="Nameservers"
          placeholder="one per line"
          multiline
          minRows={3}
          sx={{ "& textarea": { fontFamily: fonts.mono } }}
        />
        <TextField name="notes" label="Notes" multiline />
      </MoreDetails>
      {create.isError && (
        <Alert severity="error">
          {problemMessage(
            create.error,
            "That domain is already in your inventory.",
            "Could not add the domain.",
          )}
        </Alert>
      )}
      <StepActions
        onSkip={() => onDone()}
        submitLabel="Continue"
        pending={create.isPending}
      />
    </Stack>
  );
}

const providerRoles = [
  ["hosting_provider", "Hosting"],
  ["registrar", "Registrar"],
  ["hardware_vendor", "Hardware"],
  ["other", "Other"],
] as const;
type ProviderRole = (typeof providerRoles)[number][0];

function ProviderStep({
  created,
  onDone,
  onBack,
}: {
  created: Created;
  onDone: (update: Partial<Created>) => void;
  onBack: () => void;
}) {
  const [roles, setRoles] = useState<ProviderRole[]>(["hosting_provider"]);
  const pendingDomain =
    created.domain && !created.domain.registrar_provider_id
      ? created.domain
      : undefined;
  const [linkDomain, setLinkDomain] = useState(true);
  const create = useInventoryMutation(
    async ({ body, link }: { body: unknown; link?: Domain }) => {
      const provider = await sendJSON<Provider>("POST", "/providers", body);
      let domain: Domain | undefined;
      if (link) {
        domain = await sendJSON<Domain>(
          "PUT",
          `/domains/${link.id}`,
          {
            name: link.name,
            registrar_provider_id: provider.id,
            registered_on: link.registered_on,
            expires_on: link.expires_on,
            auto_renew: link.auto_renew ?? null,
            nameservers: link.nameservers,
            description: link.description ?? "",
            notes: link.notes ?? "",
            tag_ids: link.tag_ids,
          },
          revisionTag(link),
        );
      }
      return { provider, domain };
    },
  );

  const submit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const form = new FormData(event.currentTarget);
    const name = String(form.get("name") ?? "").trim();
    if (!name) return onDone({});
    create.mutate(
      {
        body: {
          name,
          website: String(form.get("website") ?? "") || undefined,
          external_account_reference:
            String(form.get("external_account_reference") ?? "") || undefined,
          description: "",
          notes: String(form.get("notes") ?? ""),
          roles,
          tag_ids: [],
        },
        link:
          pendingDomain && linkDomain && roles.includes("registrar")
            ? pendingDomain
            : undefined,
      },
      {
        onSuccess: ({ provider, domain }) =>
          onDone({ provider, ...(domain ? { domain } : {}) }),
      },
    );
  };

  return (
    <Stack component="form" spacing={3} onSubmit={submit}>
      <StepHeading
        index={2}
        title="Who do you rent from?"
        body="A hosting company, a registrar, or the shop you bought the box from."
      />
      {created.provider && (
        <Done>Added {created.provider.name}. Add another or continue.</Done>
      )}
      <TextField
        name="name"
        label="Provider name"
        placeholder="e.g. your hosting company"
        autoFocus
        sx={bigInput}
      />
      <TogglePills
        legend="What do they do for you?"
        values={roles}
        onChange={setRoles}
        options={providerRoles}
      />
      {pendingDomain && roles.includes("registrar") && (
        <FormControlLabel
          control={
            <Checkbox
              checked={linkDomain}
              onChange={(event) => setLinkDomain(event.target.checked)}
            />
          }
          label={
            <>
              Use as registrar for <Mono>{pendingDomain.name}</Mono>
            </>
          }
        />
      )}
      <TextField
        name="website"
        label="Website"
        type="url"
        placeholder="https://"
      />
      <MoreDetails>
        <TextField
          name="external_account_reference"
          label="Account reference"
          helperText="Your customer or account number with them"
        />
        <TextField name="notes" label="Notes" multiline />
      </MoreDetails>
      {create.isError && (
        <Alert severity="error">
          {problemMessage(
            create.error,
            "A provider with that name already exists.",
            "Could not add the provider.",
          )}
        </Alert>
      )}
      <StepActions
        onBack={onBack}
        onSkip={() => onDone({})}
        submitLabel="Continue"
        pending={create.isPending}
      />
    </Stack>
  );
}

function PlaceStep({
  created,
  onDone,
  onBack,
}: {
  created: Created;
  onDone: (place?: Location) => void;
  onBack: () => void;
}) {
  const providers = useProviders();
  const locations = useLocations();
  const [type, setType] = useState<"provider_region" | "site">(
    created.provider ? "provider_region" : "site",
  );
  const [provider, setProvider] = useState(created.provider?.id ?? "");
  const [parent, setParent] = useState("");
  const create = useInventoryMutation((body: unknown) =>
    sendJSON<Location>("POST", "/locations", body),
  );

  const submit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const form = new FormData(event.currentTarget);
    const name = String(form.get("name") ?? "").trim();
    if (!name) return onDone();
    create.mutate(
      {
        type,
        name,
        provider_id: provider || undefined,
        parent_id: parent || undefined,
        description: "",
        notes: String(form.get("notes") ?? ""),
        tag_ids: [],
      },
      { onSuccess: (place) => onDone(place) },
    );
  };

  return (
    <Stack component="form" spacing={3} onSubmit={submit}>
      <StepHeading index={3} title="Where does it live?" />
      {created.place && (
        <Done>Added {created.place.name}. Add another or continue.</Done>
      )}
      <ChoicePills
        legend="Kind of place"
        name="type"
        value={type}
        onChange={setType}
        options={[
          ["provider_region", "Provider region"],
          ["site", "My own site"],
        ]}
      />
      <TextField
        name="name"
        label="Name"
        placeholder={
          type === "site" ? "e.g. Home rack" : "e.g. a region or datacenter"
        }
        autoFocus
        sx={bigInput}
      />
      <TextField
        select
        label="Provider"
        value={provider}
        onChange={(event) => setProvider(event.target.value)}
      >
        <MenuItem value="">None</MenuItem>
        {(providers.data ?? []).map((item) => (
          <MenuItem key={item.id} value={item.id}>
            {item.name}
          </MenuItem>
        ))}
      </TextField>
      <MoreDetails>
        <TextField
          select
          label="Inside of"
          value={parent}
          onChange={(event) => setParent(event.target.value)}
        >
          <MenuItem value="">Nothing — top level</MenuItem>
          {(locations.data ?? []).map((item) => (
            <MenuItem key={item.id} value={item.id}>
              {item.name}
            </MenuItem>
          ))}
        </TextField>
        <TextField name="notes" label="Notes" multiline />
      </MoreDetails>
      {create.isError && (
        <Alert severity="error">
          {problemMessage(
            create.error,
            "A place with that name already exists.",
            "Could not add the place.",
          )}
        </Alert>
      )}
      <StepActions
        onBack={onBack}
        onSkip={() => onDone()}
        submitLabel="Continue"
        pending={create.isPending}
      />
    </Stack>
  );
}

function ComputeStep({
  created,
  onDone,
  onBack,
}: {
  created: Created;
  onDone: (compute: Compute) => void;
  onBack: () => void;
}) {
  const providers = useProviders();
  const locations = useLocations();
  const computes = useComputes();
  const [kind, setKind] = useState<Compute["kind"]>("vps");
  const [ownership, setOwnership] = useState<Compute["ownership"]>(
    created.provider ? "rented" : "owned",
  );
  const [provider, setProvider] = useState(created.provider?.id ?? "");
  const [place, setPlace] = useState(created.place?.id ?? "");
  const [parent, setParent] = useState("");
  const create = useInventoryMutation((body: unknown) =>
    sendJSON<Compute>("POST", "/computes", body),
  );

  const submit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const form = new FormData(event.currentTarget);
    create.mutate(
      {
        kind,
        display_name: String(form.get("display_name") ?? "").trim(),
        ownership,
        provider_id: provider || undefined,
        provider_resource_id:
          String(form.get("provider_resource_id") ?? "") || undefined,
        location_id: place || undefined,
        parent_compute_id: kind === "vm" && parent ? parent : undefined,
        description: "",
        notes: String(form.get("notes") ?? ""),
        tag_ids: [],
      },
      { onSuccess: (compute) => onDone(compute) },
    );
  };

  return (
    <Stack component="form" spacing={3} onSubmit={submit}>
      <StepHeading index={4} required title="Name the machine" />
      <TextField
        name="display_name"
        label="Hostname"
        placeholder="host.example.com"
        required
        autoFocus
        sx={monoInput}
      />
      <ChoiceCards
        legend="What is it?"
        name="kind"
        value={kind}
        onChange={setKind}
        options={[
          { value: "physical", label: "Physical", hint: "A box you can touch" },
          { value: "vps", label: "VPS", hint: "Rented virtual server" },
          { value: "vm", label: "VM", hint: "Runs on another host" },
        ]}
      />
      {kind === "vm" && (
        <TextField
          select
          label="Runs on"
          value={parent}
          onChange={(event) => setParent(event.target.value)}
        >
          <MenuItem value="">Not tracked</MenuItem>
          {(computes.data ?? []).map((item) => (
            <MenuItem
              key={item.id}
              value={item.id}
              sx={{ fontFamily: fonts.mono }}
            >
              {item.display_name}
            </MenuItem>
          ))}
        </TextField>
      )}
      <ChoicePills
        legend="Ownership"
        name="ownership"
        value={ownership}
        onChange={setOwnership}
        options={[
          ["owned", "Owned"],
          ["rented", "Rented"],
          ["other", "Other"],
        ]}
      />
      <Stack direction="row" sx={{ flexWrap: "wrap", gap: 2 }}>
        <TextField
          select
          label="Provider"
          value={provider}
          onChange={(event) => setProvider(event.target.value)}
          sx={{ flex: "1 1 220px" }}
        >
          <MenuItem value="">None</MenuItem>
          {(providers.data ?? []).map((item) => (
            <MenuItem key={item.id} value={item.id}>
              {item.name}
            </MenuItem>
          ))}
        </TextField>
        <TextField
          select
          label="Place"
          value={place}
          onChange={(event) => setPlace(event.target.value)}
          sx={{ flex: "1 1 220px" }}
        >
          <MenuItem value="">None</MenuItem>
          {(locations.data ?? []).map((item) => (
            <MenuItem key={item.id} value={item.id}>
              {item.name}
            </MenuItem>
          ))}
        </TextField>
      </Stack>
      <MoreDetails>
        <TextField
          name="provider_resource_id"
          label="Provider reference"
          helperText="The server ID in your provider’s console"
        />
        <TextField name="notes" label="Notes" multiline />
      </MoreDetails>
      {create.isError && (
        <Alert severity="error">
          {problemMessage(
            create.error,
            "That provider reference is already registered.",
            "Could not create the host.",
          )}
        </Alert>
      )}
      <StepActions
        onBack={onBack}
        submitLabel="Create compute"
        pending={create.isPending}
      />
    </Stack>
  );
}

function ConnectStep({
  created,
  onBack,
}: {
  created: Created;
  onBack: () => void;
}) {
  const host = useCompute(created.compute?.id, { pollUntilOnline: true });
  const status = host.data?.agent_status ?? created.compute?.agent_status;
  return (
    <Stack spacing={3}>
      <StepHeading
        index={5}
        title="Let it report in"
        body={
          created.compute ? (
            <>
              Install the read-only agent on{" "}
              <Mono>{created.compute.display_name}</Mono>.
            </>
          ) : undefined
        }
      />
      {created.compute && status ? (
        <ConnectAgent computeID={created.compute.id} agentStatus={status} />
      ) : (
        <Typography color="text.secondary">
          Create a host first — the agent attaches to it.
        </Typography>
      )}
      <Stack
        direction="row"
        sx={{
          alignItems: "center",
          gap: 1.5,
          pt: 2.5,
          borderTop: `1px solid ${tokens.line}`,
        }}
      >
        <Button variant="outlined" size="large" onClick={onBack}>
          Back
        </Button>
        <Box sx={{ flex: 1 }} />
        {created.compute ? (
          <Button
            component={RouterLink}
            to={`/compute/${created.compute.id}`}
            variant="contained"
            size="large"
          >
            Open host
          </Button>
        ) : (
          <Button
            component={RouterLink}
            to="/"
            variant="contained"
            size="large"
          >
            Done
          </Button>
        )}
      </Stack>
    </Stack>
  );
}
