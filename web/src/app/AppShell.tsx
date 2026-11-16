import { useEffect, useMemo, useState, type ReactNode } from "react";
import { NavLink, Link as RouterLink, useNavigate } from "react-router-dom";
import {
  Autocomplete,
  Box,
  Button,
  InputBase,
  Menu,
  MenuItem,
  Stack,
  Typography,
} from "@mui/material";

import { fonts, tokens } from "../theme/theme";
import {
  ActivityIcon,
  BrandMark,
  ComputeIcon,
  DomainIcon,
  OverviewIcon,
  PlaceIcon,
  PlusIcon,
  ProviderIcon,
  SearchIcon,
} from "../ui/icons";
import { useComputes, useDomains, useLocations, useProviders } from "./data";

const navItems = [
  { to: "/", label: "Overview", icon: <OverviewIcon size={22} />, end: true },
  { to: "/domains", label: "Domains", icon: <DomainIcon size={22} /> },
  { to: "/compute", label: "Compute", icon: <ComputeIcon size={22} /> },
  { to: "/providers", label: "Providers", icon: <ProviderIcon size={22} /> },
  { to: "/places", label: "Places", icon: <PlaceIcon size={22} /> },
];

function RailLink({
  to,
  label,
  icon,
  end,
}: {
  to: string;
  label: string;
  icon: ReactNode;
  end?: boolean;
}) {
  return (
    <Box
      component={NavLink}
      to={to}
      end={end}
      sx={{
        display: "flex",
        flexDirection: "column",
        alignItems: "center",
        justifyContent: "center",
        gap: 0.75,
        minHeight: 62,
        width: 72,
        flex: "0 0 auto",
        borderRadius: "14px",
        color: tokens.fog,
        fontSize: 11,
        fontWeight: 500,
        textDecoration: "none",
        "&:hover, &.active": { background: tokens.deck, color: tokens.foam },
      }}
    >
      {icon}
      {label}
    </Box>
  );
}

type SearchOption = {
  id: string;
  label: string;
  kind: "Host" | "Domain" | "Provider" | "Place";
  to: string;
  mono: boolean;
};

function GlobalSearch() {
  const navigate = useNavigate();
  const computes = useComputes();
  const domains = useDomains();
  const providers = useProviders();
  const locations = useLocations();
  const [input, setInput] = useState("");

  const options = useMemo<SearchOption[]>(
    () => [
      ...(computes.data ?? []).map((item) => ({
        id: item.id,
        label: item.display_name,
        kind: "Host" as const,
        to: `/compute/${item.id}`,
        mono: true,
      })),
      ...(domains.data ?? []).map((item) => ({
        id: item.id,
        label: item.name,
        kind: "Domain" as const,
        to: `/domains/${item.id}`,
        mono: true,
      })),
      ...(providers.data ?? []).map((item) => ({
        id: item.id,
        label: item.name,
        kind: "Provider" as const,
        to: `/providers/${item.id}`,
        mono: false,
      })),
      ...(locations.data ?? []).map((item) => ({
        id: item.id,
        label: item.name,
        kind: "Place" as const,
        to: `/places/${item.id}`,
        mono: false,
      })),
    ],
    [computes.data, domains.data, providers.data, locations.data],
  );

  useEffect(() => {
    const focus = (event: KeyboardEvent) => {
      const target = event.target as HTMLElement | null;
      const typing =
        target?.isContentEditable ||
        ["INPUT", "TEXTAREA", "SELECT"].includes(target?.tagName ?? "");
      if (event.key === "/" && !typing) {
        event.preventDefault();
        document.getElementById("global-search")?.focus();
      }
    };
    window.addEventListener("keydown", focus);
    return () => window.removeEventListener("keydown", focus);
  }, []);

  return (
    <Autocomplete
      id="global-search"
      options={options}
      groupBy={(option) => option.kind}
      getOptionLabel={(option) => option.label}
      inputValue={input}
      onInputChange={(_, value) => setInput(value)}
      value={null}
      onChange={(_, option) => {
        if (option) {
          setInput("");
          void navigate(option.to);
        }
      }}
      open={input.trim().length > 0}
      noOptionsText="Nothing matches"
      forcePopupIcon={false}
      clearOnBlur
      blurOnSelect
      sx={{ flex: "1 1 320px", maxWidth: 620, mx: "auto" }}
      slotProps={{
        paper: { sx: { mt: 1, borderRadius: "16px" } },
      }}
      renderOption={({ key, ...props }, option) => (
        <li key={key} {...props}>
          <Box
            component="span"
            sx={{
              fontFamily: option.mono ? fonts.mono : fonts.body,
              fontSize: 14,
            }}
          >
            {option.label}
          </Box>
        </li>
      )}
      renderInput={(params) => (
        <Box
          ref={params.slotProps.input.ref}
          sx={{
            display: "flex",
            alignItems: "center",
            gap: 1.25,
            height: 46,
            pl: 2.25,
            pr: 1,
            borderRadius: 999,
            background: tokens.hull,
            border: `1px solid ${tokens.rope}`,
            "&:focus-within": { borderColor: tokens.signal },
          }}
        >
          <SearchIcon size={18} style={{ color: tokens.fog }} />
          <InputBase
            inputProps={{
              ...params.slotProps.htmlInput,
              "aria-label": "Search hosts, domains, providers and places",
            }}
            placeholder="Search hosts, domains, providers, places"
            sx={{ flex: 1, fontSize: 15 }}
          />
          <Box
            component="kbd"
            sx={{
              display: { xs: "none", md: "inline-block" },
              fontFamily: fonts.mono,
              fontSize: 12,
              color: tokens.fog,
              border: `1px solid ${tokens.rope}`,
              borderRadius: "6px",
              px: 1,
              py: 0.25,
            }}
          >
            /
          </Box>
        </Box>
      )}
    />
  );
}

export function AppShell({
  username,
  onSignOut,
  children,
}: {
  username: string;
  onSignOut: () => void;
  children: ReactNode;
}) {
  const [menuAnchor, setMenuAnchor] = useState<HTMLElement | null>(null);
  return (
    <Box
      sx={{
        display: "flex",
        flexWrap: "wrap",
        minHeight: "100vh",
        background: tokens.night,
      }}
    >
      <Box
        component="nav"
        aria-label="Main"
        sx={{
          display: "flex",
          flexDirection: { xs: "row", md: "column" },
          alignItems: "center",
          gap: 0.5,
          width: { xs: "100%", md: 96 },
          p: { xs: 1, md: "20px 12px" },
          boxSizing: "border-box",
          borderRight: { md: `1px solid ${tokens.line}` },
          borderBottom: { xs: `1px solid ${tokens.line}`, md: 0 },
          overflowX: { xs: "auto", md: "visible" },
          flex: "0 0 auto",
          position: { md: "sticky" },
          top: 0,
          alignSelf: "flex-start",
          minHeight: { md: "100vh" },
        }}
      >
        <Box
          component={RouterLink}
          to="/"
          aria-label="Portinaia home"
          sx={{ display: { xs: "none", md: "flex" }, mb: 2.5 }}
        >
          <BrandMark />
        </Box>
        {navItems.map((item) => (
          <RailLink key={item.to} {...item} />
        ))}
        <Box sx={{ flex: 1, display: { xs: "none", md: "block" } }} />
        <RailLink
          to="/activity"
          label="Activity"
          icon={<ActivityIcon size={22} />}
        />
      </Box>

      <Box
        component="main"
        sx={{
          flex: "999 1 560px",
          minWidth: 0,
          px: { xs: 2, md: 6 },
          pt: { xs: 2, md: 2.5 },
          pb: 8,
          boxSizing: "border-box",
        }}
      >
        <Stack sx={{ maxWidth: 1180, mx: "auto", gap: 4.5 }}>
          <Stack
            component="header"
            direction="row"
            sx={{ alignItems: "center", gap: 2, flexWrap: "wrap" }}
          >
            <GlobalSearch />
            <Button
              component={RouterLink}
              to="/add"
              variant="contained"
              size="large"
              startIcon={<PlusIcon size={18} />}
            >
              Add
            </Button>
            <Box
              component="button"
              type="button"
              aria-label={`Account: ${username}`}
              aria-haspopup="menu"
              onClick={(event: React.MouseEvent<HTMLElement>) =>
                setMenuAnchor(event.currentTarget)
              }
              sx={{
                width: 44,
                height: 44,
                borderRadius: "50%",
                border: `1px solid ${tokens.rope}`,
                background: tokens.deck,
                color: tokens.foam,
                fontFamily: fonts.display,
                fontWeight: 600,
                fontSize: 15,
                cursor: "pointer",
              }}
            >
              {username.slice(0, 1).toUpperCase()}
            </Box>
            <Menu
              anchorEl={menuAnchor}
              open={Boolean(menuAnchor)}
              onClose={() => setMenuAnchor(null)}
              anchorOrigin={{ vertical: "bottom", horizontal: "right" }}
              transformOrigin={{ vertical: "top", horizontal: "right" }}
            >
              <Typography
                sx={{ px: 2, py: 1, color: tokens.fog, fontSize: 13 }}
              >
                Signed in as {username}
              </Typography>
              <MenuItem
                onClick={() => {
                  setMenuAnchor(null);
                  onSignOut();
                }}
              >
                Sign out
              </MenuItem>
            </Menu>
          </Stack>
          {children}
        </Stack>
      </Box>
    </Box>
  );
}
