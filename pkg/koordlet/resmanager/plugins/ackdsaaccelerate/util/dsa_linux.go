//go:build linux
// +build linux

package dsa

import (
	"bytes"
	"os"
	"os/exec"
)

func isDsaAvailableByRootPath(path string) (bool, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return false, nil
	} else if err != nil {
		return false, err
	}
	return true, nil
}

func isDsaAvailableByAccelConfig() (bool, error) {
	// cmd accel-config list to check if accelconfig is ready
	var errB bytes.Buffer
	command := exec.Command("accel-config", "--version")
	command.Stderr = &errB
	_, err := command.Output()
	if err != nil {
		return false, err
	}
	return true, nil
}
