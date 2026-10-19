//go:build !darwin && !windows

package secret

func Set(name, value string) error    { return ErrUnsupported }
func Get(name string) (string, error) { return "", ErrUnsupported }
func Delete(name string) error        { return ErrUnsupported }
