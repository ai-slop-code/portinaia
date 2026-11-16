import { useState, type FormEvent } from "react";
import { Button, MenuItem, Stack, TextField, Typography } from "@mui/material";
import type { components } from "../../api/generated";
import { RegistrarSelector } from "./RegistrarSelector";

type Provider = components["schemas"]["Provider"];

type Props = {
  providers: Provider[];
  onSubmit: (event: FormEvent<HTMLFormElement>) => void;
};

export function DomainCreateForm({ providers, onSubmit }: Props) {
  const [registrar, setRegistrar] = useState("");
  return (
    <Stack spacing={2} sx={{ width: "100%" }}>
      <Typography variant="subtitle1">Add domain</Typography>
      <Stack
        component="form"
        data-testid="domain-create-form"
        spacing={2}
        onSubmit={onSubmit}
      >
        <TextField
          name="name"
          label="Domain name"
          placeholder="example.com"
          required
        />
        <input type="hidden" name="registrar_provider_id" value={registrar} />
        <RegistrarSelector
          providers={providers}
          value={registrar}
          onChange={setRegistrar}
        />
        <Stack direction={{ xs: "column", sm: "row" }} spacing={2}>
          <TextField
            name="registered_on"
            label="Registered on"
            type="date"
            slotProps={{ inputLabel: { shrink: true } }}
          />
          <TextField
            name="expires_on"
            label="Expires on"
            type="date"
            slotProps={{ inputLabel: { shrink: true } }}
          />
        </Stack>
        <TextField
          name="auto_renew"
          label="Auto-renewal"
          select
          defaultValue="unknown"
        >
          <MenuItem value="unknown">Unknown</MenuItem>
          <MenuItem value="true">Yes</MenuItem>
          <MenuItem value="false">No</MenuItem>
        </TextField>
        <TextField
          name="nameservers"
          label="Nameservers"
          multiline
          helperText="Optional; one per line."
        />
        <TextField name="description" label="Description" multiline />
        <TextField name="notes" label="Notes" multiline />
        <Button
          type="submit"
          variant="contained"
          sx={{ alignSelf: "flex-start" }}
        >
          Add domain
        </Button>
      </Stack>
    </Stack>
  );
}
