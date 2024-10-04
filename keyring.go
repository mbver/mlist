package memberlist

type Keyring struct{}

func NewKeyring(keys [][]byte, primaryKey []byte) (*Keyring, error) {
	return nil, nil
}

func (r *Keyring) AddKey(key []byte) error {
	return nil
}

func ValidateKey(key []byte) error {
	return nil
}

func (r *Keyring) UseKey(key []byte) error {
	return nil
}

func (r *Keyring) GetKeys() [][]byte {
	return nil
}

func (r *Keyring) GetPrimaryKeys() []byte {
	return nil
}
