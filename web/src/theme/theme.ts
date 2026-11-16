import { alpha, createTheme } from "@mui/material/styles";

/**
 * Harbor Night — a dark, quiet harbor. Infrastructure is the water; only
 * signals worth noticing glow. Tokens are exported for components that draw
 * outside MUI's palette (status lights, rails, tiles).
 */
export const tokens = {
  night: "#0A0D13",
  hull: "#11151D",
  deck: "#151A23",
  deckHover: "#1C2230",
  line: "#1E2430",
  rope: "#2A3240",
  ghost: "#2E3746",
  foam: "#E8ECF2",
  mist: "#C3CAD6",
  fog: "#8B95A7",
  dim: "#5B6577",
  signal: "#5EEAD4",
  signalBright: "#99F6E4",
  signalWash: "#0F2A27",
  beacon: "#FBBF24",
  beaconWash: "#2A2113",
  flare: "#F87171",
  flareWash: "#2A1515",
} as const;

export const fonts = {
  body: '"Inter Variable", Inter, system-ui, sans-serif',
  display: '"Space Grotesk Variable", "Space Grotesk", Inter, sans-serif',
  mono: '"JetBrains Mono Variable", "JetBrains Mono", ui-monospace, monospace',
} as const;

export const theme = createTheme({
  palette: {
    mode: "dark",
    primary: { main: tokens.signal, contrastText: tokens.night },
    secondary: { main: tokens.mist },
    warning: { main: tokens.beacon },
    error: { main: tokens.flare },
    success: { main: tokens.signal },
    info: { main: "#60A5FA" },
    background: { default: tokens.night, paper: tokens.hull },
    text: { primary: tokens.foam, secondary: tokens.fog },
    divider: tokens.line,
  },
  shape: { borderRadius: 12 },
  typography: {
    fontFamily: fonts.body,
    h1: {
      fontFamily: fonts.display,
      fontWeight: 600,
      letterSpacing: "-0.02em",
    },
    h2: {
      fontFamily: fonts.display,
      fontWeight: 600,
      letterSpacing: "-0.02em",
    },
    h3: {
      fontFamily: fonts.display,
      fontWeight: 600,
      letterSpacing: "-0.02em",
    },
    h4: {
      fontFamily: fonts.display,
      fontWeight: 600,
      letterSpacing: "-0.02em",
    },
    h5: { fontFamily: fonts.display, fontWeight: 600 },
    h6: { fontFamily: fonts.display, fontWeight: 600, fontSize: "1.2rem" },
    subtitle1: { fontWeight: 600 },
    button: { textTransform: "none", fontWeight: 600, fontSize: "0.95rem" },
  },
  components: {
    MuiCssBaseline: {
      styleOverrides: {
        body: { backgroundColor: tokens.night },
        a: { color: tokens.signal },
        "::selection": { background: alpha(tokens.signal, 0.3) },
      },
    },
    MuiButton: {
      defaultProps: { disableElevation: true },
      styleOverrides: {
        root: { borderRadius: 999, minHeight: 40, paddingInline: 18 },
        sizeSmall: { minHeight: 32, paddingInline: 12 },
        sizeLarge: { minHeight: 48, paddingInline: 24 },
        outlined: {
          borderColor: tokens.rope,
          color: tokens.mist,
          "&:hover": {
            borderColor: tokens.ghost,
            background: tokens.deckHover,
          },
        },
        text: { "&:hover": { background: tokens.deck } },
      },
    },
    MuiIconButton: {
      styleOverrides: { root: { "&:hover": { background: tokens.deck } } },
    },
    MuiPaper: {
      styleOverrides: {
        root: { backgroundImage: "none" },
        outlined: { borderColor: tokens.line },
      },
    },
    MuiCard: {
      defaultProps: { variant: "outlined" },
      styleOverrides: {
        root: {
          borderRadius: 20,
          borderColor: tokens.line,
          background: tokens.hull,
        },
      },
    },
    MuiCardContent: {
      styleOverrides: {
        root: { padding: 24, "&:last-child": { paddingBottom: 24 } },
      },
    },
    MuiTextField: {
      // Labels sit above the field so placeholders and empty selections
      // ("None", "Not tracked") stay readable.
      defaultProps: { slotProps: { inputLabel: { shrink: true } } },
    },
    MuiSelect: { defaultProps: { displayEmpty: true } },
    MuiOutlinedInput: {
      styleOverrides: {
        root: {
          borderRadius: 12,
          background: tokens.night,
          "& .MuiOutlinedInput-notchedOutline": { borderColor: tokens.rope },
          "&:hover .MuiOutlinedInput-notchedOutline": {
            borderColor: tokens.ghost,
          },
        },
      },
    },
    MuiChip: {
      styleOverrides: {
        root: { borderRadius: 999, fontWeight: 500 },
        filled: { background: tokens.deck, color: tokens.mist },
        outlined: { borderColor: tokens.rope, color: tokens.mist },
      },
    },
    MuiAlert: {
      styleOverrides: { root: { borderRadius: 14 } },
    },
    MuiMenu: {
      styleOverrides: {
        paper: { border: `1px solid ${tokens.rope}`, background: tokens.hull },
      },
    },
    MuiDrawer: {
      styleOverrides: {
        paper: { background: tokens.hull, borderColor: tokens.line },
      },
    },
    MuiTab: {
      styleOverrides: {
        root: { textTransform: "none", fontWeight: 500, fontSize: "0.95rem" },
      },
    },
    MuiListItemButton: {
      styleOverrides: {
        root: {
          borderRadius: 14,
          "&.Mui-selected": { background: tokens.deck },
          "&:hover": { background: tokens.deck },
        },
      },
    },
    MuiDivider: { styleOverrides: { root: { borderColor: tokens.line } } },
  },
});
