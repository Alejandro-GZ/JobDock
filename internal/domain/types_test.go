package domain

import "testing"

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

func TestBuildLifecycleAndValidation(t *testing.T) {
	if !CanBuildTransition(BuildCreated, BuildAnalyzing) || !CanBuildTransition(BuildAnalyzing, BuildBuilding) || !CanBuildTransition(BuildBuilding, BuildSucceeded) {
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
}
