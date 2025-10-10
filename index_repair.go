package smarterbase

import (
	"context"
	"fmt"
	"strings"
)

// IndexRepairService provides utilities for validating and repairing indexes
// Works with any Backend implementation (S3, filesystem, etc.)
type IndexRepairService struct {
	backend Backend
}

// NewIndexRepairService creates a new index repair service
func NewIndexRepairService(backend Backend) *IndexRepairService {
	return &IndexRepairService{backend: backend}
}

// RepairReport contains results from an index repair operation
type RepairReport struct {
	IndexType       string
	Validated       int
	Repaired        int
	Errors          []string
	MissingIndexes  []string
	OrphanedIndexes []string
}

// ValidateAndRepairIndexes checks and repairs reverse indexes
// dataPrefix: prefix for data objects (e.g., "projects/")
// indexPrefix: prefix for index objects (e.g., "indexes/photo-")
// extractFunc: function to extract items from data objects
func (r *IndexRepairService) ValidateAndRepairIndexes(
	ctx context.Context,
	dataPrefix string,
	indexPrefix string,
	dataFilter func(key string) bool,
	extractFunc func(data []byte) (map[string]string, error),
	createIndexFunc func(ctx context.Context, itemID, parentID string) error,
) (*RepairReport, error) {
	report := &RepairReport{
		Errors:          []string{},
		MissingIndexes:  []string{},
		OrphanedIndexes: []string{},
	}

	// Step 1: Collect all actual items from data objects
	actualItems := make(map[string]string) // itemID -> parentID

	err := r.backend.ListPaginated(ctx, dataPrefix, func(keys []string) error {
		for _, key := range keys {
			if !dataFilter(key) {
				continue
			}

			data, err := r.backend.Get(ctx, key)
			if err != nil {
				report.Errors = append(report.Errors, fmt.Sprintf("Failed to read %s: %v", key, err))
				continue
			}

			items, err := extractFunc(data)
			if err != nil {
				report.Errors = append(report.Errors, fmt.Sprintf("Failed to extract items from %s: %v", key, err))
				continue
			}

			for itemID, parentID := range items {
				actualItems[itemID] = parentID
			}
		}
		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to list data objects: %w", err)
	}

	report.Validated = len(actualItems)

	// Step 2: Check existing indexes and find orphans
	indexedItems := make(map[string]bool)

	err = r.backend.ListPaginated(ctx, indexPrefix, func(keys []string) error {
		for _, key := range keys {
			// Extract itemID from index key
			itemID := strings.TrimPrefix(key, indexPrefix)
			itemID = strings.TrimSuffix(itemID, ".json")

			indexedItems[itemID] = true

			// Check if this index is orphaned
			if _, exists := actualItems[itemID]; !exists {
				report.OrphanedIndexes = append(report.OrphanedIndexes, itemID)
			}
		}
		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to list indexes: %w", err)
	}

	// Step 3: Find missing indexes and repair them
	for itemID, parentID := range actualItems {
		if !indexedItems[itemID] {
			report.MissingIndexes = append(report.MissingIndexes, itemID)
			if err := createIndexFunc(ctx, itemID, parentID); err != nil {
				report.Errors = append(report.Errors, fmt.Sprintf("Failed to repair index for %s: %v", itemID, err))
			} else {
				report.Repaired++
			}
		}
	}

	return report, nil
}
