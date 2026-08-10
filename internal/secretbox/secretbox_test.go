package secretbox

import (
	"bytes"
	"testing"
)

func TestRoundTrip(t *testing.T) {
	box, err := New(bytes.Repeat([]byte{7}, 32))
	if err != nil {
		t.Fatal(err)
	}
	encrypted, err := box.Encrypt([]byte("secret"), []byte("owner/name"))
	if err != nil {
		t.Fatal(err)
	}
	plain, err := box.Decrypt(encrypted, []byte("owner/name"))
	if err != nil || string(plain) != "secret" {
		t.Fatalf("round trip failed: %q, %v", plain, err)
	}
}
