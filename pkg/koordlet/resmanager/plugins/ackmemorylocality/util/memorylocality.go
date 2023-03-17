package memorylocality

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"strings"
	"sync"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/klog"
	"k8s.io/utils/pointer"

	ackapis "github.com/koordinator-sh/koordinator/apis/ackplugins"
	apiext "github.com/koordinator-sh/koordinator/apis/extension"
	sysutil "github.com/koordinator-sh/koordinator/pkg/koordlet/util/system"
	"github.com/koordinator-sh/koordinator/pkg/util"
	"github.com/koordinator-sh/koordinator/pkg/util/cpuset"
)

var (
	hasInit                 bool
	isSupportMemoryLocality bool
	initMemoryLocalityLock  sync.Mutex
)

func GetPodMemoryLocality(pod *corev1.Pod) (*ackapis.MemoryLocality, error) {
	if pod == nil || pod.Annotations == nil {
		return nil, nil
	}
	value, exist := getPodMemoryLocalityAnnotation(pod.Annotations)
	if !exist {
		return nil, nil
	}
	cfg := ackapis.MemoryLocality{}
	err := json.Unmarshal([]byte(value), &cfg)
	if err != nil {
		return nil, err
	}
	return &cfg, nil
}

func getPodMemoryLocalityAnnotation(annotations map[string]string) (string, bool) {
	if koordMemoryLocality, exist := annotations[ackapis.AnnotationPodMemoryLocality]; exist {
		return koordMemoryLocality, true
	}
	return "", false
}

func IsSupportMemoryLocality() (bool, error) {
	initMemoryLocalityLock.Lock()
	defer initMemoryLocalityLock.Unlock()
	if !hasInit {
		hasInit = true
		nodelist, err := ioutil.ReadFile(filepath.Join(sysutil.Conf.SysRootDir, OnlineNumaFile))
		if err != nil {
			isSupportMemoryLocality = false
			return false, err
		}
		// Parse the nodelist into a set of Node IDs
		nodes, err := cpuset.Parse(strings.TrimSpace(string(nodelist)))
		if err != nil {
			isSupportMemoryLocality = false
			return false, err
		}
		numNuma := len(nodes.ToSlice())
		if numNuma <= 1 {
			klog.Warningf("The instance has only %d numa", numNuma)
			isSupportMemoryLocality = false
		} else {
			isSupportMemoryLocality = true
		}
	}
	return isSupportMemoryLocality, nil
}

func getMemoryLocality(qosCfg *ackapis.MemoryLocalityQOS) *ackapis.MemoryLocality {
	if qosCfg == nil {
		return nil
	}
	return qosCfg.MemoryLocality
}

func GetPodMemoryLocalityByQoSClass(pod *corev1.Pod, strategy *ackapis.MemoryLocalityStrategy) *ackapis.MemoryLocality {
	if strategy == nil {
		return nil
	}
	var memoryLocality *ackapis.MemoryLocality
	// only consider manual set qosclass
	podQoS := apiext.GetPodQoSClass(pod)
	switch podQoS {
	case apiext.QoSLSR:
		memoryLocality = getMemoryLocality(strategy.LSRClass)
	case apiext.QoSLS:
		memoryLocality = getMemoryLocality(strategy.LSClass)
	case apiext.QoSBE:
		memoryLocality = getMemoryLocality(strategy.BEClass)
	}
	return memoryLocality
}

func CheckIfCfgVaild(podCfg *ackapis.MemoryLocality) error {
	if podCfg.Policy == nil {
		return fmt.Errorf("policy is nil")
	}
	if podCfg.MigrateIntervalMinutes != nil && *podCfg.MigrateIntervalMinutes < 0 {
		return fmt.Errorf("migrteIntervals must be equal (only migrate once) or greater to 0")
	}
	if podCfg.TargetLocalityRatio != nil && (*podCfg.TargetLocalityRatio > 100 || *podCfg.TargetLocalityRatio < 0) {
		return fmt.Errorf("targetLocalityRatio ranges from 0 to 100")
	}
	return nil
}

func MergeDefaultMemoryLocality(old *ackapis.MemoryLocality) *ackapis.MemoryLocality {
	new := old.DeepCopy()
	def := util.DefaultPodMemoryLocalityConfig()
	if old.MigrateIntervalMinutes == nil {
		new.MigrateIntervalMinutes = pointer.Int64Ptr(*def.MigrateIntervalMinutes)
	}
	if old.TargetLocalityRatio == nil {
		new.TargetLocalityRatio = pointer.Int64Ptr(*def.TargetLocalityRatio)
	}
	return new
}
