package api_test

import (
	"context"
	_ "embed"
	"slices"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
)

//go:embed openapi.yaml
var contract []byte

func TestOpenAPIContract(t *testing.T) {
	loader := openapi3.NewLoader()
	document, err := loader.LoadFromData(contract)
	if err != nil {
		t.Fatalf("load OpenAPI contract: %v", err)
	}
	if err := document.Validate(context.Background()); err != nil {
		t.Fatalf("validate OpenAPI contract: %v", err)
	}
	if document.OpenAPI != "3.0.3" {
		t.Fatalf("OpenAPI version = %q, want 3.0.3", document.OpenAPI)
	}

	for _, name := range []string{"sessionCookie", "personalAccessToken", "enrollmentBearer", "agentBearer"} {
		if document.Components.SecuritySchemes[name] == nil {
			t.Errorf("security scheme %q is missing", name)
		}
	}

	expectedPaths := []string{
		"/auth/login",
		"/providers",
		"/inventory/tree",
		"/dashboard/summary",
		"/agent/v1/enroll",
		"/agent/v1/heartbeat",
		"/agent/v1/snapshots/{report_id}",
		"/snapshot-comparisons",
		"/dns/zones",
		"/dns/zones/{zone_id}/imports/apply",
		"/dns/drafts/{draft_id}/preview",
		"/dns/drafts/{draft_id}/apply",
		"/audit-events",
		"/health/live",
		"/health/ready",
	}
	for _, path := range expectedPaths {
		if document.Paths.Find(path) == nil {
			t.Errorf("required path %q is missing", path)
		}
	}

	operationIDs := make(map[string]string)
	for path, item := range document.Paths.Map() {
		for method, operation := range item.Operations() {
			if operation.OperationID == "" {
				t.Errorf("%s %s has no operationId", method, path)
				continue
			}
			if prior, exists := operationIDs[operation.OperationID]; exists {
				t.Errorf("operationId %q is shared by %s and %s %s", operation.OperationID, prior, method, path)
			}
			operationIDs[operation.OperationID] = method + " " + path
		}
	}
	if len(operationIDs) == 0 {
		t.Fatal("OpenAPI contract has no operations")
	}
}

func TestOpenAPIFocusedContracts(t *testing.T) {
	document := loadContract(t)

	containers := document.Paths.Find("/containers")
	if containers == nil || containers.Get == nil {
		t.Fatal("GET /containers is missing")
	}
	if containers.Post != nil {
		t.Error("POST /containers must not expose discovery-owned container creation")
	}
	if document.Components.RequestBodies["ContainerCreate"] != nil {
		t.Error("ContainerCreate request body still exists")
	}
	if document.Components.Schemas["ContainerCreate"] != nil {
		t.Error("ContainerCreate schema still exists")
	}

	snapshotWrite := document.Components.Schemas["DiscoverySnapshotWrite"].Value
	if !slices.Contains(snapshotWrite.Required, "disks") {
		t.Error("DiscoverySnapshotWrite.disks is not required")
	}
	disks := snapshotWrite.Properties["disks"].Value
	if disks.MaxItems == nil || *disks.MaxItems != 1024 {
		t.Errorf("DiscoverySnapshotWrite.disks maxItems = %v, want 1024", disks.MaxItems)
	}
	if disks.Items == nil || disks.Items.Ref != "#/components/schemas/DiskSnapshot" {
		t.Error("DiscoverySnapshotWrite.disks does not contain DiskSnapshot items")
	}
	diskSnapshot := document.Components.Schemas["DiskSnapshot"].Value
	for _, field := range []string{"name", "size_bytes"} {
		if !slices.Contains(diskSnapshot.Required, field) {
			t.Errorf("DiskSnapshot.%s is not required", field)
		}
	}

	canonical := document.Components.Schemas["CanonicalSnapshot"].Value
	for _, field := range []string{"report_id", "disks"} {
		if !slices.Contains(canonical.Required, field) {
			t.Errorf("CanonicalSnapshot.%s is not required", field)
		}
	}

	snapshotOperation := document.Paths.Find("/agent/v1/snapshots/{report_id}").Put
	if !strings.Contains(snapshotOperation.Description, "MUST equal") {
		t.Error("snapshot operation does not require body report_id to equal the path parameter")
	}
	if snapshotOperation.Extensions["x-path-body-constraints"] == nil {
		t.Error("snapshot operation has no machine-readable path/body equality constraint")
	}
	conflict := snapshotOperation.Responses.Value("409")
	if conflict == nil || conflict.Value == nil || conflict.Value.Description == nil ||
		!strings.Contains(*conflict.Value.Description, "body report_id does not match") {
		t.Error("snapshot report_id mismatch has no 409 response")
	}

	snapshotDownload := document.Paths.Find("/discovery-reports/{report_id}/snapshot").Get
	if snapshotDownload.Extensions["x-streaming"] != true {
		t.Error("canonical snapshot download is not marked as streaming")
	}
	downloadResponse := snapshotDownload.Responses.Value("200").Value
	media := downloadResponse.Content.Get("application/gzip")
	if media == nil || media.Schema == nil || media.Schema.Value == nil || media.Schema.Value.Format != "binary" {
		t.Error("canonical snapshot download is not an application/gzip binary response")
	}
	if downloadResponse.Content.Get("application/json") != nil {
		t.Error("canonical snapshot download still exposes the nested JSON response")
	}
	if downloadResponse.Headers["Content-Disposition"] == nil {
		t.Error("canonical snapshot download has no Content-Disposition header")
	}

	podman := document.Components.Schemas["PodmanCollection"].Value
	if len(podman.OneOf) != 2 || podman.Discriminator == nil || podman.Discriminator.PropertyName != "status" {
		t.Error("PodmanCollection is not a two-variant status-discriminated union")
	}
	success := document.Components.Schemas["PodmanCollectionSuccess"].Value
	failed := document.Components.Schemas["PodmanCollectionFailed"].Value
	if success.Properties["containers"] == nil || success.Properties["error"] != nil {
		t.Error("successful Podman collection must contain containers and no error")
	}
	if failed.Properties["error"] == nil || failed.Properties["containers"] != nil {
		t.Error("failed Podman collection must contain an error and no containers")
	}
	if err := podman.VisitJSON(map[string]any{
		"connection": "root",
		"owner":      "root",
		"status":     "failed",
		"error":      "socket unavailable",
		"containers": []any{},
	}); err == nil {
		t.Error("failed Podman collection with containers unexpectedly validates")
	}

	costWrite := document.Components.Schemas["CostWrite"].Value
	if len(costWrite.OneOf) != 2 || costWrite.Discriminator == nil || costWrite.Discriminator.PropertyName != "type" {
		t.Error("CostWrite is not a two-variant type-discriminated union")
	}
	recurring := document.Components.Schemas["RecurringCostWrite"].Value
	for _, field := range []string{"billing_interval", "effective_from"} {
		if !slices.Contains(recurring.Required, field) {
			t.Errorf("RecurringCostWrite.%s is not required", field)
		}
	}
	oneTime := document.Components.Schemas["OneTimeCostWrite"].Value
	if !slices.Contains(oneTime.Required, "charged_on") {
		t.Error("OneTimeCostWrite.charged_on is not required")
	}
	if recurring.Not == nil || oneTime.Not == nil {
		t.Error("cost variants do not reject incompatible fields")
	}
	validRecurring := map[string]any{
		"owner_type":       "compute",
		"owner_id":         "0190d7b6-8ef9-7ca1-a8c4-6bcbca5d73b4",
		"type":             "recurring",
		"amount":           map[string]any{"amount_minor": float64(1000), "currency": "EUR"},
		"billing_interval": "P1M",
		"effective_from":   "2026-08-04",
	}
	if err := costWrite.VisitJSON(validRecurring); err != nil {
		t.Errorf("valid recurring cost does not validate: %v", err)
	}
	intervals := document.Components.Schemas["RecurringCostWrite"].Value.Properties["billing_interval"].Value.Enum
	wantIntervals := []any{"P1M", "P3M", "P6M", "P1Y"}
	if !slices.Equal(intervals, wantIntervals) {
		t.Errorf("recurring interval enum = %v, want %v", intervals, wantIntervals)
	}
	invalidInterval := map[string]any{}
	for key, value := range validRecurring {
		invalidInterval[key] = value
	}
	invalidInterval["billing_interval"] = "P2M"
	if err := costWrite.VisitJSON(invalidInterval); err == nil {
		t.Error("unsupported recurring interval unexpectedly validates")
	}
	invalidOneTime := map[string]any{
		"owner_type":       "domain",
		"owner_id":         "0190d7b6-8ef9-7ca1-a8c4-6bcbca5d73b4",
		"type":             "one_time",
		"amount":           map[string]any{"amount_minor": float64(1000), "currency": "EUR"},
		"charged_on":       "2026-08-04",
		"billing_interval": "P1M",
	}
	if err := costWrite.VisitJSON(invalidOneTime); err == nil {
		t.Error("one-time cost with recurring-only fields unexpectedly validates")
	}

	currencyTotal := document.Components.Schemas["CurrencyTotal"].Value
	amountMinor := currencyTotal.Properties["amount_minor"].Value
	if amountMinor.Type == nil || !slices.Contains(*amountMinor.Type, "string") || amountMinor.Pattern != "^(0|[1-9][0-9]*)$" {
		t.Error("CurrencyTotal.amount_minor must be an exact non-negative decimal string")
	}
	if !strings.Contains(currencyTotal.Description, "rounded half-up") {
		t.Error("CurrencyTotal does not document exact monthly-equivalent rounding semantics")
	}

	ipUpdate := document.Paths.Find("/ip-assignments/{ip_assignment_id}").Put
	if !strings.Contains(ipUpdate.Description, "Discovery-owned") || ipUpdate.Responses.Value("409") == nil {
		t.Error("IP update does not document rejection of discovery-owned assignments")
	}

	for _, schemaName := range []string{"DnsPublicView", "DnsInternalView"} {
		description := document.Components.Schemas[schemaName].Value.Description
		if !strings.Contains(description, "must be unique") || !strings.Contains(description, "CNAME is exclusive") {
			t.Errorf("%s does not document record-type uniqueness and CNAME exclusivity", schemaName)
		}
	}
}

func TestOpenAPIRequiredScopes(t *testing.T) {
	document := loadContract(t)

	for path, item := range document.Paths.Map() {
		for method, operation := range item.Operations() {
			want := requiredScopes(path, method)
			got := scopeExtension(t, operation.Extensions["x-required-scopes"])
			if !slices.Equal(got, want) {
				t.Errorf("%s %s x-required-scopes = %v, want %v", method, path, got, want)
			}
		}
	}
}

func loadContract(t *testing.T) *openapi3.T {
	t.Helper()
	loader := openapi3.NewLoader()
	document, err := loader.LoadFromData(contract)
	if err != nil {
		t.Fatalf("load OpenAPI contract: %v", err)
	}
	return document
}

func requiredScopes(path, method string) []string {
	if strings.HasPrefix(path, "/auth/") || strings.HasPrefix(path, "/health/") || strings.HasPrefix(path, "/agent/v1/") {
		return nil
	}
	if strings.HasPrefix(path, "/personal-access-tokens") {
		return []string{"admin"}
	}
	if path == "/dashboard/summary" {
		return []string{"inventory:read", "dns:read"}
	}
	if path == "/dns/names/{name_id}/audit-events" {
		return []string{"dns:read", "audit:read"}
	}
	if path == "/audit-events" {
		return []string{"audit:read"}
	}
	if strings.HasPrefix(path, "/dns/") {
		if method == "GET" {
			return []string{"dns:read"}
		}
		return []string{"dns:write"}
	}
	if strings.HasPrefix(path, "/agents") || strings.HasPrefix(path, "/enrollment-tokens") ||
		strings.HasPrefix(path, "/discovery-reports") || path == "/snapshot-comparisons" ||
		path == "/computes/{compute_id}/enrollment-tokens" || path == "/computes/{compute_id}/snapshots" {
		return []string{"agents"}
	}
	if method == "GET" {
		return []string{"inventory:read"}
	}
	return []string{"inventory:write"}
}

func scopeExtension(t *testing.T, value any) []string {
	t.Helper()
	if value == nil {
		return nil
	}
	values, ok := value.([]any)
	if !ok {
		t.Fatalf("x-required-scopes has type %T, want array", value)
	}
	result := make([]string, len(values))
	for index, value := range values {
		scope, ok := value.(string)
		if !ok {
			t.Fatalf("x-required-scopes item has type %T, want string", value)
		}
		result[index] = scope
	}
	return result
}
