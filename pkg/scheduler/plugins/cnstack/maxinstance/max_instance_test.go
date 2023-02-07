package maxinstance

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/informers"
	clientsetfake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/kubernetes/pkg/scheduler/framework"
)

type podBuilder struct {
	pod *v1.Pod
}

func createPod(configs []*MaxInstanceConfig) *podBuilder {
	p := &v1.Pod{}
	if len(configs) > 0 {
		bytes, _ := json.Marshal(configs)
		p.Annotations = map[string]string{}
		if len(configs) > 1 {
			p.Annotations[ACKMaxInstanceKey] = string(bytes)
		} else {
			p.Annotations[MaxInstanceKey] = string(bytes)
		}

	}
	return &podBuilder{pod: p}
}
func (pb *podBuilder) podKey(ns, name string) *podBuilder {
	pb.pod.UID = types.UID(fmt.Sprintf("%v/%v", ns, name))
	pb.pod.Namespace = ns
	pb.pod.Name = name
	return pb
}
func (pb *podBuilder) phase(s v1.PodPhase) *podBuilder {
	pb.pod.Status.Phase = s
	return pb
}
func (pb *podBuilder) nodeName(s string) *podBuilder {
	pb.pod.Spec.NodeName = s
	return pb
}

func createNode(name, zone, region string) *v1.Node {
	n := &v1.Node{}
	n.Name = name
	n.Labels = map[string]string{}
	n.Labels["name"] = name
	if len(zone) > 0 {
		n.Labels["zone"] = zone
	}
	if len(region) > 0 {
		n.Labels["region"] = region
	}
	return n
}

func TestMaxInstance_Cache(t *testing.T) {

	cs := clientsetfake.NewSimpleClientset(
		createNode("n1", "z1", "r1"),
		createNode("n2", "z2", "r2"),
		createNode("n3", "z2", "r2"),
	)
	informerFactory := informers.NewSharedInformerFactory(cs, 0)

	mi := &MaxInstance{
		cache:      map[string]map[string]map[string]int{},
		podCache:   map[string]struct{}{},
		nodeLister: informerFactory.Core().V1().Nodes().Lister(),
	}
	ctx := context.Background()
	informerFactory.Start(ctx.Done())
	informerFactory.WaitForCacheSync(ctx.Done())

	mi.addPod(createPod([]*MaxInstanceConfig{{"name", "g1", 10}}).podKey("ns", "1").phase(v1.PodRunning).nodeName("n1").pod)
	expect := map[string]map[string]map[string]int{
		"name": {"n1": {"g1": 1}},
	}
	assert.Equal(t, expect, mi.cache, "1")

	mi.addPod(createPod([]*MaxInstanceConfig{{"name", "g1", 10}}).podKey("ns", "2").phase(v1.PodRunning).nodeName("n1").pod)
	expect = map[string]map[string]map[string]int{
		"name": {"n1": {"g1": 2}},
	}
	assert.Equal(t, expect, mi.cache, "2")

	mi.addPod(createPod([]*MaxInstanceConfig{{"zone", "g1", 10}}).podKey("ns", "3").phase(v1.PodRunning).nodeName("n1").pod)
	expect = map[string]map[string]map[string]int{
		"name": {"n1": {"g1": 2}},
		"zone": {"z1": {"g1": 1}},
	}
	assert.Equal(t, expect, mi.cache, "3")

	mi.updatePod(createPod([]*MaxInstanceConfig{{"name", "g1", 10}}).podKey("ns", "1").phase(v1.PodRunning).nodeName("n1").pod,
		createPod([]*MaxInstanceConfig{{"name", "g1", 10}}).podKey("ns", "1").phase(v1.PodFailed).nodeName("n1").pod)
	expect = map[string]map[string]map[string]int{
		"name": {"n1": {"g1": 1}},
		"zone": {"z1": {"g1": 1}},
	}
	assert.Equal(t, expect, mi.cache, "4")

	mi.deletePod(createPod([]*MaxInstanceConfig{{"name", "g1", 10}}).podKey("ns", "2").phase(v1.PodRunning).nodeName("n1").pod)
	expect = map[string]map[string]map[string]int{
		"name": {"n1": {"g1": 0}},
		"zone": {"z1": {"g1": 1}},
	}
	assert.Equal(t, expect, mi.cache, "5")
}

func TestMaxInstance_Filter(t *testing.T) {
	type fields struct {
		cache map[string]map[string]map[string]int
	}
	type args struct {
		pod  *v1.Pod
		node *v1.Node
	}
	tests := []struct {
		name   string
		fields fields
		args   args
		want   framework.Code
	}{
		{
			name:   "node error",
			fields: fields{},
			args:   args{},
			want:   framework.Error,
		},
		{
			name:   "pod no annotations",
			fields: fields{},
			args: args{
				pod:  &v1.Pod{},
				node: &v1.Node{},
			},
			want: framework.Success,
		},
		{
			name:   "node has no key",
			fields: fields{},
			args: args{
				pod:  createPod([]*MaxInstanceConfig{{"zone", "g1", 10}}).pod,
				node: &v1.Node{},
			},
			want: framework.Success,
		},
		{
			name:   "node has no key 2",
			fields: fields{},
			args: args{
				pod:  createPod([]*MaxInstanceConfig{{"region", "g1", 10}}).pod,
				node: &v1.Node{},
			},
			want: framework.Success,
		},
		{
			name:   "node has no key 2",
			fields: fields{},
			args: args{
				pod:  createPod([]*MaxInstanceConfig{{"region", "g1", 10}}).pod,
				node: &v1.Node{},
			},
			want: framework.Success,
		},
		{
			name: "no cache",
			fields: fields{
				cache: map[string]map[string]map[string]int{},
			},
			args: args{
				pod:  createPod([]*MaxInstanceConfig{{"region", "g1", 10}}).pod,
				node: createNode("n1", "z1", "r1"),
			},
			want: framework.Success,
		},
		{
			name: "no cache 2",
			fields: fields{
				cache: map[string]map[string]map[string]int{
					"name": {},
				},
			},
			args: args{
				pod:  createPod([]*MaxInstanceConfig{{"name", "g1", 10}}).pod,
				node: createNode("n1", "z1", "r1"),
			},
			want: framework.Success,
		},
		{
			name: "no cache 3",
			fields: fields{
				cache: map[string]map[string]map[string]int{
					"name": {"n1": map[string]int{}},
				},
			},
			args: args{
				pod:  createPod([]*MaxInstanceConfig{{"name", "g1", 10}}).pod,
				node: createNode("n1", "z1", "r1"),
			},
			want: framework.Success,
		},
		{
			name: "fail",
			fields: fields{
				cache: map[string]map[string]map[string]int{
					"name": {"n1": {"g1": 10}},
				},
			},
			args: args{
				pod:  createPod([]*MaxInstanceConfig{{"name", "g1", 10}}).pod,
				node: createNode("n1", "z1", "r1"),
			},
			want: framework.Unschedulable,
		},
		{
			name: "fail 2",
			fields: fields{
				cache: map[string]map[string]map[string]int{
					"name": {"n1": {"g1": 5}},
					"zone": {"z1": {"g1": 5}},
				},
			},
			args: args{
				pod:  createPod([]*MaxInstanceConfig{{"name", "g1", 10}, {"zone", "g1", 5}}).pod,
				node: createNode("n1", "z1", "r1"),
			},
			want: framework.Unschedulable,
		},
		{
			name: "success",
			fields: fields{
				cache: map[string]map[string]map[string]int{
					"name": {"n1": {"g1": 5}},
				},
			},
			args: args{
				pod:  createPod([]*MaxInstanceConfig{{"name", "g1", 10}}).pod,
				node: createNode("n1", "z1", "r1"),
			},
			want: framework.Success,
		},
		{
			name: "success 2",
			fields: fields{
				cache: map[string]map[string]map[string]int{
					"name": {"n1": {"g1": 5}},
					"zone": {"z1": {"g1": 5}},
				},
			},
			args: args{
				pod:  createPod([]*MaxInstanceConfig{{"name", "g1", 10}, {"zone", "g1", 10}}).pod,
				node: createNode("n1", "z1", "r1"),
			},
			want: framework.Success,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mi := &MaxInstance{
				cache: tt.fields.cache,
			}
			nodeInfo := framework.NewNodeInfo()
			if tt.args.node != nil {
				nodeInfo.SetNode(tt.args.node)
			}
			if got := mi.Filter(context.Background(), nil, tt.args.pod, nodeInfo); !reflect.DeepEqual(got.Code(), tt.want) {
				t.Errorf("MaxInstance.Filter() = %v, want %v", got, tt.want)
			}
		})
	}
}
