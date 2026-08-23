package httpapi

import (
	_ "embed"
	"encoding/json"
	"net/http"
	"strings"
)

//go:embed catalog/observability.json
var observabilityCatalogJSON []byte

type semanticMetricRole struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Unit        string `json:"recommended_unit,omitempty"`
	Direction   string `json:"optimization_direction,omitempty"`
}

type semanticMetricCategory struct {
	ID          string               `json:"id"`
	Name        string               `json:"name"`
	Description string               `json:"description"`
	Roles       []semanticMetricRole `json:"roles"`
}

type semanticPhase struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

type observabilityCatalog struct {
	Version          int                      `json:"version"`
	MetricCategories []semanticMetricCategory `json:"metric_categories"`
	Phases           []semanticPhase          `json:"phases"`
}

var officialObservabilityCatalog = mustLoadObservabilityCatalog()

func mustLoadObservabilityCatalog() observabilityCatalog {
	var catalog observabilityCatalog
	if err := json.Unmarshal(observabilityCatalogJSON, &catalog); err != nil {
		panic("load observability catalog: " + err.Error())
	}
	for categoryIndex := range catalog.MetricCategories {
		category := &catalog.MetricCategories[categoryIndex]
		for roleIndex := range category.Roles {
			role := &category.Roles[roleIndex]
			role.ID = strings.TrimSpace(role.ID)
			if role.Name == "" {
				role.Name = semanticLabel(role.ID)
			}
			if role.Description == "" {
				role.Description = "Standard " + role.Name + " metric."
			}
		}
	}
	for index := range catalog.Phases {
		phase := &catalog.Phases[index]
		phase.ID = strings.TrimSpace(phase.ID)
		if phase.Name == "" {
			phase.Name = semanticLabel(phase.ID)
		}
		if phase.Description == "" {
			phase.Description = "Observations captured during " + strings.ToLower(phase.Name) + "."
		}
	}
	return catalog
}

func semanticLabel(value string) string {
	parts := strings.Split(strings.ReplaceAll(value, "_", " "), " ")
	for index := range parts {
		if parts[index] != "" {
			parts[index] = strings.ToUpper(parts[index][:1]) + parts[index][1:]
		}
	}
	return strings.Join(parts, " ")
}

func (a *API) observabilityCatalog(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, officialObservabilityCatalog)
}
