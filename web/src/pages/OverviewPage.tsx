import { useMemo, type ReactNode } from "react";
import { Link as RouterLink } from "react-router-dom";
import { Alert, Box, Link, Stack, Typography } from "@mui/material";

import {
  useAuditEvents,
  useComputes,
  useDomains,
  useInventorySummary,
  useLocations,
  useProviders,
} from "../app/data";
import { describeAuditEvent, formatTime, relativeTime } from "../app/format";
import { useNameLookup } from "../app/useNameLookup";
import { fonts, tokens } from "../theme/theme";
import {
  ActivityIcon,
  AlertIcon,
  ComputeIcon,
  ContainerIcon,
  DomainIcon,
  ProviderIcon,
} from "../ui/icons";
import {
  Mono,
  PageHeader,
  RowLink,
  SectionTitle,
  StatTile,
  StatusLight,
  Surface,
} from "../ui/primitives";

type Attention = {
  key: string;
  tone: "warn" | "neutral";
  icon: ReactNode;
  title: ReactNode;
  subtitle: ReactNode;
  to: string;
};

const numberWords = [
  "Nothing",
  "One thing",
  "Two things",
  "Three things",
  "Four things",
  "Five things",
  "Six things",
  "Seven things",
  "Eight things",
  "Nine things",
];

function greeting(now = new Date()) {
  const hour = now.getHours();
  if (hour < 5) return "Good night";
  if (hour < 12) return "Good morning";
  if (hour < 18) return "Good afternoon";
  return "Good evening";
}

export function OverviewPage() {
  const summary = useInventorySummary();
  const computes = useComputes();
  const domains = useDomains();
  const providers = useProviders();
  const locations = useLocations();
  const audit = useAuditEvents({}, 5);
  const nameOf = useNameLookup();

  const hosts = computes.data ?? [];
  const domainList = domains.data ?? [];
  const counts = summary.data;

  const attention = useMemo<Attention[]>(() => {
    const items: Attention[] = [];
    for (const host of hosts) {
      if (host.agent_status === "offline")
        items.push({
          key: `offline-${host.id}`,
          tone: "warn",
          icon: <AlertIcon size={20} />,
          title: (
            <>
              <Mono>{host.display_name}</Mono> looks offline
            </>
          ),
          subtitle: "Its agent hasn’t checked in recently",
          to: `/compute/${host.id}`,
        });
      if (host.agent_status === "revoked")
        items.push({
          key: `revoked-${host.id}`,
          tone: "warn",
          icon: <AlertIcon size={20} />,
          title: (
            <>
              <Mono>{host.display_name}</Mono> has a revoked agent
            </>
          ),
          subtitle: "Issue a new enrollment token to resume discovery",
          to: `/compute/${host.id}`,
        });
    }
    for (const domain of domainList) {
      if (
        domain.expiry_state === "expired" ||
        domain.expiry_state === "expiring"
      )
        items.push({
          key: `expiry-${domain.id}`,
          tone: "warn",
          icon: <AlertIcon size={20} />,
          title: (
            <>
              <Mono>{domain.name}</Mono>{" "}
              {domain.expiry_state === "expired"
                ? "has expired"
                : "expires soon"}
            </>
          ),
          subtitle: domain.expires_on
            ? `Expiry date ${domain.expires_on}`
            : "Check the renewal",
          to: `/domains/${domain.id}`,
        });
    }
    if ((counts?.stale_container_count ?? 0) > 0)
      items.push({
        key: "stale-containers",
        tone: "warn",
        icon: <ContainerIcon size={20} />,
        title: `${counts?.stale_container_count} containers weren’t seen in the latest snapshot`,
        subtitle: "They may have been removed",
        to: "/compute",
      });
    const unplaced = hosts.filter(
      (host) => !host.provider_id && !host.location_id,
    );
    if (unplaced.length > 0)
      items.push({
        key: "unplaced",
        tone: "neutral",
        icon: <ProviderIcon size={20} />,
        title:
          unplaced.length === 1 ? (
            <>
              <Mono>{unplaced[0].display_name}</Mono> has no provider or place
            </>
          ) : (
            `${unplaced.length} hosts have no provider or place`
          ),
        subtitle: "Say where it runs so the map fills in",
        to: unplaced.length === 1 ? `/compute/${unplaced[0].id}` : "/compute",
      });
    const noExpiry = domainList.filter((domain) => !domain.expires_on);
    if (noExpiry.length > 0)
      items.push({
        key: "no-expiry",
        tone: "neutral",
        icon: <DomainIcon size={20} />,
        title:
          noExpiry.length === 1 ? (
            <>
              <Mono>{noExpiry[0].name}</Mono> has no expiry date
            </>
          ) : (
            `${noExpiry.length} domains have no expiry date`
          ),
        subtitle: "Add it once — renewals then show up ahead of time",
        to: noExpiry.length === 1 ? `/domains/${noExpiry[0].id}` : "/domains",
      });
    if (hosts.length === 0 && computes.isSuccess)
      items.push({
        key: "first-host",
        tone: "neutral",
        icon: <ComputeIcon size={20} />,
        title: "Add your first host",
        subtitle: "A box, a VPS or a VM — the agent can do the rest",
        to: "/add?step=compute",
      });
    return items;
  }, [hosts, domainList, counts, computes.isSuccess]);

  const shown = attention.slice(0, 4);
  const online = hosts.filter((host) => host.agent_status === "online").length;
  const offline = hosts.filter(
    (host) => host.agent_status === "offline",
  ).length;
  const enrolled = hosts.filter(
    (host) => host.agent_status === "online" || host.agent_status === "offline",
  ).length;
  const expiring = domainList.filter(
    (domain) =>
      domain.expiry_state === "expiring" || domain.expiry_state === "expired",
  ).length;
  const loadFailed = summary.isError || computes.isError || domains.isError;

  const headline = !computes.isSuccess
    ? "Checking the harbor…"
    : attention.length === 0
      ? "All quiet in the harbor."
      : `${numberWords[Math.min(attention.length, 9)]}${attention.length > 9 ? "+" : ""} could use a look.`;

  const recent = audit.data?.pages[0]?.items ?? [];

  return (
    <>
      <PageHeader
        eyebrow={greeting()}
        title={headline}
        subtitle={
          counts
            ? `${hosts.length} host${hosts.length === 1 ? "" : "s"} · ${counts.observed_container_count} container${counts.observed_container_count === 1 ? "" : "s"} seen · ${domainList.length} domain${domainList.length === 1 ? "" : "s"} owned`
            : undefined
        }
      />

      {loadFailed && (
        <Alert severity="warning">
          Some inventory couldn’t be loaded. The numbers below may be
          incomplete.
        </Alert>
      )}

      <Box
        component="section"
        aria-label="Stats"
        sx={{
          display: "grid",
          gridTemplateColumns: {
            xs: "repeat(2, minmax(0, 1fr))",
            md: "repeat(auto-fit, minmax(220px, 1fr))",
          },
          gap: 2,
        }}
      >
        <StatTile
          label="Compute"
          value={counts?.compute_count ?? "–"}
          icon={<ComputeIcon />}
          to="/compute"
          detail={
            counts
              ? `${counts.vm_count} VM · ${counts.physical_compute_count} physical · ${counts.vps_count} VPS`
              : "…"
          }
        />
        <StatTile
          label="Containers"
          value={counts?.observed_container_count ?? "–"}
          icon={<ContainerIcon />}
          to="/compute"
          detail={
            counts
              ? `${counts.observed_container_count} observed · ${counts.stale_container_count} stale`
              : "…"
          }
        />
        <StatTile
          label="Domains"
          value={domains.data ? domainList.length : "–"}
          icon={<DomainIcon />}
          to="/domains"
          detail={`${expiring} expiring or expired`}
        />
        <StatTile
          label="Agents"
          value={computes.data ? enrolled : "–"}
          icon={<ActivityIcon />}
          to="/compute"
          detail={
            <Stack direction="row" spacing={2}>
              <StatusLight light="live" label={`${online} online`} size={8} />
              <StatusLight
                light="offline"
                label={`${offline} offline`}
                size={8}
              />
            </Stack>
          }
        />
      </Box>

      <Surface component="section" aria-labelledby="path-h">
        <SectionTitle
          id="path-h"
          action={
            <Typography color="text.secondary" sx={{ fontSize: 14 }}>
              Fill the gaps left to right — only compute is required
            </Typography>
          }
        >
          From provider to container
        </SectionTitle>
        <Box
          component="ol"
          sx={{
            listStyle: "none",
            m: 0,
            p: 0,
            display: "flex",
            flexWrap: "wrap",
            gap: 1.5,
          }}
        >
          <PathStep
            index={1}
            label="Providers"
            count={providers.data?.length}
            emptyAction={{ to: "/add?step=provider", label: "Add provider" }}
            filledLink={{ to: "/providers", label: "See providers" }}
          />
          <PathStep
            index={2}
            label="Places"
            count={locations.data?.length}
            emptyAction={{ to: "/add?step=place", label: "Add place" }}
            filledLink={{ to: "/places", label: "See places" }}
          />
          <PathStep
            index={3}
            label="Compute"
            count={hosts.length}
            emptyAction={{ to: "/add?step=compute", label: "Add host" }}
            filledLink={
              hosts.length === 1
                ? {
                    to: `/compute/${hosts[0].id}`,
                    label: hosts[0].display_name,
                    mono: true,
                  }
                : { to: "/compute", label: "See hosts" }
            }
          />
          <PathStep
            index={4}
            label="Containers"
            count={counts?.observed_container_count}
            emptyAction={{ to: "/compute", label: "Connect an agent" }}
            filledLink={{ to: "/compute", label: "discovered by agents" }}
          />
        </Box>
      </Surface>

      <Stack
        direction="row"
        sx={{ flexWrap: "wrap", gap: 3, alignItems: "flex-start" }}
      >
        <Box
          component="section"
          aria-labelledby="att-h"
          sx={{ flex: "1 1 420px", minWidth: 0 }}
        >
          <SectionTitle id="att-h">Needs a look</SectionTitle>
          <Stack spacing={0.5}>
            {shown.map((item) => (
              <RowLink
                key={item.key}
                to={item.to}
                tone={item.tone}
                icon={item.icon}
                title={item.title}
                subtitle={item.subtitle}
              />
            ))}
            {computes.isSuccess && attention.length === 0 && (
              <Typography color="text.secondary" sx={{ px: 2, py: 1.5 }}>
                Nothing needs you right now.
              </Typography>
            )}
            {attention.length > shown.length && (
              <Typography color="text.secondary" sx={{ px: 2, fontSize: 14 }}>
                and {attention.length - shown.length} more
              </Typography>
            )}
          </Stack>
        </Box>

        <Box
          component="section"
          aria-labelledby="act-h"
          sx={{ flex: "1 1 380px", minWidth: 0 }}
        >
          <SectionTitle
            id="act-h"
            action={
              <Link component={RouterLink} to="/activity" underline="none">
                All activity
              </Link>
            }
          >
            Lately
          </SectionTitle>
          <Stack>
            {recent.map((event) => {
              const described = describeAuditEvent(event, nameOf);
              return (
                <Stack
                  key={event.id}
                  direction="row"
                  spacing={1.75}
                  sx={{ py: 1.5, px: 0.5, alignItems: "baseline" }}
                >
                  <Box
                    component="time"
                    dateTime={event.occurred_at}
                    title={relativeTime(event.occurred_at)}
                    sx={{
                      flex: "0 0 auto",
                      minWidth: 64,
                      whiteSpace: "nowrap",
                      color: tokens.fog,
                      fontSize: 13,
                      fontFamily: fonts.mono,
                    }}
                  >
                    {formatTime(event.occurred_at)}
                  </Box>
                  <Typography sx={{ fontSize: 15, color: tokens.mist }}>
                    {described.text}
                  </Typography>
                </Stack>
              );
            })}
            {audit.isSuccess && recent.length === 0 && (
              <Typography color="text.secondary" sx={{ py: 1.5 }}>
                No activity yet.
              </Typography>
            )}
            {audit.isError && (
              <Typography color="text.secondary" sx={{ py: 1.5 }}>
                Activity is unavailable right now.
              </Typography>
            )}
          </Stack>
        </Box>
      </Stack>
    </>
  );
}

function PathStep({
  index,
  label,
  count,
  emptyAction,
  filledLink,
}: {
  index: number;
  label: string;
  count: number | undefined;
  emptyAction: { to: string; label: string };
  filledLink: { to: string; label: string; mono?: boolean };
}) {
  const filled = (count ?? 0) > 0;
  return (
    <Box
      component="li"
      sx={{
        flex: "1 1 200px",
        display: "flex",
        flexDirection: "column",
        gap: 1.25,
        p: 2.25,
        borderRadius: "16px",
        background: filled ? tokens.deck : "transparent",
        border: `1px ${filled ? "solid" : "dashed"} ${filled ? tokens.rope : tokens.ghost}`,
      }}
    >
      <Typography sx={{ color: tokens.fog, fontSize: 13, fontWeight: 500 }}>
        {index} · {label}
      </Typography>
      <Typography
        sx={{
          fontFamily: fonts.display,
          fontWeight: 600,
          fontSize: 32,
          lineHeight: 1,
          color: filled ? tokens.signal : tokens.dim,
        }}
      >
        {count ?? "–"}
      </Typography>
      {filled ? (
        <Link
          component={RouterLink}
          to={filledLink.to}
          underline="none"
          sx={{
            fontSize: 14,
            fontWeight: 500,
            color: tokens.mist,
            fontFamily: filledLink.mono ? fonts.mono : fonts.body,
          }}
        >
          {filledLink.label}
        </Link>
      ) : (
        <Link
          component={RouterLink}
          to={emptyAction.to}
          underline="none"
          sx={{ fontSize: 14, fontWeight: 500 }}
        >
          {emptyAction.label} →
        </Link>
      )}
    </Box>
  );
}
