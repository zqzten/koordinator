//go:build !linux
// +build !linux

package dsa

func isDsaAvailableByRootPath(path string) (bool, error) {
	return false, nil
}

func isDsaAvailableByAccelConfig() (bool, error) {
	// try to enable accel config systemcall or orther conditions
	return false, nil
}
