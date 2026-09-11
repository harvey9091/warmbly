package jobs

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/warmbly/warmbly/internal/app/instancesettings"
	"github.com/warmbly/warmbly/internal/config"
	"github.com/warmbly/warmbly/internal/repository"
)

// Live checks that the operator-editable machine windows survive the trip
// through the settings document's jsonb column and reach the classifier.
// Skipped unless WARMBLY_TEST_DB is set:
//
//	WARMBLY_TEST_DB=postgres://warmbly:warmbly@localhost:15432/warmbly_dev?sslmode=disable \
//	  go test ./internal/app/consumer/ -run Live -v
//
// The unit tests cover the arithmetic. What they cannot cover is the storage:
// the whole section is one jsonb document, so a section that fails to marshal,
// or that an existing instance's document lacks entirely, is only visible
// against a real row.

// An instance that upgrades has a settings row written before this section
// existed. Its document has no "tracking" key at all, and the windows have to
// come back as the shipped defaults rather than as zero, which would read as
// "no window" and let every delivery-time scan count as a person.
func TestLiveTrackingWindowsDefaultOnADocumentWithoutTheSection(t *testing.T) {
	handle := liveDB(t)
	ctx := context.Background()
	store := instancesettings.NewStore(handle.Pool)

	// A document exactly as an older version would have written it.
	old := instancesettings.Defaults()
	raw, err := json.Marshal(old)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var stripped map[string]any
	if err := json.Unmarshal(raw, &stripped); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	delete(stripped, "tracking")
	if _, ok := stripped["tracking"]; ok {
		t.Fatal("the fixture still carries a tracking section")
	}
	pruned, err := json.Marshal(stripped)
	if err != nil {
		t.Fatalf("marshal pruned: %v", err)
	}
	var doc instancesettings.Document
	if err := json.Unmarshal(pruned, &doc); err != nil {
		t.Fatalf("unmarshal pruned: %v", err)
	}
	if err := store.Put(ctx, doc, nil); err != nil {
		t.Fatalf("put: %v", err)
	}
	t.Cleanup(func() { _ = store.Put(ctx, instancesettings.Defaults(), nil) })

	got, err := store.Get(ctx)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Tracking.MachineWindowOpenSeconds != config.TrackingMachineWindowOpenSecondsDefault {
		t.Errorf("open window = %d, want the shipped %d on a document with no tracking section",
			got.Tracking.MachineWindowOpenSeconds, config.TrackingMachineWindowOpenSecondsDefault)
	}
	if got.Tracking.MachineWindowClickSeconds != config.TrackingMachineWindowClickSecondsDefault {
		t.Errorf("click window = %d, want the shipped %d on a document with no tracking section",
			got.Tracking.MachineWindowClickSeconds, config.TrackingMachineWindowClickSecondsDefault)
	}
}

// The operator's saved value has to reach the classifier, which is the whole
// point of the setting. This walks the real path: a patch through the service,
// the jsonb row, and the consumer reading it back to classify an event.
func TestLiveTrackingWindowReachesTheClassifier(t *testing.T) {
	handle := liveDB(t)
	ctx := context.Background()
	store := instancesettings.NewStore(handle.Pool)
	t.Cleanup(func() { _ = store.Put(ctx, instancesettings.Defaults(), nil) })

	widened := 300
	patch := instancesettings.Patch{Tracking: &struct {
		MachineWindowOpenSeconds  *int `json:"machine_window_open_seconds"`
		MachineWindowClickSeconds *int `json:"machine_window_click_seconds"`
	}{MachineWindowOpenSeconds: &widened}}

	if _, err := instancesettings.NewService(store).Put(ctx, patch, nil); err != nil {
		t.Fatalf("put: %v", err)
	}

	// A reader that has never cached, as the consumer process is on the next
	// poll after an edit.
	tc := &TrackingConsumer{}
	tc.WireTrackingPolicy(instancesettings.NewService(store))

	windows := tc.machineWindows(ctx)
	if got, want := windows.OpenWindow(), time.Duration(widened)*time.Second; got != want {
		t.Fatalf("open window = %v, want %v", got, want)
	}
	// The click window was not part of the patch and must keep its default
	// rather than being cleared by a partial write.
	if got, want := windows.ClickWindow(), time.Duration(config.TrackingMachineWindowClickSecondsDefault)*time.Second; got != want {
		t.Fatalf("click window = %v, want the untouched default %v", got, want)
	}

	sent := time.Now()
	chrome := strp("Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/125.0.0.0 Safari/537.36")
	at := sent.Add(4 * time.Minute)

	// Four minutes is well past the shipped 60s and inside the saved 300s, so
	// this only passes if the stored value is what the classifier used.
	if m, r := classifyOpen(chrome, nil, &sent, at, windows.OpenWindow()); !m || r != repository.EmailOpenReasonInstant {
		t.Fatalf("an open inside the saved window is automated, got %v %q", m, r)
	}
	if m, _ := classifyOpen(chrome, nil, &sent, at, instancesettings.DefaultTracking().OpenWindow()); m {
		t.Fatal("the same open is a person's under the shipped window; the test proves nothing otherwise")
	}
}

// An out-of-range value must be clamped on the way in, not stored and applied.
// Normalize runs on write and on read, so a hand-edited row is bounded too.
func TestLiveTrackingWindowClampsThroughStorage(t *testing.T) {
	handle := liveDB(t)
	ctx := context.Background()
	store := instancesettings.NewStore(handle.Pool)
	t.Cleanup(func() { _ = store.Put(ctx, instancesettings.Defaults(), nil) })

	doc := instancesettings.Defaults()
	doc.Tracking.MachineWindowOpenSeconds = config.TrackingMachineWindowSecondsMax + 10_000
	doc.Tracking.MachineWindowClickSeconds = -5
	if err := store.Put(ctx, doc, nil); err != nil {
		t.Fatalf("put: %v", err)
	}

	got, err := store.Get(ctx)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Tracking.MachineWindowOpenSeconds != config.TrackingMachineWindowSecondsMax {
		t.Errorf("open window = %d, want it clamped to %d",
			got.Tracking.MachineWindowOpenSeconds, config.TrackingMachineWindowSecondsMax)
	}
	if got.Tracking.MachineWindowClickSeconds != config.TrackingMachineWindowClickSecondsDefault {
		t.Errorf("click window = %d, want a negative to resolve to the default %d",
			got.Tracking.MachineWindowClickSeconds, config.TrackingMachineWindowClickSecondsDefault)
	}
}
