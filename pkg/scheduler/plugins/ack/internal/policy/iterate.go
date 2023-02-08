package policy

import (
	"sort"

	"github.com/koordinator-sh/koordinator/pkg/scheduler/plugins/ack/internal/types"
)

/*
availableUnits: the available resource
req: the request resource count
size: return the allocation with specify count devices, -1 represents that all
*/
func IterateWithReplica(availableDevices map[types.DeviceId]int, req int, size int, callback func(map[types.DeviceId]int)) {
	// defines a struct for searching
	type device struct {
		id    types.DeviceId
		count int
	}
	if req == 0 {
		return
	}
	if size <= 0 || size > len(availableDevices) {
		return
	}
	devices := []device{}
	for id, count := range availableDevices {
		devices = append(devices, device{id: id, count: count})
	}
	sort.Slice(devices, func(i, j int) bool {
		return devices[i].count >= devices[j].count
	})
	var iterate func(devices, allocated []device, req int, size int)
	iterate = func(devices, allocated []device, req int, size int) {
		if len(allocated) == size {
			data := map[types.DeviceId]int{}
			for _, u := range allocated {
				data[u.id] = u.count
			}
			callback(data)
			return
		}
		for i := range devices {
			cur := devices[i]
			if cur.count < req/size {
				continue
			}
			iterate(devices[i+1:], append(allocated, device{id: cur.id, count: req / size}), req, size)
		}
	}
	iterate(devices, []device{}, req, size)
}

/*
example:

	availableUnits := map[string]int{
		"0": 10,
		"1": 4,
		"2": 5,
		"5": 9,
		"7": 0,
	}
	req := 6

	result:
		item: map[0:6]
		item: map[5:6]
		item: map[0:1 2:5]
		item: map[2:5 5:1]
		item: map[1:1 2:5]
		item: map[0:2 1:4]
		item: map[1:4 5:2]
		item: map[1:4 2:2]
*/
func Iterate(availableDevices map[types.DeviceId]int, req int, size int, callback func(map[types.DeviceId]int)) {
	// defines a struct for searching
	type device struct {
		id    types.DeviceId
		count int
	}
	if req == 0 {
		return
	}
	devices := []device{}
	for id, count := range availableDevices {
		devices = append(devices, device{id: id, count: count})
	}
	sort.Slice(devices, func(i, j int) bool {
		return devices[i].count >= devices[j].count
	})
	var iterate func(devices, allocated []device, req int, size int)
	iterate = func(devices, allocated []device, req int, size int) {
		if len(allocated) == size {
			if req == 0 {
				data := map[types.DeviceId]int{}
				for _, u := range allocated {
					data[u.id] = u.count
				}
				callback(data)
			}
			return
		}
		for i := range devices {
			cur := devices[i]
			// filter index id: "7",because its' count is 0
			if cur.count == 0 {
				continue
			}
			// if found count of current unit is large than req,break searching
			if cur.count >= req {
				iterate([]device{}, append(allocated, device{id: cur.id, count: req}), 0, size)
				continue
			}
			// skip the current unit to iterate
			newUnits := []device{}
			for _, u := range devices {
				if u.id != cur.id {
					newUnits = append(newUnits, u)
				}
			}
			iterate(newUnits, append(allocated, device{id: cur.id, count: cur.count}), req-cur.count, size)
		}
	}
	if size > len(availableDevices) {
		return
	}
	if size > 0 && size <= len(availableDevices) {
		iterate(devices, []device{}, req, size)
		return
	}
	for i := 1; i <= len(devices); i++ {
		iterate(devices, []device{}, req, i)
	}
}

func IterateAllCandidateAllocations(allCandidateAllocations [][]CandidateAllocation, callback func([]CandidateAllocation)) {
	var iterate func(i int, accum []CandidateAllocation)
	iterate = func(i int, accum []CandidateAllocation) {
		if i == len(allCandidateAllocations) {
			callback(accum)
			return
		}
		for j := range allCandidateAllocations[i] {
			iterate(i+1, append(accum, allCandidateAllocations[i][j]))
		}
	}
	iterate(0, []CandidateAllocation{})
}
