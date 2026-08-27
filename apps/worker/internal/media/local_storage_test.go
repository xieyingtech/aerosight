package media

import (
	"bytes"
	"context"
	"testing"
)

func TestLocalObjectStorageRoundTripsWithinProjectRoot(t *testing.T) {
	storage, err := NewLocalObjectStorage(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	key := "projects/17/uploads/test/frame.jpg"
	written, err := storage.PutObject(context.Background(), key, bytes.NewBufferString("frame"), "image/jpeg")
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := storage.GetObject(context.Background(), key)
	if err != nil {
		t.Fatal(err)
	}
	if string(loaded.Body) != "frame" || loaded.ChecksumSHA256 != written.ChecksumSHA256 {
		t.Fatalf("local object mismatch: %#v %#v", written, loaded)
	}
}

func TestLocalObjectStorageRejectsEscapingKeys(t *testing.T) {
	storage, err := NewLocalObjectStorage(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"../secret", "projects/17/../../secret", "/projects/17/secret", "projects\\17\\secret"} {
		if _, err := storage.GetObject(context.Background(), key); err == nil {
			t.Fatalf("unsafe key was accepted: %q", key)
		}
	}
}
