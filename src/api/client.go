package api

// Client is the language-agnostic CRUD seam used by Glide syncers and imperative commands.
type Client interface {
	List(resource string) ([]any, error)
	ListAll(resource string) ([]any, error)
	Get(resource, id string) (any, error)
	Create(resource string, payload any) (any, error)
	Update(resource, id string, payload any, headers map[string]string) (any, error)
	Delete(resource, id string, headers map[string]string) error
}
