package api_test

import (
	"context"
	_ "embed"
	"slices"
	"strconv"
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

func TestUUIDSchemaDocumentsNewV7AndLegacyV4Compatibility(t *testing.T) {
	document := loadContract(t)
	schema := document.Components.Schemas["UuidV7"].Value
	if !strings.Contains(schema.Pattern, "[47]") || !strings.Contains(schema.Description, "New identifiers are UUIDv7") || !strings.Contains(schema.Description, "Existing UUIDv4") || !strings.Contains(document.Info.Description, "Existing UUIDv4 resource identifiers") {
		t.Fatalf("UUID schema/root description omit D-09 compatibility policy: pattern=%q description=%q", schema.Pattern, schema.Description)
	}
	for _, id := range []string{
		"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa",
		"0190d7b6-8ef9-7ca1-a8c4-6bcbca5d73b4",
	} {
		if err := schema.VisitJSON(id); err != nil {
			t.Errorf("UUID schema rejected compatible ID %s: %v", id, err)
		}
	}
	if err := schema.VisitJSON("00000000-0000-1000-8000-000000000001"); err == nil {
		t.Error("UUID schema accepted an unapproved identifier version")
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
	disks := snapshotWrite.Properties["disks"]
	if disks.Ref != "#/components/schemas/DiskCollectionWrite" {
		t.Error("DiscoverySnapshotWrite.disks does not reference the discriminated disk collector")
	}
	diskSuccess := document.Components.Schemas["DiskCollectionWriteSuccess"].Value
	diskItems := diskSuccess.Properties["disks"].Value
	if diskItems.MaxItems == nil || *diskItems.MaxItems != 1024 {
		t.Errorf("successful disk collection maxItems = %v, want 1024", diskItems.MaxItems)
	}
	if diskItems.Items == nil || diskItems.Items.Ref != "#/components/schemas/DiskSnapshot" {
		t.Error("successful disk collection does not contain DiskSnapshot items")
	}
	diskSnapshot := document.Components.Schemas["DiskSnapshot"].Value
	for _, field := range []string{"name", "size_bytes"} {
		if !slices.Contains(diskSnapshot.Required, field) {
			t.Errorf("DiskSnapshot.%s is not required", field)
		}
	}

	canonical := document.Components.Schemas["StructuralSnapshot"].Value
	for _, field := range []string{"format_version", "host", "disks"} {
		if !slices.Contains(canonical.Required, field) {
			t.Errorf("StructuralSnapshot.%s is not required", field)
		}
	}
	if canonical.Properties["report_id"] != nil || canonical.Properties["sha256"] != nil {
		t.Error("structural content includes report-specific identity or its own hash")
	}
	for _, field := range []string{"host", "disks", "interfaces", "filesystems"} {
		if canonical.Properties[field].Ref == "" {
			t.Errorf("StructuralSnapshot.%s does not preserve the collector outcome", field)
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
	if !strings.Contains(snapshotDownload.Description, "SnapshotDownload") || !strings.Contains(snapshotDownload.Description, "structural_sha256") {
		t.Error("snapshot download does not describe its reconstructed report envelope and structural hash")
	}
	if downloadResponse.Headers["Content-Disposition"] == nil {
		t.Error("canonical snapshot download has no Content-Disposition header")
	}

	podman := document.Components.Schemas["PodmanCollectionWrite"].Value
	if len(podman.OneOf) != 2 || podman.Discriminator == nil || podman.Discriminator.PropertyName != "status" {
		t.Error("PodmanCollection is not a two-variant status-discriminated union")
	}
	success := document.Components.Schemas["PodmanCollectionWriteSuccess"].Value
	failed := document.Components.Schemas["PodmanCollectionWriteFailed"].Value
	if success.Properties["containers"] == nil || success.Properties["error"] != nil {
		t.Error("successful Podman collection must contain containers and no error")
	}
	if failed.Properties["error"] == nil || failed.Properties["containers"] != nil {
		t.Error("failed Podman collection must contain an error and no containers")
	}
	if success.Properties["owner_uid"] == nil || failed.Properties["owner_uid"] == nil ||
		success.Properties["owner"] != nil || failed.Properties["owner"] != nil {
		t.Error("Podman collection owner is not represented as a verified numeric UID")
	}
	if err := podman.VisitJSON(map[string]any{
		"connection": "rootless", "owner_uid": 1000, "status": "success", "containers": []any{},
	}); err != nil {
		t.Errorf("Podman collection with numeric UID is invalid: %v", err)
	}
	if err := podman.VisitJSON(map[string]any{
		"connection": "rootless", "owner_uid": "alice", "status": "success", "containers": []any{},
	}); err == nil {
		t.Error("Podman collection accepts a non-numeric owner identity")
	}
	if err := podman.VisitJSON(map[string]any{
		"connection": "root",
		"owner_uid":  float64(0),
		"status":     "failed",
		"error":      "socket unavailable",
		"containers": []any{},
	}); err == nil {
		t.Error("failed Podman collection with containers unexpectedly validates")
	}
	containerSchema := document.Components.Schemas["Container"].Value
	var containerProperties openapi3.Schemas
	for _, part := range containerSchema.AllOf {
		if part.Value != nil && len(part.Value.Properties) > 0 {
			containerProperties = part.Value.Properties
		}
	}
	if containerProperties["podman_owner_uid"] == nil || containerProperties["podman_owner"] != nil {
		t.Error("Container response does not use the numeric Podman owner UID field")
	}
	ownerUID := containerProperties["podman_owner_uid"].Value
	if ownerUID.Min == nil || *ownerUID.Min != 0 || ownerUID.Max == nil || *ownerUID.Max != 4294967295 {
		t.Error("container owner UID is not bounded to the supported unsigned 32-bit range")
	}
	if !slices.Contains(document.Components.Schemas["ContainerMount"].Value.Properties["type"].Value.Enum, "other") {
		t.Error("container mount contract cannot preserve unknown types")
	}
	containerAddress := document.Components.Schemas["ContainerAddress"].Value
	if slices.Contains(containerAddress.Required, "prefix_length") || !containerAddress.Properties["prefix_length"].Value.Nullable {
		t.Error("container network addresses cannot represent an unknown prefix")
	}
	if err := containerAddress.VisitJSON(map[string]any{"address": "10.0.0.2"}); err != nil {
		t.Errorf("container address without known prefix is invalid: %v", err)
	}
	restartPolicy := document.Components.Schemas["ContainerRestartPolicy"].Value
	if !slices.Contains(restartPolicy.Enum, "unknown") || restartPolicy.Extensions["x-sql-enum-mapping"] == nil {
		t.Error("restart policy contract omits unknown state or SQL spelling mapping")
	}
	if document.Components.Schemas["ProviderRole"].Value.Extensions["x-sql-enum-mapping"] == nil {
		t.Error("provider role contract omits hosting_provider/hosting SQL mapping")
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
	currencyCode := document.Components.Schemas["CurrencyCode"].Value
	for _, code := range []string{"EUR", "JPY", "BHD", "CLF", "UYW"} {
		if err := currencyCode.VisitJSON(code); err != nil {
			t.Errorf("supported ISO currency %s is rejected: %v", code, err)
		}
	}
	if err := currencyCode.VisitJSON("ZZZ"); err == nil {
		t.Error("unsupported currency code unexpectedly validates")
	}
	currencyExponent := document.Components.Schemas["Money"].Value.Properties["currency_exponent"].Value
	if !currencyExponent.ReadOnly || currencyExponent.Max == nil || *currencyExponent.Max != 4 {
		t.Error("Money currency exponent is not server-derived and bounded through four decimal places")
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

func TestHostInterfaceDiskFilesystemCollectorUnions(t *testing.T) {
	document := loadContract(t)
	interfaceAddress := document.Components.Schemas["InterfaceAddress"].Value
	if slices.Contains(interfaceAddress.Required, "prefix_length") || !interfaceAddress.Properties["prefix_length"].Value.Nullable {
		t.Error("unknown interface prefixes are not represented without a fabricated value")
	}
	if err := interfaceAddress.VisitJSON(map[string]any{"address": "10.0.0.2"}); err != nil {
		t.Errorf("interface address with unknown prefix is invalid: %v", err)
	}
	collectorCases := []struct {
		name       string
		union      string
		success    string
		failed     string
		children   []string
		maxItems   int
		successful map[string]any
	}{
		{
			name:     "host",
			union:    "HostCollectionWrite",
			success:  "HostCollectionWriteSuccess",
			failed:   "HostCollectionWriteFailed",
			children: []string{"host"},
			successful: map[string]any{
				"status": "success",
				"host": map[string]any{
					"machine_id": "machine-1", "hostname": "host-1", "os_name": "linux",
					"os_version": "1", "kernel_version": "6.1", "architecture": "arm64",
					"logical_cpu_count": 4, "memory_bytes": 4096,
				},
			},
		},
		{
			name:       "disks",
			union:      "DiskCollectionWrite",
			success:    "DiskCollectionWriteSuccess",
			failed:     "DiskCollectionWriteFailed",
			children:   []string{"disks"},
			maxItems:   1024,
			successful: map[string]any{"status": "success", "disks": []any{}},
		},
		{
			name:       "interfaces",
			union:      "InterfaceCollectionWrite",
			success:    "InterfaceCollectionWriteSuccess",
			failed:     "InterfaceCollectionWriteFailed",
			children:   []string{"interfaces"},
			maxItems:   1024,
			successful: map[string]any{"status": "success", "interfaces": []any{}},
		},
		{
			name:       "filesystems",
			union:      "FilesystemCollectionWrite",
			success:    "FilesystemCollectionWriteSuccess",
			failed:     "FilesystemCollectionWriteFailed",
			children:   []string{"filesystems"},
			maxItems:   4096,
			successful: map[string]any{"status": "success", "filesystems": []any{}},
		},
	}
	for _, collector := range collectorCases {
		t.Run(collector.name, func(t *testing.T) {
			union := document.Components.Schemas[collector.union].Value
			if len(union.OneOf) != 2 || union.Discriminator == nil || union.Discriminator.PropertyName != "status" {
				t.Fatalf("%s is not a status-discriminated success/failure union", collector.name)
			}
			success := document.Components.Schemas[collector.success].Value
			failed := document.Components.Schemas[collector.failed].Value
			for _, child := range collector.children {
				if success.Properties[child] == nil || failed.Properties[child] != nil {
					t.Errorf("failed %s may include or successful %s omits authoritative children", collector.name, collector.name)
				}
				if collector.maxItems > 0 {
					maxItems := success.Properties[child].Value.MaxItems
					if maxItems == nil || int(*maxItems) != collector.maxItems {
						t.Errorf("successful %s maxItems = %v, want %d", collector.name, maxItems, collector.maxItems)
					}
				}
			}
			if success.Properties["error"] != nil || failed.Properties["error"] == nil {
				t.Errorf("%s failure error is not isolated to the failed variant", collector.name)
			}
			if err := union.VisitJSON(collector.successful); err != nil {
				t.Errorf("successful %s collection is invalid: %v", collector.name, err)
			}
			failedValue := map[string]any{"status": "failed", "error": "collector unavailable"}
			if err := union.VisitJSON(failedValue); err != nil {
				t.Errorf("failed %s collection is invalid: %v", collector.name, err)
			}
			for _, child := range collector.children {
				failedValue[child] = []any{}
				if collector.name == "host" {
					failedValue[child] = map[string]any{}
				}
			}
			if err := union.VisitJSON(failedValue); err == nil {
				t.Errorf("failed %s collection with authoritative children unexpectedly validates", collector.name)
			}
		})
	}

	snapshot := map[string]any{
		"report_id": "0190d7b6-8ef9-7ca1-a8c4-6bcbca5d73b4", "protocol_version": 1,
		"client_version": "1.0.0", "collection_started_at": "2026-08-03T12:34:00Z",
		"collection_finished_at": "2026-08-03T12:34:50Z",
		"host":                   map[string]any{"status": "failed", "error": "host facts unavailable"},
		"disks":                  map[string]any{"status": "success", "disks": []any{}},
		"interfaces":             map[string]any{"status": "failed", "error": "network scan unavailable"},
		"filesystems":            map[string]any{"status": "success", "filesystems": []any{}},
		"podman":                 []any{}, "errors": []any{},
	}
	if err := document.Components.Schemas["DiscoverySnapshotWrite"].Value.VisitJSON(snapshot); err != nil {
		t.Errorf("snapshot with independent collector outcomes is invalid: %v", err)
	}
}

func TestStructuralDownloadAndPaginatedHistoryContracts(t *testing.T) {
	document := loadContract(t)

	if document.Components.Schemas["CanonicalSnapshot"] != nil {
		t.Error("download still exposes the mixed legacy CanonicalSnapshot schema")
	}
	structural := document.Components.Schemas["StructuralSnapshot"].Value
	for _, field := range []string{"report_id", "sha256", "errors"} {
		if structural.Properties[field] != nil {
			t.Errorf("StructuralSnapshot includes report-specific field %q", field)
		}
	}
	if document.Components.Schemas["StructuralHostSnapshot"].Value.Properties["uptime_seconds"] != nil {
		t.Error("structural host facts include volatile uptime")
	}
	structuralFilesystem := document.Components.Schemas["StructuralFilesystemSnapshot"].Value
	if structuralFilesystem.Properties["available_bytes"] != nil {
		t.Error("structural filesystem facts include volatile available bytes")
	}
	filesystemChange := document.Components.Schemas["FilesystemChange"].Value
	for _, field := range []string{"before", "after"} {
		if filesystemChange.Properties[field].Value.AllOf[0].Ref != "#/components/schemas/StructuralFilesystemSnapshot" {
			t.Errorf("filesystem diff %s includes non-structural measurements", field)
		}
	}
	for _, name := range []string{"HostCollectionFailed", "DiskCollectionFailed", "InterfaceCollectionFailed", "FilesystemCollectionFailed", "StructuralPodmanCollectionFailed"} {
		failed := document.Components.Schemas[name].Value
		if failed.Properties["error_code"] == nil || failed.Properties["error"] != nil {
			t.Errorf("%s does not store only a stable structural error code", name)
		}
		validFailure := map[string]any{"status": "failed", "error_code": "collector_unavailable"}
		if name == "StructuralPodmanCollectionFailed" {
			validFailure["connection"] = "root"
			validFailure["owner_uid"] = 1000
		}
		unionName := strings.TrimSuffix(name, "Failed")
		if err := document.Components.Schemas[unionName].Value.VisitJSON(validFailure); err != nil {
			t.Errorf("%s stable failure code does not validate: %v", name, err)
		}
		validFailure["error"] = "machine-specific diagnostic text"
		if err := document.Components.Schemas[unionName].Value.VisitJSON(validFailure); err == nil {
			t.Errorf("%s unexpectedly permits diagnostic text in structural content", name)
		}
	}
	volatile := document.Components.Schemas["VolatileMeasurements"].Value
	if !slices.Contains(volatile.Required, "filesystems") || volatile.Properties["host_uptime_seconds"] == nil {
		t.Error("report envelope omits required filesystem measurements or host uptime")
	}
	download := document.Components.Schemas["SnapshotDownload"].Value
	for _, field := range []string{"report", "structural_sha256", "structural"} {
		if !slices.Contains(download.Required, field) {
			t.Errorf("SnapshotDownload.%s is not required", field)
		}
	}
	if !strings.Contains(download.Properties["structural_sha256"].Value.Description, "only") {
		t.Error("download does not document structural-only hash semantics")
	}

	comparison := document.Components.Schemas["SnapshotComparison"].Value
	for _, field := range []string{"disks", "measurements", "unavailable_sections", "page"} {
		if !slices.Contains(comparison.Required, field) {
			t.Errorf("SnapshotComparison.%s is not required", field)
		}
	}
	if comparison.Properties["truncated"] != nil {
		t.Error("snapshot comparison uses an unpageable truncated result")
	}
	comparisonOperation := document.Paths.Find("/snapshot-comparisons").Get
	if !operationHasParameter(comparisonOperation, "PageAfter") || !operationHasParameter(comparisonOperation, "PageSize") {
		t.Error("snapshot differences are missing bounded cursor pagination")
	}
	validComparison := map[string]any{
		"from_report_id": "0190d7b6-8ef9-7ca1-a8c4-6bcbca5d73b4",
		"to_report_id":   "0190d7b6-8ef9-7ca1-a8c4-6bcbca5d73b5",
		"host":           []any{}, "interfaces": []any{}, "disks": []any{},
		"filesystems": []any{}, "containers": []any{}, "collectors": []any{},
		"measurements": []any{}, "unavailable_sections": []any{},
		"page": map[string]any{"size": 0, "has_more": false},
	}
	if err := comparison.VisitJSON(validComparison); err != nil {
		t.Errorf("empty paginated snapshot comparison is invalid: %v", err)
	}
	if err := document.Components.Schemas["VolatileMeasurementChange"].Value.VisitJSON(map[string]any{
		"measurement": "filesystem_available_bytes", "device": "/dev/sda1", "mount_point": "/", "before": 100, "after": 50,
	}); err != nil {
		t.Errorf("filesystem available-byte difference is invalid: %v", err)
	}
	if err := document.Components.Schemas["DiskChange"].Value.VisitJSON(map[string]any{
		"key": "sda", "change": "changed",
		"before": map[string]any{"name": "sda", "size_bytes": 1000},
		"after":  map[string]any{"name": "sda", "size_bytes": 2000},
	}); err != nil {
		t.Errorf("disk difference is invalid: %v", err)
	}

	containerHistory := document.Paths.Find("/containers/{container_id}/discovery-reports").Get
	if containerHistory == nil || !operationHasParameter(containerHistory, "PageAfter") || !operationHasParameter(containerHistory, "PageSize") {
		t.Fatal("container discovery history is missing bounded pagination")
	}
	if !strings.Contains(containerHistory.Description, "unavailable") || !strings.Contains(containerHistory.Description, "authoritatively") {
		t.Error("container history does not distinguish absence from collector failure")
	}
	page := document.Components.Schemas["ContainerHistoryPage"].Value
	if page.Properties["items"].Value.MaxItems == nil || *page.Properties["items"].Value.MaxItems != 200 {
		t.Error("container history page is not bounded to 200 items")
	}
	entry := document.Components.Schemas["ContainerHistoryEntry"].Value
	for _, presence := range []string{"present", "absent", "unavailable"} {
		if err := entry.Properties["presence"].Value.VisitJSON(presence); err != nil {
			t.Errorf("container history presence %q is invalid: %v", presence, err)
		}
	}
}

func TestDnsImportCandidateAndPreviewBounds(t *testing.T) {
	document := loadContract(t)

	candidate := document.Components.Schemas["DnsImportCandidate"].Value
	if len(candidate.OneOf) != 2 || candidate.Discriminator == nil || candidate.Discriminator.PropertyName != "status" {
		t.Fatal("DNS import candidate is not a status-discriminated importable/rejected union")
	}
	validImportable := map[string]any{
		"id": "candidate-1", "name": "www.example.com", "type": "A", "ttl": 60,
		"values": []any{"1.1.1.1"}, "proxied": false, "status": "importable",
	}
	if err := candidate.VisitJSON(validImportable); err != nil {
		t.Errorf("supported import candidate is invalid: %v", err)
	}
	validRejected := map[string]any{
		"id": "candidate-2", "status": "unsupported", "reason_code": "unsupported_type",
		"reason": "record type is not supported", "record_count": 1,
		"raw_name": "bad..owner", "raw_type": "CAA", "raw_ttl": 90000,
	}
	if err := candidate.VisitJSON(validRejected); err != nil {
		t.Errorf("rejected provider candidate cannot represent malformed raw type/TTL summary: %v", err)
	}
	rejectedSchema := document.Components.Schemas["RejectedDnsImportCandidate"].Value
	for _, status := range []string{"unsupported", "malformed", "conflicting", "already_managed"} {
		rejectedValue := make(map[string]any, len(validRejected))
		for key, value := range validRejected {
			rejectedValue[key] = value
		}
		rejectedValue["status"] = status
		if err := candidate.VisitJSON(rejectedValue); err != nil {
			t.Errorf("rejected candidate status %q is invalid: %v", status, err)
		}
	}
	missingSummary := make(map[string]any, len(validRejected))
	for key, value := range validRejected {
		missingSummary[key] = value
	}
	missingSummary["status"] = "malformed"
	missingSummary["raw_type"] = nil
	missingSummary["raw_ttl"] = nil
	if err := rejectedSchema.VisitJSON(missingSummary); err != nil {
		t.Errorf("rejected candidate cannot represent unavailable raw summaries: %v", err)
	}
	for _, field := range []string{"type", "ttl", "values", "proxied"} {
		if rejectedSchema.Properties[field] != nil {
			t.Errorf("rejected candidate pretends to have a supported %s", field)
		}
	}
	invalidImportable := make(map[string]any, len(validImportable))
	for key, value := range validImportable {
		invalidImportable[key] = value
	}
	invalidImportable["ttl"] = 90000
	if err := candidate.VisitJSON(invalidImportable); err == nil {
		t.Error("importable candidate with an unsupported TTL unexpectedly validates")
	}
	importableValues := document.Components.Schemas["DnsImportableCandidate"].Value.Properties["values"].Value
	if importableValues.MaxItems == nil || *importableValues.MaxItems != 100 {
		t.Errorf("importable candidate values maxItems = %v, want 100", importableValues.MaxItems)
	}

	selection := document.Components.Schemas["DnsImportSelection"].Value
	candidateIDs := selection.Properties["candidate_ids"].Value
	if candidateIDs.MaxItems == nil || *candidateIDs.MaxItems != 10 {
		t.Fatalf("DNS import selection maxItems = %v, want 10", candidateIDs.MaxItems)
	}
	selectionValue := map[string]any{
		"candidate_ids":           []any{"one", "two", "three", "four", "five", "six", "seven", "eight", "nine", "ten"},
		"observation_fingerprint": strings.Repeat("a", 64),
	}
	if err := selection.VisitJSON(selectionValue); err != nil {
		t.Errorf("maximum import selection is invalid: %v", err)
	}
	selectionValue["candidate_ids"] = append(selectionValue["candidate_ids"].([]any), "eleven")
	if err := selection.VisitJSON(selectionValue); err == nil {
		t.Error("oversized import selection unexpectedly validates")
	}
	apply := document.Components.Schemas["DnsImportApply"].Value
	if apply.Properties["candidate_ids"].Value.MaxItems == nil || *apply.Properties["candidate_ids"].Value.MaxItems != 10 {
		t.Error("DNS import apply permits selections larger than the complete preview bound")
	}

	preview := document.Components.Schemas["DnsPreview"].Value
	changes := preview.Properties["changes"].Value
	if changes.MaxItems == nil || *changes.MaxItems != 1000 || !strings.Contains(changes.Description, "never silently truncated") {
		t.Error("DNS preview does not expose a complete bounded change set")
	}
	if int(*candidateIDs.MaxItems)*int(*importableValues.MaxItems) != int(*changes.MaxItems) {
		t.Error("DNS import selection limits do not prove that every value change fits the preview")
	}
	selectionEndpoint := document.Paths.Find("/dns/zones/{zone_id}/imports/preview").Post
	if !strings.Contains(selectionEndpoint.Description, "10 candidates") || selectionEndpoint.Responses.Value("422") == nil {
		t.Error("DNS import preview does not document its enforced selection bound")
	}
}

func TestNavigationFilterAndLazyTreeContracts(t *testing.T) {
	document := loadContract(t)

	tree := document.Paths.Find("/inventory/tree").Get
	if !operationHasParameter(tree, "PageAfter") || !operationHasParameter(tree, "PageSize") {
		t.Error("inventory root nodes are missing cursor pagination")
	}
	if !operationHasParameter(tree, "Archived") {
		t.Error("inventory root nodes are missing archived-mode filtering")
	}
	if strings.Contains(tree.Description, "max_depth") || document.Components.Schemas["InventoryTree"].Value.Properties["truncated"] != nil {
		t.Error("inventory tree retains an uncontinuable depth/truncation contract")
	}
	children := document.Paths.Find("/inventory/tree/{parent_id}/children").Get
	if children == nil || !operationHasParameter(children, "PageAfter") || !operationHasParameter(children, "PageSize") {
		t.Fatal("inventory child nodes are missing a paged lazy-lookup contract")
	}
	if children.Responses.Value("422") == nil || !strings.Contains(children.Description, "typed containment") {
		t.Error("inventory child lookup does not constrain invalid parent/relationship types")
	}
	node := document.Components.Schemas["InventoryTreeNode"].Value
	if !slices.Contains(node.Required, "has_children") || node.Properties["children"] != nil {
		t.Error("inventory node does not expose lazy child availability")
	}
	treePage := document.Components.Schemas["InventoryTree"].Value
	if !slices.Equal(treePage.Required, []string{"items", "page"}) ||
		treePage.Properties["items"].Value.MaxItems == nil || *treePage.Properties["items"].Value.MaxItems != 200 {
		t.Error("inventory tree page is not bounded and continuable")
	}

	dnsNames := document.Paths.Find("/dns/names").Get
	if !operationHasParameter(dnsNames, "DnsLinkTargetTypeFilter") || !operationHasParameter(dnsNames, "DnsLinkTargetIdFilter") ||
		dnsNames.Extensions["x-query-constraints"] == nil || !strings.Contains(dnsNames.Description, "all-or-none") {
		t.Error("DNS name list lacks its typed, paired inventory-link target filter")
	}
	linkTypes := document.Components.Schemas["DnsLinkTargetType"].Value.Enum
	if !slices.Equal(linkTypes, []any{"compute", "container", "ip_assignment"}) {
		t.Errorf("DNS link target types = %v, want compute/container/ip_assignment", linkTypes)
	}

	domains := document.Paths.Find("/domains").Get
	if !operationHasParameter(domains, "DomainExpiryStateFilter") {
		t.Error("domain list cannot filter by expiry state")
	}
	expiryStates := document.Components.Schemas["DomainExpiryState"].Value.Enum
	if !slices.Equal(expiryStates, []any{"unknown", "current", "expiring", "expired"}) {
		t.Errorf("domain expiry states = %v", expiryStates)
	}

	dashboard := document.Paths.Find("/dashboard/summary").Get
	for _, source := range []string{
		"/computes?freshness=offline",
		"/containers?stale=true",
		"/ip-assignments?stale=true",
		"/domains?expiry_state=expiring",
		"/dns/names?projection_state=pending",
		"/dns/names?projection_state=failed",
		"/costs?currency=",
	} {
		if !strings.Contains(dashboard.Description, source) {
			t.Errorf("dashboard source %q is not documented", source)
		}
	}
	for _, source := range []struct {
		path      string
		parameter string
	}{
		{"/computes", "FreshnessFilter"},
		{"/containers", "StaleFilter"},
		{"/ip-assignments", "StaleFilter"},
		{"/dns/names", "ProjectionStateFilter"},
		{"/costs", "CurrencyFilter"},
	} {
		if !operationHasParameter(document.Paths.Find(source.path).Get, source.parameter) {
			t.Errorf("dashboard source %s lacks filter %s", source.path, source.parameter)
		}
	}
}

func TestSecretIssuanceIsNeverIdempotentlyReplayed(t *testing.T) {
	document := loadContract(t)
	secretOperations := []struct {
		path     string
		method   string
		response string
		recovery string
	}{
		{"/personal-access-tokens", "post", "PersonalAccessTokenSecretResponse", "revoke"},
		{"/computes/{compute_id}/enrollment-tokens", "post", "EnrollmentTokenSecretResponse", "replacement"},
		{"/agents/{agent_id}/rotate", "post", "EnrollmentTokenSecretResponse", "replacement"},
		{"/agent/v1/enroll", "post", "AgentEnrollmentResponse", "rotating the agent"},
	}
	for _, secret := range secretOperations {
		item := document.Paths.Find(secret.path)
		var operation *openapi3.Operation
		switch secret.method {
		case "post":
			operation = item.Post
		default:
			t.Fatalf("unexpected secret issuance method %q", secret.method)
		}
		if operationHasParameter(operation, "IdempotencyKey") || operationHasParameter(operation, "IdempotencyKeyRequired") {
			t.Errorf("%s advertises generic idempotent replay for a plaintext secret", operation.OperationID)
		}
		if !strings.Contains(strings.ToLower(operation.Description), secret.recovery) {
			t.Errorf("%s does not document lost-response recovery by %q", operation.OperationID, secret.recovery)
		}
		response := operation.Responses.Value("201")
		if response == nil || response.Ref != "#/components/responses/"+secret.response {
			t.Errorf("%s does not use its one-time secret response", operation.OperationID)
		}
	}
	for _, name := range []string{
		"PersonalAccessTokenSecretResponse",
		"EnrollmentTokenSecretResponse",
		"AgentEnrollmentResponse",
	} {
		response := document.Components.Responses[name].Value
		if response.Headers["Cache-Control"] == nil {
			t.Errorf("%s does not set Cache-Control: no-store", name)
		}
	}
}

func TestPurgeAndAuditContracts(t *testing.T) {
	document := loadContract(t)
	purgePaths := []string{
		"/providers/{provider_id}/purge",
		"/locations/{location_id}/purge",
		"/computes/{compute_id}/purge",
		"/containers/{container_id}/purge",
		"/ip-assignments/{ip_assignment_id}/purge",
		"/domains/{domain_id}/purge",
		"/costs/{cost_id}/purge",
		"/tags/{tag_id}/purge",
	}
	for _, path := range purgePaths {
		operation := document.Paths.Find(path).Delete
		if operation == nil {
			t.Fatalf("purge operation %s is missing", path)
		}
		got := scopeExtension(t, operation.Extensions["x-required-scopes"])
		if !slices.Equal(got, []string{"admin", "inventory:write"}) {
			t.Errorf("%s scopes = %v, want [admin inventory:write]", path, got)
		}
		conflict := operation.Responses.Value("409")
		if conflict == nil || conflict.Ref != "#/components/responses/PurgeConflict" {
			t.Errorf("%s does not use the typed purge conflict response", path)
		}
	}
	problem := document.Components.Schemas["Problem"].Value
	if problem.Properties["dependency_details"] == nil {
		t.Fatal("Problem has no bounded purge dependency detail extension")
	}
	group := document.Components.Schemas["PurgeDependencyGroup"].Value
	for _, field := range []string{"kind", "count", "list_href"} {
		if !slices.Contains(group.Required, field) {
			t.Errorf("PurgeDependencyGroup.%s is not required", field)
		}
	}
	details := document.Components.Schemas["PurgeDependencyDetails"].Value
	if details.Properties["groups"].Value.MaxItems == nil || *details.Properties["groups"].Value.MaxItems != 32 ||
		details.Properties["references"].Value.MaxItems == nil || *details.Properties["references"].Value.MaxItems != 100 {
		t.Error("purge dependency details are not bounded")
	}
	if !slices.Contains(details.Required, "references_truncated") {
		t.Error("bounded dependency references do not disclose truncation")
	}
	if details.Properties["groups"].Value.Items.Ref != "#/components/schemas/PurgeDependencyGroup" ||
		document.Components.Schemas["PurgeDependencyReference"].Value.Properties["href"] == nil {
		t.Error("purge dependencies lack typed counts or navigable reference identifiers")
	}

	auditResult := document.Components.Schemas["AuditResult"].Value.Enum
	if !slices.Equal(auditResult, []any{"success", "rejected", "failure"}) {
		t.Errorf("AuditResult = %v, want SQL-aligned success/rejected/failure", auditResult)
	}
	audit := document.Components.Schemas["AuditEvent"].Value
	if !slices.Contains(audit.Required, "actor_id") {
		t.Error("AuditEvent.actor_id is not required")
	}
	systemActor := document.Components.Schemas["SystemActorId"].Value.Enum
	const systemActorID = "0190d7b6-8ef9-7ca1-a8c4-6bcbca5d73c0"
	if !slices.Equal(systemActor, []any{systemActorID}) {
		t.Errorf("SystemActorId = %v, want fixed UUIDv7 %s", systemActor, systemActorID)
	}
	if !strings.Contains(audit.Properties["actor_id"].Value.Description, "SystemActorId") {
		t.Error("AuditEvent does not require the fixed system actor ID for system events")
	}
	if audit.Extensions["x-system-actor-id"] != systemActorID {
		t.Error("AuditEvent machine-readable system actor ID constraint is missing")
	}
}

func TestFailureResponseCoverage(t *testing.T) {
	document := loadContract(t)
	for path, item := range document.Paths.Map() {
		for method, operation := range item.Operations() {
			if !strings.HasPrefix(path, "/health/") && operation.Responses.Value("401") == nil {
				t.Errorf("%s %s is authenticated but omits 401", method, path)
			}
			if operation.Extensions["x-required-scopes"] != nil && operation.Responses.Value("403") == nil {
				t.Errorf("%s %s declares scopes but omits 403", method, path)
			}
			if strings.Contains(path, "{") && path != "/agent/v1/snapshots/{report_id}" && operation.Responses.Value("404") == nil {
				t.Errorf("%s %s has a resource path parameter but omits 404", method, path)
			}
			for status, responseRef := range operation.Responses.Map() {
				statusCode, err := strconv.Atoi(status)
				if err != nil || statusCode < 400 {
					continue
				}
				response := responseRef.Value
				if response == nil && strings.HasPrefix(responseRef.Ref, "#/components/responses/") {
					name := strings.TrimPrefix(responseRef.Ref, "#/components/responses/")
					if component := document.Components.Responses[name]; component != nil {
						response = component.Value
					}
				}
				if response == nil {
					t.Errorf("%s %s %s response cannot be resolved", method, path, status)
					continue
				}
				problem := response.Content.Get("application/problem+json")
				if problem == nil || problem.Schema == nil || problem.Schema.Ref != "#/components/schemas/Problem" {
					t.Errorf("%s %s %s failure is not application/problem+json Problem", method, path, status)
				}
			}

			if operationHasParameter(operation, "IfMatch") {
				for _, status := range []string{"400", "412"} {
					if operation.Responses.Value(status) == nil {
						t.Errorf("%s %s requires If-Match but omits %s", method, path, status)
					}
				}
			}
			if operation.RequestBody != nil {
				statuses := []string{"400", "413"}
				if path != "/auth/login" {
					statuses = append(statuses, "422")
				}
				for _, status := range statuses {
					if operation.Responses.Value(status) == nil {
						t.Errorf("%s %s has a request body but omits applicable %s", method, path, status)
					}
				}
			}
			if (path == "/auth/login" || path == "/agent/v1/enroll" ||
				path == "/agent/v1/heartbeat" || path == "/agent/v1/snapshots/{report_id}") &&
				operation.Responses.Value("429") == nil {
				t.Errorf("rate-limited operation %s %s omits 429", method, path)
			}
		}
	}
}

func TestStrictRequestBodiesAndDateNullSemantics(t *testing.T) {
	document := loadContract(t)
	if document.Extensions["x-unknown-json-properties"] != "reject" {
		t.Fatal("OpenAPI does not declare rejection of unknown JSON properties")
	}
	if !strings.Contains(document.Info.Description, "Explicit null is accepted only on nullable properties") ||
		!strings.Contains(document.Info.Description, "omitted writable optional fields") ||
		!strings.Contains(document.Info.Description, "cleared/unknown") {
		t.Error("API does not document null and omitted-field replacement semantics")
	}
	for path, item := range document.Paths.Map() {
		for method, operation := range item.Operations() {
			if operation.RequestBody == nil {
				continue
			}
			requestBody := operation.RequestBody.Value
			if requestBody == nil && strings.HasPrefix(operation.RequestBody.Ref, "#/components/requestBodies/") {
				name := strings.TrimPrefix(operation.RequestBody.Ref, "#/components/requestBodies/")
				requestBody = document.Components.RequestBodies[name].Value
			}
			if requestBody == nil {
				t.Errorf("%s %s request body cannot be resolved", method, path)
				continue
			}
			media := requestBody.Content.Get("application/json")
			if media == nil || media.Schema == nil {
				t.Errorf("%s %s request body has no JSON schema", method, path)
				continue
			}
			assertStrictSchema(t, document, media.Schema, method+" "+path, make(map[string]bool))
		}
	}

	timestamp := document.Components.Schemas["Timestamp"].Value
	if !strings.Contains(timestamp.Pattern, "\\.\\d{1,9}") || !strings.Contains(timestamp.Pattern, "Z$") ||
		!strings.Contains(timestamp.Description, "normalized to whole seconds") {
		t.Error("Timestamp does not document fractional UTC input and whole-second persistence")
	}
	date := document.Components.Schemas["Date"].Value
	if date.Format != "date" || !strings.Contains(date.Description, "no timezone conversion") {
		t.Error("Date does not document date-only calendar semantics")
	}
	pageSize := document.Components.Parameters["PageSize"].Value.Schema.Value
	if pageSize.Default != float64(50) || pageSize.Max == nil || *pageSize.Max != 200 {
		t.Errorf("page[size] defaults/caps = %v/%v, want 50/200", pageSize.Default, pageSize.Max)
	}
	entityTag := document.Components.Schemas["EntityTag"].Value
	if entityTag.Pattern != `^"revision-[1-9][0-9]*"$` {
		t.Error("strong revision ETag format changed")
	}
	for _, name := range []string{"Money", "RecurringCostWrite", "OneTimeCostWrite", "CostWrite"} {
		if document.Components.Schemas[name] == nil {
			t.Errorf("cost request schema %s is missing", name)
		}
	}
}

func TestHealthContractsIncludeTimeChecksAndRFC7807ReadinessFailure(t *testing.T) {
	document := loadContract(t)
	live := document.Components.Schemas["HealthStatus"].Value
	if err := live.VisitJSON(map[string]any{"status": "live", "time": "2026-08-03T12:34:56Z"}); err != nil {
		t.Errorf("valid liveness status is rejected: %v", err)
	}
	ready := document.Components.Schemas["ReadinessStatus"].Value
	for _, status := range []string{"ready", "not_ready"} {
		value := map[string]any{
			"status": status,
			"time":   "2026-08-03T12:34:56Z",
			"checks": []any{map[string]any{"name": "database", "status": map[bool]string{true: "ready", false: "not_ready"}[status == "ready"]}},
		}
		if err := ready.VisitJSON(value); err != nil {
			t.Errorf("readiness status %q is invalid: %v", status, err)
		}
	}
	problem := document.Components.Schemas["Problem"].Value
	if problem.Properties["readiness"] == nil {
		t.Fatal("RFC 7807 Problem cannot carry readiness failure checks")
	}
	if err := problem.VisitJSON(map[string]any{
		"type":  "https://portinaia.invalid/problems/service_unavailable",
		"title": "Server is not ready", "status": 503, "code": "service_unavailable", "request_id": "req_01J4ABCDEF",
		"readiness": map[string]any{
			"status": "not_ready", "time": "2026-08-03T12:34:56Z",
			"checks": []any{map[string]any{"name": "database", "status": "not_ready", "message": "database unavailable"}},
		},
	}); err != nil {
		t.Errorf("readiness 503 problem with diagnostic checks is invalid: %v", err)
	}
	if document.Paths.Find("/health/live").Get.Responses.Value("200").Value.Content.Get("application/json").Schema.Ref != "#/components/schemas/HealthStatus" {
		t.Error("liveness endpoint is not wired to HealthStatus")
	}
	readyOperation := document.Paths.Find("/health/ready").Get
	if readyOperation.Responses.Value("200").Value.Content.Get("application/json").Schema.Ref != "#/components/schemas/ReadinessStatus" ||
		readyOperation.Responses.Value("503").Value.Content.Get("application/problem+json").Schema.Ref != "#/components/schemas/Problem" {
		t.Error("readiness responses do not use the readiness schema and RFC 7807 failure")
	}
}

func assertStrictSchema(t *testing.T, document *openapi3.T, ref *openapi3.SchemaRef, path string, visited map[string]bool) {
	t.Helper()
	if ref == nil {
		return
	}
	if ref.Ref != "" {
		const prefix = "#/components/schemas/"
		if !strings.HasPrefix(ref.Ref, prefix) {
			return
		}
		name := strings.TrimPrefix(ref.Ref, prefix)
		if visited[name] {
			return
		}
		visited[name] = true
		ref = document.Components.Schemas[name]
	}
	if ref == nil || ref.Value == nil {
		return
	}
	schema := ref.Value
	if schema.Type != nil && slices.Contains(*schema.Type, "object") &&
		(schema.AdditionalProperties.Has == nil || *schema.AdditionalProperties.Has) {
		t.Errorf("%s object schema permits unknown properties", path)
	}
	for name, property := range schema.Properties {
		assertStrictSchema(t, document, property, path+"."+name, visited)
	}
	if schema.Items != nil {
		assertStrictSchema(t, document, schema.Items, path+"[]", visited)
	}
	for _, nested := range schema.OneOf {
		assertStrictSchema(t, document, nested, path+" oneOf", visited)
	}
	for _, nested := range schema.AnyOf {
		assertStrictSchema(t, document, nested, path+" anyOf", visited)
	}
	for _, nested := range schema.AllOf {
		assertStrictSchema(t, document, nested, path+" allOf", visited)
	}
}

func TestZonePublicationUsesZoneWideGeneration(t *testing.T) {
	document := loadContract(t)
	publication := document.Components.Schemas["DnsPublicationStatus"].Value
	for _, field := range []string{"state", "desired_generation", "next_serial", "attempt_count", "fencing_token"} {
		if !slices.Contains(publication.Required, field) {
			t.Errorf("DnsPublicationStatus.%s is not required", field)
		}
	}
	if publication.Properties["desired_revision"] != nil || publication.Properties["applied_revision"] != nil {
		t.Error("internal publication still derives zone state from unrelated DNS-name revisions")
	}
	for _, field := range []string{"published_generation", "serial", "checksum", "file_name", "published_at"} {
		if schema := publication.Properties[field].Value; schema == nil || !schema.Nullable {
			t.Errorf("DnsPublicationStatus.%s cannot represent an unpublished or legacy/failed output", field)
		}
	}
}

func TestTagAndDnsDraftLifecycleContracts(t *testing.T) {
	document := loadContract(t)

	tag := document.Components.Schemas["Tag"].Value
	if tag.Description == "" || len(tag.AllOf) != 2 ||
		tag.AllOf[0].Ref != "#/components/schemas/ResourceMetadata" ||
		tag.AllOf[1].Ref != "#/components/schemas/TagWrite" {
		t.Fatal("Tag does not combine mutable resource metadata and writable fields")
	}
	metadata := document.Components.Schemas["ResourceMetadata"].Value
	if !slices.Contains(metadata.Required, "revision") || metadata.Properties["archived_at"] == nil {
		t.Fatal("tag resource metadata lacks revision or archive state")
	}

	for _, path := range []string{
		"/tags",
		"/tags/{tag_id}",
		"/tags/{tag_id}/archive",
		"/tags/{tag_id}/restore",
		"/tags/{tag_id}/purge",
	} {
		if document.Paths.Find(path) == nil {
			t.Errorf("tag lifecycle path %q is missing", path)
		}
	}
	for _, operation := range []*openapi3.Operation{
		document.Paths.Find("/tags/{tag_id}").Put,
		document.Paths.Find("/tags/{tag_id}/archive").Post,
		document.Paths.Find("/tags/{tag_id}/restore").Post,
		document.Paths.Find("/tags/{tag_id}/purge").Delete,
	} {
		if !operationHasParameter(operation, "IfMatch") {
			t.Errorf("%s does not require If-Match", operation.OperationID)
		}
	}
	for _, path := range []string{"/tags/{tag_id}", "/tags/{tag_id}/archive", "/tags/{tag_id}/restore"} {
		method := document.Paths.Find(path)
		var operation *openapi3.Operation
		if method.Put != nil {
			operation = method.Put
		} else {
			operation = method.Post
		}
		response := operation.Responses.Value("200")
		if response == nil || response.Value == nil || response.Value.Headers["ETag"] == nil {
			t.Errorf("%s does not return the tag revision ETag", operation.OperationID)
		}
	}
	purge := document.Paths.Find("/tags/{tag_id}/purge").Delete
	if !strings.Contains(strings.ToLower(purge.Description), "archived") ||
		!strings.Contains(strings.ToLower(purge.Description), "detached") {
		t.Error("tag purge does not require archival and detachment")
	}
	purgeScopes := scopeExtension(t, purge.Extensions["x-required-scopes"])
	if !slices.Equal(purgeScopes, []string{"admin", "inventory:write"}) {
		t.Errorf("tag purge scopes = %v, want [admin inventory:write]", purgeScopes)
	}
	if purge.Responses.Value("409") == nil {
		t.Error("tag purge does not document dependency/lifecycle conflicts")
	}

	draft := document.Components.Schemas["DnsDraft"].Value
	if !slices.Contains(draft.Required, "revision") {
		t.Fatal("DNS draft has no independent revision")
	}
	if !strings.Contains(draft.Properties["revision"].Value.Description, "independent") {
		t.Error("DNS draft revision is not distinguished from target-name revision")
	}
	if !strings.Contains(draft.Properties["base_revision"].Value.Description, "DNS name") {
		t.Error("DNS draft base_revision does not identify the target DNS name revision")
	}
	apply := document.Paths.Find("/dns/drafts/{draft_id}/apply").Post
	if !strings.Contains(apply.Description, "draft") || !strings.Contains(apply.Description, "name") {
		t.Error("DNS draft apply does not distinguish draft revision from name revision")
	}
	applyBody := document.Components.Schemas["DnsDraftApply"].Value
	if !slices.Contains(applyBody.Required, "expected_base_revision") {
		t.Error("DNS draft apply does not require the expected target-name revision")
	}
}

func TestDiscoveryReportEnvelopeContracts(t *testing.T) {
	document := loadContract(t)

	acceptance := document.Components.Schemas["SnapshotAcceptance"].Value
	for _, field := range []string{
		"id",
		"report_id",
		"protocol_version",
		"client_version",
		"collection_started_at",
		"collection_finished_at",
		"received_at",
		"result",
		"content_reused",
	} {
		if !slices.Contains(acceptance.Required, field) {
			t.Errorf("SnapshotAcceptance.%s is not required", field)
		}
	}

	reports := document.Components.Schemas["DiscoveryReport"].Value
	if len(reports.OneOf) != 2 || reports.Discriminator == nil || reports.Discriminator.PropertyName != "result" {
		t.Fatal("DiscoveryReport is not a result-discriminated accepted/rejected union")
	}
	accepted := document.Components.Schemas["AcceptedDiscoveryReport"].Value
	rejected := document.Components.Schemas["RejectedDiscoveryReport"].Value
	for _, field := range []string{"id", "report_id", "client_version", "collection_started_at", "collection_finished_at", "received_at"} {
		if !slices.Contains(accepted.Required, field) || !slices.Contains(rejected.Required, field) {
			t.Errorf("accepted/rejected history metadata must require %s", field)
		}
	}
	if !slices.Contains(accepted.Required, "content_id") || !slices.Contains(accepted.Required, "content_reused") {
		t.Error("accepted discovery history omits its content reference/reuse marker")
	}
	if rejected.Properties["content_id"] != nil || rejected.Properties["content_reused"] != nil {
		t.Error("rejected history report requires or exposes a nonexistent snapshot content reference")
	}
	validRejected := map[string]any{
		"id":                       "0190d7b6-8ef9-7ca1-a8c4-6bcbca5d73b4",
		"report_id":                "0190d7b6-8ef9-7ca1-a8c4-6bcbca5d73b5",
		"agent_id":                 "0190d7b6-8ef9-7ca1-a8c4-6bcbca5d73b6",
		"compute_id":               "0190d7b6-8ef9-7ca1-a8c4-6bcbca5d73b7",
		"protocol_version":         1,
		"client_version":           "1.0.0",
		"received_at":              "2026-08-03T12:34:56Z",
		"collection_started_at":    "2026-08-03T12:34:00Z",
		"collection_finished_at":   "2026-08-03T12:34:50Z",
		"result":                   "rejected",
		"collection_error_summary": nil,
		"collector_results":        []any{},
		"diagnostics":              []any{},
	}
	if err := rejected.VisitJSON(validRejected); err != nil {
		t.Errorf("rejected history report without content validates as invalid: %v", err)
	}
	if err := reports.VisitJSON(validRejected); err != nil {
		t.Errorf("result-discriminated rejected report does not validate: %v", err)
	}
	validAccepted := make(map[string]any, len(validRejected)+2)
	for key, value := range validRejected {
		validAccepted[key] = value
	}
	validAccepted["result"] = "accepted_with_errors"
	validAccepted["content_id"] = "0190d7b6-8ef9-7ca1-a8c4-6bcbca5d73b8"
	validAccepted["content_reused"] = false
	validAccepted["volatile_measurements"] = map[string]any{"filesystems": []any{}}
	if err := reports.VisitJSON(validAccepted); err != nil {
		t.Errorf("result-discriminated accepted report does not validate: %v", err)
	}
	validAccepted["client_version"] = nil
	validAccepted["collection_started_at"] = nil
	validAccepted["collection_finished_at"] = nil
	validAccepted["content_reused"] = nil
	validAccepted["volatile_measurements"] = nil
	if err := reports.VisitJSON(validAccepted); err != nil {
		t.Errorf("schema-one accepted report with unknown legacy envelope values does not validate: %v", err)
	}
	withMissingContent := make(map[string]any, len(validRejected))
	for key, value := range validRejected {
		withMissingContent[key] = value
	}
	withMissingContent["content_id"] = "0190d7b6-8ef9-7ca1-a8c4-6bcbca5d73b8"
	if err := rejected.VisitJSON(withMissingContent); err == nil {
		t.Error("rejected history report with content_id unexpectedly validates")
	}

	for _, path := range []string{"/discovery-reports/{report_id}", "/discovery-reports/{report_id}/snapshot"} {
		parameters := document.Paths.Find(path).Parameters
		if len(parameters) != 1 || parameters[0].Ref != "#/components/parameters/HistoryReportId" {
			t.Errorf("%s does not use the server history identifier parameter", path)
		}
	}
	historyID := document.Components.Parameters["HistoryReportId"].Value
	historyDescription := strings.ToLower(historyID.Description)
	if !strings.Contains(historyDescription, "server-assigned") || !strings.Contains(historyDescription, "not the client") {
		t.Error("history path parameter does not clarify server/client report ID distinction")
	}
	putParameters := document.Paths.Find("/agent/v1/snapshots/{report_id}").Parameters
	if len(putParameters) != 1 || putParameters[0].Ref != "#/components/parameters/ReportId" {
		t.Error("snapshot PUT path does not use the client report identifier")
	}
}

func operationHasParameter(operation *openapi3.Operation, name string) bool {
	for _, parameter := range operation.Parameters {
		if parameter.Ref == "#/components/parameters/"+name {
			return true
		}
	}
	return false
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
	if method == "DELETE" && strings.HasSuffix(path, "/purge") {
		return []string{"admin", "inventory:write"}
	}
	if strings.HasPrefix(path, "/dns/") {
		if method == "GET" {
			return []string{"dns:read"}
		}
		return []string{"dns:write"}
	}
	if strings.HasPrefix(path, "/agents") || strings.HasPrefix(path, "/enrollment-tokens") ||
		strings.HasPrefix(path, "/discovery-reports") || path == "/snapshot-comparisons" ||
		path == "/computes/{compute_id}/enrollment-tokens" || path == "/computes/{compute_id}/snapshots" ||
		path == "/containers/{container_id}/discovery-reports" {
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
