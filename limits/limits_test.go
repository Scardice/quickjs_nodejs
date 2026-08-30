package limits

import "testing"

func TestNewRuntimeRejectsNegativeLimit(t *testing.T) {
	_, err := NewRuntime(Config{MaxFetchConcurrent: -1})
	if err == nil {
		t.Fatal("NewRuntime accepted a negative fetch concurrency limit")
	}
}

func TestRuntimeFetchSlotsRejectExcessAndRelease(t *testing.T) {
	runtime, err := NewRuntime(Config{MaxFetchConcurrent: 1})
	if err != nil {
		t.Fatal(err)
	}

	releaseFirst, err := runtime.AcquireFetch()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.AcquireFetch(); err == nil {
		t.Fatal("second fetch acquired the sole configured slot")
	}

	releaseFirst()
	releaseSecond, err := runtime.AcquireFetch()
	if err != nil {
		t.Fatalf("fetch slot was not released: %v", err)
	}
	releaseSecond()
}

func TestRuntimeUnlimitedFetchSlotsDoNotBlock(t *testing.T) {
	runtime, err := NewRuntime(Config{})
	if err != nil {
		t.Fatal(err)
	}

	releaseFirst, err := runtime.AcquireFetch()
	if err != nil {
		t.Fatal(err)
	}
	releaseSecond, err := runtime.AcquireFetch()
	if err != nil {
		t.Fatalf("unlimited fetch slot unexpectedly blocked: %v", err)
	}
	releaseFirst()
	releaseSecond()
}
