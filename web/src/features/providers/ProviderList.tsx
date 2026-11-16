import {
  Button,
  Card,
  CardContent,
  List,
  ListItemButton,
  ListItemText,
  TextField,
  Typography,
} from "@mui/material";
import type { components } from "../../api/generated";

type Provider = components["schemas"]["Provider"];

type Props = {
  providers: Provider[];
  selectedID?: string;
  query: string;
  hasMore: boolean;
  cursor: string | null;
  onQueryChange: (value: string) => void;
  onSelect: (provider: Provider) => void;
  onLoadMore: (cursor: string) => void;
};

export function ProviderList({
  providers,
  selectedID,
  query,
  hasMore,
  cursor,
  onQueryChange,
  onSelect,
  onLoadMore,
}: Props) {
  return (
    <Card sx={{ width: { xs: "100%", md: 380 } }}>
      <CardContent>
        <Typography variant="h6">Providers</Typography>
        <TextField
          fullWidth
          size="small"
          label="Search providers"
          value={query}
          onChange={(event) => onQueryChange(event.target.value)}
          sx={{ my: 2 }}
        />
        <List disablePadding>
          {providers.map((provider) => (
            <ListItemButton
              key={provider.id}
              selected={selectedID === provider.id}
              onClick={() => onSelect(provider)}
            >
              <ListItemText
                primary={provider.name}
                secondary={provider.roles.join(", ") || "No roles"}
              />
            </ListItemButton>
          ))}
          {providers.length === 0 && (
            <Typography color="text.secondary" sx={{ p: 1 }}>
              No providers yet.
            </Typography>
          )}
        </List>
        {hasMore && cursor && (
          <Button size="small" onClick={() => onLoadMore(cursor)}>
            Load more providers
          </Button>
        )}
      </CardContent>
    </Card>
  );
}
