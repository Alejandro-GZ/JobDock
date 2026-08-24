package httpapi

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jobdock/jobdock/internal/auth"
	"github.com/jobdock/jobdock/internal/config"
	"github.com/jobdock/jobdock/internal/domain"
	"github.com/jobdock/jobdock/internal/filestore"
	"github.com/jobdock/jobdock/internal/ids"
	"github.com/jobdock/jobdock/internal/secretbox"
	"github.com/jobdock/jobdock/internal/store"
)

func TestRichObservationsAreAttemptAwareIncrementalAndBounded(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	repository, err := store.Open(root + "/jobdock.db")
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	owner := createSeriesUser(t, repository, "observation-owner")
	node := domain.Node{ID: ids.New(), Name: "observation-node", Status: domain.NodeOnline, ProtocolVersion: 1, CPUTotalMillis: 4000, MemoryTotalBytes: 8 << 30, WorkspaceFreeBytes: 10 << 30, Labels: map[string]string{}, LastHeartbeat: time.Now().UTC(), CreatedAt: time.Now().UTC()}
	if err = repository.UpsertNode(ctx, node, auth.TokenHash("node-token")); err != nil {
		t.Fatal(err)
	}
	job := domain.Job{ID: ids.New(), OwnerID: owner.ID, Spec: domain.JobSpec{Name: "observed job", Image: "alpine:3", Command: []string{"true"}, Resources: domain.Resources{CPUMillis: 100, MemoryBytes: 1024}}, Status: domain.JobRunning, DesiredStatus: domain.JobRunning, ObservedStatus: domain.JobRunning, CreatedAt: time.Now().UTC()}
	if err = repository.CreateJob(ctx, job); err != nil {
		t.Fatal(err)
	}
	attemptID, token := ids.New(), "rich-observation-token"
	if _, err = repository.DB().ExecContext(ctx, `INSERT INTO job_attempts(id,job_id,attempt_number,node_id,assignment_id,status,job_token_hash,created_at) VALUES(?,?,1,?,?,?,?,?)`, attemptID, job.ID, node.ID, ids.New(), "RUNNING", auth.TokenHash(token), time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	if _, err = repository.DB().ExecContext(ctx, `UPDATE jobs SET assigned_node_id=?,attempt_id=? WHERE id=?`, node.ID, attemptID, job.ID); err != nil {
		t.Fatal(err)
	}
	files, _ := filestore.New(root, 1<<20, 1<<20, 1<<20)
	box, _ := secretbox.New(bytes.Repeat([]byte{9}, 32))
	server := httptest.NewServer(New(config.Server{AllowInsecureHTTP: true, SessionTTL: time.Hour}, repository, files, box, slog.New(slog.NewTextHandler(io.Discard, nil))).Handler())
	defer server.Close()
	ownerClient := loginSeriesUser(t, server.URL, owner.Username)

	postJobContext(t, server.URL, token, "milestones", `{"items":[{"name":"prepare","weight":0.2},{"name":"train","weight":0.8,"metadata":{"phase":"fit"}}]}`, http.StatusAccepted)
	postJobContext(t, server.URL, token, "milestones/reached", `{"milestone":"prepare","step":1}`, http.StatusAccepted)
	postJobContext(t, server.URL, token, "progress", `{"value":0.5,"milestone":"train","step":5}`, http.StatusAccepted)
	postJobContext(t, server.URL, token, "progress", `{"value":0.25}`, http.StatusAccepted)
	postJobContext(t, server.URL, token, "progress", `{"value":0.5,"milestone":"unknown"}`, http.StatusUnprocessableEntity)

	var progress domain.ProgressState
	getSeriesJSON(t, ownerClient, server.URL+"/api/v1/jobs/"+job.ID+"/progress?attempt_id="+attemptID, &progress)
	if progress.GlobalProgress == nil || math.Abs(*progress.GlobalProgress-0.6) > 1e-9 || progress.Simple == nil || progress.Current == nil || progress.Current.Milestone != "train" || len(progress.Reached) != 1 {
		t.Fatalf("progress state: %#v", progress)
	}

	postJobContext(t, server.URL, token, "matrices", `{"name":"confusion","values":[[8,2],[1,9]],"labels":["cat","dog"],"step":5}`, http.StatusAccepted)
	postJobContext(t, server.URL, token, "matrices", `{"name":"confusion","values":[[9,1],[0,10]],"labels":["cat","dog"],"step":6}`, http.StatusAccepted)
	postJobContext(t, server.URL, token, "matrices", `{"name":"broken","values":[[1,2]],"labels":["cat"]}`, http.StatusUnprocessableEntity)
	postJobContext(t, server.URL, token, "matrices", `{"name":"attention","matrix_type":"heatmap","values":[[0.8,null,0.2],[0.1,0.7,0.2]],"row_labels":["q1","q2"],"column_labels":["k1","k2","k3"],"unit":"score"}`, http.StatusAccepted)
	postJobContext(t, server.URL, token, "matrices", `{"name":"attention","matrix_type":"heatmap","values":[[1]],"unit":"ms"}`, http.StatusConflict)
	postJobContext(t, server.URL, token, "matrices", `{"name":"features","matrix_type":"correlation","values":[[1,-0.4],[-0.4,1]],"row_labels":["age","income"],"column_labels":["age","income"]}`, http.StatusAccepted)
	postJobContext(t, server.URL, token, "matrices", `{"name":"invalid-correlation","matrix_type":"correlation","values":[[1,0.2],[0.3,1]],"row_labels":["a","b"],"column_labels":["a","b"]}`, http.StatusUnprocessableEntity)
	var matrices struct {
		AttemptID string                     `json:"attempt_id"`
		Items     []domain.MatrixObservation `json:"items"`
	}
	getSeriesJSON(t, ownerClient, server.URL+"/api/v1/jobs/"+job.ID+"/matrices?attempt_id="+attemptID+"&name=confusion", &matrices)
	if matrices.AttemptID != attemptID || len(matrices.Items) != 1 || matrices.Items[0].Step == nil || *matrices.Items[0].Step != 6 || len(matrices.Items[0].Values) != 2 {
		t.Fatalf("latest matrix: %#v", matrices)
	}
	getSeriesJSON(t, ownerClient, server.URL+"/api/v1/jobs/"+job.ID+"/matrices?attempt_id="+attemptID+"&name=confusion&step=5", &matrices)
	if len(matrices.Items) != 1 || matrices.Items[0].Step == nil || *matrices.Items[0].Step != 5 {
		t.Fatalf("matrix by step: %#v", matrices)
	}
	getSeriesJSON(t, ownerClient, server.URL+"/api/v1/jobs/"+job.ID+"/matrices?attempt_id="+attemptID+"&name=attention", &matrices)
	if len(matrices.Items) != 1 || matrices.Items[0].MatrixType != "heatmap" || matrices.Items[0].Values[0][1] != nil || len(matrices.Items[0].ColumnLabels) != 3 || matrices.Items[0].Unit != "score" || !containsString(matrices.Items[0].Tags, "matrix:heatmap") {
		t.Fatalf("typed heatmap: %#v", matrices)
	}

	postJobContext(t, server.URL, token, "distributions", `{"name":"residual","group":"baseline","unit":"ms","values":[1,2,2,3,100],"scores":{"psi":0.12},"tags":["histogram:error"]}`, http.StatusAccepted)
	postJobContext(t, server.URL, token, "distributions", `{"name":"residual","group":"current","unit":"ms","values":[2,3,3,4,5]}`, http.StatusAccepted)
	postJobContext(t, server.URL, token, "distributions", `{"name":"broken","values":[]}`, http.StatusUnprocessableEntity)
	var distributions struct {
		AttemptID string `json:"attempt_id"`
		Items     []struct {
			Name    string               `json:"name"`
			Group   string               `json:"group"`
			Samples []float64            `json:"samples"`
			Bins    []distributionBin    `json:"bins"`
			Density []map[string]float64 `json:"density"`
			Summary distributionSummary  `json:"summary"`
			Scores  map[string]float64   `json:"scores"`
		} `json:"items"`
	}
	getSeriesJSON(t, ownerClient, server.URL+"/api/v1/jobs/"+job.ID+"/distributions?attempt_id="+attemptID+"&name=residual&bins=8", &distributions)
	if distributions.AttemptID != attemptID || len(distributions.Items) != 2 || len(distributions.Items[0].Bins) != 8 || len(distributions.Items[0].Density) != 8 || distributions.Items[0].Summary.Count != 5 {
		t.Fatalf("distribution views: %#v", distributions)
	}
	groups := map[string]bool{}
	for _, item := range distributions.Items {
		groups[item.Group] = true
	}
	if !groups["baseline"] || !groups["current"] {
		t.Fatalf("distribution populations lost: %#v", groups)
	}

	postJobContext(t, server.URL, token, "tables", `{"name":"predictions","subtype":"table","columns":[{"name":"sample","type":"string"},{"name":"score","type":"number","unit":"ratio"},{"name":"accepted","type":"boolean"}],"rows":[{"sample":"a","score":0.3,"accepted":false},{"sample":"b","score":0.9,"accepted":true}],"step":6}`, http.StatusAccepted)
	postJobContext(t, server.URL, token, "tables", `{"name":"predictions","subtype":"table","columns":[{"name":"sample","type":"string"},{"name":"score","type":"number","unit":"ratio"},{"name":"accepted","type":"boolean"}],"rows":[{"sample":"c","score":0.6,"accepted":true}],"step":7}`, http.StatusAccepted)
	postJobContext(t, server.URL, token, "tables", `{"name":"predictions","columns":[{"name":"sample","type":"string"},{"name":"score","type":"integer"},{"name":"accepted","type":"boolean"}],"rows":[{"sample":"d","score":1,"accepted":true}]}`, http.StatusConflict)
	postJobContext(t, server.URL, token, "tables", `{"name":"roc","subtype":"roc","columns":[{"name":"fpr","type":"number"},{"name":"tpr","type":"number"},{"name":"threshold","type":"number","nullable":true}],"rows":[{"fpr":0,"tpr":0,"threshold":1},{"fpr":0.2,"tpr":0.8,"threshold":0.6},{"fpr":1,"tpr":1,"threshold":0}]}`, http.StatusAccepted)
	postJobContext(t, server.URL, token, "tables", `{"name":"classes","subtype":"categorical","columns":[{"name":"category","type":"string"},{"name":"value","type":"number"}],"rows":[{"category":"a","value":2}]}`, http.StatusAccepted)
	postJobContext(t, server.URL, token, "tables", `{"name":"classes","subtype":"categorical","columns":[{"name":"category","type":"string"},{"name":"value","type":"number"}],"rows":[{"category":"b","value":3}]}`, http.StatusAccepted)
	postJobContext(t, server.URL, token, "tables", `{"name":"bad-waterfall","subtype":"waterfall","columns":[{"name":"label","type":"string"},{"name":"value","type":"number"},{"name":"kind","type":"string"}],"rows":[{"label":"revenue","value":2,"kind":"contribution"}]}`, http.StatusUnprocessableEntity)
	var table domain.TablePage
	getSeriesJSON(t, ownerClient, server.URL+"/api/v1/jobs/"+job.ID+"/tables?attempt_id="+attemptID+"&name=predictions&limit=1&offset=0&sort=score&order=desc&filter=accepted=true", &table)
	if table.AttemptID != attemptID || table.Total != 2 || len(table.Items) != 1 || table.Items[0].Values["sample"] != "b" || table.Columns[1].Unit != "ratio" || !containsString(table.Tags, "table:table") {
		t.Fatalf("typed table page: %#v", table)
	}
	getSeriesJSON(t, ownerClient, server.URL+"/api/v1/jobs/"+job.ID+"/tables?attempt_id="+attemptID+"&name=classes", &table)
	if table.Total != 1 || len(table.Items) != 1 || table.Items[0].Values["category"] != "b" {
		t.Fatalf("categorical snapshots must replace prior records: %#v", table)
	}

	checkpointResponse := postJobContext(t, server.URL, token, "checkpoints", `{"label":"best model","step":6,"metadata":{"score":0.9}}`, http.StatusAccepted)
	var checkpoint domain.CheckpointSync
	if err = json.Unmarshal(checkpointResponse, &checkpoint); err != nil {
		t.Fatal(err)
	}
	if err = repository.ConfirmCheckpointSync(ctx, checkpoint.ID, []domain.CheckpointFile{{Path: "model.pt", Size: 42}}); err != nil {
		t.Fatal(err)
	}
	if err = repository.AppendObservationUpdate(ctx, job.ID, attemptID, "checkpoint", time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	var checkpoints struct {
		Items []domain.CheckpointSync `json:"items"`
	}
	getSeriesJSON(t, ownerClient, server.URL+"/api/v1/jobs/"+job.ID+"/checkpoints?attempt_id="+attemptID+"&after=0&limit=1", &checkpoints)
	if len(checkpoints.Items) != 1 || checkpoints.Items[0].Label != "best model" || checkpoints.Items[0].Step == nil || *checkpoints.Items[0].Step != 6 || checkpoints.Items[0].Metadata["score"] != 0.9 {
		t.Fatalf("checkpoint observations: %#v", checkpoints)
	}

	update := readObservationSSE(t, ownerClient, server.URL+"/api/v1/jobs/"+job.ID+"/observations/stream?attempt_id="+attemptID+"&after=0")
	if update["attempt_id"] != attemptID || update["cursor"].(float64) < 1 {
		t.Fatalf("observation update: %#v", update)
	}
	secondAttempt := ids.New()
	if _, err = repository.DB().ExecContext(ctx, `INSERT INTO job_attempts(id,job_id,attempt_number,node_id,assignment_id,status,job_token_hash,created_at) VALUES(?,?,2,?,?,'FAILED',?,?)`, secondAttempt, job.ID, node.ID, ids.New(), auth.TokenHash("second-attempt-token"), time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	progress = domain.ProgressState{}
	getSeriesJSON(t, ownerClient, server.URL+"/api/v1/jobs/"+job.ID+"/progress?attempt_id="+secondAttempt, &progress)
	if progress.Simple != nil || progress.Current != nil || len(progress.Milestones) != 0 {
		t.Fatalf("attempt progress leaked: %#v", progress)
	}
}

func TestGenericHeatmapResolutionIsExplicitAndSemanticMatricesRemainExact(t *testing.T) {
	values := make([][]*float64, 70)
	for row := range values {
		values[row] = make([]*float64, 90)
		for column := range values[row] {
			value := float64(row + column)
			values[row][column] = &value
		}
	}
	values[0][0] = nil
	item := resolveMatrixResolution(domain.MatrixObservation{MatrixType: "heatmap", Values: values}, "auto")
	if item.Resolution == nil || item.Resolution.Mode != "aggregated" || item.Resolution.OriginalRows != 70 || item.Resolution.OriginalColumns != 90 || len(item.Values) > 64 || len(item.Values[0]) > 64 {
		t.Fatalf("automatic heatmap resolution: %#v", item.Resolution)
	}
	correlation := resolveMatrixResolution(domain.MatrixObservation{MatrixType: "correlation", Values: values}, "32")
	if correlation.Resolution == nil || correlation.Resolution.Mode != "full" || len(correlation.Values) != 70 || len(correlation.Values[0]) != 90 {
		t.Fatalf("correlation matrix must remain exact: %#v", correlation.Resolution)
	}
}

func postJobContext(t *testing.T, serverURL, token, endpoint, payload string, expected int) []byte {
	t.Helper()
	request, _ := http.NewRequest(http.MethodPost, serverURL+"/api/v1/job-context/"+endpoint, strings.NewReader(payload))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+token)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(response.Body)
	if response.StatusCode != expected {
		t.Fatalf("POST %s returned %d, want %d: %s", endpoint, response.StatusCode, expected, body)
	}
	return body
}

func readObservationSSE(t *testing.T, client *http.Client, endpoint string) map[string]any {
	t.Helper()
	response, err := client.Get(endpoint)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	scanner := bufio.NewScanner(response.Body)
	for scanner.Scan() {
		if line := scanner.Text(); strings.HasPrefix(line, "data: ") {
			var update map[string]any
			if err = json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &update); err != nil {
				t.Fatal(err)
			}
			return update
		}
	}
	t.Fatalf("observation stream ended without data: %v", scanner.Err())
	return nil
}
