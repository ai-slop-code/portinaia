import { useCallback, useEffect, useRef, useState } from "react";
import { apiClient, currentSessionGeneration } from "../../api/client";
import type { components } from "../../api/generated";

type EnrollmentSecret = components["schemas"]["EnrollmentTokenSecret"];

/** One-time secret controller. Secret lives only in component memory, never query data. */
export function useEnrollment(computeId?: string) {
  const [issued, setIssued] = useState<{
    secret: EnrollmentSecret;
    computeId: string;
    generation: number;
  }>();
  const [pending, setPending] = useState(false);
  const [error, setError] = useState<string>();
  const target = useRef(computeId);
  const inFlight = useRef(false);
  const mounted = useRef(true);
  const requestVersion = useRef(0);
  const secret =
    issued &&
    issued.computeId === computeId &&
    issued.generation === currentSessionGeneration()
      ? issued.secret
      : undefined;

  useEffect(() => {
    if (target.current !== computeId) {
      requestVersion.current += 1;
      inFlight.current = false;
      setIssued(undefined);
      setPending(false);
    }
    target.current = computeId;
    setError(undefined);
  }, [computeId]);

  useEffect(() => {
    mounted.current = true;
    const clear = () => {
      requestVersion.current += 1;
      setIssued(undefined);
      setPending(false);
    };
    window.addEventListener("portinaia:session-changed", clear);
    window.addEventListener("portinaia:session-expired", clear);
    return () => {
      mounted.current = false;
      requestVersion.current += 1;
      window.removeEventListener("portinaia:session-changed", clear);
      window.removeEventListener("portinaia:session-expired", clear);
      setIssued(undefined);
    };
  }, []);

  const issue = useCallback(
    async (expiresInSeconds = 3600) => {
      if (!computeId || inFlight.current) return;
      inFlight.current = true;
      setPending(true);
      setError(undefined);
      setIssued(undefined);
      const requestedCompute = computeId;
      const generation = currentSessionGeneration();
      const version = ++requestVersion.current;
      try {
        const { data, error: requestError } = await apiClient.POST(
          "/computes/{compute_id}/enrollment-tokens",
          {
            params: { path: { compute_id: requestedCompute } },
            body: { expires_in_seconds: expiresInSeconds },
          },
        );
        if (requestError || !data)
          throw (
            requestError ?? new Error("Could not issue an enrollment token.")
          );
        if (
          mounted.current &&
          version === requestVersion.current &&
          target.current === requestedCompute &&
          computeId === requestedCompute &&
          currentSessionGeneration() === generation
        )
          setIssued({ secret: data, computeId: requestedCompute, generation });
      } catch {
        if (
          mounted.current &&
          version === requestVersion.current &&
          target.current === requestedCompute &&
          currentSessionGeneration() === generation
        ) {
          setError(
            "The issuance result could not be confirmed. Do not retry blindly; check the compute's enrollment state, revoke any outstanding token, then issue a fresh token.",
          );
        }
      } finally {
        if (version === requestVersion.current) {
          inFlight.current = false;
          if (mounted.current) setPending(false);
        }
      }
    },
    [computeId],
  );

  const clear = useCallback(() => {
    requestVersion.current += 1;
    inFlight.current = false;
    setIssued(undefined);
    setPending(false);
  }, []);
  return { secret, pending, error, issue, clear };
}
