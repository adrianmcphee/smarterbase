package smarterbase

import (
	"context"
	"encoding/json"
	"testing"
)

func TestIndexer(t *testing.T) {
	ctx := context.Background()
	backend := NewFilesystemBackend(t.TempDir())
	store := NewStore(backend)
	indexer := NewIndexer(store)

	// Define a simple User type
	type User struct {
		ID    string `json:"id"`
		Email string `json:"email"`
		Name  string `json:"name"`
	}

	// Register an index: email → user object key
	indexer.RegisterIndex(&IndexSpec{
		Name: "users-by-email",
		KeyFunc: func(data interface{}) (string, error) {
			user := data.(*User)
			return user.Email, nil
		},
		ExtractFunc: func(data []byte) (interface{}, error) {
			var user User
			err := json.Unmarshal(data, &user)
			return &user, err
		},
		IndexKey: func(email string) string {
			return "indexes/users-by-email/" + email + ".json"
		},
	})

	// Create a user
	user := &User{
		ID:    NewID(),
		Email: "john@example.com",
		Name:  "John Doe",
	}

	userKey := "users/" + user.ID + ".json"
	userData, _ := json.Marshal(user)

	// Store user
	store.PutJSON(ctx, userKey, user)

	// Update indexes
	err := indexer.UpdateIndexes(ctx, userKey, userData)
	if err != nil {
		t.Fatalf("UpdateIndexes failed: %v", err)
	}

	// Query by email
	foundKey, err := indexer.QueryIndex(ctx, "users-by-email", "john@example.com")
	if err != nil {
		t.Fatalf("QueryIndex failed: %v", err)
	}

	if foundKey != userKey {
		t.Errorf("Expected %s, got %s", userKey, foundKey)
	}

	// Retrieve user via index
	var retrievedUser User
	err = store.GetJSON(ctx, foundKey, &retrievedUser)
	if err != nil {
		t.Fatalf("GetJSON failed: %v", err)
	}

	if retrievedUser.Email != user.Email {
		t.Errorf("Expected email %s, got %s", user.Email, retrievedUser.Email)
	}
}

func TestReverseIndexSpec(t *testing.T) {
	ctx := context.Background()
	backend := NewFilesystemBackend(t.TempDir())
	store := NewStore(backend)
	indexer := NewIndexer(store)

	// Register reverse index: photoID → project key
	indexer.RegisterIndex(ReverseIndexSpec(
		"photos-to-projects",
		"indexes/photo-",
		func(data interface{}) string {
			m := data.(map[string]interface{})
			return m["id"].(string)
		},
	))

	// Create photo object
	photoData := map[string]interface{}{
		"id":         "photo123",
		"project_id": "proj456",
		"url":        "https://example.com/photo.jpg",
	}

	objectKey := "projects/proj456/photos/photo123.json"
	photoJSON, _ := json.Marshal(photoData)

	store.PutJSON(ctx, objectKey, photoData)
	indexer.UpdateIndexes(ctx, objectKey, photoJSON)

	// Query reverse index
	foundKey, err := indexer.QueryIndex(ctx, "photos-to-projects", "photo123")
	if err != nil {
		t.Fatalf("QueryIndex failed: %v", err)
	}

	if foundKey != objectKey {
		t.Errorf("Expected %s, got %s", objectKey, foundKey)
	}
}
