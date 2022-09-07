/*
Copyright 2022 The Koordinator Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package cgroupscrd

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"golang.org/x/sys/unix"
	"k8s.io/klog/v2"

	resourcesv1alpha1 "gitlab.alibaba-inc.com/cos/unified-resource-api/apis/resources/v1alpha1"
)

func GenerateBklioContent(dv []resourcesv1alpha1.DeviceValue) string {
	content := ""
	for _, device := range dv {
		// TODO gather device info by collector
		devID, err := DeviceFromPath(device.Device)
		if err != nil {
			klog.Errorf("DeviceFromPath %s typeInfo error %v", device.Device, err)
			continue
		}
		if device.Value == "" {
			klog.Warningf("Skip %s for empty value", device.Device)
			continue
		}
		content += fmt.Sprintf("%s %s\n", devID, device.Value)
	}
	return content
}

// DeviceFromPath given the path to a device and its cgroup_permissions(which cannot be easily queried)
// look up the information about a linux device and return that information as a Device struct.
func DeviceFromPath(devicePath string) (deviceId string, err error) {
	klog.Infof("DeviceFromPath path %s", devicePath)
	if devicePath == "rootfs" {
		devicePath = getRootfsDevice("/var/lib/kubelet")
	}
	if devicePath == "" {
		return "", errors.New("device path is empty")
	}
	defer func() {
		if e := recover(); e != nil {
			err = fmt.Errorf("%s", e)
		}
	}()

	// cgroup not support partition
	mainDevicePath := getMainDevice(devicePath)
	if mainDevicePath == "" {
		klog.Warningf("get mainDevicePath error with empty out %s", devicePath)
	}

	var stat *syscall.Stat_t
	fsinfo, err := os.Stat(mainDevicePath)
	if err != nil {
		return "", err
	}
	stat = fsinfo.Sys().(*syscall.Stat_t)
	devNumber := stat.Rdev
	major := unix.Major(uint64(devNumber))
	if major == 0 {
		return "", errors.New("not a device node")
	}

	majorStr := strconv.FormatInt(int64(major), 10)
	minorStr := strconv.FormatInt(int64(unix.Minor(uint64(devNumber))), 10)
	return majorStr + ":" + minorStr, nil
}

func getMainDevice(devicePath string) string {
	cmd := fmt.Sprintf("lsblk -no pkname %s", devicePath)
	out, err := exec.Command("sh", "-c", cmd).CombinedOutput()
	if err != nil {
		return ""
	}
	mainDevName := strings.TrimSpace(string(out))
	if mainDevName == "" {
		cmd := fmt.Sprintf("lsblk -n %s | grep -v grep | awk '{print $1}'", devicePath)
		out, err := exec.Command("sh", "-c", cmd).CombinedOutput()
		if err != nil {
			return ""
		}
		mainDevName = strings.TrimSpace(string(out))
	}
	mainDevPath := filepath.Join("/dev", mainDevName)
	if IsFileExisting(mainDevPath) {
		return mainDevPath
	} else {
		klog.Warningf("device path got %s, but not exist.", mainDevPath)
		return ""
	}
}

func getRootfsDevice(path string) string {
	curPath := path
	if path == "" {
		curPath = "/var/lib/kubelet"
	}
	cmd := fmt.Sprintf("df %s | grep -v Filesystem | awk '{print $1}'", curPath)
	out, err := exec.Command("sh", "-c", cmd).CombinedOutput()
	if err != nil {
		return ""
	}
	outStr := strings.TrimSpace(string(out))
	return outStr
}

func IsFileExisting(filename string) bool {
	_, err := os.Stat(filename)
	if err == nil {
		return true
	}
	if os.IsNotExist(err) {
		return false
	}
	return true
}
