package storage

// Store combines schema and data storage
type Store struct {
	Schema *SchemaStore
	Data   *DataStore
}

// NewStore creates a new combined store
func NewStore(dataDir string) (*Store, error) {
	schema, err := NewSchemaStore(dataDir)
	if err != nil {
		return nil, err
	}

	data := NewDataStore(dataDir, schema)

	return &Store{
		Schema: schema,
		Data:   data,
	}, nil
}
