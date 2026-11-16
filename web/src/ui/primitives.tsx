import { Fragment, type ReactNode } from "react";
import { Link as RouterLink } from "react-router-dom";
import {
  Box,
  Stack,
  Typography,
  type SxProps,
  type Theme,
} from "@mui/material";

import { fonts, tokens } from "../theme/theme";
import { ChevronIcon } from "./icons";

export type Light = "live" | "stale" | "offline" | "failed" | "unset";

const lightLabel: Record<Light, string> = {
  live: "Live",
  stale: "Stale",
  offline: "Offline",
  failed: "Failed",
  unset: "Not set",
};

/** Status is a small light; shape differs too, so hue is never the only cue. */
export function StatusLight({
  light,
  label,
  size = 10,
}: {
  light: Light;
  label?: string;
  size?: number;
}) {
  const ring = (color: string) => `0 0 0 4px ${color}`;
  const styles: Record<Light, SxProps<Theme>> = {
    live: {
      background: tokens.signal,
      boxShadow: ring("rgba(94,234,212,.16)"),
    },
    stale: {
      background: tokens.beacon,
      boxShadow: ring("rgba(251,191,36,.16)"),
    },
    failed: {
      background: tokens.flare,
      boxShadow: ring("rgba(248,113,113,.16)"),
    },
    offline: { border: `2px solid ${tokens.fog}` },
    unset: { border: `2px dashed ${tokens.dim}`, borderRadius: "3px" },
  };
  return (
    <Box
      component="span"
      sx={{ display: "inline-flex", alignItems: "center", gap: 1 }}
    >
      <Box
        component="span"
        aria-hidden
        sx={{
          width: size,
          height: size,
          flex: "0 0 auto",
          borderRadius: "50%",
          boxSizing: "border-box",
          ...styles[light],
        }}
      />
      {label !== undefined ? (
        label
      ) : (
        <Box component="span" sx={visuallyHidden}>
          {lightLabel[light]}
        </Box>
      )}
    </Box>
  );
}

export const visuallyHidden = {
  position: "absolute",
  width: "1px",
  height: "1px",
  overflow: "hidden",
  clip: "rect(0 0 0 0)",
  whiteSpace: "nowrap",
} as const;

/** Monospace is reserved for things you could paste into a terminal. */
export function Mono({
  children,
  sx,
}: {
  children: ReactNode;
  sx?: SxProps<Theme>;
}) {
  return (
    <Box
      component="span"
      sx={{ fontFamily: fonts.mono, overflowWrap: "anywhere", ...sx }}
    >
      {children}
    </Box>
  );
}

export function PageHeader({
  eyebrow,
  title,
  subtitle,
  actions,
  mono,
}: {
  eyebrow?: ReactNode;
  title: ReactNode;
  subtitle?: ReactNode;
  actions?: ReactNode;
  mono?: boolean;
}) {
  return (
    <Stack
      direction="row"
      sx={{
        flexWrap: "wrap",
        gap: 3,
        alignItems: "flex-end",
        justifyContent: "space-between",
      }}
    >
      <Stack spacing={1.25} sx={{ minWidth: 0 }}>
        {eyebrow && (
          <Typography color="text.secondary" sx={{ fontSize: 15 }}>
            {eyebrow}
          </Typography>
        )}
        <Typography
          component="h1"
          sx={{
            m: 0,
            fontFamily: mono ? fonts.mono : fonts.display,
            fontWeight: mono ? 500 : 600,
            fontSize: { xs: 32, md: 44 },
            lineHeight: 1.1,
            letterSpacing: "-0.02em",
            overflowWrap: "anywhere",
          }}
        >
          {title}
        </Typography>
        {subtitle && (
          <Typography color="text.secondary" sx={{ fontSize: 16 }}>
            {subtitle}
          </Typography>
        )}
      </Stack>
      {actions && (
        <Stack direction="row" spacing={1.25} sx={{ flexWrap: "wrap" }}>
          {actions}
        </Stack>
      )}
    </Stack>
  );
}

export function SectionTitle({
  children,
  action,
  id,
}: {
  children: ReactNode;
  action?: ReactNode;
  id?: string;
}) {
  return (
    <Stack
      direction="row"
      sx={{
        alignItems: "baseline",
        justifyContent: "space-between",
        gap: 2,
        flexWrap: "wrap",
        mb: 1.5,
      }}
    >
      <Typography
        id={id}
        component="h2"
        sx={{ fontFamily: fonts.display, fontWeight: 600, fontSize: 20 }}
      >
        {children}
      </Typography>
      {action}
    </Stack>
  );
}

export function Surface({
  children,
  sx,
  dashed,
  component = "div",
  ...rest
}: {
  children: ReactNode;
  sx?: SxProps<Theme>;
  dashed?: boolean;
  component?: React.ElementType;
  "aria-labelledby"?: string;
  "data-testid"?: string;
}) {
  return (
    <Box
      component={component}
      {...rest}
      sx={{
        borderRadius: "20px",
        p: { xs: 2.5, md: 3 },
        background: dashed ? "transparent" : tokens.hull,
        border: `1px ${dashed ? "dashed" : "solid"} ${dashed ? tokens.ghost : tokens.line}`,
        ...sx,
      }}
    >
      {children}
    </Box>
  );
}

/** A large count with a short breakdown line. */
export function StatTile({
  label,
  value,
  detail,
  icon,
  to,
}: {
  label: string;
  value: ReactNode;
  detail: ReactNode;
  icon: ReactNode;
  to: string;
}) {
  return (
    <Box
      component={RouterLink}
      to={to}
      sx={{
        display: "flex",
        flexDirection: "column",
        gap: 1.75,
        p: { xs: 2, md: 2.75 },
        minWidth: 0,
        borderRadius: "20px",
        background: tokens.hull,
        border: `1px solid ${tokens.line}`,
        color: "inherit",
        textDecoration: "none",
        transition: "background .15s, border-color .15s",
        "&:hover": { background: tokens.deck, borderColor: tokens.rope },
      }}
    >
      <Stack
        direction="row"
        sx={{
          alignItems: "center",
          justifyContent: "space-between",
          color: tokens.fog,
        }}
      >
        <Typography sx={{ fontSize: 14, fontWeight: 500, color: tokens.fog }}>
          {label}
        </Typography>
        {icon}
      </Stack>
      <Typography
        sx={{
          fontFamily: fonts.display,
          fontWeight: 600,
          fontSize: { xs: 40, md: 52 },
          lineHeight: 1,
        }}
      >
        {value}
      </Typography>
      <Typography component="div" sx={{ fontSize: 13, color: tokens.fog }}>
        {detail}
      </Typography>
    </Box>
  );
}

/** A soft pill; `gap` renders the dashed "+ Add" variant that fills a hole. */
export function Pill({
  children,
  gap,
  to,
  onClick,
  selected,
}: {
  children: ReactNode;
  gap?: boolean;
  to?: string;
  onClick?: () => void;
  selected?: boolean;
}) {
  const sx: SxProps<Theme> = {
    display: "inline-flex",
    alignItems: "center",
    gap: 1,
    minHeight: 32,
    px: 1.75,
    borderRadius: 999,
    fontSize: 13,
    fontWeight: 500,
    fontFamily: fonts.body,
    textDecoration: "none",
    whiteSpace: "nowrap",
    cursor: to || onClick ? "pointer" : "default",
    color: gap ? tokens.fog : selected ? tokens.night : tokens.mist,
    background: gap ? "transparent" : selected ? tokens.foam : tokens.deck,
    border: gap ? `1px dashed ${tokens.ghost}` : "1px solid transparent",
    ...(to || onClick
      ? {
          "&:hover": gap
            ? { color: tokens.foam, borderColor: tokens.fog }
            : { background: selected ? tokens.foam : tokens.deckHover },
        }
      : {}),
  };
  if (to)
    return (
      <Box component={RouterLink} to={to} sx={sx}>
        {children}
      </Box>
    );
  if (onClick)
    return (
      <Box
        component="button"
        type="button"
        onClick={onClick}
        aria-pressed={selected}
        sx={sx}
      >
        {children}
      </Box>
    );
  return (
    <Box component="span" sx={sx}>
      {children}
    </Box>
  );
}

/** A full-width row that navigates: icon tile, two lines, chevron. */
export function RowLink({
  to,
  onClick,
  icon,
  title,
  subtitle,
  tone = "neutral",
  trailing,
  ariaLabel,
}: {
  to?: string;
  onClick?: () => void;
  icon?: ReactNode;
  title: ReactNode;
  subtitle?: ReactNode;
  tone?: "neutral" | "warn";
  trailing?: ReactNode;
  ariaLabel?: string;
}) {
  const content = (
    <>
      {icon && (
        <Box
          component="span"
          sx={{
            flex: "0 0 auto",
            width: 40,
            height: 40,
            borderRadius: "12px",
            display: "flex",
            alignItems: "center",
            justifyContent: "center",
            background: tone === "warn" ? tokens.beaconWash : tokens.deck,
            color: tone === "warn" ? tokens.beacon : tokens.mist,
          }}
        >
          {icon}
        </Box>
      )}
      <Box
        component="span"
        sx={{
          flex: 1,
          minWidth: 0,
          display: "flex",
          flexDirection: "column",
          gap: 0.25,
          textAlign: "left",
        }}
      >
        <Box component="span" sx={{ fontWeight: 500, fontSize: 15 }}>
          {title}
        </Box>
        {subtitle && (
          <Box component="span" sx={{ color: tokens.fog, fontSize: 13 }}>
            {subtitle}
          </Box>
        )}
      </Box>
      {trailing}
      <ChevronIcon size={18} style={{ color: tokens.fog, flex: "0 0 auto" }} />
    </>
  );
  const sx: SxProps<Theme> = {
    display: "flex",
    alignItems: "center",
    gap: 2,
    width: "100%",
    minHeight: 56,
    px: 2,
    py: 1.5,
    border: 0,
    borderRadius: "16px",
    background: "transparent",
    color: "inherit",
    font: "inherit",
    textDecoration: "none",
    cursor: "pointer",
    "&:hover": { background: tokens.deck },
  };
  if (to)
    return (
      <Box component={RouterLink} to={to} aria-label={ariaLabel} sx={sx}>
        {content}
      </Box>
    );
  return (
    <Box
      component="button"
      type="button"
      onClick={onClick}
      aria-label={ariaLabel}
      sx={sx}
    >
      {content}
    </Box>
  );
}

export function EmptyHint({
  title,
  body,
  action,
}: {
  title: ReactNode;
  body?: ReactNode;
  action?: ReactNode;
}) {
  return (
    <Surface dashed>
      <Stack spacing={1.25} sx={{ alignItems: "flex-start" }}>
        <Typography
          sx={{ fontFamily: fonts.display, fontWeight: 600, fontSize: 20 }}
        >
          {title}
        </Typography>
        {body && <Typography color="text.secondary">{body}</Typography>}
        {action}
      </Stack>
    </Surface>
  );
}

/** Two-column definition list for facts about a resource. */
export function FactList({ rows }: { rows: [string, ReactNode][] }) {
  return (
    <Box
      component="dl"
      sx={{
        m: 0,
        display: "grid",
        gridTemplateColumns: "max-content minmax(0, 1fr)",
        columnGap: 3,
        rowGap: 1.25,
        fontSize: 14,
      }}
    >
      {rows.map(([key, value]) => (
        <Fragment key={key}>
          <Box component="dt" sx={{ color: tokens.fog }}>
            {key}
          </Box>
          <Box component="dd" sx={{ m: 0, overflowWrap: "anywhere" }}>
            {value}
          </Box>
        </Fragment>
      ))}
    </Box>
  );
}
