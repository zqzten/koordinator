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
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math"
	"strconv"
	"strings"

	"k8s.io/klog/v2"

	sysutil "github.com/koordinator-sh/koordinator/pkg/util/system"

	"gitlab.alibaba-inc.com/cos/unified-resource-api/apis/extension/cpuset"
)

// input: '{"container1":{"0":{"elems":{"0":{},"1":{},"2":{},"3":{}}}},"container2":{"1":{"elems":{"26":{},"27":{},"28":{},"29":{}}}}}'
// output: {"container1": "0,1,2,3", "container2": "26,27,28,29"}
func parseCPUSetAllocResult(cpuSetAllocStr string) (map[string]string, error) {
	containerCPUSetMap := make(map[string]string, 0)
	var numaNodeInfo map[string]map[int]cpuset.CPUSet
	err := json.Unmarshal([]byte(cpuSetAllocStr), &numaNodeInfo)
	if err != nil {
		return containerCPUSetMap, err
	}
	for containerName, containerCpuSet := range numaNodeInfo {
		containerCPUSetStr := ""
		for _, containerNumaNodeCpuSet := range containerCpuSet {
			if containerCPUSetStr != "" {
				containerCPUSetStr += ","
			}
			containerCPUSetStr += containerNumaNodeCpuSet.String()
		}
		if strings.HasSuffix(containerCPUSetStr, ",") {
			containerCPUSetStr = containerCPUSetStr[:len(containerCPUSetStr)-1]
		}
		containerCPUSetMap[containerName] = containerCPUSetStr
	}
	return containerCPUSetMap, nil
}

func getTaskCpuset(percent, cpuNum int) string {
	return strconv.Itoa(cpuNum-(cpuNum*percent)/100) + "-" + strconv.Itoa(cpuNum-1)
}

func getIsolateCpuset(lowCPUPercent int) (string, string) {
	nodelist, err := ioutil.ReadFile(sysutil.Conf.SysRootDir + "/devices/system/node/online")
	if err != nil {
		klog.Warning("Failed to get NUMA info for calculating isolate cpuset.")
		//return "0-" + strconv.Itoa(numcpu-1), "0-" + strconv.Itoa(numcpu-1)
		return "", ""
	}

	// Parse the nodelist into a set of Node IDs
	nodes, err := parseCPUSet(strings.TrimSpace(string(nodelist)))
	if err != nil {
		klog.Warning("Failed to parse NUMA info for calculating isolate cpuset.")
		//return "0-" + strconv.Itoa(numcpu-1), "0-" + strconv.Itoa(numcpu-1)
		return "", ""
	}
	var highResult, lowResult string

	// ex: 4 numa for 25%
	numNuma := len(nodes.ToSlice())
	if 100%lowCPUPercent == 0 && numNuma%(100/lowCPUPercent) == 0 {
		// TODO: 直接return完整的numa
		numLowNuma := numNuma * lowCPUPercent / 100
		numHighNuma := numNuma - numLowNuma
		cnt := 0
		for _, node := range nodes.ToSlice() {
			cnt += 1
			// Read the 'cpulist' of the NUMA node from sysfs.
			path := fmt.Sprintf(sysutil.Conf.SysRootDir+"/devices/system/node/node%d/cpulist", node)
			cpulist, err := ioutil.ReadFile(path)
			if err != nil {
				klog.Warningf("Failed to parse NUMA %v info for calculating isolate cpuset.", node)
				//return "0-" + strconv.Itoa(numcpu-1), "0-" + strconv.Itoa(numcpu-1)
				return "", ""
			}
			//cpulists := strings.Split(strings.TrimSpace(string(cpulist)), ",")
			if cnt <= numHighNuma {
				if "" != highResult {
					highResult += ","
				}
				highResult += string(cpulist)
			} else {
				if "" != lowResult {
					lowResult += ","
				}
				lowResult += string(cpulist)
			}
		}
		highResult = strings.Replace(highResult, "\n", "", -1)
		lowResult = strings.Replace(lowResult, "\n", "", -1)
		klog.Infof("Cpuset was successfully set to, high: %s, low: %s", highResult, lowResult)
		return highResult, lowResult
	} else {
		// ex: 2 numa for 25%
		intpart, div := math.Modf(float64(numNuma) * float64(lowCPUPercent) / 100)
		numHighNuma := numNuma - int(intpart) - 1
		lowCPUPercent = int(div * 100)
		cnt := 0
		for _, node := range nodes.ToSlice() {
			cnt += 1
			// Read the 'cpulist' of the NUMA node from sysfs.
			path := fmt.Sprintf(sysutil.Conf.SysRootDir+"/devices/system/node/node%d/cpulist", node)
			cpulist, err := ioutil.ReadFile(path)
			if err != nil {
				klog.Warningf("Failed to parse NUMA %v info for calculating isolate cpuset.", node)
				//return "0-" + strconv.Itoa(numcpu-1), "0-" + strconv.Itoa(numcpu-1)
				return "", ""
			}
			if cnt <= numHighNuma {
				if "" != highResult {
					highResult += ","
				}
				highResult += string(cpulist)
				continue
			}
			cpulists := strings.Split(strings.TrimSpace(string(cpulist)), ",")
			for _, cpurange := range cpulists {
				boundaries := strings.Split(cpurange, "-")
				beg, _ := strconv.Atoi(boundaries[0])
				end, _ := strconv.Atoi(boundaries[1])
				numLowCpu := (end - beg + 1) * lowCPUPercent / 100
				numHighCpu := end - numLowCpu
				if "" != highResult {
					highResult += ","
				}
				if "" != lowResult {
					lowResult += ","
				}
				//highResult += fmt.Sprintf("%d-%d", beg, numHighCpu)
				//lowResult += fmt.Sprint("%d-%d", numHighCpu+1, end)
				highResult += strconv.Itoa(beg) + "-" + strconv.Itoa(numHighCpu)
				lowResult += strconv.Itoa(numHighCpu+1) + "-" + strconv.Itoa(end)
			}
		}
		highResult = strings.Replace(highResult, "\n", "", -1)
		lowResult = strings.Replace(lowResult, "\n", "", -1)
		klog.Infof("Cpuset was successfully planed, high: %s, low: %s", highResult, lowResult)
		return highResult, lowResult
		//return strconv.Itoa(numcpu-(numcpu*i)/100) + "-" + strconv.Itoa(numcpu-1)
	}
}

func parseCPUSet(s string) (cpuset.CPUSet, error) {
	b := cpuset.NewBuilder()

	// Handle empty string.
	if s == "" {
		return b.Result(), nil
	}

	// Split CPU list string:
	// "0-5,34,46-48 => ["0-5", "34", "46-48"]
	ranges := strings.Split(s, ",")

	for _, r := range ranges {
		boundaries := strings.Split(r, "-")
		if len(boundaries) == 1 {
			// Handle ranges that consist of only one element like "34".
			elem, err := strconv.Atoi(boundaries[0])
			if err != nil {
				return cpuset.NewCPUSet(), err
			}
			b.Add(elem)
		} else if len(boundaries) == 2 {
			// Handle multi-element ranges like "0-5".
			start, err := strconv.Atoi(boundaries[0])
			if err != nil {
				return cpuset.NewCPUSet(), err
			}
			end, err := strconv.Atoi(boundaries[1])
			if err != nil {
				return cpuset.NewCPUSet(), err
			}
			// Add all elements to the result.
			// e.g. "0-5", "46-48" => [0, 1, 2, 3, 4, 5, 46, 47, 48].
			for e := start; e <= end; e++ {
				b.Add(e)
			}
		}
	}
	return b.Result(), nil
}
