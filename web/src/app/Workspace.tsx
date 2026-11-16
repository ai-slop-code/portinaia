import { useEffect, useRef, useState } from "react";
import { useSearchParams } from "react-router-dom";
import {
  Alert,
  AppBar,
  Button,
  Card,
  CardContent,
  CssBaseline,
  Container,
  Divider,
  FormControlLabel,
  Checkbox,
  MenuItem,
  List,
  ListItemButton,
  ListItemText,
  Stack,
  TextField,
  ThemeProvider,
  Toolbar,
  Typography,
} from "@mui/material";

import { theme } from "../theme/theme";
import type { components } from "../api/generated";
import { apiClient, apiFetch, setCSRFToken } from "../api/client";
import { LoginForm } from "../features/auth/LoginForm";
import { InventoryTree } from "../features/inventory/InventoryTree";
import { ComputeCreateForm } from "../features/inventory/ComputeCreateForm";
import { LocationCreateForm } from "../features/inventory/LocationCreateForm";
import { LocationDetails } from "../features/inventory/LocationDetails";
import { ComputeObservationPanel } from "../features/discovery/ComputeObservationPanel";
import { DomainCreateForm } from "../features/domains/DomainCreateForm";
import { DomainDetails } from "../features/domains/DomainDetails";
import { DomainList } from "../features/domains/DomainList";
import { domainWriteFromForm } from "../features/domains/domainAdapter";
import type { DomainEditSnapshot } from "../features/domains/domainAdapter";
import type { DomainEditSubmission } from "./integration";
import { ProviderList } from "../features/providers/ProviderList";
import { ProviderWorkspace } from "../features/providers/ProviderWorkspace";
import { AuditWorkspace } from "../features/audit/AuditWorkspace";

const fetch = apiFetch;

type Session = components["schemas"]["Session"];
type Provider = components["schemas"]["Provider"];
type Location = components["schemas"]["Location"];
type Compute = components["schemas"]["Compute"];
type AgentDetails = components["schemas"]["Agent"];
type CollectorObservation = components["schemas"]["CollectorObservation"];
type EnrollmentSecret = components["schemas"]["EnrollmentTokenSecret"];
type TreeNode = components["schemas"]["InventoryTreeNode"];
type InventorySummary = components["schemas"]["InventorySummary"];
type IPAssignment = components["schemas"]["IpAssignment"];
type Domain = components["schemas"]["Domain"];
type Container = components["schemas"]["Container"];

async function readSession(): Promise<Session | null> {
  const { data, response } = await apiClient.GET("/auth/session", {
    headers: { "Cache-Control": "no-store" },
  });
  if (response.status === 401) {
    setCSRFToken(undefined);
    return null;
  }
  if (!response.ok) {
    throw new Error(`session bootstrap failed with ${response.status}`);
  }
  const session = data ?? null;
  setCSRFToken(session?.csrf_token);
  return session;
}

export function Workspace() {
  const providerDetailRequest = useRef(0);
  const locationDetailRequest = useRef(0);
  const domainDetailRequest = useRef(0);
  const domainListRequest = useRef(0);
  const computeDetailRequest = useRef(0);
  const [searchParams, setSearchParams] = useSearchParams();
  const [session, setSession] = useState<Session | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [providers, setProviders] = useState<Provider[]>([]);
  const [selectedProvider, setSelectedProvider] = useState<Provider | null>(
    null,
  );
  const [search, setSearch] = useState(() => searchParams.get("q") ?? "");
  const [locations, setLocations] = useState<Location[]>([]);
  const [selectedLocation, setSelectedLocation] = useState<Location | null>(
    null,
  );
  const [computes, setComputes] = useState<Compute[]>([]);
  const [selectedCompute, setSelectedCompute] = useState<Compute | null>(null);
  const [agentDetails, setAgentDetails] = useState<AgentDetails | null>(null);
  const [collectorStatus, setCollectorStatus] = useState<
    CollectorObservation[]
  >([]);
  const [enrollmentSecret, setEnrollmentSecret] =
    useState<EnrollmentSecret | null>(null);
  const [editingProvider, setEditingProvider] = useState(false);
  const [editingLocation, setEditingLocation] = useState(false);
  const [editingCompute, setEditingCompute] = useState(false);
  const [inventorySummary, setInventorySummary] =
    useState<InventorySummary | null>(null);
  const [treeRoots, setTreeRoots] = useState<TreeNode[]>([]);
  const [treeChildren, setTreeChildren] = useState<Record<string, TreeNode[]>>(
    {},
  );
  const [expandedTreeNodes, setExpandedTreeNodes] = useState<Set<string>>(
    new Set(),
  );
  const [treeRootCursor, setTreeRootCursor] = useState<string | null>(null);
  const [treeRootHasMore, setTreeRootHasMore] = useState(false);
  const [treeChildCursors, setTreeChildCursors] = useState<
    Record<string, string | null>
  >({});
  const [treeChildHasMore, setTreeChildHasMore] = useState<
    Record<string, boolean>
  >({});
  const [includeArchived, setIncludeArchived] = useState(false);
  const [providerCursor, setProviderCursor] = useState<string | null>(null);
  const [providerHasMore, setProviderHasMore] = useState(false);
  const [locationCursor, setLocationCursor] = useState<string | null>(null);
  const [locationHasMore, setLocationHasMore] = useState(false);
  const [computeCursor, setComputeCursor] = useState<string | null>(null);
  const [computeHasMore, setComputeHasMore] = useState(false);
  const [ipAssignments, setIPAssignments] = useState<IPAssignment[]>([]);
  const [ipCursor, setIPCursor] = useState<string | null>(null);
  const [ipHasMore, setIPHasMore] = useState(false);
  const [editingIP, setEditingIP] = useState<string | null>(null);
  const [domains, setDomains] = useState<Domain[]>([]);
  const [domainCursor, setDomainCursor] = useState<string | null>(null);
  const [domainsHaveMore, setDomainsHaveMore] = useState(false);
  const [selectedDomain, setSelectedDomain] = useState<Domain | null>(null);
  const [editingDomain, setEditingDomain] = useState(false);
  const [containers, setContainers] = useState<Container[]>([]);
  const [selectedContainer, setSelectedContainer] = useState<Container | null>(
    null,
  );
  const [containerCursor, setContainerCursor] = useState<string | null>(null);
  const [containersHaveMore, setContainersHaveMore] = useState(false);

  async function loadProviders(query = "", cursor?: string) {
    const { data, error } = await apiClient.GET("/providers", {
      params: {
        query: {
          q: query,
          archived: includeArchived ? "include" : "exclude",
          "page[size]": 50,
          ...(cursor ? { "page[after]": cursor } : {}),
        },
      },
    });
    if (error || !data) {
      setError("Could not load providers.");
      return;
    }
    const result = data;
    setProviders((current) =>
      cursor ? [...current, ...result.items] : result.items,
    );
    setProviderCursor(result.page.next_cursor ?? null);
    setProviderHasMore(result.page.has_more);
  }

  async function loadLocations(query = "", cursor?: string) {
    const { data, error } = await apiClient.GET("/locations", {
      params: {
        query: {
          q: query,
          archived: includeArchived ? "include" : "exclude",
          "page[size]": 50,
          ...(cursor ? { "page[after]": cursor } : {}),
        },
      },
    });
    if (error || !data) {
      setError("Could not load locations.");
      return;
    }
    const result = data;
    setLocations((current) =>
      cursor ? [...current, ...result.items] : result.items,
    );
    setLocationCursor(result.page.next_cursor ?? null);
    setLocationHasMore(result.page.has_more);
  }

  async function loadComputes(query = "", cursor?: string) {
    const { data, error } = await apiClient.GET("/computes", {
      params: {
        query: {
          q: query,
          archived: includeArchived ? "include" : "exclude",
          "page[size]": 50,
          ...(cursor ? { "page[after]": cursor } : {}),
        },
      },
    });
    if (error || !data) {
      setError("Could not load computes.");
      return;
    }
    const result = data;
    setComputes((current) =>
      cursor ? [...current, ...result.items] : result.items,
    );
    setComputeCursor(result.page.next_cursor ?? null);
    setComputeHasMore(result.page.has_more);
  }

  async function loadIPAssignments(computeID: string, cursor?: string) {
    const { data, error } = await apiClient.GET("/ip-assignments", {
      params: {
        query: {
          compute_id: computeID,
          archived: includeArchived ? "include" : "exclude",
          "page[size]": 50,
          ...(cursor ? { "page[after]": cursor } : {}),
        },
      },
    });
    if (error || !data) {
      setError("Could not load IP assignments.");
      return;
    }
    setIPAssignments((current) =>
      cursor ? [...current, ...data.items] : data.items,
    );
    setIPCursor(data.page.next_cursor ?? null);
    setIPHasMore(data.page.has_more);
  }

  async function loadContainers(computeID: string, cursor?: string) {
    const { data, error } = await apiClient.GET(
      "/computes/{compute_id}/containers",
      {
        params: {
          path: { compute_id: computeID },
          query: {
            archived: includeArchived ? "include" : "exclude",
            "page[size]": 50,
            ...(cursor ? { "page[after]": cursor } : {}),
          },
        },
      },
    );
    if (error || !data) {
      setError("Could not load containers for this compute.");
      return;
    }
    const result = data;
    setContainers((current) =>
      cursor ? [...current, ...result.items] : result.items,
    );
    setContainerCursor(result.page.next_cursor ?? null);
    setContainersHaveMore(result.page.has_more);
  }

  async function loadCollectorStatus(computeID: string) {
    const { data, error } = await apiClient.GET(
      "/computes/{compute_id}/collector-status",
      { params: { path: { compute_id: computeID } } },
    );
    if (error || !data) {
      setCollectorStatus([]);
      return;
    }
    setCollectorStatus(data.items);
  }

  async function showContainer(container: Container) {
    const { data, error } = await apiClient.GET("/containers/{container_id}", {
      params: { path: { container_id: container.id } },
    });
    if (error || !data) {
      setError("Could not load container details.");
      return;
    }
    setSelectedContainer(data);
  }

  async function loadDomains(query = "", cursor?: string) {
    const request = ++domainListRequest.current;
    const requestedSession = session;
    const requestedArchived = includeArchived;
    const { data, error } = await apiClient.GET("/domains", {
      params: {
        query: {
          q: query,
          archived: includeArchived ? "include" : "exclude",
          "page[size]": 50,
          ...(cursor ? { "page[after]": cursor } : {}),
        },
      },
    });
    if (
      request !== domainListRequest.current ||
      requestedSession !== session ||
      requestedArchived !== includeArchived ||
      query !== search
    )
      return;
    if (error || !data) {
      if (!cursor) {
        setDomains([]);
        setDomainCursor(null);
        setDomainsHaveMore(false);
      }
      setError("Could not load domains.");
      return;
    }
    setDomains((current) =>
      cursor ? [...current, ...data.items] : data.items,
    );
    setDomainCursor(data.page.next_cursor ?? null);
    setDomainsHaveMore(data.page.has_more);
  }

  async function loadInventorySummary() {
    const { data } = await apiClient.GET("/inventory/summary");
    if (data) setInventorySummary(data);
  }

  async function loadTreeRoots(cursor?: string) {
    const { data, error } = await apiClient.GET("/inventory/tree", {
      params: {
        query: {
          archived: includeArchived ? "include" : "exclude",
          "page[size]": 50,
          ...(cursor ? { "page[after]": cursor } : {}),
        },
      },
    });
    if (error || !data) {
      setError("Could not load inventory hierarchy.");
      return;
    }
    const result = data;
    setTreeRoots((current) =>
      cursor ? [...current, ...result.items] : result.items,
    );
    setTreeRootHasMore(result.page.has_more);
    setTreeRootCursor(result.page.next_cursor ?? null);
  }

  async function loadTreeChildren(parentID: string, cursor?: string) {
    const { data, error } = await apiClient.GET(
      "/inventory/tree/{parent_id}/children",
      {
        params: {
          path: { parent_id: parentID },
          query: {
            archived: includeArchived ? "include" : "exclude",
            "page[size]": 50,
            ...(cursor ? { "page[after]": cursor } : {}),
          },
        },
      },
    );
    if (error || !data) {
      setError("Could not load hierarchy children.");
      return;
    }
    const result = data;
    setTreeChildren((current) => ({
      ...current,
      [parentID]: cursor
        ? [...(current[parentID] ?? []), ...result.items]
        : result.items,
    }));
    setTreeChildHasMore((current) => ({
      ...current,
      [parentID]: result.page.has_more,
    }));
    setTreeChildCursors((current) => ({
      ...current,
      [parentID]: result.page.next_cursor ?? null,
    }));
  }

  function refreshTree() {
    setTreeChildren({});
    setExpandedTreeNodes(new Set());
    void loadTreeRoots();
  }

  async function toggleTreeNode(node: TreeNode) {
    if (!node.has_children) return;
    const next = new Set(expandedTreeNodes);
    if (next.has(node.id)) next.delete(node.id);
    else {
      next.add(node.id);
      if (!treeChildren[node.id]) await loadTreeChildren(node.id);
    }
    setExpandedTreeNodes(next);
  }

  useEffect(() => {
    setSearch(searchParams.get("q") ?? "");
    setIncludeArchived(searchParams.get("archived") === "include");
  }, [searchParams]);

  useEffect(() => {
    const next = new URLSearchParams(searchParams);
    if (search) next.set("q", search);
    else next.delete("q");
    if (includeArchived) next.set("archived", "include");
    else next.delete("archived");
    if (next.toString() !== searchParams.toString()) {
      setSearchParams(next, { replace: true });
    }
  }, [search, includeArchived, searchParams, setSearchParams]);

  useEffect(() => {
    void readSession()
      .then((current) => {
        setSession(current);
        setLoading(false);
      })
      .catch(() => {
        setError(
          "Could not check the current session. Retry by reloading the page.",
        );
        setLoading(false);
      });
  }, []);

  useEffect(() => {
    const resetSessionState = () => {
      setSession(null);
      setProviders([]);
      setSelectedProvider(null);
      setEditingProvider(false);
      setLocations([]);
      setSelectedLocation(null);
      setEditingLocation(false);
      setComputes([]);
      setSelectedCompute(null);
      setEditingCompute(false);
      setSelectedDomain(null);
      setEditingDomain(false);
      setContainers([]);
      setSelectedContainer(null);
      setContainerCursor(null);
      setContainersHaveMore(false);
      setIPAssignments([]);
      setIPCursor(null);
      setIPHasMore(false);
      setEditingIP(null);
      setDomains([]);
      setDomainCursor(null);
      setDomainsHaveMore(false);
      setProviderCursor(null);
      setProviderHasMore(false);
      setLocationCursor(null);
      setLocationHasMore(false);
      setComputeCursor(null);
      setComputeHasMore(false);
      setAgentDetails(null);
      setCollectorStatus([]);
      setEnrollmentSecret(null);
      setInventorySummary(null);
      setTreeRoots([]);
      setTreeChildren({});
      setExpandedTreeNodes(new Set());
      setTreeRootCursor(null);
      setTreeRootHasMore(false);
      setTreeChildCursors({});
      setTreeChildHasMore({});
    };
    const expired = () => {
      setCSRFToken(undefined);
      resetSessionState();
    };
    window.addEventListener("portinaia:session-expired", expired);
    window.addEventListener("portinaia:session-changed", resetSessionState);
    return () => {
      window.removeEventListener("portinaia:session-expired", expired);
      window.removeEventListener(
        "portinaia:session-changed",
        resetSessionState,
      );
    };
  }, []);

  useEffect(() => {
    if (session) {
      void loadProviders(search);
      void loadLocations(search);
      void loadComputes(search);
      void loadDomains(search);
    }
  }, [session, search, includeArchived]);

  useEffect(() => {
    if (session) {
      setTreeChildren({});
      setExpandedTreeNodes(new Set());
      void loadInventorySummary();
      void loadTreeRoots();
    }
  }, [session, includeArchived]);

  useEffect(() => {
    if (session && selectedCompute) void loadIPAssignments(selectedCompute.id);
    else {
      setIPAssignments([]);
      setIPCursor(null);
      setIPHasMore(false);
    }
  }, [session, selectedCompute?.id, includeArchived]);

  useEffect(() => {
    if (session && selectedCompute) {
      setSelectedContainer(null);
      void loadContainers(selectedCompute.id);
    } else {
      setContainers([]);
      setSelectedContainer(null);
    }
  }, [session, selectedCompute?.id, includeArchived]);

  async function login(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setError("");
    const values = new FormData(event.currentTarget);
    const { data, response } = await apiClient.POST("/auth/login", {
      body: {
        username: String(values.get("username") ?? ""),
        password: String(values.get("password") ?? ""),
      },
    });
    if (!response.ok || !data) {
      setError(
        response.status === 401
          ? "Username or password is incorrect."
          : "Unable to sign in.",
      );
      return;
    }
    const current = data;
    setCSRFToken(current.csrf_token);
    setSession(current);
  }

  async function logout() {
    if (!session) return;
    await apiClient.POST("/auth/logout");
    window.dispatchEvent(new Event("portinaia:session-expired"));
  }

  async function createProvider(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!session) return;
    setError("");
    const formElement = event.currentTarget;
    const form = new FormData(formElement);
    const payload: components["schemas"]["ProviderWrite"] = {
      name: String(form.get("name") ?? ""),
      website: String(form.get("website") ?? "") || undefined,
      external_account_reference:
        String(form.get("external_account_reference") ?? "") || undefined,
      description: String(form.get("description") ?? ""),
      notes: String(form.get("notes") ?? ""),
      roles: [
        "hosting_provider",
        "registrar",
        "hardware_vendor",
        "other",
      ].filter((role) => form.get(role) === "on") as Provider["roles"],
      tag_ids: [],
    };
    const { data: provider, response } = await apiClient.POST("/providers", {
      body: payload,
    });
    if (!response.ok || !provider) {
      setError(
        response.status === 409
          ? "A provider with that name already exists."
          : "Could not create provider.",
      );
      return;
    }
    setProviders((current) =>
      [...current, provider].sort((a, b) => a.name.localeCompare(b.name)),
    );
    setSelectedProvider(provider);
    formElement.reset();
  }

  async function updateProvider(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!session || !selectedProvider) return;
    const form = new FormData(event.currentTarget);
    const payload: components["schemas"]["ProviderWrite"] = {
      name: String(form.get("name") ?? ""),
      website: String(form.get("website") ?? "") || undefined,
      external_account_reference:
        String(form.get("external_account_reference") ?? "") || undefined,
      description: String(form.get("description") ?? ""),
      notes: String(form.get("notes") ?? ""),
      roles: [
        "hosting_provider",
        "registrar",
        "hardware_vendor",
        "other",
      ].filter((role) => form.get(role) === "on") as Provider["roles"],
      tag_ids: [],
    };
    const { data: updated, response } = await apiClient.PUT(
      "/providers/{provider_id}",
      {
        params: {
          path: { provider_id: selectedProvider.id },
          header: {
            "If-Match": `"revision-${selectedProvider.revision}"`,
            "X-CSRF-Token": session.csrf_token,
          },
        },
        body: payload,
      },
    );
    if (!response.ok || !updated) {
      setError(
        response.status === 412
          ? "Provider changed elsewhere. Reload its details before editing."
          : "Could not update provider.",
      );
      return;
    }
    setSelectedProvider(updated);
    setProviders((current) =>
      current.map((item) => (item.id === updated.id ? updated : item)),
    );
    setEditingProvider(false);
  }

  async function showProvider(provider: Provider) {
    const request = ++providerDetailRequest.current;
    const { data, error } = await apiClient.GET("/providers/{provider_id}", {
      params: { path: { provider_id: provider.id } },
    });
    if (request !== providerDetailRequest.current) return;
    if (error || !data) {
      setError("Could not load provider details.");
      return;
    }
    setSelectedProvider(data);
  }

  async function createLocation(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!session) return;
    setError("");
    const formElement = event.currentTarget;
    const form = new FormData(formElement);
    const payload = {
      type: form.get("type"),
      name: form.get("name"),
      provider_id: form.get("provider_id") || undefined,
      parent_id: form.get("parent_id") || undefined,
      description: form.get("description") || "",
      notes: form.get("notes") || "",
      tag_ids: [],
    };
    const response = await fetch("/api/v1/locations", {
      method: "POST",
      credentials: "include",
      headers: {
        "Content-Type": "application/json",
        "X-CSRF-Token": session.csrf_token,
      },
      body: JSON.stringify(payload),
    });
    if (!response.ok) {
      setError(
        response.status === 409
          ? "A location with that name already exists."
          : "Could not create location.",
      );
      return;
    }
    const location = (await response.json()) as Location;
    setLocations((current) =>
      [...current, location].sort((a, b) => a.name.localeCompare(b.name)),
    );
    setSelectedLocation(location);
    formElement.reset();
    refreshTree();
  }

  async function updateLocation(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!session || !selectedLocation) return;
    const form = new FormData(event.currentTarget);
    const payload = {
      type: form.get("type"),
      name: form.get("name"),
      provider_id: form.get("provider_id") || undefined,
      parent_id: form.get("parent_id") || undefined,
      description: form.get("description") || "",
      notes: form.get("notes") || "",
      tag_ids: [],
    };
    const response = await fetch(`/api/v1/locations/${selectedLocation.id}`, {
      method: "PUT",
      credentials: "include",
      headers: {
        "Content-Type": "application/json",
        "X-CSRF-Token": session.csrf_token,
        "If-Match": `"revision-${selectedLocation.revision}"`,
      },
      body: JSON.stringify(payload),
    });
    if (!response.ok) {
      setError(
        response.status === 412
          ? "Location changed elsewhere. Reload its details before editing."
          : "Could not update location.",
      );
      return;
    }
    const updated = (await response.json()) as Location;
    setSelectedLocation(updated);
    setLocations((current) =>
      current.map((item) => (item.id === updated.id ? updated : item)),
    );
    setEditingLocation(false);
    refreshTree();
  }

  async function showLocation(location: Location) {
    const request = ++locationDetailRequest.current;
    const { data, error } = await apiClient.GET("/locations/{location_id}", {
      params: { path: { location_id: location.id } },
    });
    if (request !== locationDetailRequest.current) return;
    if (error || !data) {
      setError("Could not load location details.");
      return;
    }
    setSelectedLocation(data);
  }

  async function createCompute(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!session) return;
    setError("");
    const formElement = event.currentTarget;
    const form = new FormData(formElement);
    const payload = {
      kind: form.get("kind"),
      display_name: form.get("display_name"),
      ownership: form.get("ownership"),
      provider_id: form.get("provider_id") || undefined,
      provider_resource_id: form.get("provider_resource_id") || undefined,
      location_id: form.get("location_id") || undefined,
      parent_compute_id: form.get("parent_compute_id") || undefined,
      description: form.get("description") || "",
      notes: form.get("notes") || "",
      tag_ids: [],
    };
    const response = await fetch("/api/v1/computes", {
      method: "POST",
      credentials: "include",
      headers: {
        "Content-Type": "application/json",
        "X-CSRF-Token": session.csrf_token,
      },
      body: JSON.stringify(payload),
    });
    if (!response.ok) {
      setError(
        response.status === 409
          ? "That provider resource is already registered."
          : "Could not create compute.",
      );
      return;
    }
    const compute = (await response.json()) as Compute;
    setComputes((current) =>
      [...current, compute].sort((a, b) =>
        a.display_name.localeCompare(b.display_name),
      ),
    );
    setSelectedCompute(compute);
    setAgentDetails(null);
    setCollectorStatus([]);
    setEnrollmentSecret(null);
    formElement.reset();
    void loadInventorySummary();
    refreshTree();
  }

  async function updateCompute(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!session || !selectedCompute) return;
    const form = new FormData(event.currentTarget);
    const payload = {
      kind: form.get("kind"),
      display_name: form.get("display_name"),
      ownership: form.get("ownership"),
      provider_id: form.get("provider_id") || undefined,
      provider_resource_id: form.get("provider_resource_id") || undefined,
      location_id: form.get("location_id") || undefined,
      parent_compute_id: form.get("parent_compute_id") || undefined,
      description: form.get("description") || "",
      notes: form.get("notes") || "",
      tag_ids: [],
    };
    const response = await fetch(`/api/v1/computes/${selectedCompute.id}`, {
      method: "PUT",
      credentials: "include",
      headers: {
        "Content-Type": "application/json",
        "X-CSRF-Token": session.csrf_token,
        "If-Match": `"revision-${selectedCompute.revision}"`,
      },
      body: JSON.stringify(payload),
    });
    if (!response.ok) {
      setError(
        response.status === 412
          ? "Compute changed elsewhere. Reload its details before editing."
          : "Could not update compute.",
      );
      return;
    }
    const updated = (await response.json()) as Compute;
    setSelectedCompute(updated);
    setComputes((current) =>
      current.map((item) => (item.id === updated.id ? updated : item)),
    );
    setEditingCompute(false);
    void loadInventorySummary();
    refreshTree();
  }

  async function changeLifecycle(
    kind: "provider" | "location" | "compute",
    resource: Provider | Location | Compute,
    action: "archive" | "restore",
  ) {
    if (!session) return;
    setError("");
    const plural = kind === "provider" ? "providers" : `${kind}s`;
    const response = await fetch(`/api/v1/${plural}/${resource.id}/${action}`, {
      method: "POST",
      credentials: "include",
      headers: {
        "X-CSRF-Token": session.csrf_token,
        "If-Match": `"revision-${resource.revision}"`,
      },
    });
    if (!response.ok) {
      setError(
        response.status === 412
          ? "This resource changed elsewhere. Reload it before changing its lifecycle."
          : `Could not ${action} ${kind}.`,
      );
      return;
    }
    if (kind === "provider") {
      const updated = (await response.json()) as Provider;
      setSelectedProvider(updated);
      void loadProviders(search);
    } else if (kind === "location") {
      const updated = (await response.json()) as Location;
      setSelectedLocation(updated);
      void loadLocations(search);
    } else {
      const updated = (await response.json()) as Compute;
      setSelectedCompute(updated);
      void loadComputes(search);
      void loadInventorySummary();
      if (updated.agent_id) {
        const agentResponse = await fetch(
          `/api/v1/agents/${updated.agent_id}`,
          { credentials: "include" },
        );
        setAgentDetails(
          agentResponse.ok
            ? ((await agentResponse.json()) as AgentDetails)
            : null,
        );
      } else {
        setAgentDetails(null);
      }
      void loadCollectorStatus(updated.id);
    }
    setEditingProvider(false);
    setEditingLocation(false);
    setEditingCompute(false);
    refreshTree();
  }

  async function createIPAssignment(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!session || !selectedCompute) return;
    const formElement = event.currentTarget;
    const form = new FormData(formElement);
    const prefix = String(form.get("prefix_length") ?? "");
    const payload = {
      address: form.get("address"),
      prefix_length: prefix === "" ? undefined : Number(prefix),
      compute_id: selectedCompute.id,
      interface_or_network: form.get("interface_or_network") || undefined,
      scope: form.get("scope"),
      role: form.get("role"),
      primary: form.get("primary") === "on",
      description: form.get("description") || "",
    };
    const response = await fetch("/api/v1/ip-assignments", {
      method: "POST",
      credentials: "include",
      headers: {
        "Content-Type": "application/json",
        "X-CSRF-Token": session.csrf_token,
      },
      body: JSON.stringify(payload),
    });
    if (!response.ok) {
      setError(
        response.status === 409
          ? "A primary IP already exists for this address family."
          : "Could not add IP assignment.",
      );
      return;
    }
    await loadIPAssignments(selectedCompute.id);
    formElement.reset();
  }

  async function updateIPAssignment(
    event: React.FormEvent<HTMLFormElement>,
    assignment: IPAssignment,
  ) {
    event.preventDefault();
    if (!session || !selectedCompute) return;
    const form = new FormData(event.currentTarget);
    const prefix = String(form.get("prefix_length") ?? "");
    const payload = {
      address: form.get("address"),
      prefix_length: prefix === "" ? undefined : Number(prefix),
      compute_id: selectedCompute.id,
      interface_or_network: form.get("interface_or_network") || undefined,
      scope: form.get("scope"),
      role: form.get("role"),
      primary: form.get("primary") === "on",
      description: form.get("description") || "",
    };
    const response = await fetch(`/api/v1/ip-assignments/${assignment.id}`, {
      method: "PUT",
      credentials: "include",
      headers: {
        "Content-Type": "application/json",
        "X-CSRF-Token": session.csrf_token,
        "If-Match": `"revision-${assignment.revision}"`,
      },
      body: JSON.stringify(payload),
    });
    if (!response.ok) {
      setError(
        response.status === 412
          ? "IP assignment changed elsewhere; reload before editing."
          : response.status === 409
            ? "Discovery-owned IP assignments cannot be edited."
            : "Could not update IP assignment.",
      );
      return;
    }
    setEditingIP(null);
    await loadIPAssignments(selectedCompute.id);
  }

  async function changeIPAssignmentLifecycle(
    assignment: IPAssignment,
    action: "archive" | "restore",
  ) {
    if (!session || !selectedCompute) return;
    const response = await fetch(
      `/api/v1/ip-assignments/${assignment.id}/${action}`,
      {
        method: "POST",
        credentials: "include",
        headers: {
          "X-CSRF-Token": session.csrf_token,
          "If-Match": `"revision-${assignment.revision}"`,
        },
      },
    );
    if (!response.ok) {
      setError(
        response.status === 412
          ? "IP assignment changed elsewhere; reload before continuing."
          : "Could not change IP assignment lifecycle.",
      );
      return;
    }
    await loadIPAssignments(selectedCompute.id);
  }

  function domainPayload(form: FormData) {
    const renewal = form.get("auto_renew");
    const registrar = String(form.get("registrar_provider_id") ?? "");
    return {
      name: form.get("name"),
      registrar_provider_id: registrar || undefined,
      registered_on: form.get("registered_on") || undefined,
      expires_on: form.get("expires_on") || undefined,
      auto_renew:
        renewal === "true" ? true : renewal === "false" ? false : null,
      nameservers: String(form.get("nameservers") ?? "")
        .split(/[\s,]+/)
        .map((value) => value.trim())
        .filter(Boolean),
      description: form.get("description") || "",
      notes: form.get("notes") || "",
      tag_ids: [],
    };
  }

  async function createDomain(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!session) return;
    const formElement = event.currentTarget;
    const response = await fetch("/api/v1/domains", {
      method: "POST",
      credentials: "include",
      headers: {
        "Content-Type": "application/json",
        "X-CSRF-Token": session.csrf_token,
      },
      body: JSON.stringify(domainPayload(new FormData(formElement))),
    });
    if (!response.ok) {
      setError(
        response.status === 409
          ? "That domain is already in inventory."
          : "Could not create domain.",
      );
      return;
    }
    const domain = (await response.json()) as Domain;
    setDomains((current) =>
      [...current, domain].sort((a, b) => a.name.localeCompare(b.name)),
    );
    setSelectedDomain(domain);
    formElement.reset();
  }

  async function updateDomain(
    event: React.FormEvent<HTMLFormElement>,
    snapshot: DomainEditSnapshot,
  ) {
    event.preventDefault();
    if (
      !session ||
      selectedDomain?.id !== snapshot.id ||
      editingDomain !== true
    )
      return;
    const response = await fetch(`/api/v1/domains/${snapshot.id}`, {
      method: "PUT",
      credentials: "include",
      headers: {
        "Content-Type": "application/json",
        "X-CSRF-Token": session.csrf_token,
        "If-Match": snapshot.etag,
      },
      body: JSON.stringify(
        domainWriteFromForm(new FormData(event.currentTarget), {
          tag_ids: [...snapshot.preservedTagIDs],
        }),
      ),
    });
    if (!response.ok) {
      setError(
        response.status === 412
          ? "Domain changed elsewhere; reload before editing."
          : "Could not update domain.",
      );
      return;
    }
    const updated = (await response.json()) as Domain;
    setSelectedDomain(updated);
    setDomains((current) =>
      current.map((item) => (item.id === updated.id ? updated : item)),
    );
    setEditingDomain(false);
  }

  async function showDomain(domain: Domain) {
    const request = ++domainDetailRequest.current;
    const response = await fetch(`/api/v1/domains/${domain.id}`, {
      credentials: "include",
    });
    if (request !== domainDetailRequest.current) return;
    if (!response.ok) {
      setError("Could not load domain details.");
      return;
    }
    setSelectedDomain((await response.json()) as Domain);
  }

  async function changeDomainLifecycle(action: "archive" | "restore") {
    if (!session || !selectedDomain) return;
    const response = await fetch(
      `/api/v1/domains/${selectedDomain.id}/${action}`,
      {
        method: "POST",
        credentials: "include",
        headers: {
          "X-CSRF-Token": session.csrf_token,
          "If-Match": `"revision-${selectedDomain.revision}"`,
        },
      },
    );
    if (!response.ok) {
      setError(
        response.status === 412
          ? "Domain changed elsewhere; reload before continuing."
          : `Could not ${action} domain.`,
      );
      return;
    }
    const updated = (await response.json()) as Domain;
    setSelectedDomain(updated);
    setDomains((current) =>
      current.map((item) => (item.id === updated.id ? updated : item)),
    );
    setEditingDomain(false);
    void loadDomains(search);
  }

  async function showCompute(compute: Compute) {
    await showComputeByID(compute.id);
  }

  async function showComputeByID(computeID: string) {
    const request = ++computeDetailRequest.current;
    const { data: current, error } = await apiClient.GET(
      "/computes/{compute_id}",
      {
        params: { path: { compute_id: computeID } },
      },
    );
    if (request !== computeDetailRequest.current) return;
    if (error || !current) {
      setError("Could not load compute details.");
      return;
    }
    setSelectedCompute(current);
    setEnrollmentSecret(null);
    void loadCollectorStatus(current.id);
    if (current.agent_id) {
      const { data, error } = await apiClient.GET("/agents/{agent_id}", {
        params: { path: { agent_id: current.agent_id } },
      });
      if (!error && data) {
        setAgentDetails(data);
      } else {
        setAgentDetails(null);
      }
    } else {
      setAgentDetails(null);
    }
  }

  async function createComputeEnrollmentToken() {
    if (!session || !selectedCompute) return;
    const response = await fetch(
      `/api/v1/computes/${selectedCompute.id}/enrollment-tokens`,
      {
        method: "POST",
        credentials: "include",
        headers: {
          "Content-Type": "application/json",
          "X-CSRF-Token": session.csrf_token,
        },
        body: JSON.stringify({ expires_in_seconds: 3600 }),
      },
    );
    if (!response.ok) {
      setError(
        response.status === 409
          ? "Revoke the active agent first, or issue a replacement for the outstanding token."
          : "Could not issue enrollment token.",
      );
      return;
    }
    setEnrollmentSecret((await response.json()) as EnrollmentSecret);
  }

  async function revokeComputeAgent() {
    if (!session || !agentDetails) return;
    const response = await fetch(`/api/v1/agents/${agentDetails.id}/revoke`, {
      method: "POST",
      credentials: "include",
      headers: {
        "X-CSRF-Token": session.csrf_token,
        "If-Match": `"revision-${agentDetails.revision}"`,
      },
    });
    if (!response.ok) {
      setError(
        response.status === 412
          ? "Agent state changed elsewhere; reload the compute details."
          : "Could not revoke agent.",
      );
      return;
    }
    const updated = (await response.json()) as AgentDetails;
    setAgentDetails(updated);
    setSelectedCompute((current) =>
      current ? { ...current, agent_status: "revoked" } : current,
    );
    setEnrollmentSecret(null);
  }

  async function revokeOutstandingEnrollmentToken() {
    if (!session || !enrollmentSecret) return;
    const response = await fetch(
      `/api/v1/enrollment-tokens/${enrollmentSecret.id}`,
      {
        method: "DELETE",
        credentials: "include",
        headers: { "X-CSRF-Token": session.csrf_token },
      },
    );
    if (!response.ok) {
      setError("Could not revoke outstanding enrollment token.");
      return;
    }
    setEnrollmentSecret(null);
  }

  return (
    <ThemeProvider theme={theme}>
      <CssBaseline />
      <div className="app-root">
        {session ? (
          <>
            <AppBar position="static">
              <Toolbar sx={{ justifyContent: "space-between" }}>
                <Typography variant="h6">Portinaia</Typography>
                <Stack
                  direction="row"
                  spacing={2}
                  sx={{ alignItems: "center" }}
                >
                  <Typography variant="body2">{session.username}</Typography>
                  <Button color="inherit" onClick={() => void logout()}>
                    Sign out
                  </Button>
                </Stack>
              </Toolbar>
            </AppBar>
            <Container maxWidth="lg" sx={{ py: 5 }}>
              <Typography component="h1" variant="h4" gutterBottom>
                Infrastructure inventory
              </Typography>
              <Typography color="text.secondary">
                Manage providers, locations, computes, and owned domains.
              </Typography>
              <AuditWorkspace />
              <FormControlLabel
                sx={{ mt: 1 }}
                control={
                  <Checkbox
                    checked={includeArchived}
                    onChange={(event) =>
                      setIncludeArchived(event.target.checked)
                    }
                  />
                }
                label="Include archived inventory"
              />
              {error && (
                <Alert severity="error" sx={{ mt: 3 }}>
                  {error}
                </Alert>
              )}
              {inventorySummary && (
                <Card sx={{ mt: 3 }}>
                  <CardContent>
                    <Typography variant="h6">Inventory counts</Typography>
                    <Stack
                      direction="row"
                      spacing={3}
                      sx={{ mt: 2, flexWrap: "wrap" }}
                    >
                      <Typography>
                        Total computes: {inventorySummary.compute_count}
                      </Typography>
                      <Typography>
                        Physical: {inventorySummary.physical_compute_count}
                      </Typography>
                      <Typography>VPS: {inventorySummary.vps_count}</Typography>
                      <Typography>VMs: {inventorySummary.vm_count}</Typography>
                      <Typography>
                        Observed containers:{" "}
                        {inventorySummary.observed_container_count}
                      </Typography>
                      <Typography>
                        Stale containers:{" "}
                        {inventorySummary.stale_container_count}
                      </Typography>
                    </Stack>
                    <Typography variant="caption" color="text.secondary">
                      Active inventory counts; stale containers are reported
                      separately from current observations.
                    </Typography>
                  </CardContent>
                </Card>
              )}
              <InventoryTree
                roots={treeRoots}
                childrenByID={treeChildren}
                expanded={expandedTreeNodes}
                childHasMore={treeChildHasMore}
                childCursors={treeChildCursors}
                rootHasMore={treeRootHasMore}
                rootCursor={treeRootCursor}
                onToggle={(node) => void toggleTreeNode(node)}
                onNavigate={(node) =>
                  node.kind === "compute"
                    ? void showComputeByID(node.id)
                    : void toggleTreeNode(node)
                }
                onLoadChildren={(id, cursor) =>
                  void loadTreeChildren(id, cursor)
                }
                onLoadRoots={(cursor) => void loadTreeRoots(cursor)}
              />
              <Card data-testid="domain-workspace" sx={{ mt: 3 }}>
                <CardContent>
                  <Typography variant="h6" gutterBottom>
                    Owned domains
                  </Typography>
                  <Stack
                    direction={{ xs: "column", lg: "row" }}
                    spacing={3}
                    sx={{ alignItems: "flex-start" }}
                  >
                    <DomainList
                      domains={domains}
                      selectedID={selectedDomain?.id}
                      hasMore={domainsHaveMore}
                      cursor={domainCursor}
                      onSelect={(domain) => void showDomain(domain)}
                      onLoadMore={(cursor) => void loadDomains(search, cursor)}
                    />
                    <DomainCreateForm
                      providers={providers}
                      onSubmit={(event) => void createDomain(event)}
                    />
                    {selectedDomain && (
                      <DomainDetails
                        domain={selectedDomain}
                        providers={providers}
                        editing={editingDomain}
                        onToggleEdit={() => setEditingDomain((value) => !value)}
                        onLifecycle={() =>
                          void changeDomainLifecycle(
                            selectedDomain.archived_at ? "restore" : "archive",
                          )
                        }
                        onUpdate={
                          ((event, snapshot) =>
                            void updateDomain(
                              event,
                              snapshot,
                            )) satisfies DomainEditSubmission
                        }
                      />
                    )}
                  </Stack>
                </CardContent>
              </Card>
              <Stack
                direction={{ xs: "column", md: "row" }}
                spacing={3}
                sx={{ mt: 4, alignItems: "flex-start" }}
              >
                <ProviderList
                  providers={providers}
                  selectedID={selectedProvider?.id}
                  query={search}
                  hasMore={providerHasMore}
                  cursor={providerCursor}
                  onQueryChange={setSearch}
                  onSelect={(provider) => void showProvider(provider)}
                  onLoadMore={(cursor) => void loadProviders(search, cursor)}
                />
                <Stack spacing={3} sx={{ width: "100%" }}>
                  <Card>
                    <CardContent>
                      <Typography variant="h6">Compute overview</Typography>
                      <Stack
                        direction="row"
                        spacing={3}
                        sx={{ mt: 2, flexWrap: "wrap" }}
                      >
                        <Typography>
                          Total: {inventorySummary?.compute_count ?? 0}
                        </Typography>
                        <Typography>
                          Physical:{" "}
                          {inventorySummary?.physical_compute_count ?? 0}
                        </Typography>
                        <Typography>
                          VPS: {inventorySummary?.vps_count ?? 0}
                        </Typography>
                        <Typography>
                          VMs: {inventorySummary?.vm_count ?? 0}
                        </Typography>
                        <Typography>
                          Containers:{" "}
                          {inventorySummary?.observed_container_count ?? 0}
                        </Typography>
                        <Typography>
                          Stale containers:{" "}
                          {inventorySummary?.stale_container_count ?? 0}
                        </Typography>
                      </Stack>
                      <Typography variant="caption" color="text.secondary">
                        Counts cover all active inventory; stale observations
                        are shown separately.
                      </Typography>
                      <List disablePadding sx={{ mt: 1 }}>
                        {computes.map((compute) => (
                          <ListItemButton
                            key={compute.id}
                            selected={selectedCompute?.id === compute.id}
                            onClick={() => void showCompute(compute)}
                            sx={{ pl: compute.parent_compute_id ? 5 : 2 }}
                          >
                            <ListItemText
                              primary={compute.display_name}
                              secondary={`${compute.kind.toUpperCase()} · ${compute.ownership} · ${compute.agent_status}`}
                            />
                          </ListItemButton>
                        ))}
                        {computes.length === 0 && (
                          <Typography color="text.secondary" sx={{ p: 1 }}>
                            No computes yet.
                          </Typography>
                        )}
                      </List>
                      {computeHasMore && computeCursor && (
                        <Button
                          size="small"
                          onClick={() =>
                            void loadComputes(search, computeCursor)
                          }
                        >
                          Load more computes
                        </Button>
                      )}
                    </CardContent>
                  </Card>
                  <ComputeCreateForm
                    providers={providers}
                    locations={locations}
                    computes={computes}
                    onSubmit={(event) => void createCompute(event)}
                  />
                  {selectedCompute && (
                    <ComputeObservationPanel
                      observation={selectedCompute.observation}
                      collectors={collectorStatus}
                      containers={containers}
                      ipAssignments={ipAssignments}
                    />
                  )}
                  {selectedCompute && (
                    <Card>
                      <CardContent>
                        <Stack
                          direction="row"
                          sx={{ justifyContent: "space-between" }}
                        >
                          <Typography variant="h6">
                            {selectedCompute.display_name}
                          </Typography>
                          <Stack direction="row" spacing={1}>
                            {!selectedCompute.archived_at && (
                              <Button
                                onClick={() =>
                                  setEditingCompute((value) => !value)
                                }
                              >
                                {editingCompute ? "Cancel edit" : "Edit"}
                              </Button>
                            )}
                            <Button
                              color={
                                selectedCompute.archived_at
                                  ? "primary"
                                  : "error"
                              }
                              onClick={() =>
                                void changeLifecycle(
                                  "compute",
                                  selectedCompute,
                                  selectedCompute.archived_at
                                    ? "restore"
                                    : "archive",
                                )
                              }
                            >
                              {selectedCompute.archived_at
                                ? "Restore"
                                : "Archive"}
                            </Button>
                          </Stack>
                        </Stack>
                        {editingCompute ? (
                          <Stack
                            component="form"
                            spacing={2}
                            sx={{ mt: 2 }}
                            onSubmit={(event) => void updateCompute(event)}
                          >
                            <TextField
                              name="display_name"
                              label="Name"
                              defaultValue={selectedCompute.display_name}
                              required
                            />
                            <TextField
                              name="kind"
                              label="Kind"
                              select
                              defaultValue={selectedCompute.kind}
                            >
                              <MenuItem value="physical">Physical</MenuItem>
                              <MenuItem value="vps">VPS</MenuItem>
                              <MenuItem value="vm">VM</MenuItem>
                            </TextField>
                            <TextField
                              name="ownership"
                              label="Ownership"
                              select
                              defaultValue={selectedCompute.ownership}
                            >
                              <MenuItem value="owned">Owned</MenuItem>
                              <MenuItem value="rented">Rented</MenuItem>
                              <MenuItem value="other">Other</MenuItem>
                            </TextField>
                            <TextField
                              name="provider_id"
                              label="Provider"
                              select
                              defaultValue={selectedCompute.provider_id ?? ""}
                            >
                              <MenuItem value="">No provider</MenuItem>
                              {providers.map((provider) => (
                                <MenuItem key={provider.id} value={provider.id}>
                                  {provider.name}
                                </MenuItem>
                              ))}
                            </TextField>
                            <TextField
                              name="provider_resource_id"
                              label="Provider reference"
                              defaultValue={
                                selectedCompute.provider_resource_id ?? ""
                              }
                            />
                            <TextField
                              name="location_id"
                              label="Location"
                              select
                              defaultValue={selectedCompute.location_id ?? ""}
                            >
                              <MenuItem value="">No location</MenuItem>
                              {locations.map((location) => (
                                <MenuItem key={location.id} value={location.id}>
                                  {location.name}
                                </MenuItem>
                              ))}
                            </TextField>
                            <TextField
                              name="parent_compute_id"
                              label="Parent compute"
                              select
                              defaultValue={
                                selectedCompute.parent_compute_id ?? ""
                              }
                            >
                              <MenuItem value="">No parent</MenuItem>
                              {computes
                                .filter(
                                  (item) => item.id !== selectedCompute.id,
                                )
                                .map((compute) => (
                                  <MenuItem key={compute.id} value={compute.id}>
                                    {compute.display_name}
                                  </MenuItem>
                                ))}
                            </TextField>
                            <TextField
                              name="description"
                              label="Description"
                              defaultValue={selectedCompute.description}
                              multiline
                            />
                            <TextField
                              name="notes"
                              label="Notes"
                              defaultValue={selectedCompute.notes}
                              multiline
                            />
                            <Button
                              type="submit"
                              variant="contained"
                              sx={{ alignSelf: "flex-start" }}
                            >
                              Save compute
                            </Button>
                          </Stack>
                        ) : (
                          <>
                            <Divider sx={{ my: 2 }} />
                            <Typography color="text.secondary">
                              {selectedCompute.kind.toUpperCase()} ·{" "}
                              {selectedCompute.ownership} ·{" "}
                              {selectedCompute.agent_status}
                            </Typography>
                            {selectedCompute.provider_id && (
                              <Typography sx={{ mt: 1 }}>
                                Provider:{" "}
                                {providers.find(
                                  (item) =>
                                    item.id === selectedCompute.provider_id,
                                )?.name ?? selectedCompute.provider_id}
                              </Typography>
                            )}
                            {selectedCompute.location_id && (
                              <Typography sx={{ mt: 1 }}>
                                Location:{" "}
                                {locations.find(
                                  (item) =>
                                    item.id === selectedCompute.location_id,
                                )?.name ?? selectedCompute.location_id}
                              </Typography>
                            )}
                            {selectedCompute.parent_compute_id && (
                              <Typography sx={{ mt: 1 }}>
                                Parent:{" "}
                                {computes.find(
                                  (item) =>
                                    item.id ===
                                    selectedCompute.parent_compute_id,
                                )?.display_name ??
                                  selectedCompute.parent_compute_id}
                              </Typography>
                            )}
                            {selectedCompute.provider_resource_id && (
                              <Typography sx={{ mt: 1 }}>
                                Provider reference:{" "}
                                {selectedCompute.provider_resource_id}
                              </Typography>
                            )}
                            {selectedCompute.description && (
                              <Typography sx={{ mt: 1 }}>
                                {selectedCompute.description}
                              </Typography>
                            )}
                            {selectedCompute.notes && (
                              <Typography sx={{ mt: 1 }}>
                                Notes: {selectedCompute.notes}
                              </Typography>
                            )}
                          </>
                        )}
                      </CardContent>
                    </Card>
                  )}
                  {selectedCompute && (
                    <Card>
                      <CardContent>
                        <Stack
                          direction="row"
                          sx={{
                            justifyContent: "space-between",
                            alignItems: "center",
                          }}
                        >
                          <Typography variant="h6">Discovery agent</Typography>
                          <Button
                            size="small"
                            onClick={() => void showCompute(selectedCompute)}
                          >
                            Refresh status
                          </Button>
                        </Stack>
                        <Typography color="text.secondary" sx={{ mt: 1 }}>
                          Status:{" "}
                          {agentDetails?.status ?? selectedCompute.agent_status}
                        </Typography>
                        {agentDetails && (
                          <>
                            <Typography variant="body2">
                              Client: {agentDetails.client_version} · protocol{" "}
                              {agentDetails.protocol_version} · credential
                              generation {agentDetails.credential_generation}
                            </Typography>
                            <Typography variant="body2">
                              Last contact:{" "}
                              {agentDetails.last_heartbeat_at ?? "Never"}
                            </Typography>
                            <Typography variant="body2">
                              Last successful snapshot:{" "}
                              {agentDetails.last_successful_snapshot_at ??
                                "Never"}
                            </Typography>
                            {agentDetails.last_error && (
                              <Alert severity="warning" sx={{ mt: 1 }}>
                                {agentDetails.last_error}
                              </Alert>
                            )}
                            {agentDetails.status !== "revoked" && (
                              <Button
                                color="error"
                                onClick={() => void revokeComputeAgent()}
                              >
                                Revoke agent credential
                              </Button>
                            )}
                          </>
                        )}
                        <Divider sx={{ my: 2 }} />
                        <Typography variant="subtitle2">
                          Collector status
                        </Typography>
                        <List dense disablePadding>
                          {collectorStatus.map((collector) => (
                            <ListItemButton
                              key={collector.collector_key}
                              disableRipple
                            >
                              <ListItemText
                                primary={`${collector.collector}${collector.owner_uid === undefined ? "" : ` · UID ${collector.owner_uid}`}${collector.connection_name ? ` · ${collector.connection_name}` : ""}`}
                                secondary={
                                  collector.last_status === "failed"
                                    ? `Failed ${collector.last_received_at}: ${collector.last_error_code ?? "error"} ${collector.last_error_message ?? ""}`
                                    : `Last successful collection: ${collector.last_successful_at ?? collector.last_received_at}`
                                }
                              />
                            </ListItemButton>
                          ))}
                          {collectorStatus.length === 0 && (
                            <Typography variant="body2" color="text.secondary">
                              No snapshot collector results have been reported
                              yet.
                            </Typography>
                          )}
                        </List>
                        {!selectedCompute.archived_at &&
                          (!agentDetails ||
                            agentDetails.status === "revoked") && (
                            <Button
                              variant="contained"
                              sx={{ mt: 1 }}
                              onClick={() =>
                                void createComputeEnrollmentToken()
                              }
                            >
                              Create one-time enrollment token
                            </Button>
                          )}
                        {enrollmentSecret && (
                          <Stack spacing={1} sx={{ mt: 2 }}>
                            <Alert severity="warning">
                              Copy this token now. It is shown only once and
                              expires at {enrollmentSecret.expires_at}.
                            </Alert>
                            <TextField
                              label="One-time enrollment token"
                              value={enrollmentSecret.token}
                              slotProps={{ input: { readOnly: true } }}
                            />
                            <Button
                              color="error"
                              sx={{ alignSelf: "flex-start" }}
                              onClick={() =>
                                void revokeOutstandingEnrollmentToken()
                              }
                            >
                              Revoke unused token
                            </Button>
                          </Stack>
                        )}
                      </CardContent>
                    </Card>
                  )}
                  {selectedCompute && (
                    <Card>
                      <CardContent>
                        <Typography variant="h6" gutterBottom>
                          IP assignments
                        </Typography>
                        <List disablePadding>
                          {ipAssignments.map((assignment) => (
                            <div key={assignment.id}>
                              {editingIP === assignment.id ? (
                                <Stack
                                  component="form"
                                  spacing={2}
                                  sx={{ py: 2 }}
                                  onSubmit={(event) =>
                                    void updateIPAssignment(event, assignment)
                                  }
                                >
                                  <TextField
                                    name="address"
                                    label="Address"
                                    defaultValue={assignment.address}
                                    required
                                  />
                                  <TextField
                                    name="prefix_length"
                                    label="Prefix length"
                                    type="number"
                                    defaultValue={
                                      assignment.prefix_length ?? ""
                                    }
                                  />
                                  <TextField
                                    name="interface_or_network"
                                    label="Interface"
                                    defaultValue={
                                      assignment.interface_or_network ?? ""
                                    }
                                  />
                                  <TextField
                                    name="scope"
                                    label="Scope"
                                    select
                                    defaultValue={assignment.scope}
                                  >
                                    <MenuItem value="public">Public</MenuItem>
                                    <MenuItem value="private">Private</MenuItem>
                                    <MenuItem value="loopback">
                                      Loopback
                                    </MenuItem>
                                    <MenuItem value="link_local">
                                      Link-local
                                    </MenuItem>
                                    <MenuItem value="other">Other</MenuItem>
                                  </TextField>
                                  <TextField
                                    name="role"
                                    label="Role"
                                    select
                                    defaultValue={assignment.role}
                                  >
                                    <MenuItem value="primary">Primary</MenuItem>
                                    <MenuItem value="management">
                                      Management
                                    </MenuItem>
                                    <MenuItem value="service">Service</MenuItem>
                                    <MenuItem value="other">Other</MenuItem>
                                  </TextField>
                                  <FormControlLabel
                                    control={
                                      <Checkbox
                                        name="primary"
                                        defaultChecked={assignment.primary}
                                      />
                                    }
                                    label="Primary address"
                                  />
                                  <TextField
                                    name="description"
                                    label="Description"
                                    defaultValue={assignment.description ?? ""}
                                    multiline
                                  />
                                  <Stack direction="row" spacing={1}>
                                    <Button type="submit" variant="contained">
                                      Save IP
                                    </Button>
                                    <Button onClick={() => setEditingIP(null)}>
                                      Cancel
                                    </Button>
                                  </Stack>
                                </Stack>
                              ) : (
                                <Stack
                                  direction="row"
                                  spacing={1}
                                  sx={{ alignItems: "center", py: 1 }}
                                >
                                  <ListItemText
                                    primary={`${assignment.address}${assignment.prefix_length === undefined ? "" : `/${assignment.prefix_length}`}`}
                                    secondary={`${assignment.scope} · ${assignment.role}${assignment.primary ? " · primary" : ""} · ${assignment.source}${assignment.archived_at ? " · archived" : ""}`}
                                  />
                                  {assignment.source === "manual" && (
                                    <>
                                      {!assignment.archived_at && (
                                        <Button
                                          size="small"
                                          onClick={() =>
                                            setEditingIP(assignment.id)
                                          }
                                        >
                                          Edit
                                        </Button>
                                      )}
                                      <Button
                                        size="small"
                                        color={
                                          assignment.archived_at
                                            ? "primary"
                                            : "error"
                                        }
                                        onClick={() =>
                                          void changeIPAssignmentLifecycle(
                                            assignment,
                                            assignment.archived_at
                                              ? "restore"
                                              : "archive",
                                          )
                                        }
                                      >
                                        {assignment.archived_at
                                          ? "Restore"
                                          : "Archive"}
                                      </Button>
                                    </>
                                  )}
                                </Stack>
                              )}
                            </div>
                          ))}
                          {ipAssignments.length === 0 && (
                            <Typography color="text.secondary" sx={{ p: 1 }}>
                              No IP assignments for this compute.
                            </Typography>
                          )}
                        </List>
                        {ipHasMore && ipCursor && (
                          <Button
                            size="small"
                            onClick={() =>
                              void loadIPAssignments(
                                selectedCompute.id,
                                ipCursor,
                              )
                            }
                          >
                            Load more IP assignments
                          </Button>
                        )}
                        {!selectedCompute.archived_at && (
                          <>
                            <Divider sx={{ my: 2 }} />
                            <Typography variant="subtitle1" gutterBottom>
                              Add manual IP
                            </Typography>
                            <Stack
                              component="form"
                              spacing={2}
                              onSubmit={(event) =>
                                void createIPAssignment(event)
                              }
                            >
                              <TextField
                                name="address"
                                label="Address"
                                placeholder="192.0.2.10"
                                required
                              />
                              <TextField
                                name="prefix_length"
                                label="Prefix length (optional)"
                                type="number"
                              />
                              <TextField
                                name="interface_or_network"
                                label="Interface"
                              />
                              <TextField
                                name="scope"
                                label="Scope"
                                select
                                defaultValue="private"
                              >
                                <MenuItem value="public">Public</MenuItem>
                                <MenuItem value="private">Private</MenuItem>
                                <MenuItem value="loopback">Loopback</MenuItem>
                                <MenuItem value="link_local">
                                  Link-local
                                </MenuItem>
                                <MenuItem value="other">Other</MenuItem>
                              </TextField>
                              <TextField
                                name="role"
                                label="Role"
                                select
                                defaultValue="management"
                              >
                                <MenuItem value="primary">Primary</MenuItem>
                                <MenuItem value="management">
                                  Management
                                </MenuItem>
                                <MenuItem value="service">Service</MenuItem>
                                <MenuItem value="other">Other</MenuItem>
                              </TextField>
                              <FormControlLabel
                                control={<Checkbox name="primary" />}
                                label="Primary address"
                              />
                              <TextField
                                name="description"
                                label="Description"
                                multiline
                              />
                              <Button
                                type="submit"
                                variant="contained"
                                sx={{ alignSelf: "flex-start" }}
                              >
                                Add IP assignment
                              </Button>
                            </Stack>
                          </>
                        )}
                      </CardContent>
                    </Card>
                  )}
                  {selectedCompute && (
                    <Card>
                      <CardContent>
                        <Typography variant="h6">Containers</Typography>
                        <List disablePadding>
                          {containers.map((container) => (
                            <ListItemButton
                              key={container.id}
                              selected={selectedContainer?.id === container.id}
                              onClick={() => void showContainer(container)}
                            >
                              <ListItemText
                                primary={container.name}
                                secondary={`${container.observation?.state ?? "unknown"}${container.observation?.stale_at ? " · stale observation" : ""} · owner UID ${container.podman_owner_uid}`}
                              />
                            </ListItemButton>
                          ))}
                          {containers.length === 0 && (
                            <Typography color="text.secondary" sx={{ p: 1 }}>
                              No container observations for this compute.
                            </Typography>
                          )}
                        </List>
                        {containersHaveMore && containerCursor && (
                          <Button
                            size="small"
                            onClick={() =>
                              void loadContainers(
                                selectedCompute.id,
                                containerCursor,
                              )
                            }
                          >
                            Load more containers
                          </Button>
                        )}
                        {selectedContainer?.observation && (
                          <>
                            <Divider sx={{ my: 2 }} />
                            <Typography variant="subtitle1">
                              {selectedContainer.name}
                            </Typography>
                            <Typography color="text.secondary">
                              Image: {selectedContainer.observation.image_name}
                            </Typography>
                            <Typography color="text.secondary">
                              State: {selectedContainer.observation.state} ·
                              health: {selectedContainer.observation.health}
                              {selectedContainer.observation.stale_at
                                ? " · stale, not verified running"
                                : ""}
                            </Typography>
                            {selectedContainer.observation.image_id && (
                              <Typography variant="body2">
                                Image ID:{" "}
                                {selectedContainer.observation.image_id}
                              </Typography>
                            )}
                            {selectedContainer.observation.image_digest && (
                              <Typography variant="body2">
                                Digest:{" "}
                                {selectedContainer.observation.image_digest}
                              </Typography>
                            )}
                            <Typography variant="body2">
                              Restart:{" "}
                              {selectedContainer.observation.restart_policy}
                            </Typography>
                            <Typography variant="body2">
                              Ports:{" "}
                              {selectedContainer.observation.ports
                                .map(
                                  (port) =>
                                    `${port.host_port ? `${port.host_port}:` : ""}${port.container_port}/${port.protocol}`,
                                )
                                .join(", ") || "None"}
                            </Typography>
                            <Typography variant="body2">
                              Networks:{" "}
                              {selectedContainer.observation.networks
                                .map(
                                  (network) =>
                                    `${network.name}${network.addresses.length ? ` (${network.addresses.map((address) => `${address.address}${address.prefix_length === undefined ? "" : `/${address.prefix_length}`}`).join(", ")})` : ""}`,
                                )
                                .join(", ") || "None"}
                            </Typography>
                            <Typography variant="body2">
                              Mounts:{" "}
                              {selectedContainer.observation.mounts
                                .map(
                                  (mount) =>
                                    `${mount.source}:${mount.destination}${mount.read_only ? " (read-only)" : ""}`,
                                )
                                .join(", ") || "None"}
                            </Typography>
                            <Typography variant="body2">
                              Labels:{" "}
                              {selectedContainer.observation.labels
                                .map((label) => `${label.key}=${label.value}`)
                                .join(", ") || "None"}
                            </Typography>
                          </>
                        )}
                      </CardContent>
                    </Card>
                  )}
                  <Card>
                    <CardContent>
                      <Typography variant="h6">Locations</Typography>
                      <List disablePadding>
                        {locations.map((location) => (
                          <ListItemButton
                            key={location.id}
                            selected={selectedLocation?.id === location.id}
                            onClick={() => void showLocation(location)}
                          >
                            <ListItemText
                              primary={location.name}
                              secondary={
                                location.type === "provider_region"
                                  ? "Provider region"
                                  : "Site"
                              }
                            />
                          </ListItemButton>
                        ))}
                        {locations.length === 0 && (
                          <Typography color="text.secondary" sx={{ p: 1 }}>
                            No locations yet.
                          </Typography>
                        )}
                      </List>
                      {locationHasMore && locationCursor && (
                        <Button
                          size="small"
                          onClick={() =>
                            void loadLocations(search, locationCursor)
                          }
                        >
                          Load more locations
                        </Button>
                      )}
                    </CardContent>
                  </Card>
                  <LocationCreateForm
                    locations={locations}
                    providers={providers}
                    onSubmit={(event) => void createLocation(event)}
                  />
                  {selectedLocation && (
                    <LocationDetails
                      location={selectedLocation}
                      providers={providers}
                      locations={locations}
                      editing={editingLocation}
                      onToggleEdit={() => setEditingLocation((value) => !value)}
                      onLifecycle={() =>
                        void changeLifecycle(
                          "location",
                          selectedLocation,
                          selectedLocation.archived_at ? "restore" : "archive",
                        )
                      }
                      onUpdate={(event) => void updateLocation(event)}
                    />
                  )}
                  <ProviderWorkspace
                    selected={selectedProvider}
                    editing={editingProvider}
                    onCreate={(event) => void createProvider(event)}
                    onUpdate={(event) => void updateProvider(event)}
                    onToggleEdit={() => setEditingProvider((value) => !value)}
                    onLifecycle={() => {
                      if (selectedProvider) {
                        void changeLifecycle(
                          "provider",
                          selectedProvider,
                          selectedProvider.archived_at ? "restore" : "archive",
                        );
                      }
                    }}
                  />
                </Stack>
              </Stack>
            </Container>
          </>
        ) : (
          <LoginForm error={error} loading={loading} onSubmit={login} />
        )}
      </div>
    </ThemeProvider>
  );
}
