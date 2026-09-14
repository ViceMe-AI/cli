package command

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestWriteCommercePaymentPresentationAllowsConcurrentIdenticalWrites(t *testing.T) {
	directory := t.TempDir()
	filename := filepath.Join(directory, "wechat-concurrent.png")
	data := []byte("identical-payment-qr")
	const writers = 8
	errs := make(chan error, writers)
	var group sync.WaitGroup
	for i := 0; i < writers; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			errs <- writeCommercePaymentPresentation(filename, data)
		}()
	}
	group.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent identical payment QR write failed: %v", err)
		}
	}
	got, err := os.ReadFile(filename)
	if err != nil || string(got) != string(data) {
		t.Fatalf("concurrent write result = %q err=%v", got, err)
	}
}
