import { render, screen } from "@testing-library/react";
import { expect, test } from "vitest";

import { App } from "./App";

test("renders the application identity", () => {
  render(<App />);

  expect(
    screen.getByRole("heading", { name: "Portinaia" }),
  ).toBeInTheDocument();
  expect(
    screen.getByText("Infrastructure inventory and DNS control plane"),
  ).toBeInTheDocument();
});
