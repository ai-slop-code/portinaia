import type { FormEvent } from "react";
import {
  Button,
  Card,
  CardContent,
  Checkbox,
  FormControlLabel,
  Stack,
  TextField,
  Typography,
} from "@mui/material";

type Props = {
  onSubmit: (event: FormEvent<HTMLFormElement>) => void;
};

const roles = [
  ["hosting_provider", "Hosting"],
  ["registrar", "Registrar"],
  ["hardware_vendor", "Hardware vendor"],
  ["other", "Other"],
] as const;

export function ProviderCreateForm({ onSubmit }: Props) {
  return (
    <Card>
      <CardContent>
        <Typography variant="h6" gutterBottom>
          Add provider
        </Typography>
        <Stack
          component="form"
          data-testid="provider-create-form"
          spacing={2}
          onSubmit={onSubmit}
        >
          <TextField
            name="name"
            label="Name"
            required
            slotProps={{ htmlInput: { maxLength: 200 } }}
          />
          <TextField name="website" label="Website" type="url" />
          <TextField
            name="external_account_reference"
            label="External account reference"
          />
          <TextField name="description" label="Description" multiline />
          <TextField name="notes" label="Notes" multiline />
          <Stack direction="row" sx={{ flexWrap: "wrap" }}>
            {roles.map(([name, label]) => (
              <FormControlLabel
                key={name}
                control={<Checkbox name={name} />}
                label={label}
              />
            ))}
          </Stack>
          <Button
            type="submit"
            variant="contained"
            sx={{ alignSelf: "flex-start" }}
          >
            Create provider
          </Button>
        </Stack>
      </CardContent>
    </Card>
  );
}
