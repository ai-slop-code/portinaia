import {
  Button,
  Card,
  CardContent,
  List,
  ListItemButton,
  ListItemText,
  Stack,
  Typography,
} from "@mui/material";
import type { components } from "../../api/generated";

type Compute = components["schemas"]["Compute"];
type Summary = components["schemas"]["InventorySummary"];

type Props = {
  computes: Compute[];
  selectedID?: string;
  summary: Summary | null;
  hasMore: boolean;
  cursor: string | null;
  onSelect: (compute: Compute) => void;
  onLoadMore: (cursor: string) => void;
};

export function ComputeOverview({
  computes,
  selectedID,
  summary,
  hasMore,
  cursor,
  onSelect,
  onLoadMore,
}: Props) {
  return (
    <Card>
      <CardContent>
        <Typography variant="h6">Compute overview</Typography>
        <Stack direction="row" spacing={3} sx={{ mt: 2, flexWrap: "wrap" }}>
          <Typography>
            Total: {summary ? summary.compute_count : "Unavailable"}
          </Typography>
          <Typography>
            Physical: {summary ? summary.physical_compute_count : "Unavailable"}
          </Typography>
          <Typography>
            VPS: {summary ? summary.vps_count : "Unavailable"}
          </Typography>
          <Typography>
            VMs: {summary ? summary.vm_count : "Unavailable"}
          </Typography>
          <Typography>
            Containers:{" "}
            {summary ? summary.observed_container_count : "Unavailable"}
          </Typography>
          <Typography>
            Stale containers:{" "}
            {summary ? summary.stale_container_count : "Unavailable"}
          </Typography>
        </Stack>
        <Typography variant="caption" color="text.secondary">
          Counts cover all active inventory; stale observations are shown
          separately.
        </Typography>
        <List disablePadding sx={{ mt: 1 }}>
          {computes.map((compute) => (
            <ListItemButton
              key={compute.id}
              selected={selectedID === compute.id}
              onClick={() => onSelect(compute)}
              sx={{ pl: compute.parent_compute_id ? 5 : 2 }}
            >
              <ListItemText
                primary={compute.display_name}
                secondary={`${compute.kind.toUpperCase()} · ${compute.ownership} · ${compute.agent_status}`}
              />
            </ListItemButton>
          ))}
          {computes.length === 0 && (
            <Typography color="text.secondary" sx={{ p: 1 }}>
              No computes yet.
            </Typography>
          )}
        </List>
        {hasMore && cursor && (
          <Button size="small" onClick={() => onLoadMore(cursor)}>
            Load more computes
          </Button>
        )}
      </CardContent>
    </Card>
  );
}
