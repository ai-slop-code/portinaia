import { useEffect, useMemo, useState } from "react";
import {
  Alert,
  Button,
  Chip,
  MenuItem,
  Paper,
  Stack,
  TextField,
  Typography,
} from "@mui/material";
import { apiClient } from "../../api/client";
import type { components } from "../../api/generated";

type AuditEvent = components["schemas"]["AuditEvent"];
type AuditPage = components["schemas"]["AuditEventPage"];
type AuditFilters = {
  from_time: string;
  to_time: string;
  actor_type: string;
  actor_id: string;
  action: string;
  resource_type: string;
  resource_id: string;
};

const emptyFilters: AuditFilters = {
  from_time: "",
  to_time: "",
  actor_type: "",
  actor_id: "",
  action: "",
  resource_type: "",
  resource_id: "",
};

type Props = {
  /** A fixture is deliberately labeled; use only before the B1 route is wired. */
  fixture?: AuditPage;
  fixtureLabel?: string;
};

export function AuditWorkspace({ fixture, fixtureLabel }: Props) {
  const [filters, setFilters] = useState(emptyFilters);
  const [page, setPage] = useState<AuditPage | null>(fixture ?? null);
  const [cursor, setCursor] = useState<string | undefined>();
  const [error, setError] = useState("");
  const [unavailable, setUnavailable] = useState(false);
  const [loading, setLoading] = useState(false);
  const [retry, setRetry] = useState(0);
  const query = useMemo(
    () => ({
      "page[size]": 50,
      ...(cursor ? { "page[after]": cursor } : {}),
      ...Object.fromEntries(
        Object.entries(filters)
          .filter(([, value]) => value)
          .map(([key, value]) => [key, utcFilterValue(key, value)]),
      ),
    }),
    [cursor, filters],
  );

  useEffect(() => {
    if (fixture) {
      setPage(fixture);
      return;
    }
    let current = true;
    setLoading(true);
    void apiClient
      .GET("/audit-events", { params: { query } })
      .then(({ data, error: problem, response }) => {
        if (!current) return;
        if (data) {
          setPage((previous) =>
            cursor && previous
              ? { ...data, items: appendUnique(previous.items, data.items) }
              : data,
          );
          setError("");
          setUnavailable(false);
        } else {
          if (!cursor) setPage(null);
          setUnavailable(response.status === 404 || response.status === 503);
          setError(
            problem
              ? "Audit history is unavailable. Retry after the audit service is ready."
              : "Could not load audit history.",
          );
        }
        setLoading(false);
      })
      .catch(() => {
        if (current) {
          setLoading(false);
          if (!cursor) setPage(null);
          setUnavailable(true);
          setError(
            "Audit history is unavailable. Check the server connection and retry.",
          );
        }
      });
    return () => {
      current = false;
    };
  }, [cursor, fixture, query, retry]);

  const changeFilter = (key: keyof AuditFilters, value: string) => {
    setPage(null);
    setFilters((previous) => ({ ...previous, [key]: value }));
    setCursor(undefined);
  };
  const setNextPage = () => {
    const next = page?.page.next_cursor;
    if (page?.page.has_more && next && next !== cursor) setCursor(next);
  };

  return (
    <Stack spacing={2}>
      <Typography component="h1" variant="h5">
        Audit history
      </Typography>
      {fixture && (
        <Alert severity="info">
          {fixtureLabel ?? "Fixture data — audit server wiring is pending."}
        </Alert>
      )}
      <Stack
        direction={{ xs: "column", md: "row" }}
        spacing={1}
        sx={{ flexWrap: "wrap" }}
      >
        <TextField
          select
          label="Actor type"
          value={filters.actor_type}
          onChange={(e) => changeFilter("actor_type", e.target.value)}
          sx={{ minWidth: 180 }}
        >
          <MenuItem value="">All actors</MenuItem>
          {(
            [
              "administrator",
              "personal_access_token",
              "agent",
              "system",
            ] as const
          ).map((item) => (
            <MenuItem key={item} value={item}>
              {item.replaceAll("_", " ")}
            </MenuItem>
          ))}
        </TextField>
        <TextField
          label="Actor ID"
          value={filters.actor_id}
          onChange={(e) => changeFilter("actor_id", e.target.value)}
        />
        <TextField
          label="Action"
          value={filters.action}
          onChange={(e) => changeFilter("action", e.target.value)}
        />
        <TextField
          label="Resource type"
          value={filters.resource_type}
          onChange={(e) => changeFilter("resource_type", e.target.value)}
        />
        <TextField
          label="Resource ID"
          value={filters.resource_id}
          onChange={(e) => changeFilter("resource_id", e.target.value)}
        />
        <TextField
          label="From"
          type="datetime-local"
          value={filters.from_time}
          onChange={(e) => changeFilter("from_time", e.target.value)}
          slotProps={{ inputLabel: { shrink: true } }}
        />
        <TextField
          label="To"
          type="datetime-local"
          value={filters.to_time}
          onChange={(e) => changeFilter("to_time", e.target.value)}
          slotProps={{ inputLabel: { shrink: true } }}
        />
      </Stack>
      {error && (
        <Alert severity={unavailable ? "warning" : "error"}>{error}</Alert>
      )}
      {error && !fixture && (
        <Button
          onClick={() => setRetry((value) => value + 1)}
          disabled={loading}
        >
          Retry audit history
        </Button>
      )}
      {loading && !page && (
        <Typography role="status">Loading audit history…</Typography>
      )}
      {(page?.items ?? []).map((event) => (
        <AuditEventCard key={event.id} event={event} />
      ))}
      {page?.items.length === 0 && (
        <Typography color="text.secondary">
          No audit events match these filters.
        </Typography>
      )}
      {page?.page.has_more && (
        <Button
          onClick={setNextPage}
          disabled={!page.page.next_cursor || loading}
          sx={{ alignSelf: "flex-start" }}
        >
          Load more audit events
        </Button>
      )}
    </Stack>
  );
}

function appendUnique(
  previous: AuditEvent[],
  next: AuditEvent[],
): AuditEvent[] {
  const seen = new Set(previous.map((event) => event.id));
  return [...previous, ...next.filter((event) => !seen.has(event.id))];
}

function utcFilterValue(key: string, value: string): string {
  if (key !== "from_time" && key !== "to_time") return value;
  const match = /^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2})(?::(\d{2}))?$/.exec(
    value,
  );
  if (match) {
    const [, year, month, day, hour, minute, second = "0"] = match;
    return new Date(
      Date.UTC(+year, +month - 1, +day, +hour, +minute, +second),
    ).toISOString();
  }
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? value : date.toISOString();
}

function AuditEventCard({ event }: { event: AuditEvent }) {
  return (
    <Paper variant="outlined" sx={{ p: 2 }}>
      <Stack spacing={1}>
        <Stack
          direction={{ xs: "column", sm: "row" }}
          spacing={1}
          sx={{ alignItems: { sm: "center" } }}
        >
          <Typography variant="subtitle1">
            {event.action} · {event.resource_type}
          </Typography>
          <Chip
            size="small"
            label={event.result}
            color={event.result === "success" ? "success" : "warning"}
          />
          <Chip
            size="small"
            variant="outlined"
            label={`Request ${event.request_id}`}
          />
        </Stack>
        <Typography variant="body2" color="text.secondary">
          {formatAuditTime(event.occurred_at)} · {event.actor_type} (
          {event.actor_id}) ·{" "}
          {event.resource_id ?? "Resource unavailable (possibly purged)"}
        </Typography>
        {event.changes.map((change, index) => (
          <Typography
            key={`${change.path}-${index}`}
            variant="body2"
            sx={{ overflowWrap: "anywhere" }}
          >
            {change.path}:{" "}
            {change.redacted
              ? "[REDACTED]"
              : `${change.before ?? "—"} → ${change.after ?? "—"}`}
          </Typography>
        ))}
      </Stack>
    </Paper>
  );
}

function formatAuditTime(value: string): string {
  const date = new Date(value);
  return Number.isNaN(date.getTime())
    ? "Timestamp unavailable"
    : `${date.toLocaleString()} (local time)`;
}
