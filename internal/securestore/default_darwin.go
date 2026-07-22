//go:build darwin

package securestore

func NewDefault(service, configRoot string) Store {
	return NewEncryptedFile(configRoot, service, NewKeyring(service))
}
