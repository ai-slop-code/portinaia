import type { Light } from "../ui/primitives";
import type { AuditEvent, Compute, Domain } from "./data";

export function computeLight(compute: Pick<Compute, "agent_status">): Light {
  switch (compute.agent_status) {
    case "online":
      return "live";
    case "offline":
      return "offline";
    case "revoked":
      return "failed";
    default:
      return "unset";
  }
}

export const agentStatusLabel: Record<Compute["agent_status"], string> = {
  online: "Online",
  offline: "Offline",
  revoked: "Agent revoked",
  unenrolled: "No agent",
};

export const kindLabel: Record<Compute["kind"], string> = {
  physical: "Physical",
  vps: "VPS",
  vm: "VM",
};

export const ownershipLabel: Record<Compute["ownership"], string> = {
  owned: "Owned",
  rented: "Rented",
  other: "Other",
};

export function domainLight(domain: Pick<Domain, "expiry_state">): Light {
  switch (domain.expiry_state) {
    case "current":
      return "live";
    case "expiring":
      return "stale";
    case "expired":
      return "failed";
    default:
      return "unset";
  }
}

export function domainExpiryText(domain: Domain): string {
  if (!domain.expires_on) return "Expiry not set";
  const days = daysUntil(domain.expires_on);
  if (days < 0) return `Expired ${formatDate(domain.expires_on)}`;
  if (days === 0) return "Expires today";
  if (days <= 60) return `Expires in ${days} day${days === 1 ? "" : "s"}`;
  return `Expires ${formatDate(domain.expires_on)}`;
}

export function daysUntil(isoDate: string, now = new Date()): number {
  const [year, month, day] = isoDate.split("-").map(Number);
  const target = Date.UTC(year, month - 1, day);
  const today = Date.UTC(now.getFullYear(), now.getMonth(), now.getDate());
  return Math.round((target - today) / 86_400_000);
}

export function formatDate(isoDate: string): string {
  const [year, month, day] = isoDate.split("-").map(Number);
  if (!year || !month || !day) return isoDate;
  return new Date(year, month - 1, day).toLocaleDateString(undefined, {
    day: "numeric",
    month: "short",
    year: "numeric",
  });
}

export function formatTime(value: string): string {
  const date = new Date(value);
  if (Number.isNaN(date.valueOf())) return value;
  return date.toLocaleTimeString(undefined, {
    hour: "2-digit",
    minute: "2-digit",
  });
}

export function formatDateTime(value?: string | null): string {
  if (!value) return "never";
  const date = new Date(value);
  if (Number.isNaN(date.valueOf())) return value;
  return date.toLocaleString(undefined, {
    day: "numeric",
    month: "short",
    hour: "2-digit",
    minute: "2-digit",
  });
}

export function relativeTime(value?: string | null, now = Date.now()): string {
  if (!value) return "never";
  const then = new Date(value).valueOf();
  if (Number.isNaN(then)) return value;
  const seconds = Math.round((now - then) / 1000);
  if (seconds < 45) return "just now";
  const minutes = Math.round(seconds / 60);
  if (minutes < 60) return `${minutes} min ago`;
  const hours = Math.round(minutes / 60);
  if (hours < 24) return `${hours} h ago`;
  const days = Math.round(hours / 24);
  if (days < 30) return `${days} day${days === 1 ? "" : "s"} ago`;
  return formatDateTime(value);
}

export function dayHeading(value: string, now = new Date()): string {
  const date = new Date(value);
  if (Number.isNaN(date.valueOf())) return "Unknown day";
  const key = (d: Date) => d.toDateString();
  const yesterday = new Date(now);
  yesterday.setDate(now.getDate() - 1);
  const label = date.toLocaleDateString(undefined, {
    day: "numeric",
    month: "short",
  });
  if (key(date) === key(now)) return `Today · ${label}`;
  if (key(date) === key(yesterday)) return `Yesterday · ${label}`;
  return date.toLocaleDateString(undefined, {
    weekday: "long",
    day: "numeric",
    month: "short",
  });
}

export function formatBytes(bytes: number): string {
  if (!Number.isFinite(bytes) || bytes <= 0) return "0 B";
  const units = ["B", "KB", "MB", "GB", "TB"];
  const index = Math.min(
    units.length - 1,
    Math.floor(Math.log(bytes) / Math.log(1024)),
  );
  const value = bytes / 1024 ** index;
  return `${value >= 10 || index === 0 ? Math.round(value) : value.toFixed(1)} ${units[index]}`;
}

const resourceNoun: Record<string, string> = {
  compute: "host",
  provider: "provider",
  location: "place",
  domain: "domain",
  ip_assignment: "IP address",
  container: "container",
  agent: "agent",
  agent_enrollment_token: "enrollment token",
  discovery_report: "discovery snapshot",
};

const verbs: Record<string, string> = {
  create: "added",
  update: "updated",
  archive: "archived",
  restore: "restored",
  purge: "permanently deleted",
  revoke: "revoked",
};

/**
 * Turns an audit event into a sentence. `nameOf` resolves resource IDs to the
 * names the person knows; unknown resources fall back to their noun.
 */
export function describeAuditEvent(
  event: AuditEvent,
  nameOf: (id?: string) => string | undefined,
): { text: string; who: string; category: ActivityCategory } {
  const name = nameOf(event.resource_id);
  const noun = resourceNoun[event.resource_type] ?? event.resource_type;
  const subject = name ?? `a ${noun}`;
  const actor =
    event.actor_type === "administrator"
      ? "You"
      : event.actor_type === "agent"
        ? "Agent"
        : event.actor_type === "personal_access_token"
          ? "API token"
          : "System";
  const outcome =
    event.result === "success"
      ? ""
      : event.result === "rejected"
        ? " (rejected)"
        : " (failed)";

  const computeChange = event.changes.find(
    (change) => change.path === "/compute_id",
  )?.after;
  const hostName =
    event.resource_type === "discovery_report"
      ? undefined
      : nameOf(parseJSONString(computeChange));

  let text: string;
  let category: ActivityCategory = "inventory";
  switch (event.action) {
    case "discovery.snapshot.accept":
      text = hostName
        ? `${hostName} sent a discovery snapshot`
        : "Discovery snapshot accepted";
      category = "discovery";
      break;
    case "agent.enroll":
      text = `Agent enrolled${hostName ? ` on ${hostName}` : ""}`;
      category = "agents";
      break;
    case "agent.revoke":
      text = `${actor === "You" ? "You revoked" : "Revoked"} an agent${hostName ? ` on ${hostName}` : ""}`;
      category = "agents";
      break;
    case "agent.heartbeat_state":
      text = "Agent connection state changed";
      category = "agents";
      break;
    case "agent_enrollment_token.create":
      text = `${actor === "You" ? "You created" : "Created"} an enrollment token${hostName ? ` for ${hostName}` : ""}`;
      category = "agents";
      break;
    case "agent_enrollment_token.consume":
      text = "Enrollment token used";
      category = "agents";
      break;
    case "agent_enrollment_token.revoke":
      text = `${actor === "You" ? "You revoked" : "Revoked"} an unused enrollment token`;
      category = "agents";
      break;
    case "administrator.bootstrap":
      text = "Administrator account created";
      break;
    case "administrator.password_reset":
      text = "Administrator password reset";
      break;
    case "backup.db":
      text = "Database backed up";
      break;
    case "restored.db":
      text = "Database restored from a backup";
      break;
    case "container.rediscover":
      text = `${name ? `Container ${name}` : "A container"} was discovered again`;
      category = "discovery";
      break;
    default: {
      const verb = verbs[event.action.split(".").pop() ?? ""];
      const article = /^[aeiou]/i.test(noun) ? "an" : "a";
      text = verb
        ? `${actor} ${verb} ${name ? `${noun} ${name}` : `${article} ${noun}`}`
        : `${event.action} · ${subject}`;
    }
  }
  return { text: text + outcome, who: actor, category };
}

export type ActivityCategory = "discovery" | "agents" | "inventory";

function parseJSONString(value?: string | null): string | undefined {
  if (!value) return undefined;
  try {
    const parsed: unknown = JSON.parse(value);
    return typeof parsed === "string" ? parsed : undefined;
  } catch {
    return value;
  }
}
