import { useEffect, useMemo, useState } from "react";
import { Alert, Button, MenuItem, Stack, TextField } from "@mui/material";
import { apiClient } from "../../api/client";
import type { components } from "../../api/generated";
import { mergeRegistrarOptions, type RegistrarOption } from "./domainAdapter";

type Provider = components["schemas"]["Provider"];

type Props = {
  providers: Provider[];
  value: string;
  onChange: (value: string) => void;
  label?: string;
};

export function RegistrarSelector({
  providers,
  value,
  onChange,
  label = "Registrar",
}: Props) {
  const [search, setSearch] = useState("");
  const [results, setResults] = useState<RegistrarOption[]>([]);
  const [nextCursor, setNextCursor] = useState<string>();
  const [afterCursor, setAfterCursor] = useState<string>();
  const [error, setError] = useState("");
  const [retry, setRetry] = useState(0);
  const [hydrated, setHydrated] = useState<RegistrarOption>();

  useEffect(() => {
    let active = true;
    void apiClient
      .GET("/providers", {
        params: {
          query: {
            q: search || undefined,
            "page[size]": 50,
            ...(afterCursor ? { "page[after]": afterCursor } : {}),
          },
        },
      })
      .then(({ data }) => {
        if (!active) return;
        if (data) {
          setResults((previous) => {
            const combined = afterCursor
              ? [...previous, ...data.items]
              : data.items;
            return [
              ...new Map(combined.map((item) => [item.id, item])).values(),
            ];
          });
          setNextCursor(data.page.next_cursor ?? undefined);
          setError("");
        } else {
          setError(
            "Registrar lookup is unavailable. You can retry the search.",
          );
        }
      })
      .catch(() => {
        if (active)
          setError(
            "Registrar lookup is unavailable. You can retry the search.",
          );
      });
    return () => {
      active = false;
    };
  }, [afterCursor, retry, search]);

  useEffect(() => {
    if (
      !value ||
      providers.some((provider) => provider.id === value) ||
      results.some((item) => item.id === value)
    ) {
      setHydrated(undefined);
      return;
    }
    let active = true;
    void apiClient
      .GET("/providers/{provider_id}", {
        params: { path: { provider_id: value } },
      })
      .then(({ data }) => {
        if (active && data?.roles.includes("registrar")) setHydrated(data);
      })
      .catch(() => {
        if (active) setHydrated(undefined);
      });
    return () => {
      active = false;
    };
  }, [providers, results, value]);

  const options = useMemo(
    () => mergeRegistrarOptions([...providers, ...results], value, hydrated),
    [hydrated, providers, results, value],
  );

  return (
    <Stack spacing={1}>
      <TextField
        label="Search registrars"
        value={search}
        onChange={(event) => {
          setSearch(event.target.value);
          setResults([]);
          setNextCursor(undefined);
          setAfterCursor(undefined);
        }}
      />
      <TextField
        select
        label={label}
        value={value}
        onChange={(event) => onChange(event.target.value)}
      >
        <MenuItem value="">Unknown / not tracked</MenuItem>
        {!options.some((option) => option.id === value) && value && (
          <MenuItem value={value}>Current registrar ({value})</MenuItem>
        )}
        {options.map((provider) => (
          <MenuItem key={provider.id} value={provider.id}>
            {provider.name}
          </MenuItem>
        ))}
      </TextField>
      {error && (
        <Alert
          severity="warning"
          action={
            <Button onClick={() => setRetry((count) => count + 1)}>
              Retry
            </Button>
          }
        >
          {error}
        </Alert>
      )}
      {nextCursor && (
        <Button onClick={() => setAfterCursor(nextCursor)}>
          Load more registrars
        </Button>
      )}
    </Stack>
  );
}
