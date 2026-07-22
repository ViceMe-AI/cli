//go:build !darwin

package securestore

func NewDefault(service, _ string) Store {
	return NewKeyring(service)
}
