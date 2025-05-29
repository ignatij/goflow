package storage

func InitStore(dbConnStr string) (*PostgresStore, error) {
	store, err := NewPostgresStore(dbConnStr)
	if err != nil {
		return nil, err
	}
	return store, nil
}
