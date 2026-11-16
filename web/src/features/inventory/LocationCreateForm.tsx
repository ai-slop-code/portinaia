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

type Location = components["schemas"]["Location"];
type Provider = components["schemas"]["Provider"];

type Props = {
  locations: Location[];
  providers: Provider[];
  onSubmit: (event: FormEvent<HTMLFormElement>) => void;
};

export function LocationCreateForm({ locations, providers, onSubmit }: Props) {
  return (
    <Card>
      <CardContent>
        <Typography variant="h6" gutterBottom>
          Add location
        </Typography>
        <Stack
          component="form"
          data-testid="location-create-form"
          spacing={2}
          onSubmit={onSubmit}
        >
          <TextField
            name="type"
            label="Type"
            select
            defaultValue="site"
            required
          >
            <MenuItem value="site">Site</MenuItem>
            <MenuItem value="provider_region">Provider region</MenuItem>
          </TextField>
          <TextField
            name="name"
            label="Name"
            required
            slotProps={{ htmlInput: { maxLength: 200 } }}
          />
          <TextField name="provider_id" label="Provider" select defaultValue="">
            <MenuItem value="">No provider</MenuItem>
            {providers.map((provider) => (
              <MenuItem key={provider.id} value={provider.id}>
                {provider.name}
              </MenuItem>
            ))}
          </TextField>
          <TextField
            name="parent_id"
            label="Parent location"
            select
            defaultValue=""
          >
            <MenuItem value="">No parent</MenuItem>
            {locations.map((location) => (
              <MenuItem key={location.id} value={location.id}>
                {location.name}
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
            Create location
          </Button>
        </Stack>
      </CardContent>
    </Card>
  );
}
