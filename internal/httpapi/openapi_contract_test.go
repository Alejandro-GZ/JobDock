package httpapi

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/jobdock/jobdock/internal/domain"
)

var pathParameterPattern = regexp.MustCompile(`\{[^}]+\}`)
var componentReferencePattern = regexp.MustCompile(`#/components/(schemas|parameters|responses)/([A-Za-z0-9_-]+)`)

func TestOpenAPICoversRegisteredAPI(t *testing.T) {
	registered := registeredOperations(t)
	documented, metadata := documentedOperations(t)

	missing := difference(registered, documented)
	stale := difference(documented, registered)
	if len(missing) > 0 || len(stale) > 0 {
		t.Fatalf("OpenAPI drift detected\nmissing from api/openapi.yaml: %v\nnot registered by API: %v", missing, stale)
	}
	for operation, values := range metadata {
		if !values.operationID || !values.responses {
			t.Errorf("OpenAPI operation %s must declare operationId and responses", operation)
		}
	}
}

func TestOpenAPIComponentReferencesResolve(t *testing.T) {
	data, err := os.ReadFile("../../api/openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	definitions := map[string]bool{}
	section := ""
	operationIDs := map[string]bool{}
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(line, "  ") && !strings.HasPrefix(line, "    ") && strings.HasSuffix(trimmed, ":") {
			candidate := strings.TrimSuffix(trimmed, ":")
			switch candidate {
			case "schemas", "parameters", "responses":
				section = candidate
			default:
				section = ""
			}
		}
		if section != "" && strings.HasPrefix(line, "    ") && !strings.HasPrefix(line, "      ") {
			if name, _, found := strings.Cut(trimmed, ":"); found && !strings.Contains(name, " ") {
				definitions[section+"/"+name] = true
			}
		}
		if strings.HasPrefix(trimmed, "operationId:") {
			id := strings.TrimSpace(strings.TrimPrefix(trimmed, "operationId:"))
			if operationIDs[id] {
				t.Errorf("duplicate operationId %q", id)
			}
			operationIDs[id] = true
		}
	}
	for _, match := range componentReferencePattern.FindAllSubmatch(data, -1) {
		key := string(match[1]) + "/" + string(match[2])
		if !definitions[key] {
			t.Errorf("unresolved OpenAPI component reference %s", key)
		}
	}
}

func TestOpenAPIDomainSchemasCoverJSONFields(t *testing.T) {
	data, err := os.ReadFile("../../api/openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	properties := openAPISchemaProperties(string(data))
	contracts := map[string]any{
		"User": domain.User{}, "Job": domain.Job{}, "Node": domain.Node{},
		"Build": domain.Build{}, "BuildSource": domain.BuildSource{}, "BuildEvent": domain.BuildEvent{}, "BuildPlan": domain.BuildPlan{}, "BuildAssignment": domain.BuildAssignment{}, "BuildWork": domain.BuildWork{}, "ManagedArtifact": domain.ManagedArtifact{},
		"GPU": domain.GPU{}, "GPUDiscovery": domain.GPUDiscovery{}, "CPUPackage": domain.CPUPackage{}, "MetricPoint": domain.MetricPoint{}, "MetricSeries": domain.MetricSeries{},
		"ResourceSample": domain.ResourceSample{}, "CheckpointSync": domain.CheckpointSync{},
		"Assignment": domain.Assignment{}, "InputFile": domain.InputFile{}, "JobAttempt": domain.JobAttempt{}, "OutputFile": domain.OutputFile{},
	}
	for schema, contract := range contracts {
		for field := range jsonFields(reflect.TypeOf(contract)) {
			if !properties[schema][field] {
				t.Errorf("OpenAPI schema %s is missing JSON field %q", schema, field)
			}
		}
	}
}

func openAPISchemaProperties(data string) map[string]map[string]bool {
	result := map[string]map[string]bool{}
	schema, inProperties := "", false
	for _, line := range strings.Split(data, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(line, "    ") && !strings.HasPrefix(line, "      ") && strings.HasSuffix(trimmed, ":") {
			schema, inProperties = strings.TrimSuffix(trimmed, ":"), false
			if result[schema] == nil {
				result[schema] = map[string]bool{}
			}
			continue
		}
		if schema != "" && strings.HasPrefix(line, "      properties:") {
			inProperties = true
			continue
		}
		if inProperties && strings.HasPrefix(line, "        ") && !strings.HasPrefix(line, "          ") {
			if name, _, found := strings.Cut(trimmed, ":"); found {
				result[schema][name] = true
			}
		} else if inProperties && strings.HasPrefix(line, "      ") && !strings.HasPrefix(line, "        ") {
			inProperties = false
		}
	}
	return result
}

func jsonFields(value reflect.Type) map[string]bool {
	result := map[string]bool{}
	for i := 0; i < value.NumField(); i++ {
		tag := strings.Split(value.Field(i).Tag.Get("json"), ",")[0]
		if tag != "" && tag != "-" {
			result[tag] = true
		}
	}
	return result
}

func registeredOperations(t *testing.T) map[string]bool {
	t.Helper()
	packages, err := parser.ParseDir(token.NewFileSet(), ".", func(info os.FileInfo) bool {
		return strings.HasSuffix(info.Name(), ".go") && !strings.HasSuffix(info.Name(), "_test.go")
	}, 0)
	if err != nil {
		t.Fatal(err)
	}
	operations := map[string]bool{}
	for _, pkg := range packages {
		for _, file := range pkg.Files {
			ast.Inspect(file, func(node ast.Node) bool {
				call, ok := node.(*ast.CallExpr)
				if !ok || len(call.Args) == 0 {
					return true
				}
				selector, ok := call.Fun.(*ast.SelectorExpr)
				literal, literalOK := call.Args[0].(*ast.BasicLit)
				if !ok || selector.Sel.Name != "HandleFunc" || !literalOK || literal.Kind != token.STRING {
					return true
				}
				pattern, unquoteErr := strconv.Unquote(literal.Value)
				if unquoteErr != nil {
					t.Fatal(unquoteErr)
				}
				method, path, found := strings.Cut(pattern, " ")
				if found && strings.HasPrefix(path, "/api/v1/") {
					operations[operationKey(method, strings.TrimPrefix(path, "/api/v1"))] = true
				}
				return true
			})
		}
	}
	return operations
}

type operationMetadata struct{ operationID, responses bool }

func documentedOperations(t *testing.T) (map[string]bool, map[string]operationMetadata) {
	t.Helper()
	data, err := os.ReadFile("../../api/openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	operations := map[string]bool{}
	metadata := map[string]operationMetadata{}
	path, current := "", ""
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "  /") && strings.HasSuffix(strings.TrimSpace(line), ":") {
			path = strings.TrimSuffix(strings.TrimSpace(line), ":")
			current = ""
			continue
		}
		trimmed := strings.TrimSpace(line)
		if path != "" && strings.HasPrefix(line, "    ") && !strings.HasPrefix(line, "      ") && strings.HasSuffix(trimmed, ":") {
			method := strings.TrimSuffix(trimmed, ":")
			switch method {
			case "get", "post", "put", "patch", "delete":
				current = operationKey(strings.ToUpper(method), path)
				operations[current] = true
				metadata[current] = operationMetadata{}
			default:
				current = ""
			}
			continue
		}
		if current != "" {
			value := metadata[current]
			value.operationID = value.operationID || strings.HasPrefix(trimmed, "operationId:")
			value.responses = value.responses || strings.HasPrefix(trimmed, "responses:")
			metadata[current] = value
		}
	}
	return operations, metadata
}

func operationKey(method, path string) string {
	return method + " " + pathParameterPattern.ReplaceAllString(path, "{}")
}

func difference(left, right map[string]bool) []string {
	items := make([]string, 0)
	for item := range left {
		if !right[item] {
			items = append(items, item)
		}
	}
	sort.Strings(items)
	return items
}
