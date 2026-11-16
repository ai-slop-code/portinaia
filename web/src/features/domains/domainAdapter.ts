import type { components } from "../../api/generated";

export type Domain = components["schemas"]["Domain"];
export type DomainWrite = components["schemas"]["DomainWrite"];
export type RegistrarOption = Pick<
  components["schemas"]["Provider"],
  "id" | "name" | "roles"
>;

export type DomainFormValues = {
  name: string;
  registrar_provider_id: string;
  registered_on: string;
  expires_on: string;
  auto_renew: "unknown" | "true" | "false";
  nameservers: string;
  description: string;
  notes: string;
};

export type DomainEditSnapshot = Readonly<{
  id: string;
  etag: string;
  initialValues: DomainFormValues;
  preservedTagIDs: readonly string[];
}>;

export function domainFormValues(domain: Domain): DomainFormValues {
  return {
    name: domain.name,
    registrar_provider_id: domain.registrar_provider_id ?? "",
    registered_on: domain.registered_on ?? "",
    expires_on: domain.expires_on ?? "",
    auto_renew:
      domain.auto_renew == null
        ? "unknown"
        : (String(domain.auto_renew) as "true" | "false"),
    nameservers: domain.nameservers.join("\n"),
    description: domain.description ?? "",
    notes: domain.notes ?? "",
  };
}

export function domainEditSnapshot(domain: Domain): DomainEditSnapshot {
  return Object.freeze({
    id: domain.id,
    etag: `"revision-${domain.revision}"`,
    initialValues: Object.freeze(domainFormValues(domain)),
    preservedTagIDs: Object.freeze([...domain.tag_ids]),
  });
}

export function domainWriteFromForm(
  form: FormData,
  existing?: Pick<Domain, "tag_ids">,
): DomainWrite {
  const renewal = String(form.get("auto_renew") ?? "unknown");
  const registrar = String(form.get("registrar_provider_id") ?? "");
  return {
    name: String(form.get("name") ?? "").trim(),
    ...(registrar ? { registrar_provider_id: registrar } : {}),
    ...(String(form.get("registered_on") ?? "")
      ? { registered_on: String(form.get("registered_on")) }
      : {}),
    ...(String(form.get("expires_on") ?? "")
      ? { expires_on: String(form.get("expires_on")) }
      : {}),
    auto_renew: renewal === "true" ? true : renewal === "false" ? false : null,
    nameservers: String(form.get("nameservers") ?? "")
      .split(/[\s,]+/)
      .map((value) => value.trim())
      .filter(Boolean),
    description: String(form.get("description") ?? ""),
    notes: String(form.get("notes") ?? ""),
    tag_ids: existing?.tag_ids ?? [],
  };
}

export function mergeRegistrarOptions<T extends RegistrarOption>(
  page: readonly T[],
  selectedID?: string,
  hydrated?: T,
): T[] {
  const options = [...page];
  if (
    selectedID &&
    !options.some((option) => option.id === selectedID) &&
    hydrated?.id === selectedID
  ) {
    options.push(hydrated);
  }
  return options.filter((option) => option.roles.includes("registrar"));
}

export function normalizeDomainError(status: number, code?: string): string {
  if (status === 412)
    return "This domain changed elsewhere. Your edits are preserved; reload before saving again.";
  if (status === 409)
    return code === "duplicate_domain" || code === "domain_conflict"
      ? "A domain with this normalized name already exists."
      : "This domain conflicts with a linked DNS zone or another domain. Review the conflict before retrying.";
  if (status === 422)
    return code === "invalid_domain" || code === "domain_normalization_failed"
      ? "Enter a valid DNS domain name. International names are normalized to lowercase IDNA form."
      : "Some domain details are invalid. Review the highlighted fields.";
  return "Could not save the domain. Try again.";
}
