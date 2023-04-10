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

package custompodaffinity

import (
	"k8s.io/apimachinery/pkg/util/sets"

	extunified "github.com/koordinator-sh/koordinator/apis/extension/unified"
)

type serviceUnitStats struct {
	AppCounter         map[string]int
	ServiceUnitCounter map[extunified.PodSpreadInfo]int
	AllocSet           sets.String
}

func newServiceUnitStats() *serviceUnitStats {
	return &serviceUnitStats{
		AppCounter:         make(map[string]int),
		ServiceUnitCounter: make(map[extunified.PodSpreadInfo]int),
		AllocSet:           sets.NewString(),
	}
}

func (s *serviceUnitStats) GetAllocCount(spreadInfo *extunified.PodSpreadInfo, preemptivePods sets.String) int {
	preemptiveCount := 0
	if preemptivePods != nil {
		preemptiveCount = preemptivePods.Intersection(s.AllocSet).Len()
	}
	if spreadInfo.ServiceUnit == "" {
		return s.getAllocCountByAppName(spreadInfo.AppName) - preemptiveCount
	}
	return s.getAllocCountByServiceUnit(spreadInfo) - preemptiveCount
}

func (s *serviceUnitStats) getAllocCountByAppName(appName string) int {
	return s.AppCounter[appName]
}

func (s *serviceUnitStats) getAllocCountByServiceUnit(serviceUnit *extunified.PodSpreadInfo) int {
	return s.ServiceUnitCounter[*serviceUnit]
}

func (s *serviceUnitStats) clear() {
	for key := range s.AppCounter {
		delete(s.AppCounter, key)
	}
	for key := range s.ServiceUnitCounter {
		delete(s.ServiceUnitCounter, key)
	}
}

func (s *serviceUnitStats) copyFrom(stats *serviceUnitStats) {
	for key := range s.AppCounter {
		if _, ok := stats.AppCounter[key]; !ok {
			delete(s.AppCounter, key)
		}
	}
	for key, value := range stats.AppCounter {
		s.AppCounter[key] = value
	}

	for key := range s.ServiceUnitCounter {
		if _, ok := stats.ServiceUnitCounter[key]; !ok {
			delete(s.ServiceUnitCounter, key)
		}
	}
	for key, value := range stats.ServiceUnitCounter {
		s.ServiceUnitCounter[key] = value
	}
	s.AllocSet = sets.NewString(stats.AllocSet.List()...)
}

func (s *serviceUnitStats) setMaxValue(appName, serviceUnit string) {
	if appName != "" {
		if s.AppCounter[appName] < 1 {
			s.AppCounter[appName] = 1
		}
	}
	if serviceUnit != "" {
		spreadInfo := extunified.PodSpreadInfo{
			AppName:     appName,
			ServiceUnit: serviceUnit,
		}
		if s.ServiceUnitCounter[spreadInfo] < 1 {
			s.ServiceUnitCounter[spreadInfo] = 1
		}
	}
}

func (s *serviceUnitStats) add(stats *serviceUnitStats) {
	for app, count := range stats.AppCounter {
		s.AppCounter[app] += count
	}
	for serviceUnit, count := range stats.ServiceUnitCounter {
		s.ServiceUnitCounter[serviceUnit] += count
	}
}

func (s *serviceUnitStats) delete(stats *serviceUnitStats) {
	for app, count := range stats.AppCounter {
		if newCnt := s.AppCounter[app] - count; newCnt > 0 {
			s.AppCounter[app] = newCnt
		} else {
			delete(s.AppCounter, app)
		}
	}
	for serviceUnit, count := range stats.ServiceUnitCounter {
		if newCnt := s.ServiceUnitCounter[serviceUnit] - count; newCnt > 0 {
			s.ServiceUnitCounter[serviceUnit] = newCnt
		} else {
			delete(s.ServiceUnitCounter, serviceUnit)
		}
	}
}

func (s *serviceUnitStats) incCounter(appName, serviceUnit string, cnt int) {
	if appName != "" {
		if newCnt := s.AppCounter[appName] + cnt; newCnt > 0 {
			s.AppCounter[appName] = newCnt
		} else {
			delete(s.AppCounter, appName)
		}
	}
	if serviceUnit != "" {
		spreadInfo := extunified.PodSpreadInfo{
			AppName:     appName,
			ServiceUnit: serviceUnit,
		}
		if newCnt := s.ServiceUnitCounter[spreadInfo] + cnt; newCnt > 0 {
			s.ServiceUnitCounter[spreadInfo] = newCnt
		} else {
			delete(s.ServiceUnitCounter, spreadInfo)
		}
	}
}

func (s *serviceUnitStats) isZero() bool {
	return len(s.AppCounter) == 0 && len(s.ServiceUnitCounter) == 0
}
