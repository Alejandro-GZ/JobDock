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
