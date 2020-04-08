package basecomponent

import (
	"errors"

	pstore "github.com/libp2p/go-libp2p-peerstore"
	record "github.com/libp2p/go-libp2p-record"
)

type LibP2PValidator struct {
	KeyBook pstore.KeyBook
}

func (v LibP2PValidator) Validate(key string, value []byte) error {
	ns, _, err := record.SplitKey(key)
	if err != nil || ns != "matrix" {
		return errors.New("not Matrix path")
	}
	return nil
}

func (v LibP2PValidator) Select(k string, vals [][]byte) (int, error) {
	return 0, nil
}
