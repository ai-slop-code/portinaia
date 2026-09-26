import { CssBaseline, Stack, ThemeProvider, Typography } from "@mui/material";

import { theme } from "../theme/theme";

export function App() {
  return (
    <ThemeProvider theme={theme}>
      <CssBaseline />
      <Stack
        component="main"
        spacing={1}
        sx={{
          minHeight: "100vh",
          alignItems: "center",
          justifyContent: "center",
          padding: 3,
        }}
      >
        <Typography component="h1" variant="h3">
          Portinaia
        </Typography>
        <Typography color="text.secondary">
          Infrastructure inventory and DNS control plane
        </Typography>
      </Stack>
    </ThemeProvider>
  );
}
