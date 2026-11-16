import { useEffect, useState, type FormEvent, type ReactNode } from "react";
import { Button, MenuItem, Stack, TextField, Typography } from "@mui/material";
import type { components } from "../../api/generated";
import { apiClient } from "../../api/client";
import { domainEditSnapshot, type DomainEditSnapshot } from "./domainAdapter";
import { RegistrarSelector } from "./RegistrarSelector";
import { FactList, Mono } from "../../ui/primitives";

type Domain = components["schemas"]["Domain"];
type Provider = components["schemas"]["Provider"];

type Props = {
  domain: Domain;
  providers: Provider[];
  editing: boolean;
  onToggleEdit: () => void;
  onLifecycle: () => void;
  onUpdate: (
    event: FormEvent<HTMLFormElement>,
    snapshot: DomainEditSnapshot,
  ) => void;
};

export function DomainDetails({
  domain,
  providers,
  editing,
  onToggleEdit,
  onLifecycle,
  onUpdate,
}: Props) {
  const [editSnapshot, setEditSnapshot] = useState<DomainEditSnapshot | null>(
    null,
  );
  const [editRegistrarID, setEditRegistrarID] = useState("");
  const [hydratedRegistrar, setHydratedRegistrar] = useState<Provider | null>(
    null,
  );
  useEffect(() => {
    if (editSnapshot?.id === domain.id)
      setEditRegistrarID(editSnapshot.initialValues.registrar_provider_id);
  }, [domain.id, editSnapshot]);
  useEffect(() => {
    const registrarID = domain.registrar_provider_id;
    if (
      !registrarID ||
      providers.some((provider) => provider.id === registrarID)
    ) {
      setHydratedRegistrar(null);
      return;
    }
    let active = true;
    void apiClient
      .GET("/providers/{provider_id}", {
        params: { path: { provider_id: registrarID } },
      })
      .then(({ data }) => {
        if (active && data?.roles.includes("registrar"))
          setHydratedRegistrar(data);
      })
      .catch(() => {
        if (active) setHydratedRegistrar(null);
      });
    return () => {
      active = false;
    };
  }, [domain.registrar_provider_id, providers]);
  const availableRegistrars = [
    ...providers.filter((provider) => provider.roles.includes("registrar")),
    ...(hydratedRegistrar ? [hydratedRegistrar] : []),
  ];

  return (
    <Stack spacing={2} sx={{ width: "100%" }}>
      <Stack direction="row" sx={{ alignItems: "center" }}>
        <Stack direction="row" spacing={1}>
          {!domain.archived_at && (
            <Button
              variant="outlined"
              size="small"
              onClick={() => {
                if (editing) setEditSnapshot(null);
                else {
                  const snapshot = domainEditSnapshot(domain);
                  setEditSnapshot(snapshot);
                  setEditRegistrarID(
                    snapshot.initialValues.registrar_provider_id,
                  );
                }
                onToggleEdit();
              }}
            >
              {editing ? "Cancel" : "Edit"}
            </Button>
          )}
          <Button
            size="small"
            color={domain.archived_at ? "primary" : "error"}
            onClick={onLifecycle}
          >
            {domain.archived_at ? "Restore" : "Archive"}
          </Button>
        </Stack>
      </Stack>
      {editing ? (
        <Stack
          component="form"
          spacing={2}
          onSubmit={(event) => {
            event.preventDefault();
            if (editSnapshot?.id === domain.id) onUpdate(event, editSnapshot);
          }}
        >
          <TextField
            name="name"
            label="Domain name"
            defaultValue={editSnapshot?.initialValues.name ?? ""}
            required
          />
          <input
            type="hidden"
            name="registrar_provider_id"
            value={editRegistrarID}
          />
          <RegistrarSelector
            providers={providers}
            value={editRegistrarID}
            onChange={setEditRegistrarID}
          />
          <TextField
            name="registered_on"
            label="Registered on"
            type="date"
            defaultValue={editSnapshot?.initialValues.registered_on ?? ""}
            slotProps={{ inputLabel: { shrink: true } }}
          />
          <TextField
            name="expires_on"
            label="Expires on"
            type="date"
            defaultValue={editSnapshot?.initialValues.expires_on ?? ""}
            slotProps={{ inputLabel: { shrink: true } }}
          />
          <TextField
            name="auto_renew"
            label="Auto-renewal"
            select
            defaultValue={editSnapshot?.initialValues.auto_renew ?? "unknown"}
          >
            <MenuItem value="unknown">Unknown</MenuItem>
            <MenuItem value="true">Yes</MenuItem>
            <MenuItem value="false">No</MenuItem>
          </TextField>
          <TextField
            name="nameservers"
            label="Nameservers"
            multiline
            defaultValue={editSnapshot?.initialValues.nameservers ?? ""}
          />
          <TextField
            name="description"
            label="Description"
            defaultValue={editSnapshot?.initialValues.description ?? ""}
            multiline
          />
          <TextField
            name="notes"
            label="Notes"
            defaultValue={editSnapshot?.initialValues.notes ?? ""}
            multiline
          />
          <Button
            type="submit"
            variant="contained"
            sx={{ alignSelf: "flex-start" }}
          >
            Save domain
          </Button>
        </Stack>
      ) : (
        <FactList
          rows={[
            [
              "Registrar",
              availableRegistrars.find(
                (item) => item.id === domain.registrar_provider_id,
              )?.name ??
                (domain.registrar_provider_id
                  ? "Registrar not loaded"
                  : "Not tracked"),
            ],
            ["Registered", domain.registered_on ?? "Unknown"],
            ["Expires", domain.expires_on ?? "Unknown"],
            [
              "Auto-renew",
              domain.auto_renew == null
                ? "Unknown"
                : domain.auto_renew
                  ? "Yes"
                  : "No",
            ],
            [
              "Nameservers",
              domain.nameservers.length ? (
                <Stack key="ns" spacing={0.5}>
                  {domain.nameservers.map((server) => (
                    <Mono key={server}>{server}</Mono>
                  ))}
                </Stack>
              ) : (
                "Not tracked"
              ),
            ],
            ...(domain.description
              ? ([["Description", domain.description]] as [string, string][])
              : []),
            ...(domain.notes
              ? ([
                  [
                    "Notes",
                    <Typography
                      key="n"
                      sx={{ whiteSpace: "pre-wrap", fontSize: 14 }}
                    >
                      {domain.notes}
                    </Typography>,
                  ],
                ] as [string, ReactNode][])
              : []),
          ]}
        />
      )}
    </Stack>
  );
}
