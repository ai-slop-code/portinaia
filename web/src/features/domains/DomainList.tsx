import {
  Button,
  List,
  ListItemButton,
  ListItemText,
  Typography,
} from "@mui/material";
import type { components } from "../../api/generated";

type Domain = components["schemas"]["Domain"];

type Props = {
  domains: Domain[];
  selectedID?: string;
  hasMore: boolean;
  cursor: string | null;
  onSelect: (domain: Domain) => void;
  onLoadMore: (cursor: string) => void;
};

export function DomainList({
  domains,
  selectedID,
  hasMore,
  cursor,
  onSelect,
  onLoadMore,
}: Props) {
  return (
    <List disablePadding sx={{ width: { xs: "100%", lg: 300 } }}>
      {domains.map((domain) => (
        <ListItemButton
          key={domain.id}
          selected={selectedID === domain.id}
          onClick={() => onSelect(domain)}
        >
          <ListItemText
            primary={domain.name}
            secondary={`${domain.expiry_state}${domain.archived_at ? " · archived" : ""}`}
          />
        </ListItemButton>
      ))}
      {domains.length === 0 && (
        <Typography color="text.secondary" sx={{ p: 1 }}>
          No domains yet.
        </Typography>
      )}
      {hasMore && cursor && (
        <Button size="small" onClick={() => onLoadMore(cursor)}>
          Load more domains
        </Button>
      )}
    </List>
  );
}
