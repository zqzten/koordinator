package memorylocality

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"

	"gitlab.alibaba-inc.com/cos/unified-resource-api/apis/extension/cpuset"

	"github.com/koordinator-sh/koordinator/pkg/koordlet/util/system"
)

var (
	hasInitNumaInfo  bool
	initNumaInfoLock sync.Mutex
	defaultNumaInfo  *NumaInfo
)

const (
	OnlineNumaFile = "devices/system/node/online"
)

// GetMemoryLocalityStatus parses MemoryLocalityStatus from annotations
func GetMemoryLocalityStatus(annotations map[string]string) (*MemoryLocalityStatus, error) {
	if annotations == nil {
		return nil, nil
	}
	memoryLocalityStatus := &MemoryLocalityStatus{}
	data, ok := annotations[AnnotationMemoryLocalityStatus]
	if !ok {
		return nil, nil
	}
	err := json.Unmarshal([]byte(data), &memoryLocalityStatus)
	if err != nil {
		return nil, err
	}
	return memoryLocalityStatus, nil
}

func MigrateBetweenNuma(podNodeList map[int]struct{}, tasks []int32) MigratePagesResult {
	res := MigratePagesResult{}
	res.Status = MigratedFailed
	if len(podNodeList) == 0 {
		res.Message = "failed to get the local numa of pod"
		return res
	}
	if len(tasks) == 0 {
		res.Message = "failed to get the tasks of the pod"
		return res
	}
	basicNumaInfo, err := getBasicNumaInfo()
	if err != nil {
		res.Message = "failed to parse basic numa info"
		return res
	}
	NumaInfo, err := getNumaNodes(basicNumaInfo, podNodeList)
	if err != nil {
		res.Message = "failed to parse the numa info"
		return res
	}
	if !isNumaInfoValid(NumaInfo) {
		res.Message = fmt.Sprintf("parsed numa info is invalid, %+v", NumaInfo)
		return res
	}
	if !isTargetNumaInfoValid(NumaInfo) {
		res.Message = fmt.Sprintf("no target numa, %+v", NumaInfo)
		res.Status = MigratedSkipped
		return res
	}

	podRemoteNodeStr := strings.Join(NumaInfo.podRemoteNumaInfo, ",")
	podLocalNodeStr := strings.Join(NumaInfo.podLocalNumaInfo, ",")

	var failedMsg []string
	for _, taskId := range tasks {
		migrationStr := []string{"nice", "-19", "migratepages", strconv.FormatInt(int64(taskId), 10), podRemoteNodeStr, podLocalNodeStr}
		err := execMigrateCommand(migrationStr)
		if err != nil && !strings.Contains(err.Error(), "No such process") {
			msg := fmt.Sprintf("failed pid: %d, error: %s", taskId, err.Error())
			failedMsg = append(failedMsg, msg)
		}
	}

	if len(failedMsg) != 0 {
		res.Message = fmt.Sprintf("failed to migrate the following processes: %s ,from remote numa: %s to local numa: %s",
			strings.Join(failedMsg, "; "), podRemoteNodeStr, podLocalNodeStr)
		res.Status = MigratedCompleted
		return res
	}
	res.Message = fmt.Sprintf("migrated memory from remote numa: %s to local numa: %s", podRemoteNodeStr, podLocalNodeStr)
	res.Status = MigratedCompleted
	return res
}

func getBasicNumaInfo() (*NumaInfo, error) {
	initNumaInfoLock.Lock()
	defer initNumaInfoLock.Unlock()
	if hasInitNumaInfo {
		return defaultNumaInfo, nil
	}
	NumaInfo := &NumaInfo{}
	nodelist, err := ioutil.ReadFile(filepath.Join(system.Conf.SysRootDir, OnlineNumaFile))
	if err != nil {
		return nil, err
	}
	// Parse the nodelist into a set of Node IDs
	nodes, err := cpuset.Parse(strings.TrimSpace(string(nodelist)))
	if err != nil {
		return nil, err
	}
	// For each node...
	for _, node := range nodes.ToSlice() {
		NumaInfo.totalNumaInfo = append(NumaInfo.totalNumaInfo, strconv.Itoa(node))
		// Read the 'cpulist' of the NUMA node from sysfs.
		path := fmt.Sprintf("/sys/devices/system/node/node%d/cpulist", node)

		cpulist, err := ioutil.ReadFile(path)
		if err != nil {
			return nil, err
		}
		cpulistStr := string(cpulist)
		cpulistStr = strings.Replace(cpulistStr, "\n", "", -1)
		if cpulistStr != "" {
			NumaInfo.nonAepNumaInfo = append(NumaInfo.nonAepNumaInfo, strconv.Itoa(node))
		} else {
			NumaInfo.aepNumaInfo = append(NumaInfo.aepNumaInfo, strconv.Itoa(node))
		}
	}
	if !isBasicNumaInfoValid(NumaInfo) {
		return nil, fmt.Errorf("parsed basic numa invalid, NumaInfo %+v", NumaInfo)
	}
	hasInitNumaInfo = true
	defaultNumaInfo = NumaInfo
	return NumaInfo, nil
}

// input: podNodeList: map[int]struct{}{}
// output: sourceNodeList, targetNodeList, aepNodeList: string
func getNumaNodes(basicNumaInfo *NumaInfo, podNodeList map[int]struct{}) (*NumaInfo, error) {
	if basicNumaInfo == nil {
		return nil, fmt.Errorf("basic node info is nil")
	}
	if len(podNodeList) == 0 {
		return nil, fmt.Errorf("pod node list is nil")
	}
	NumaInfo := basicNumaInfo.DeepCopy()
	// For each node...
	for _, nodeStr := range NumaInfo.totalNumaInfo {
		node, err := strconv.Atoi(nodeStr)
		if err != nil {
			return NumaInfo, err
		}
		if _, ok := podNodeList[node]; !ok {
			NumaInfo.podRemoteNumaInfo = append(NumaInfo.podRemoteNumaInfo, nodeStr)
		} else {
			NumaInfo.podLocalNumaInfo = append(NumaInfo.podLocalNumaInfo, nodeStr)
		}
	}
	return NumaInfo, nil
}

//  nonAep + aep == total; local + remote = total
func isBasicNumaInfoValid(NumaInfo *NumaInfo) bool {
	if NumaInfo == nil {
		return false
	}
	if len(NumaInfo.totalNumaInfo) == 0 ||
		len(NumaInfo.aepNumaInfo)+len(NumaInfo.nonAepNumaInfo) != len(NumaInfo.totalNumaInfo) {
		return false
	}
	return true
}

//  local != nil; remote != nil
func isNumaInfoValid(NumaInfo *NumaInfo) bool {
	if NumaInfo == nil {
		return false
	}
	if len(NumaInfo.podLocalNumaInfo) == 0 ||
		len(NumaInfo.podLocalNumaInfo)+len(NumaInfo.podRemoteNumaInfo) != len(NumaInfo.totalNumaInfo) ||
		reflect.DeepEqual(NumaInfo.podLocalNumaInfo, NumaInfo.podRemoteNumaInfo) {
		return false
	}
	return true
}

func isTargetNumaInfoValid(NumaInfo *NumaInfo) bool {
	return NumaInfo.podRemoteNumaInfo != nil && len(NumaInfo.podRemoteNumaInfo) != 0
}

func execMigrateCommand(migrationStr []string) error {
	var errB bytes.Buffer
	command := exec.Command(migrationStr[0], migrationStr[1:]...)
	command.Stderr = &errB
	_, err := command.Output()
	return err
}
