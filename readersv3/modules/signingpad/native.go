package signingpad

// padDriver is implemented by each manufacturer's native adapter. A driver is
// owned by one websocket session and all calls run on the same OS thread.
type padDriver interface {
	Count() (int32, error)
	Start() error
	Retry() error
	Confirm() ([]byte, error)
	Cancel() error
	Close()
}
