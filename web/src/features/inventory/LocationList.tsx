import {
  Button,
  Card,
  CardContent,
  List,
  ListItemButton,
  ListItemText,
  Typography,
} from "@mui/material";
import type { components } from "../../api/generated";

type Location = components["schemas"]["Location"];

type Props = {
  locations: Location[];
  hasMore: boolean;
  cursor: string | null;
  onSelect: (location: Location) => void;
  onLoadMore: (cursor: string) => void;
};

export function LocationList({
  locations,
  hasMore,
  cursor,
  onSelect,
  onLoadMore,
}: Props) {
  return (
    <Card>
      <CardContent>
        <Typography variant="h6">Locations</Typography>
        <List disablePadding>
          {locations.map((location) => (
            <ListItemButton
              key={location.id}
              onClick={() => onSelect(location)}
            >
              <ListItemText primary={location.name} secondary={location.type} />
            </ListItemButton>
          ))}
          {locations.length === 0 && (
            <Typography color="text.secondary" sx={{ p: 1 }}>
              No locations yet.
            </Typography>
          )}
        </List>
        {hasMore && cursor && (
          <Button size="small" onClick={() => onLoadMore(cursor)}>
            Load more locations
          </Button>
        )}
      </CardContent>
    </Card>
  );
}
