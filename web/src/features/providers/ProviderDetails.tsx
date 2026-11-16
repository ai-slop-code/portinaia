import { useEffect, useRef, type FormEvent } from "react";
import {
  Button,
  Checkbox,
  Divider,
  FormControlLabel,
  Stack,
  TextField,
  Typography,
  Alert,
} from "@mui/material";
import type { components } from "../../api/generated";

type Provider = components["schemas"]["Provider"];

type Props = {
  provider: Provider;
  editing: boolean;
  onToggleEdit: () => void;
  onLifecycle: () => void;
  onUpdate: (event: FormEvent<HTMLFormElement>) => void;
};

const roles = [
  ["hosting_provider", "Hosting"],
  ["registrar", "Registrar"],
  ["hardware_vendor", "Hardware vendor"],
  ["other", "Other"],
] as const;

export function ProviderDetails({
  provider,
  editing,
  onToggleEdit,
  onLifecycle,
  onUpdate,
}: Props) {
  // Keep the identity that supplied the uncontrolled form fields. A parent may
  // replace `provider` while the form is open; submitting that form to the newly
  // selected resource would otherwise cross-apply edits.
  const formProviderID = useRef(provider.id);
  useEffect(() => {
    if (!editing) formProviderID.current = provider.id;
  }, [editing, provider.id]);
  const selectionChanged = editing && formProviderID.current !== provider.id;
  const submitForOriginal = (event: FormEvent<HTMLFormElement>) => {
    if (selectionChanged) {
      event.preventDefault();
      return;
    }
    onUpdate(event);
  };
  return (
    <>
      <Stack direction="row" sx={{ justifyContent: "space-between" }}>
        <Typography variant="h6">{provider.name}</Typography>
        <Stack direction="row" spacing={1}>
          {!provider.archived_at && (
            <Button onClick={onToggleEdit}>
              {editing ? "Cancel edit" : "Edit"}
            </Button>
          )}
          <Button
            color={provider.archived_at ? "primary" : "error"}
            onClick={onLifecycle}
          >
            {provider.archived_at ? "Restore" : "Archive"}
          </Button>
        </Stack>
      </Stack>
      {editing ? (
        <Stack
          component="form"
          data-testid="provider-edit-form"
          spacing={2}
          sx={{ mt: 2 }}
          onSubmit={submitForOriginal}
        >
          {selectionChanged && (
            <Alert severity="warning">
              The selected provider changed while this form was open. Cancel
              this edit and reopen the intended provider before saving.
            </Alert>
          )}
          <TextField
            name="name"
            label="Name"
            defaultValue={provider.name}
            required
          />
          <TextField
            name="website"
            label="Website"
            type="url"
            defaultValue={provider.website ?? ""}
          />
          <TextField
            name="external_account_reference"
            label="External account reference"
            defaultValue={provider.external_account_reference ?? ""}
          />
          <TextField
            name="description"
            label="Description"
            defaultValue={provider.description ?? ""}
            multiline
          />
          <TextField
            name="notes"
            label="Notes"
            defaultValue={provider.notes ?? ""}
            multiline
          />
          <Stack direction="row" sx={{ flexWrap: "wrap" }}>
            {roles.map(([name, label]) => (
              <FormControlLabel
                key={name}
                control={
                  <Checkbox
                    name={name}
                    defaultChecked={provider.roles.includes(name)}
                  />
                }
                label={label}
              />
            ))}
          </Stack>
          <Button
            type="submit"
            variant="contained"
            disabled={selectionChanged}
            sx={{ alignSelf: "flex-start" }}
          >
            Save provider
          </Button>
        </Stack>
      ) : (
        <>
          <Divider sx={{ my: 2 }} />
          <Typography color="text.secondary">
            Roles: {provider.roles.join(", ") || "None"}
          </Typography>
          {provider.website && (
            <Typography sx={{ mt: 1 }}>
              <a href={provider.website}>{provider.website}</a>
            </Typography>
          )}
          {provider.external_account_reference && (
            <Typography sx={{ mt: 1 }}>
              Account reference: {provider.external_account_reference}
            </Typography>
          )}
          {provider.description && (
            <Typography sx={{ mt: 1 }}>{provider.description}</Typography>
          )}
          {provider.notes && (
            <Typography sx={{ mt: 1 }}>Notes: {provider.notes}</Typography>
          )}
        </>
      )}
    </>
  );
}
