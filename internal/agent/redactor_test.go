package agent

import "testing"

func TestRedactorAcrossChunks(t *testing.T) {
	redactor := newRedactor(map[string]string{"token": "supersecret"})
	output := append(redactor.Push([]byte("value=super")), redactor.Push([]byte("secret done"))...)
	output = append(output, redactor.Flush()...)
	if string(output) != "value=*********** done" {
		t.Fatalf("unexpected output %q", output)
	}
}
