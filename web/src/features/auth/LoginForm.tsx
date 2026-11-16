import type { FormEvent } from "react";
import {
  Alert,
  Button,
  Card,
  CardContent,
  Container,
  Stack,
  TextField,
  Typography,
} from "@mui/material";

type LoginFormProps = {
  error: string;
  loading: boolean;
  onSubmit: (event: FormEvent<HTMLFormElement>) => void;
};

export function LoginForm({ error, loading, onSubmit }: LoginFormProps) {
  return (
    <Container
      maxWidth="sm"
      sx={{ minHeight: "100vh", display: "grid", placeItems: "center" }}
    >
      <Card sx={{ width: "100%" }}>
        <CardContent sx={{ p: 4 }}>
          <Stack component="form" spacing={2} onSubmit={onSubmit}>
            <Typography component="h1" variant="h4">
              Portinaia
            </Typography>
            <Typography color="text.secondary">
              Sign in to your infrastructure inventory.
            </Typography>
            {error && <Alert severity="error">{error}</Alert>}
            {loading && (
              <Typography color="text.secondary">Checking session…</Typography>
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
            <Button type="submit" variant="contained">
              Sign in
            </Button>
          </Stack>
        </CardContent>
      </Card>
    </Container>
  );
}
