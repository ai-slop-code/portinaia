import { useEffect, useRef, type FormEvent } from "react";
import { Button, MenuItem, Stack, TextField, Alert } from "@mui/material";
import type { components } from "../../api/generated";
import { FactList } from "../../ui/primitives";

type Location = components["schemas"]["Location"];
type Provider = components["schemas"]["Provider"];

type Props = {
  location: Location;
  providers: Provider[];
  locations: Location[];
  editing: boolean;
  onToggleEdit: () => void;
  onLifecycle: () => void;
  onUpdate: (event: FormEvent<HTMLFormElement>) => void;
};

export function LocationDetails({
  location,
  providers,
  locations,
  editing,
  onToggleEdit,
  onLifecycle,
  onUpdate,
}: Props) {
  const formLocationID = useRef(location.id);
  useEffect(() => {
    if (!editing) formLocationID.current = location.id;
  }, [editing, location.id]);
  const selectionChanged = editing && formLocationID.current !== location.id;
  const submitForOriginal = (event: FormEvent<HTMLFormElement>) => {
    if (selectionChanged) {
      event.preventDefault();
      return;
    }
    onUpdate(event);
  };
  return (
    <>
      <div>
        <Stack direction="row" sx={{ justifyContent: "space-between" }}>
          <span />
          <Stack direction="row" spacing={1}>
            {!location.archived_at && (
              <Button onClick={onToggleEdit}>
                {editing ? "Cancel edit" : "Edit"}
              </Button>
            )}
            <Button
              color={location.archived_at ? "primary" : "error"}
              onClick={onLifecycle}
            >
              {location.archived_at ? "Restore" : "Archive"}
            </Button>
          </Stack>
        </Stack>
        {editing ? (
          <Stack
            component="form"
            spacing={2}
            sx={{ mt: 2 }}
            onSubmit={submitForOriginal}
          >
            {selectionChanged && (
              <Alert severity="warning">
                The selected location changed while this form was open. Cancel
                this edit and reopen the intended location before saving.
              </Alert>
            )}
            <TextField
              name="type"
              label="Type"
              select
              defaultValue={location.type}
            >
              <MenuItem value="site">Site</MenuItem>
              <MenuItem value="provider_region">Provider region</MenuItem>
            </TextField>
            <TextField
              name="name"
              label="Name"
              defaultValue={location.name}
              required
            />
            <TextField
              name="provider_id"
              label="Provider"
              select
              defaultValue={location.provider_id ?? ""}
            >
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
              defaultValue={location.parent_id ?? ""}
            >
              <MenuItem value="">No parent</MenuItem>
              {locations
                .filter((item) => item.id !== location.id)
                .map((item) => (
                  <MenuItem key={item.id} value={item.id}>
                    {item.name}
                  </MenuItem>
                ))}
            </TextField>
            <TextField
              name="description"
              label="Description"
              defaultValue={location.description}
              multiline
            />
            <TextField
              name="notes"
              label="Notes"
              defaultValue={location.notes}
              multiline
            />
            <Button
              type="submit"
              variant="contained"
              disabled={selectionChanged}
              sx={{ alignSelf: "flex-start" }}
            >
              Save location
            </Button>
          </Stack>
        ) : (
          <FactList
            rows={[
              [
                "Kind",
                location.type === "provider_region"
                  ? "Provider region"
                  : "Own site",
              ],
              [
                "Provider",
                location.provider_id
                  ? (providers.find((item) => item.id === location.provider_id)
                      ?.name ?? "Not loaded")
                  : "None",
              ],
              [
                "Inside of",
                location.parent_id
                  ? (locations.find((item) => item.id === location.parent_id)
                      ?.name ?? "Not loaded")
                  : "Top level",
              ],
              ...(location.description
                ? ([["Description", location.description]] as [
                    string,
                    string,
                  ][])
                : []),
              ...(location.notes
                ? ([["Notes", location.notes]] as [string, string][])
                : []),
            ]}
          />
        )}
      </div>
    </>
  );
}
