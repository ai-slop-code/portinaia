import { Fragment, useMemo, useState } from "react";
import { Alert, Box, Button, Stack, Typography } from "@mui/material";

import {
  useAuditEvents,
  type AuditEvent,
  type AuditQuery,
} from "../../app/data";
import {
  dayHeading,
  describeAuditEvent,
  formatTime,
  type ActivityCategory,
} from "../../app/format";
import { useNameLookup } from "../../app/useNameLookup";
import { fonts, tokens } from "../../theme/theme";

const dotColor: Record<ActivityCategory, string> = {
  discovery: tokens.signal,
  agents: tokens.mist,
  inventory: tokens.fog,
};

/**
 * Audit history as plain sentences grouped by day. Each entry opens to show the
 * raw record: action, actor, request and field changes (redactions kept).
 */
export function ActivityFeed({
  query,
  category,
  onlyMine,
  emptyText = "Nothing here yet.",
}: {
  query: AuditQuery;
  category?: ActivityCategory;
  onlyMine?: boolean;
  emptyText?: string;
}) {
  const audit = useAuditEvents(query);
  const nameOf = useNameLookup();
  const [open, setOpen] = useState<string | null>(null);

  const events = useMemo(() => {
    const all = (audit.data?.pages ?? []).flatMap((page) => page.items);
    const seen = new Set<string>();
    return all
      .filter((event) => (seen.has(event.id) ? false : seen.add(event.id)))
      .map((event) => ({ event, ...describeAuditEvent(event, nameOf) }))
      .filter((item) => !category || item.category === category)
      .filter((item) => !onlyMine || item.event.actor_type === "administrator");
  }, [audit.data, nameOf, category, onlyMine]);

  let lastDay = "";
  return (
    <Stack spacing={0.5}>
      {audit.isError && (
        <Alert
          severity="warning"
          action={<Button onClick={() => void audit.refetch()}>Retry</Button>}
        >
          Activity is unavailable right now.
        </Alert>
      )}
      {audit.isPending && (
        <Typography color="text.secondary" role="status">
          Loading activity…
        </Typography>
      )}
      {events.map(({ event, text, who, category: kind }) => {
        const day = dayHeading(event.occurred_at);
        const showDay = day !== lastDay;
        lastDay = day;
        const expanded = open === event.id;
        return (
          <Fragment key={event.id}>
            {showDay && (
              <Typography
                component="h3"
                sx={{
                  pt: 2,
                  pb: 1,
                  fontFamily: fonts.mono,
                  fontSize: 13,
                  fontWeight: 500,
                  letterSpacing: "0.08em",
                  color: tokens.fog,
                  textTransform: "uppercase",
                }}
              >
                {day}
              </Typography>
            )}
            <Box
              sx={{
                borderRadius: "16px",
                background: expanded ? tokens.hull : "transparent",
              }}
            >
              <Box
                component="button"
                type="button"
                aria-expanded={expanded}
                onClick={() => setOpen(expanded ? null : event.id)}
                sx={{
                  width: "100%",
                  display: "flex",
                  alignItems: "center",
                  gap: 2,
                  px: 2,
                  py: 1.75,
                  minHeight: 56,
                  border: 0,
                  borderRadius: "16px",
                  background: "transparent",
                  color: "inherit",
                  font: "inherit",
                  textAlign: "left",
                  cursor: "pointer",
                  "&:hover": { background: tokens.hull },
                }}
              >
                <Box
                  component="time"
                  dateTime={event.occurred_at}
                  sx={{
                    flex: "0 0 auto",
                    minWidth: 56,
                    whiteSpace: "nowrap",
                    color: tokens.fog,
                    fontFamily: fonts.mono,
                    fontSize: 13,
                  }}
                >
                  {formatTime(event.occurred_at)}
                </Box>
                <Box
                  component="span"
                  aria-hidden
                  sx={{
                    flex: "0 0 auto",
                    width: 8,
                    height: 8,
                    borderRadius: "50%",
                    background:
                      event.result === "success"
                        ? dotColor[kind]
                        : tokens.beacon,
                  }}
                />
                <Box
                  component="span"
                  sx={{
                    flex: 1,
                    minWidth: 0,
                    display: "flex",
                    flexDirection: "column",
                    gap: 0.25,
                  }}
                >
                  <Box component="span" sx={{ fontSize: 15 }}>
                    {text}
                  </Box>
                  <Box
                    component="span"
                    sx={{ color: tokens.fog, fontSize: 13 }}
                  >
                    {who}
                  </Box>
                </Box>
                <Box component="span" sx={{ color: tokens.fog, fontSize: 13 }}>
                  {expanded ? "Hide" : "Details"}
                </Box>
              </Box>
              {expanded && <RawRecord event={event} />}
            </Box>
          </Fragment>
        );
      })}
      {audit.isSuccess && events.length === 0 && (
        <Typography color="text.secondary" sx={{ py: 3, px: 2 }}>
          {emptyText}
        </Typography>
      )}
      {audit.hasNextPage && (
        <Button
          onClick={() => void audit.fetchNextPage()}
          disabled={audit.isFetchingNextPage}
          sx={{ alignSelf: "flex-start", mt: 1 }}
        >
          {audit.isFetchingNextPage ? "Loading…" : "Load older activity"}
        </Button>
      )}
    </Stack>
  );
}

function RawRecord({ event }: { event: AuditEvent }) {
  const rows: [string, string][] = [
    ["action", event.action],
    ["result", event.result],
    ["actor", `${event.actor_type} ${event.actor_id}`],
    [
      "resource",
      `${event.resource_type} ${event.resource_id ?? "(unavailable)"}`,
    ],
    ["request", event.request_id],
    ["at", new Date(event.occurred_at).toLocaleString()],
    ...event.changes.map(
      (change) =>
        [
          change.path,
          change.redacted
            ? "[redacted]"
            : `${change.before ?? "—"} → ${change.after ?? "—"}`,
        ] as [string, string],
    ),
  ];
  return (
    <Box
      component="dl"
      sx={{
        m: 0,
        px: 2,
        pb: 2.25,
        pl: { xs: 2, sm: 13 },
        display: "grid",
        gridTemplateColumns: "max-content minmax(0, 1fr)",
        columnGap: 2.25,
        rowGap: 1,
        fontFamily: fonts.mono,
        fontSize: 13,
      }}
    >
      {rows.map(([key, value], index) => (
        <Fragment key={`${key}-${index}`}>
          <Box component="dt" sx={{ color: tokens.fog }}>
            {key}
          </Box>
          <Box
            component="dd"
            sx={{ m: 0, color: tokens.mist, overflowWrap: "anywhere" }}
          >
            {value}
          </Box>
        </Fragment>
      ))}
    </Box>
  );
}
