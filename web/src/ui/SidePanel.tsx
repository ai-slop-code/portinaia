import type { ReactNode } from "react";
import { Drawer, IconButton, Stack, Typography } from "@mui/material";

import { fonts } from "../theme/theme";
import { CloseIcon } from "./icons";

/** Right-hand panel for details and edits that keep the list in view. */
export function SidePanel({
  open,
  onClose,
  title,
  mono,
  children,
}: {
  open: boolean;
  onClose: () => void;
  title: ReactNode;
  mono?: boolean;
  children: ReactNode;
}) {
  return (
    <Drawer
      anchor="right"
      open={open}
      onClose={onClose}
      slotProps={{
        paper: {
          sx: { width: { xs: "100%", sm: 520 }, p: { xs: 2.5, sm: 4 } },
        },
      }}
    >
      <Stack direction="row" sx={{ alignItems: "flex-start", gap: 2, mb: 3 }}>
        <Typography
          component="h2"
          sx={{
            flex: 1,
            minWidth: 0,
            fontFamily: mono ? fonts.mono : fonts.display,
            fontWeight: mono ? 500 : 600,
            fontSize: 24,
            overflowWrap: "anywhere",
          }}
        >
          {title}
        </Typography>
        <IconButton aria-label="Close" onClick={onClose}>
          <CloseIcon size={20} />
        </IconButton>
      </Stack>
      {children}
    </Drawer>
  );
}
