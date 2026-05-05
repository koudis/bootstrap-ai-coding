package agent

// CredentialPreparer is an optional interface that agents can implement to
// perform additional credential synchronisation before the container starts.
// For example, copying external state files into the credential store so they
// are available inside the bind-mounted directory.
type CredentialPreparer interface {
	PrepareCredentials(storePath string) error
}
