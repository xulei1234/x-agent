package common

// ExecRes respone
type ExecRes struct {
	Buf []byte
	Err error
}

var AddressChangeBuffer chan struct{}

func init() {
	AddressChangeBuffer = make(chan struct{}, 50)
}
