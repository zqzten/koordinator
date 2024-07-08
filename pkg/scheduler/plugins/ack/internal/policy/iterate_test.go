package policy

import (
	"testing"

	"github.com/koordinator-sh/koordinator/pkg/scheduler/plugins/ack/internal/types"
)

func Test_IterateUnits(t *testing.T) {
	type args struct {
		availableDevices map[types.DeviceId]int
		req              int
		size             int
	}
	tests := []struct {
		name string
		args args
		want []map[types.DeviceId]int
	}{
		{
			name: "available units is 0",
			args: args{
				availableDevices: map[types.DeviceId]int{},
				req:              3,
				size:             1,
			},
			want: []map[types.DeviceId]int{},
		},
		{
			name: "req is 0",
			args: args{
				availableDevices: map[types.DeviceId]int{
					"0": 3,
					"1": 4,
					"2": 5,
				},
				req:  0,
				size: 1,
			},
			want: []map[types.DeviceId]int{},
		},
		{
			name: "req is not 0",
			args: args{
				availableDevices: map[types.DeviceId]int{
					"0": 3,
					"1": 4,
					"2": 5,
				},
				req:  2,
				size: 1,
			},
			want: []map[types.DeviceId]int{
				{
					"0": 2,
				},
				{
					"1": 2,
				},
				{
					"2": 2,
				},
			},
		},
		{
			name: "size is -1",
			args: args{
				availableDevices: map[types.DeviceId]int{
					"0": 3,
					"1": 4,
					"2": 5,
				},
				req:  4,
				size: -1,
			},
			want: []map[types.DeviceId]int{
				{
					"0": 3,
					"1": 1,
				},
				{
					"0": 3,
					"2": 1,
				},
				{
					"1": 4,
				},
				{
					"2": 4,
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := []map[types.DeviceId]int{}
			Iterate(tt.args.availableDevices, tt.args.req, tt.args.size, func(item map[types.DeviceId]int) {
				got = append(got, item)
			})
			if len(got) != len(tt.want) {
				t.Errorf("IterateUnits() = got: %v, want: %v", got, tt.want)
				return
			}
			for _, item := range got {
				matched := false
				for _, w := range tt.want {
					if allocationIsMatched(item, w) {
						matched = true
						break
					}
				}
				if !matched {
					t.Errorf("IterateUnits() = got: %v, want: %v", got, tt.want)
					return
				}
			}
		})
	}
}

func Test_IterateUnitsWithReplicaRequests(t *testing.T) {
	type args struct {
		availableDevices map[types.DeviceId]int
		req              int
		size             int
	}
	tests := []struct {
		name string
		args args
		want []map[types.DeviceId]int
	}{
		{
			name: "available units is 0",
			args: args{
				availableDevices: map[types.DeviceId]int{},
				req:              3,
				size:             1,
			},
			want: []map[types.DeviceId]int{},
		},
		{
			name: "req is 0",
			args: args{
				availableDevices: map[types.DeviceId]int{
					"0": 3,
					"1": 4,
					"2": 5,
				},
				req:  0,
				size: 1,
			},
			want: []map[types.DeviceId]int{},
		},
		{
			name: "req is not 0",
			args: args{
				availableDevices: map[types.DeviceId]int{
					"0": 3,
					"1": 4,
					"2": 5,
				},
				req:  2,
				size: 1,
			},
			want: []map[types.DeviceId]int{
				{
					"0": 2,
				},
				{
					"1": 2,
				},
				{
					"2": 2,
				},
			},
		},
		{
			name: "size is not 0",
			args: args{
				availableDevices: map[types.DeviceId]int{
					"0": 3,
					"1": 4,
					"2": 5,
				},
				req:  4,
				size: 2,
			},
			want: []map[types.DeviceId]int{
				{
					"0": 2,
					"1": 2,
				},
				{
					"0": 2,
					"2": 2,
				},
				{
					"1": 2,
					"2": 2,
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := []map[types.DeviceId]int{}
			IterateWithReplica(tt.args.availableDevices, tt.args.req, tt.args.size, func(item map[types.DeviceId]int) {
				got = append(got, item)
			})
			if len(got) != len(tt.want) {
				t.Errorf("IterateUnitsWithReplicaRequests() = got: %v, want: %v", got, tt.want)
				return
			}
			for _, item := range got {
				matched := false
				for _, w := range tt.want {
					if allocationIsMatched(item, w) {
						matched = true
						break
					}
				}
				if !matched {
					t.Errorf("IterateUnitsWithReplicaRequests() = got: %v, want: %v", got, tt.want)
					return
				}
			}
		})
	}
}

func allocationIsMatched(a, b map[types.DeviceId]int) bool {
	if len(a) != len(b) {
		return false
	}
	for key, val := range a {
		if b[key] != val {
			return false
		}
	}
	return true
}
