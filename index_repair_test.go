package smarterbase

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestIndexRepairService_ValidateAndRepair(t *testing.T) {
	ctx := context.Background()
	backend := NewFilesystemBackend(t.TempDir())
	store := NewStore(backend)
	repairService := NewIndexRepairService(backend)

	// Setup test data - generic structure (not domain-specific)
	type Item struct {
		ID       string `json:"id"`
		ParentID string `json:"parent_id"`
	}

	type Container struct {
		ID    string `json:"id"`
		Items []Item `json:"items"`
	}

	containers := []Container{
		{
			ID: "container1",
			Items: []Item{
				{ID: "item1", ParentID: "container1"},
				{ID: "item2", ParentID: "container1"},
			},
		},
		{
			ID: "container2",
			Items: []Item{
				{ID: "item3", ParentID: "container2"},
			},
		},
	}

	// Create test data
	for _, c := range containers {
		key := fmt.Sprintf("containers/%s/data.json", c.ID)
		store.PutJSON(ctx, key, c)
	}

	t.Run("DetectAndRepairMissingIndexes", func(t *testing.T) {
		extractFunc := func(data []byte) (map[string]string, error) {
			var container Container
			if err := json.Unmarshal(data, &container); err != nil {
				return nil, err
			}

			items := make(map[string]string)
			for _, item := range container.Items {
				items[item.ID] = container.ID
			}
			return items, nil
		}

		createIndexFunc := func(ctx context.Context, itemID, containerID string) error {
			indexKey := fmt.Sprintf("indexes/item-%s.json", itemID)
			return store.PutJSON(ctx, indexKey, map[string]string{"container_id": containerID})
		}

		report, err := repairService.ValidateAndRepairIndexes(
			ctx,
			"containers/",
			"indexes/item-",
			func(key string) bool { return strings.HasSuffix(key, "data.json") },
			extractFunc,
			createIndexFunc,
		)

		if err != nil {
			t.Fatalf("ValidateAndRepairIndexes failed: %v", err)
		}

		if report.Validated != 3 {
			t.Errorf("Expected 3 items validated, got %d", report.Validated)
		}

		if report.Repaired != 3 {
			t.Errorf("Expected 3 indexes repaired, got %d", report.Repaired)
		}

		// Verify indexes were created
		for _, itemID := range []string{"item1", "item2", "item3"} {
			indexKey := fmt.Sprintf("indexes/item-%s.json", itemID)
			exists, _ := backend.Exists(ctx, indexKey)
			if !exists {
				t.Errorf("Index for %s was not created", itemID)
			}
		}
	})
}
