package util

// User represents a named user.
type User struct {
	Name string
}

// NewUser creates a User with the given name.
func NewUser(name string) *User {
	return &User{Name: name}
}
