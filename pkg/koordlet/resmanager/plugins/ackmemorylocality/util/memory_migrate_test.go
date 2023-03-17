package memorylocality

import (
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"gitlab.alibaba-inc.com/cos/unified-resource-api/apis/extension/cpuset"

	"github.com/koordinator-sh/koordinator/pkg/koordlet/util/system"
)

func Test_getNumaNodes(t *testing.T) {
	testingBasicNoodeInfo := &NumaInfo{
		totalNumaInfo:  []string{"0", "1", "2"},
		nonAepNumaInfo: []string{"0", "1"},
		aepNumaInfo:    []string{"2"},
	}

	testingPodNodeList1 := map[int]struct{}{}

	testingPodNodeList2 := map[int]struct{}{0: {}}
	expectPodNodeList2 := testingBasicNoodeInfo.DeepCopy()
	expectPodNodeList2.podLocalNumaInfo = []string{"0"}
	expectPodNodeList2.podRemoteNumaInfo = []string{"1", "2"}

	testingPodNodeList3 := map[int]struct{}{0: {}, 1: {}, 2: {}}
	expectPodNodeList3 := testingBasicNoodeInfo.DeepCopy()
	expectPodNodeList3.podLocalNumaInfo = []string{"0", "1", "2"}

	type args struct {
		name        string
		podNodeList map[int]struct{}
		want        *NumaInfo
		valid       bool
		err         bool
	}

	tests := []args{
		{
			name:        "pod node list is nil",
			podNodeList: testingPodNodeList1,
			want:        nil,
			valid:       false,
			err:         true,
		},
		{
			name:        "has remote target",
			podNodeList: testingPodNodeList2,
			want:        expectPodNodeList2,
			valid:       true,
			err:         false,
		},
		{
			name:        "has no remote target",
			podNodeList: testingPodNodeList3,
			want:        expectPodNodeList3,
			valid:       false,
			err:         false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := getNumaNodes(testingBasicNoodeInfo, tt.podNodeList)
			assert.Equal(t, tt.want, got, "get node info")
			if tt.err {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			valid := isNumaInfoValid(got)
			assert.Equal(t, true, valid, "if node info valid")
			valid = isTargetNumaInfoValid(got)
			assert.Equal(t, tt.valid, valid, "if remote info valid")
		})
	}
}

func Test_execMigrateCommand(t *testing.T) {
	nodelist, err := ioutil.ReadFile(filepath.Join(system.Conf.SysRootDir, OnlineNumaFile))
	if err != nil {
		return
	}
	// Parse the nodelist into a set of Node IDs
	nodes, err := cpuset.Parse(strings.TrimSpace(string(nodelist)))
	assert.NoError(t, err)
	if len(nodes.ToSlice()) <= 1 {
		return
	}

	testingTaskId1 := os.Getpid()
	testingMigrateCommand1 := []string{"nice", "-19", "migratepages", strconv.FormatInt(int64(testingTaskId1), 10), "0", strconv.Itoa(len(nodes.ToSlice()) - 1)}
	err = execMigrateCommand(testingMigrateCommand1)
	assert.NoError(t, err)

	testingMigrateCommand2 := []string{"nice", "-19", "migratepages", strconv.FormatInt(int64(testingTaskId1), 10), "0", "0"}
	err = execMigrateCommand(testingMigrateCommand2)
	assert.Error(t, err, "local numa equals to remote numa")

	var testingTaskId3 int
	for {
		testingTaskId3 = rand.Intn(30000)
		_, err := os.Stat(fmt.Sprintf("/proc/%d/stat", testingTaskId3))
		if err != nil {
			break
		}
	}
	testingMigrateCommand3 := []string{"nice", "-19", "migratepages", strconv.FormatInt(int64(testingTaskId3), 10), "0", strconv.Itoa(len(nodes.ToSlice()) - 1)}
	err = execMigrateCommand(testingMigrateCommand3)
	assert.Error(t, err, "no such process")
	assert.Contains(t, err.Error(), "No such process")
}
