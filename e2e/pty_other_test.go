//go:build !linux

package e2e

import "errors"

func (p *project) runWithTTY(answer string, args ...string) (result, error) {
	return result{}, errors.New("pseudo-terminal helper is Linux only")
}
