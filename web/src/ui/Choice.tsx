import type { ReactNode } from "react";
import { Box, Stack, Typography } from "@mui/material";

import { fonts, tokens } from "../theme/theme";

/** A row of pill-shaped single-choice options, backed by real radio inputs. */
export function ChoicePills<T extends string>({
  legend,
  name,
  value,
  options,
  onChange,
}: {
  legend: string;
  name: string;
  value: T;
  options: readonly (readonly [T, string])[];
  onChange: (value: T) => void;
}) {
  return (
    <Box component="fieldset" sx={{ border: 0, m: 0, p: 0, minWidth: 0 }}>
      <Typography
        component="legend"
        sx={{ fontSize: 13, fontWeight: 500, color: tokens.mist, mb: 1.25 }}
      >
        {legend}
      </Typography>
      <Stack direction="row" sx={{ flexWrap: "wrap", gap: 1 }}>
        {options.map(([optionValue, label]) => (
          <PillOption
            key={optionValue}
            type="radio"
            name={name}
            checked={value === optionValue}
            onChange={() => onChange(optionValue)}
          >
            {label}
          </PillOption>
        ))}
      </Stack>
    </Box>
  );
}

/** Multi-choice pills backed by real checkboxes (named for FormData). */
export function TogglePills<T extends string>({
  legend,
  values,
  options,
  onChange,
}: {
  legend: string;
  values: readonly T[];
  options: readonly (readonly [T, string])[];
  onChange: (values: T[]) => void;
}) {
  return (
    <Box component="fieldset" sx={{ border: 0, m: 0, p: 0, minWidth: 0 }}>
      <Typography
        component="legend"
        sx={{ fontSize: 13, fontWeight: 500, color: tokens.mist, mb: 1.25 }}
      >
        {legend}
      </Typography>
      <Stack direction="row" sx={{ flexWrap: "wrap", gap: 1 }}>
        {options.map(([optionValue, label]) => (
          <PillOption
            key={optionValue}
            type="checkbox"
            name={optionValue}
            checked={values.includes(optionValue)}
            onChange={(checked) =>
              onChange(
                checked
                  ? [...values, optionValue]
                  : values.filter((item) => item !== optionValue),
              )
            }
          >
            {label}
          </PillOption>
        ))}
      </Stack>
    </Box>
  );
}

function PillOption({
  type,
  name,
  checked,
  onChange,
  children,
}: {
  type: "radio" | "checkbox";
  name: string;
  checked: boolean;
  onChange: (checked: boolean) => void;
  children: ReactNode;
}) {
  return (
    <Box
      component="label"
      sx={{
        display: "inline-flex",
        alignItems: "center",
        gap: 1,
        minHeight: 40,
        px: 1.75,
        borderRadius: 999,
        cursor: "pointer",
        fontSize: 14,
        border: `1px solid ${checked ? tokens.signal : tokens.rope}`,
        background: checked ? tokens.signalWash : "transparent",
        color: checked ? tokens.foam : tokens.mist,
        "&:focus-within": {
          outline: `2px solid ${tokens.signal}`,
          outlineOffset: 2,
        },
        "& input": { accentColor: tokens.signal, m: 0 },
      }}
    >
      <input
        type={type}
        name={name}
        checked={checked}
        onChange={(event) => onChange(event.target.checked)}
      />
      {children}
    </Box>
  );
}

/** Large selectable cards (e.g. compute kind) backed by radio inputs. */
export function ChoiceCards<T extends string>({
  legend,
  name,
  value,
  options,
  onChange,
}: {
  legend: string;
  name: string;
  value: T;
  options: readonly { value: T; label: string; hint: string }[];
  onChange: (value: T) => void;
}) {
  return (
    <Box component="fieldset" sx={{ border: 0, m: 0, p: 0, minWidth: 0 }}>
      <Typography
        component="legend"
        sx={{ fontSize: 13, fontWeight: 500, color: tokens.mist, mb: 1.25 }}
      >
        {legend}
      </Typography>
      <Box
        sx={{
          display: "grid",
          gridTemplateColumns: "repeat(auto-fit, minmax(150px, 1fr))",
          gap: 1.5,
        }}
      >
        {options.map((option) => {
          const checked = option.value === value;
          return (
            <Box
              component="label"
              key={option.value}
              sx={{
                position: "relative",
                display: "flex",
                flexDirection: "column",
                gap: 1,
                p: 2.25,
                borderRadius: "16px",
                cursor: "pointer",
                border: `1px solid ${checked ? tokens.signal : tokens.rope}`,
                background: checked ? tokens.signalWash : tokens.night,
                "&:focus-within": {
                  outline: `2px solid ${tokens.signal}`,
                  outlineOffset: 2,
                },
              }}
            >
              <input
                type="radio"
                name={name}
                value={option.value}
                checked={checked}
                onChange={() => onChange(option.value)}
                style={{ position: "absolute", opacity: 0, inset: 0 }}
              />
              <Typography
                sx={{
                  fontFamily: fonts.display,
                  fontWeight: 600,
                  fontSize: 18,
                }}
              >
                {option.label}
              </Typography>
              <Typography sx={{ fontSize: 13, color: tokens.fog }}>
                {option.hint}
              </Typography>
            </Box>
          );
        })}
      </Box>
    </Box>
  );
}
