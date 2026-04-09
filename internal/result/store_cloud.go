package result

import "fmt"

// CloudStore reads results from the Chiperka Cloud API.
// Currently a stub — will be implemented when cloud result endpoints are available.
type CloudStore struct{}

// NewCloudStore creates a new cloud result store.
func NewCloudStore() *CloudStore {
	return &CloudStore{}
}

func (s *CloudStore) ListRuns(limit int) ([]RunSummary, error) {
	return nil, fmt.Errorf("cloud result listing is not yet supported")
}

func (s *CloudStore) GetRun(uuid string) (*RunSummary, error) {
	return nil, fmt.Errorf("cloud result retrieval is not yet supported")
}

func (s *CloudStore) GetTest(uuid string) (*TestDetail, error) {
	return nil, fmt.Errorf("cloud result retrieval is not yet supported")
}

func (s *CloudStore) GetArtifact(testUUID string, name string) ([]byte, error) {
	return nil, fmt.Errorf("cloud result retrieval is not yet supported")
}
