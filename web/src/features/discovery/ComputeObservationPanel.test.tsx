import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { ComputeObservationPanel } from "./ComputeObservationPanel";

describe("ComputeObservationPanel", () => {
  it("distinguishes unknown facts, failed collectors, and successful absence", () => {
    render(
      <ComputeObservationPanel
        collectors={[
          {
            collector_key: "podman:root",
            collector: "podman",
            owner_uid: 0,
            last_status: "failed",
            last_received_at: "2026-09-30T12:00:00Z",
            last_error_message: "socket unavailable",
          },
          {
            collector_key: "interfaces",
            collector: "interfaces",
            last_status: "success",
            last_received_at: "2026-09-30T12:00:00Z",
            last_successful_at: "2026-09-30T12:00:00Z",
          },
        ]}
      />,
    );
    expect(
      screen.getByText(/facts are unknown, not confirmed absent/),
    ).toBeInTheDocument();
    expect(
      screen.getByText(/Failed — previous facts retained/),
    ).toBeInTheDocument();
    expect(
      screen.getByText(/an empty result confirms absence/),
    ).toBeInTheDocument();
  });

  it("does not present unavailable reads as empty or confirmed absent", () => {
    render(
      <ComputeObservationPanel
        unavailable={{ containers: true, ipAssignments: true }}
      />,
    );
    expect(
      screen.getByText(/Containers are unavailable; no absence is inferred/),
    ).toBeInTheDocument();
    expect(
      screen.getByText(
        /Discovered IP assignments are unavailable; no absence is inferred/,
      ),
    ).toBeInTheDocument();
  });
});
