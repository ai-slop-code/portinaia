import { createTheme } from "@mui/material/styles";

export const theme = createTheme({
  palette: {
    mode: "light",
    primary: {
      main: "#205b73",
    },
    background: {
      default: "#f3f0e8",
    },
  },
  typography: {
    fontFamily: 'Inter, "Segoe UI", sans-serif',
  },
});
