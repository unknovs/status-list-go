package storage

// Storage defines the interface for pluggable storage backends.
// All storage operations must implement this interface to ensure consistent
// behavior across local and S3 backends.
type Storage interface {
	// Create creates a new file with the given content.
	// Returns an error if the file already exists or creation fails.
	Create(path string, content []byte) error

	// Read retrieves the content of a file.
	// Returns the content bytes and any error encountered.
	Read(path string) ([]byte, error)

	// Write updates an existing file with optimistic locking.
	// The version parameter must match the current file version.
	// Returns an error if version mismatch (concurrent modification) or write fails.
	Write(path string, content []byte, version int) error

	// Exists checks if a file exists at the given path.
	// Returns true if exists, false otherwise, and any error encountered.
	Exists(path string) (bool, error)

	// List returns a list of file paths with the given prefix.
	// Used by renewal process to discover status list files.
	// Returns slice of relative paths and any error encountered.
	List(prefix string) ([]string, error)

	// GetVersion retrieves the current version number of a file.
	// Returns 0 if file doesn't exist, current version if exists, and any error encountered.
	GetVersion(path string) (int, error)

	// DeleteTree removes all files under the given path prefix.
	// Implementations must support deleting nested directories or object prefixes.
	DeleteTree(prefix string) error
}
