package domain

import (
	"encoding/json"
	"testing"
)

func TestJobStateTransitions(t *testing.T) {
	tests := []struct {
		from, to JobStatus
		want     bool
	}{
		{JobQueued, JobAssigned, true},
		{JobQueued, JobSucceeded, false},
		{JobRunning, JobStopping, true},
		{JobLost, JobRunning, true},
		{JobSucceeded, JobRunning, false},
	}
	for _, test := range tests {
		if got := CanTransition(test.from, test.to); got != test.want {
			t.Fatalf("CanTransition(%s, %s) = %v, want %v", test.from, test.to, got, test.want)
		}
	}
}

func TestValidateJobSpec(t *testing.T) {
	valid := JobSpec{Name: "test-job", Image: "alpine:3", Command: []string{"echo", "ok"}, Resources: Resources{CPUMillis: 100, MemoryBytes: 1024}}
	if err := ValidateJobSpec(valid); err != nil {
		t.Fatal(err)
	}
	valid.Inputs = []InputFile{{Path: "dataset/value.txt", Size: 5, SHA256: "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"}}
	if err := ValidateJobSpec(valid); err != nil {
		t.Fatal(err)
	}
	valid.Inputs[0].Path = "../escape"
	if err := ValidateJobSpec(valid); err == nil {
		t.Fatal("expected input traversal to be rejected")
	}
	valid.Inputs = nil
	valid.Environment = map[string]string{"JOBDOCK_JOB_ID": "spoof"}
	if err := ValidateJobSpec(valid); err == nil {
		t.Fatal("expected reserved environment variable to be rejected")
	}
}

func TestValidateExplicitHardwareAffinity(t *testing.T) {
	spec := JobSpec{Name: "hardware-job", Image: "alpine", TargetNodeID: "node", Resources: Resources{CPUMillis: 1000, CPUPackageID: "0", MemoryBytes: 1024, GPU: GPURequest{Count: 2, UUIDs: []string{"GPU-1", "GPU-2"}}}}
	if err := ValidateJobSpec(spec); err != nil {
		t.Fatal(err)
	}
	spec.TargetNodeID = ""
	if err := ValidateJobSpec(spec); err == nil {
		t.Fatal("expected target node validation")
	}
	spec.TargetNodeID = "node"
	spec.Resources.GPU.Count = 1
	if err := ValidateJobSpec(spec); err == nil {
		t.Fatal("expected UUID count validation")
	}
}

func TestBuildLifecycleAndValidation(t *testing.T) {
	if !CanBuildTransition(BuildCreated, BuildAnalyzing) || !CanBuildTransition(BuildCreated, BuildBuilding) || !CanBuildTransition(BuildAnalyzing, BuildBuilding) || !CanBuildTransition(BuildBuilding, BuildSucceeded) {
		t.Fatal("expected happy-path build transitions")
	}
	if CanBuildTransition(BuildCreated, BuildSucceeded) || CanBuildTransition(BuildFailed, BuildBuilding) {
		t.Fatal("invalid build transition was accepted")
	}
	build := Build{Name: "source build", Mode: BuildModeRailpack, Status: BuildCreated, Source: BuildSource{Filename: "source.tar.gz", Size: 10, SHA256: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"}}
	if err := ValidateBuild(build); err != nil {
		t.Fatal(err)
	}
	build.Status = BuildSucceeded
	if err := ValidateBuild(build); err == nil {
		t.Fatal("successful build without OCI digest was accepted")
	}
	build.OCIDigest = "sha256:abcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcd"
	if err := ValidateBuild(build); err != nil {
		t.Fatal(err)
	}
	build.Status, build.OCIDigest, build.FailureReason = BuildFailed, "", ""
	if err := ValidateBuild(build); err == nil {
		t.Fatal("failed build without visible failure reason was accepted")
	}
	build = Build{Name: "custom build", Mode: BuildModeDockerfile, Status: BuildCreated, Source: BuildSource{Filename: "source.zip", Size: 10, SHA256: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"}, ContextPath: "services/api", DockerfilePath: "docker/api.Dockerfile"}
	if err := ValidateBuild(build); err != nil {
		t.Fatal(err)
	}
	build.DockerfilePath = "../Dockerfile"
	if err := ValidateBuild(build); err == nil {
		t.Fatal("Dockerfile traversal was accepted")
	}
}

func TestManagedArtifactReferenceIsImmutableAndStrict(t *testing.T) {
	digest := "sha256:abcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcd"
	reference := ManagedArtifactReference("build-id", digest)
	buildID, parsedDigest, managed, err := ParseManagedArtifactReference(reference)
	if err != nil || !managed || buildID != "build-id" || parsedDigest != digest {
		t.Fatalf("parsed managed reference = %q %q %v %v", buildID, parsedDigest, managed, err)
	}
	if _, _, managed, err = ParseManagedArtifactReference("alpine:3.21"); err != nil || managed {
		t.Fatalf("external OCI reference classified as managed: %v %v", managed, err)
	}
	if _, _, managed, err = ParseManagedArtifactReference("jobdock://build/build-id:latest"); err == nil || !managed {
		t.Fatalf("mutable managed reference was accepted: %v %v", managed, err)
	}
}

func TestBuildPlanRequiresRailpackDetectionAndValidJSON(t *testing.T) {
	plan := BuildPlan{BuildID: "build-one", Provider: "python", Runtime: "python 3.13", PackageManager: "uv", Entrypoint: "python main.py", RailpackVersion: "0.36.0", Plan: json.RawMessage(`{"deploy":{}}`), Info: json.RawMessage(`{"success":true}`)}
	if err := ValidateBuildPlan(plan); err != nil {
		t.Fatal(err)
	}
	plan.Provider = ""
	if err := ValidateBuildPlan(plan); err == nil {
		t.Fatal("plan without detected provider was accepted")
	}
	plan.Provider, plan.Plan = "python", json.RawMessage(`not-json`)
	if err := ValidateBuildPlan(plan); err == nil {
		t.Fatal("invalid Railpack JSON was accepted")
	}
}

func TestBuildAssignmentValidation(t *testing.T) {
	assignment := BuildAssignment{ID: "assignment", BuildID: "build", Status: BuildAssignmentPending}
	if err := ValidateBuildAssignment(assignment); err != nil {
		t.Fatal(err)
	}
	assignment.Status = BuildAssignmentRunning
	if err := ValidateBuildAssignment(assignment); err == nil {
		t.Fatal("running assignment without builder identity was accepted")
	}
	assignment.BuilderID = "builder"
	if err := ValidateBuildAssignment(assignment); err != nil {
		t.Fatal(err)
	}
}
