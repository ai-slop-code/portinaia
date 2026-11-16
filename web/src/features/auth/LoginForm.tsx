import type { FormEvent } from "react";
import {
  Alert,
  Box,
  Button,
  Stack,
  TextField,
  Typography,
} from "@mui/material";

import { fonts, tokens } from "../../theme/theme";
import { BrandMark } from "../../ui/icons";

type LoginFormProps = {
  error: string;
  loading: boolean;
  onSubmit: (event: FormEvent<HTMLFormElement>) => void;
};

export function LoginForm({ error, loading, onSubmit }: LoginFormProps) {
  return (
    <Box
      sx={{
        minHeight: "100vh",
        display: "grid",
        placeItems: "center",
        px: 2,
        background: tokens.night,
      }}
    >
      <Box
        sx={{
          width: "100%",
          maxWidth: 420,
          p: { xs: 3, sm: 5 },
          borderRadius: "24px",
          background: tokens.hull,
          border: `1px solid ${tokens.line}`,
        }}
      >
        <Stack component="form" spacing={2.5} onSubmit={onSubmit}>
          <Stack direction="row" spacing={1.5} sx={{ alignItems: "center" }}>
            <BrandMark size={40} />
            <Typography
              component="h1"
              sx={{
                fontFamily: fonts.display,
                fontWeight: 600,
                fontSize: 28,
                letterSpacing: "-0.02em",
              }}
            >
              Portinaia
            </Typography>
          </Stack>
          <Typography color="text.secondary">
            Sign in to keep watch over your estate.
          </Typography>
          {error && <Alert severity="error">{error}</Alert>}
          {loading && (
            <Typography color="text.secondary" role="status">
              Checking session…
            </Typography>
          )}
          <TextField
            name="username"
            label="Username"
            autoComplete="username"
            required
          />
          <TextField
            name="password"
            label="Password"
            type="password"
            autoComplete="current-password"
            required
          />
          <Button type="submit" variant="contained" size="large">
            Sign in
          </Button>
        </Stack>
      </Box>
    </Box>
  );
}
