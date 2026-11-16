package api_test

import (
	"fmt"
	"github.com/getkin/kin-openapi/openapi3"
)

func validateOperationResponse(document *openapi3.T, path, method string, status int, value any) error {
	item := document.Paths.Find(path)
	if item == nil {
		return fmt.Errorf("operation %s %s not found", method, path)
	}
	operation := item.Operations()[method]
	if operation == nil {
		return fmt.Errorf("operation %s %s not found", method, path)
	}
	response := operation.Responses.Status(status)
	if response == nil || response.Value == nil {
		return fmt.Errorf("operation %s %s has no response for %d", method, path, status)
	}
	if response.Value.Content == nil {
		return nil
	}
	media := response.Value.Content.Get("application/json")
	if media == nil || media.Schema == nil || media.Schema.Value == nil {
		return fmt.Errorf("operation %s %s response %d has no application/json schema", method, path, status)
	}
	return media.Schema.Value.VisitJSON(value)
}
