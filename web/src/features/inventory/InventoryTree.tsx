import {
  Button,
  Card,
  CardContent,
  List,
  ListItemButton,
  ListItemText,
  Typography,
} from "@mui/material";
import { useRef } from "react";
import type { components } from "../../api/generated";

type TreeNode = components["schemas"]["InventoryTreeNode"];

type InventoryTreeProps = {
  roots: TreeNode[];
  childrenByID: Record<string, TreeNode[]>;
  expanded: Set<string>;
  childHasMore: Record<string, boolean>;
  childCursors: Record<string, string | null>;
  rootHasMore: boolean;
  rootCursor: string | null;
  onToggle: (node: TreeNode) => void;
  onLoadChildren: (parentID: string, cursor?: string) => void;
  onLoadRoots: (cursor?: string) => void;
  onNavigate?: (node: TreeNode) => void;
};

export function InventoryTree({
  roots,
  childrenByID,
  expanded,
  childHasMore,
  childCursors,
  rootHasMore,
  rootCursor,
  onToggle,
  onLoadChildren,
  onLoadRoots,
  onNavigate,
}: InventoryTreeProps) {
  const requestedPages = useRef(new Set<string>());
  const continueOnce = (
    key: string,
    cursor: string | null | undefined,
    load: () => void,
  ) => {
    if (!cursor) return;
    const requestKey = `${key}:${cursor}`;
    if (requestedPages.current.has(requestKey)) return;
    requestedPages.current.add(requestKey);
    load();
  };
  const renderNode = (node: TreeNode, depth = 0): React.ReactNode => (
    <div key={node.id}>
      <ListItemButton
        onClick={() => (onNavigate ? onNavigate(node) : onToggle(node))}
        onDoubleClick={() => onToggle(node)}
        aria-label={`${onNavigate ? "Open" : "Expand"} ${node.name}`}
        sx={{ pl: 2 + depth * 3 }}
      >
        <ListItemText
          primary={`${node.has_children ? (expanded.has(node.id) ? "▾ " : "▸ ") : "　"}${node.name}`}
          secondary={`${node.kind}${node.archived ? " · archived" : ""}`}
        />
      </ListItemButton>
      {expanded.has(node.id) &&
        (childrenByID[node.id] ?? []).map((child) =>
          renderNode(child, depth + 1),
        )}
      {expanded.has(node.id) && childHasMore[node.id] && (
        <Button
          size="small"
          sx={{ ml: 2 + depth * 3 }}
          onClick={() => {
            const cursor = childCursors[node.id];
            continueOnce(`child:${node.id}`, cursor, () =>
              onLoadChildren(node.id, cursor ?? undefined),
            );
          }}
        >
          Load more
        </Button>
      )}
    </div>
  );

  return (
    <Card sx={{ mt: 3 }}>
      <CardContent>
        <Typography variant="h6">Inventory hierarchy</Typography>
        <List disablePadding sx={{ mt: 1 }}>
          {roots.map((node) => renderNode(node))}
        </List>
        {rootHasMore && rootCursor && (
          <Button
            size="small"
            onClick={() =>
              continueOnce("root", rootCursor, () =>
                onLoadRoots(rootCursor ?? undefined),
              )
            }
          >
            Load more roots
          </Button>
        )}
      </CardContent>
    </Card>
  );
}
