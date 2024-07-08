package ackml

import (
	"os"
	"strconv"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	core "k8s.io/client-go/testing"
	"k8s.io/client-go/tools/record"

	ackapis "github.com/koordinator-sh/koordinator/apis/ackplugins"
	slov1alpha1 "github.com/koordinator-sh/koordinator/apis/slo/v1alpha1"
	"github.com/koordinator-sh/koordinator/pkg/koordlet/metriccache"
	mockmetriccache "github.com/koordinator-sh/koordinator/pkg/koordlet/metriccache/mockmetriccache"
	mlutil "github.com/koordinator-sh/koordinator/pkg/koordlet/resmanager/plugins/ackmemorylocality/util"
	"github.com/koordinator-sh/koordinator/pkg/koordlet/resourceexecutor"
	"github.com/koordinator-sh/koordinator/pkg/koordlet/statesinformer"
	mockstatesinformer "github.com/koordinator-sh/koordinator/pkg/koordlet/statesinformer/mockstatesinformer"
	"github.com/koordinator-sh/koordinator/pkg/koordlet/util"
	"github.com/koordinator-sh/koordinator/pkg/koordlet/util/system"
)

func newTestMLReconcile(client clientset.Interface, metricCache metriccache.MetricCache, statesInformer statesinformer.StatesInformer) *plugin {
	memoryLocalityPlugin := &plugin{
		config: NewDefaultConfig(),
	}
	memoryLocalityPlugin.cgroupReader = resourceexecutor.NewCgroupReader()
	memoryLocalityPlugin.kubeClient = client
	memoryLocalityPlugin.cgroupReader = resourceexecutor.NewCgroupReader()
	memoryLocalityPlugin.eventRecorder = record.NewFakeRecorder(1024)
	memoryLocalityPlugin.metricCache = metricCache
	memoryLocalityPlugin.statesInformer = statesInformer
	return memoryLocalityPlugin
}

func createPod(kubeQosClass corev1.PodQOSClass) *corev1.Pod {
	pod := &corev1.Pod{
		TypeMeta: metav1.TypeMeta{Kind: "Pod"},
		ObjectMeta: metav1.ObjectMeta{
			Name:        "pod0",
			UID:         "p0",
			Labels:      map[string]string{},
			Annotations: map[string]string{"scheduling.koordinator.sh/resource-status": "{\"cpuset\": \"0-3\"}"},
		},

		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name: "container0",
				},
			},
		},
		Status: corev1.PodStatus{
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name:        "container0",
					ContainerID: "containerd://c0",
				},
			},
			QOSClass: kubeQosClass,
			Phase:    corev1.PodRunning,
		},
	}

	return pod
}

func testingPrepareContainerCgroup(t *testing.T, helper *system.FileTestUtil, resoureFileName, containerParentPath, str string) {
	resource, err := system.GetCgroupResource(system.ResourceType(resoureFileName))
	assert.NoError(t, err)
	helper.WriteCgroupFileContents(containerParentPath, resource, str)
}

func Test_getPodCGroupCpuList(t *testing.T) {
	type args struct {
		podMeta *statesinformer.PodMeta
	}
	type fields struct {
		containerParentDir string
		containerCPUSetStr string
		invalidPath        bool
		useCgroupsV2       bool
	}
	tests := []struct {
		name   string
		args   args
		fields fields
		want   []int
	}{
		{
			name: "do nothing for empty pod",
			args: args{
				podMeta: &statesinformer.PodMeta{Pod: &corev1.Pod{}},
			},
			want: nil,
		},
		{
			name: "successfully get cpu set for the pod",
			fields: fields{
				containerParentDir: "kubepods.slice/p0/cri-containerd-c0.scope",
				containerCPUSetStr: "0-3",
			},
			args: args{
				podMeta: &statesinformer.PodMeta{
					Pod: &corev1.Pod{
						ObjectMeta: metav1.ObjectMeta{
							Name: "pod0",
							UID:  "p0",
						},
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name: "container0",
								},
							},
						},
						Status: corev1.PodStatus{
							ContainerStatuses: []corev1.ContainerStatus{
								{
									Name:        "container0",
									ContainerID: "containerd://c0",
								},
							},
						},
					},
					CgroupDir: "p0",
				},
			},
			want: []int{0, 1, 2, 3},
		},
		{
			name: "return empty for invalid path",
			fields: fields{
				containerParentDir: "p0/cri-containerd-c0.scope",
				containerCPUSetStr: "0-3",
				invalidPath:        true,
			},
			args: args{
				podMeta: &statesinformer.PodMeta{
					Pod: &corev1.Pod{
						ObjectMeta: metav1.ObjectMeta{
							Name: "pod0",
							UID:  "p0",
						},
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name: "container0",
								},
							},
						},
						Status: corev1.PodStatus{
							ContainerStatuses: []corev1.ContainerStatus{
								{
									Name:        "container0",
									ContainerID: "containerd://c0",
								},
							},
						},
					},
					CgroupDir: "p0",
				},
			},
			want: nil,
		},
		{
			name: "missing container's status",
			fields: fields{
				containerParentDir: "p0/cri-containerd-c0.scope",
				containerCPUSetStr: "0-3",
				invalidPath:        true,
			},
			args: args{
				podMeta: &statesinformer.PodMeta{
					Pod: &corev1.Pod{
						ObjectMeta: metav1.ObjectMeta{
							Name: "pod0",
							UID:  "p0",
						},
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name: "container0",
								},
							},
						},
						Status: corev1.PodStatus{
							Phase:             corev1.PodRunning,
							ContainerStatuses: []corev1.ContainerStatus{},
						},
					},
					CgroupDir: "p0",
				},
			},
			want: nil,
		},
		{
			name: "successfully get cpu set on cgroups v2",
			fields: fields{
				containerParentDir: "kubepods.slice/p0/cri-containerd-c0.scope",
				containerCPUSetStr: "0-3",
				useCgroupsV2:       true,
			},
			args: args{
				podMeta: &statesinformer.PodMeta{
					Pod: &corev1.Pod{
						ObjectMeta: metav1.ObjectMeta{
							Name: "pod0",
							UID:  "p0",
						},
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name: "container0",
								},
							},
						},
						Status: corev1.PodStatus{
							ContainerStatuses: []corev1.ContainerStatus{
								{
									Name:        "container0",
									ContainerID: "containerd://c0",
								},
							},
						},
					},
					CgroupDir: "p0",
				},
			},
			want: []int{0, 1, 2, 3},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			helper := system.NewFileTestUtil(t)
			helper.SetCgroupsV2(tt.fields.useCgroupsV2)
			defer helper.Cleanup()

			var resourceFileName string
			if tt.fields.useCgroupsV2 {
				resourceFileName = system.CPUSetCPUSEffectiveName
			} else {
				resourceFileName = system.CPUSetCPUSName
			}
			testingPrepareContainerCgroup(t, helper, resourceFileName, tt.fields.containerParentDir, tt.fields.containerCPUSetStr)

			system.CommonRootDir = ""
			if tt.fields.invalidPath {
				system.Conf.CgroupRootDir = "invalidPath"
			}

			r := newTestMLReconcile(&fake.Clientset{}, nil, nil)

			fakeMap := r.getPodCGroupCpuList(tt.args.podMeta, mlutil.ContainerInfoMap{})
			fakeInfo, exist := fakeMap["container0"]
			if !exist {
				assert.Nil(t, tt.want)
				return
			}
			assert.Equal(t, tt.want, fakeInfo.Cpulist)
		})
	}
}

func Test_getPodCgroupNumaStat(t *testing.T) {
	type args struct {
		podMeta *statesinformer.PodMeta
	}
	type fields struct {
		containerParentDir   string
		containerNumaStatStr string
		invalidPath          bool
		useCgroupsV2         bool
	}
	tests := []struct {
		name   string
		args   args
		fields fields
		want   []system.NumaMemoryPages
	}{
		{
			name: "do nothing for empty pod",
			args: args{
				podMeta: &statesinformer.PodMeta{Pod: &corev1.Pod{}},
			},
			want: nil,
		},
		{
			name: "successfully get numa stat for the pod",
			fields: fields{
				containerParentDir: "kubepods.slice/p0/cri-containerd-c0.scope",
				containerNumaStatStr: `total=6728070 N0=6722945 N1=5180
file=8415 N0=3943 N1=4506
anon=6719655 N0=6719002 N1=674
unevictable=0 N0=0 N1=0
hierarchical_total=6728070 N0=6722935 N1=5154
hierarchical_file=8415 N0=3927 N1=4488
hierarchical_anon=6719655 N0=6719008 N1=666
hierarchical_unevictable=0 N0=0 N1=0`,
			},
			args: args{
				podMeta: &statesinformer.PodMeta{
					Pod: &corev1.Pod{
						ObjectMeta: metav1.ObjectMeta{
							Name: "pod0",
							UID:  "p0",
						},
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name: "container0",
								},
							},
						},
						Status: corev1.PodStatus{
							ContainerStatuses: []corev1.ContainerStatus{
								{
									Name:        "container0",
									ContainerID: "containerd://c0",
								},
							},
						},
					},
					CgroupDir: "p0",
				},
			},
			want: []system.NumaMemoryPages{{NumaId: 0, PagesNum: 6722945}, {NumaId: 1, PagesNum: 5180}},
			//want: map[int]uint64{0: 6722945, 1: 5180},
		},
		{
			name: "return empty for invalid path",
			fields: fields{
				containerParentDir: "p0/cri-containerd-c0.scope",
				containerNumaStatStr: `total=6728070 N0=6722945 N1=5180
file=8415 N0=3943 N1=4506
anon=6719655 N0=6719002 N1=674
unevictable=0 N0=0 N1=0
hierarchical_total=6728070 N0=6722935 N1=5154
hierarchical_file=8415 N0=3927 N1=4488
hierarchical_anon=6719655 N0=6719008 N1=666
hierarchical_unevictable=0 N0=0 N1=0`,
				invalidPath: true,
			},
			args: args{
				podMeta: &statesinformer.PodMeta{
					Pod: &corev1.Pod{
						ObjectMeta: metav1.ObjectMeta{
							Name: "pod0",
							UID:  "p0",
						},
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name: "container0",
								},
							},
						},
						Status: corev1.PodStatus{
							ContainerStatuses: []corev1.ContainerStatus{
								{
									Name:        "container0",
									ContainerID: "containerd://c0",
								},
							},
						},
					},
					CgroupDir: "p0",
				},
			},
			want: nil,
		},
		{
			name: "missing container's status",
			fields: fields{
				containerParentDir: "p0/cri-containerd-c0.scope",
				containerNumaStatStr: `total=6728070 N0=6722945 N1=5180
file=8415 N0=3943 N1=4506
anon=6719655 N0=6719002 N1=674
unevictable=0 N0=0 N1=0
hierarchical_total=6728070 N0=6722935 N1=5154
hierarchical_file=8415 N0=3927 N1=4488
hierarchical_anon=6719655 N0=6719008 N1=666
hierarchical_unevictable=0 N0=0 N1=0`,
				invalidPath: true,
			},
			args: args{
				podMeta: &statesinformer.PodMeta{
					Pod: &corev1.Pod{
						ObjectMeta: metav1.ObjectMeta{
							Name: "pod0",
							UID:  "p0",
						},
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name: "container0",
								},
							},
						},
						Status: corev1.PodStatus{
							Phase:             corev1.PodRunning,
							ContainerStatuses: []corev1.ContainerStatus{},
						},
					},
					CgroupDir: "p0",
				},
			},
			want: nil,
		},
		{
			name: "successfully get numa stat on cgroups v2",
			fields: fields{
				containerParentDir: "kubepods.slice/p0/cri-containerd-c0.scope",
				containerNumaStatStr: `anon N0=193236992 N1=4096
file N0=1367764992 N1=0
kernel_stack N0=0 N1=0
shmem N0=1486848 N1=0
file_mapped N0=224243712 N1=0
file_dirty N0=3108864 N1=0
file_writeback N0=0 N1=0
anon_thp N0=0 N1=0
file_thp N0=0 N1=0
shmem_thp N0=0 N1=0
inactive_anon N0=203943936 N1=0
active_anon N0=135168 N1=4096
inactive_file N0=1152172032 N1=0
active_file N0=215052288 N1=0
unevictable N0=0 N1=0
slab_reclaimable N0=0 N1=0
slab_unreclaimable N0=0 N1=0
workingset_refault_anon N0=0 N1=0
workingset_refault_file N0=0 N1=0
workingset_activate_anon N0=0 N1=0
workingset_activate_file N0=0 N1=0
workingset_restore_anon N0=0 N1=0
workingset_restore_file N0=0 N1=0
workingset_nodereclaim N0=0 N1=0`,
				useCgroupsV2: true,
			},
			args: args{
				podMeta: &statesinformer.PodMeta{
					Pod: &corev1.Pod{
						ObjectMeta: metav1.ObjectMeta{
							Name: "pod0",
							UID:  "p0",
						},
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name: "container0",
								},
							},
						},
						Status: corev1.PodStatus{
							ContainerStatuses: []corev1.ContainerStatus{
								{
									Name:        "container0",
									ContainerID: "containerd://c0",
								},
							},
						},
					},
					CgroupDir: "p0",
				},
			},
			want: []system.NumaMemoryPages{{NumaId: 0, PagesNum: 381104}, {NumaId: 1, PagesNum: 1}},
			//want: map[int]uint64{0: 381104, 1: 1},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			helper := system.NewFileTestUtil(t)
			helper.SetCgroupsV2(tt.fields.useCgroupsV2)
			defer helper.Cleanup()

			resourceFileName := system.MemoryNumaStatName
			testingPrepareContainerCgroup(t, helper, resourceFileName, tt.fields.containerParentDir, tt.fields.containerNumaStatStr)

			system.CommonRootDir = ""
			if tt.fields.invalidPath {
				system.Conf.CgroupRootDir = "invalidPath"
			}

			r := newTestMLReconcile(&fake.Clientset{}, nil, nil)

			fakeMap := r.getPodCgroupNumaStat(tt.args.podMeta, mlutil.ContainerInfoMap{})
			fakeInfo, exist := fakeMap["container0"]
			if !exist {
				assert.Nil(t, tt.want)
				return
			}
			assert.Equal(t, tt.want, fakeInfo.NumaStat)
		})
	}
}

func Test_setMemoryLocalityStatus(t *testing.T) {
	testingPhase := mlutil.MemoryLocalityStatusClosed
	testingLastResult := &mlutil.MemoryMigrateResult{CompletedTime: metav1.Now(),
		Result: mlutil.MigratedCompleted, CompletedTimeLocalityRatio: 100, CompletedTimeRemotePages: 0}
	testingStatus1 := &mlutil.MemoryLocalityStatus{Phase: &testingPhase, LastResult: testingLastResult}
	testingStatus2 := &mlutil.MemoryLocalityStatus{Phase: &testingPhase, LastResult: nil}

	testingPod1 := createPod(corev1.PodQOSBestEffort)
	testingPod2 := createPod(corev1.PodQOSBestEffort)
	testingPod2.Annotations["koordinator.sh/memory-locality-status"] = "{\"phase\":\"InProgress\",\"lastResult\":{\"completedTime\":\"2023-04-01T07:52:30Z\",\"result\":\"MemoryLocalityCompleted\",\"completedTimeLocalityRatio\":80}}"
	testingPod3 := createPod(corev1.PodQOSBestEffort)
	testingPod3.Annotations["koordinator.sh/memory-locality-status"] = "{\"phase\":\"Closed\"}"
	testingPod4 := createPod(corev1.PodQOSBestEffort)
	testingPod4.Annotations["koordinator.sh/memory-locality-status"] = "{\"phase\":\"Completed\"}"

	type args struct {
		name   string
		pod    *corev1.Pod
		status *mlutil.MemoryLocalityStatus
	}

	tests := []args{
		{
			name:   "set phase to nil",
			pod:    testingPod1.DeepCopy(),
			status: testingStatus2,
		},
		{
			name:   "set phase and result to nil",
			pod:    testingPod1.DeepCopy(),
			status: testingStatus1,
		},
		{
			name:   "set deep equal status",
			pod:    testingPod3.DeepCopy(),
			status: testingStatus2,
		},
		{
			name:   "set phase and result 1",
			pod:    testingPod2.DeepCopy(),
			status: testingStatus2,
		},
		{
			name:   "set phase and result 2",
			pod:    testingPod2.DeepCopy(),
			status: testingStatus1,
		},
		{
			name:   "set phase and result 3",
			pod:    testingPod3.DeepCopy(),
			status: testingStatus1,
		},
		{
			name:   "set phase and result 4",
			pod:    testingPod4.DeepCopy(),
			status: testingStatus1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakeClient := &fake.Clientset{}
			fakeClient.Fake.AddReactor("get", "pods", func(action core.Action) (bool, runtime.Object, error) {
				return true, tt.pod, nil
			})
			fakeClient.Fake.AddReactor("update", "pods", func(action core.Action) (bool, runtime.Object, error) {
				return true, tt.pod, nil
			})
			m := newTestMLReconcile(fakeClient, nil, nil)
			_, err := m.setMemoryLocalityStatus(tt.pod, tt.status)
			assert.NoError(t, err)
		})
	}
}

func mockNodeCPUInfo() *metriccache.NodeCPUInfo {
	return &metriccache.NodeCPUInfo{
		TotalInfo: util.CPUTotalInfo{
			NumberCPUs:  5,
			NumberNodes: 2,
		},
		ProcessorInfos: []util.ProcessorInfo{
			{
				CPUID:  0,
				NodeID: 0,
			},
			{
				CPUID:  1,
				NodeID: 0,
			},
			{
				CPUID:  2,
				NodeID: 0,
			},
			{
				CPUID:  3,
				NodeID: 0,
			},
			{
				CPUID:  4,
				NodeID: 1,
			},
		},
	}
}

func createNodeSLO() *slov1alpha1.NodeSLO {
	nodeSLO := &slov1alpha1.NodeSLO{
		TypeMeta: metav1.TypeMeta{Kind: "NodeSLO"},
		ObjectMeta: metav1.ObjectMeta{
			Name:        "test_nodeslo",
			UID:         "test_nodeslo",
			Labels:      map[string]string{},
			Annotations: map[string]string{},
		},
	}
	return nodeSLO
}

func Test_memoryLocalityReconcile(t *testing.T) {

	testingPod1 := createPod(corev1.PodQOSBestEffort)
	testingPod1.Annotations["koordinator.sh/memoryLocality"] = "{\"policy\": \"bestEffort\"}"
	testingPod1.Annotations["koordinator.sh/memory-locality-status"] = "{\"phase\":\"Completed\"}"
	testingPod2 := createPod(corev1.PodQOSBestEffort)
	testingPod2.Annotations["koordinator.sh/memoryLocality"] = "{\"policy\": \"bestEffort\", \"migrateIntervalMinutes\": 1}"
	testingPod2.Annotations["koordinator.sh/memory-locality-status"] = "{\"phase\":\"InProgress\",\"lastResult\":{\"completedTime\":\"2023-04-01T07:52:30Z\",\"result\":\"MemoryLocalityCompleted\",\"completedTimeLocalityRatio\":80}}"
	testingPod3 := createPod(corev1.PodQOSBestEffort)
	testingPod3.Annotations["koordinator.sh/memoryLocality"] = "{\"policy\": \"none\"}"
	testingPod4 := createPod(corev1.PodQOSBestEffort)
	testingPod5 := createPod(corev1.PodQOSBestEffort)
	testingPod5.Annotations["koordinator.sh/memoryLocality"] = "{\"migrateIntervalMinutes\": 1}"

	testingMLStrategy1 := &ackapis.MemoryLocalityStrategy{}
	testingObject1 := make(map[string]interface{})
	testingObject1[ackapis.MemoryLocalityExtKey] = testingMLStrategy1
	testingExtensions1 := &slov1alpha1.ExtensionsMap{
		Object: testingObject1,
	}
	testingNodeSLO1 := createNodeSLO()
	testingNodeSLO1.Spec = slov1alpha1.NodeSLOSpec{
		Extensions: testingExtensions1,
	}

	testingMLConfig := &ackapis.MemoryLocality{}
	testingMLConfig.Policy = ackapis.MemoryLocalityPolicyBesteffort.Pointer()
	testingMLStrategy2 := &ackapis.MemoryLocalityStrategy{}
	testingMLStrategy2.BEClass = &ackapis.MemoryLocalityQOS{MemoryLocality: testingMLConfig}
	testingObject2 := make(map[string]interface{})
	testingObject2[ackapis.MemoryLocalityExtKey] = testingMLStrategy2
	testingExtensions2 := &slov1alpha1.ExtensionsMap{
		Object: testingObject2,
	}
	testingNodeSLO2 := createNodeSLO()
	testingNodeSLO2.Spec = slov1alpha1.NodeSLOSpec{
		Extensions: testingExtensions2,
	}

	type args struct {
		name    string
		pod     *corev1.Pod
		nodeSLO *slov1alpha1.NodeSLO
	}

	tests := []args{
		{
			name:    "pod policy is besteffort with completed status",
			pod:     testingPod1,
			nodeSLO: testingNodeSLO1,
		},
		{
			name:    "pod policy is besteffort with inprocess status",
			pod:     testingPod2,
			nodeSLO: testingNodeSLO1,
		},
		{
			name:    "qos policy is besteffort",
			pod:     testingPod4,
			nodeSLO: testingNodeSLO2,
		},
		{
			name:    "pod policy is none and qos policy is besteffort",
			pod:     testingPod3,
			nodeSLO: testingNodeSLO2,
		},
		{
			name:    "no qos",
			pod:     testingPod4,
			nodeSLO: testingNodeSLO1,
		},
		{
			name:    "no policy",
			pod:     testingPod3,
			nodeSLO: testingNodeSLO2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			helper := system.NewFileTestUtil(t)
			defer helper.Cleanup()
			dirCache := system.Conf.SysRootDir
			system.Conf.SysRootDir = helper.TempDir
			helper.CreateFile(mlutil.OnlineNumaFile)
			helper.WriteFileContents(mlutil.OnlineNumaFile, "0-1")
			//cgroupDir := "kubepods.slice/kubepods-besteffort.slice/kubepods-besteffort-p0/cri-containerd-c0.scope"
			cgroupDir := "kubepods.slice/p0/cri-containerd-c0.scope"
			testingPrepareContainerCgroup(t, helper, system.CPUSetCPUSName, cgroupDir, "0-3")
			containerNumaStatStr := `total=6728070 N0=6722945 N1=5180
file=8415 N0=3943 N1=4506
anon=6719655 N0=6719002 N1=674
unevictable=0 N0=0 N1=0
hierarchical_total=6728070 N0=6722935 N1=5154
hierarchical_file=8415 N0=3927 N1=4488
hierarchical_anon=6719655 N0=6719008 N1=666
hierarchical_unevictable=0 N0=0 N1=0`
			testingPrepareContainerCgroup(t, helper, system.MemoryNumaStatName, cgroupDir, containerNumaStatStr)
			testingPrepareContainerCgroup(t, helper, system.CPUTasksName, cgroupDir, strconv.Itoa(os.Getpid()))

			fakeClient := &fake.Clientset{}
			fakeClient.Fake.AddReactor("get", "pods", func(action core.Action) (bool, runtime.Object, error) {
				return true, tt.pod, nil
			})
			fakeClient.Fake.AddReactor("update", "pods", func(action core.Action) (bool, runtime.Object, error) {
				return true, tt.pod, nil
			})

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			mockStatesInformer := mockstatesinformer.NewMockStatesInformer(ctrl)
			mockMetricCache := mockmetriccache.NewMockMetricCache(ctrl)
			//cpuInfo := mockNodeCPUInfo()
			podMeta := &statesinformer.PodMeta{Pod: tt.pod, CgroupDir: "p0/"}
			mockStatesInformer.EXPECT().GetAllPods().Return([]*statesinformer.PodMeta{podMeta}).AnyTimes()
			mockStatesInformer.EXPECT().HasSynced().Return(true).AnyTimes()
			mockStatesInformer.EXPECT().GetNodeSLO().Return(tt.nodeSLO).AnyTimes()
			mockMetricCache.EXPECT().GetNodeCPUInfo(gomock.Any()).Return(mockNodeCPUInfo(), nil).AnyTimes()

			m := newTestMLReconcile(fakeClient, mockMetricCache, mockStatesInformer)
			m.reconcile()
			system.Conf.SysRootDir = dirCache
		})
	}

}
