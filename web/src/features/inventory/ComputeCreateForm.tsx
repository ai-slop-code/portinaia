import type { FormEvent } from "react";
import {
  Button,
  Card,
  CardContent,
  MenuItem,
  Stack,
  TextField,
  Typography,
} from "@mui/material";
import type { components } from "../../api/generated";

type Compute = components["schemas"]["Compute"];
type Location = components["schemas"]["Location"];
type Provider = components["schemas"]["Provider"];

type Props = {
  providers: Provider[];
  locations: Location[];
  computes: Compute[];
  onSubmit: (event: FormEvent<HTMLFormElement>) => void;
};

export function ComputeCreateForm({
  providers,
  locations,
  computes,
  onSubmit,
}: Props) {
  return (
    <Card>
      <CardContent>
        <Typography variant="h6" gutterBottom>
          Add compute
        </Typography>
        <Stack
          component="form"
          data-testid="compute-create-form"
          spacing={2}
          onSubmit={onSubmit}
        >
          <TextField
            name="display_name"
            label="Name"
            required
            slotProps={{ htmlInput: { maxLength: 200 } }}
          />
          <TextField
            name="kind"
            label="Kind"
            select
            defaultValue="physical"
            required
          >
            <MenuItem value="physical">Physical</MenuItem>
            <MenuItem value="vps">VPS</MenuItem>
            <MenuItem value="vm">VM</MenuItem>
          </TextField>
          <TextField
            name="ownership"
            label="Ownership"
            select
            defaultValue="owned"
            required
          >
            <MenuItem value="owned">Owned</MenuItem>
            <MenuItem value="rented">Rented</MenuItem>
            <MenuItem value="other">Other</MenuItem>
          </TextField>
          <TextField name="provider_id" label="Provider" select defaultValue="">
            <MenuItem value="">No provider</MenuItem>
            {providers.map((provider) => (
              <MenuItem key={provider.id} value={provider.id}>
                {provider.name}
              </MenuItem>
            ))}
          </TextField>
          <TextField
            name="provider_resource_id"
            label="Provider resource reference"
          />
          <TextField name="location_id" label="Location" select defaultValue="">
            <MenuItem value="">No location</MenuItem>
            {locations.map((location) => (
              <MenuItem key={location.id} value={location.id}>
                {location.name}
              </MenuItem>
            ))}
          </TextField>
          <TextField
            name="parent_compute_id"
            label="Parent compute (VM only)"
            select
            defaultValue=""
          >
            <MenuItem value="">No parent</MenuItem>
            {computes.map((compute) => (
              <MenuItem key={compute.id} value={compute.id}>
                {compute.display_name}
              </MenuItem>
            ))}
          </TextField>
          <TextField name="description" label="Description" multiline />
          <TextField name="notes" label="Notes" multiline />
          <Button
            type="submit"
            variant="contained"
            sx={{ alignSelf: "flex-start" }}
          >
            Create compute
          </Button>
        </Stack>
      </CardContent>
    </Card>
  );
}
