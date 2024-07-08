package dsa

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"k8s.io/klog"

	ackapis "github.com/koordinator-sh/koordinator/apis/ackplugins"
	"github.com/koordinator-sh/koordinator/pkg/koordlet/util/system"
	"github.com/koordinator-sh/koordinator/pkg/util"
)

const (
	statePath          = "bus/dsa"
	sysKernelMmDir     = "/host-sys-kernel-mm/"
	migratePath        = "migrate/"
	batchMigrateFile   = "batch_migrate_enabled"
	dmaMigrateFile     = "dma_migrate_enabled"
	dmaMigrateModeFile = "dma_migrate_polling"

	binPath       = "/usr/bin/"
	enableScript  = "enable-dsa.sh"
	disableScript = "disable-dsa.sh"
	stateFile     = "bus/dsa/devices/dsa*/state"
)

var (
	hasInit       bool
	isSupportDsa  bool
	enabledDsa    bool
	initDsaLock   sync.Mutex
	configDsaLock sync.Mutex
)

func isNodeSupportDsa() (bool, error) {
	stateReady, err := isDsaAvailableByRootPath(GetDsaStatePath())
	if err != nil {
		return false, fmt.Errorf("isDsaAvailableByDevicePath error: %v", err)
	}
	migrateReady, err := isDsaAvailableByRootPath(GetMigrateConfigPath())
	if err != nil {
		return false, fmt.Errorf("isDsaAvailableByDevicePath error: %v", err)
	}
	hasInit = true
	return stateReady && migrateReady, nil
}

func isAccelConfigReady() (bool, error) {
	// check accel config systemcall or orther conditions
	ready, err := isDsaAvailableByAccelConfig()
	if err != nil {
		return false, fmt.Errorf("isDsaAvailableByAccelConfig error: %v", err)
	}
	return ready, nil
}

func IsSupportDsa() (bool, error) {
	initDsaLock.Lock()
	defer initDsaLock.Unlock()
	if !hasInit {
		deviceOk, err := isNodeSupportDsa()
		if err != nil {
			isSupportDsa = false
			return false, err
		}
		hasInit = true
		systemCallOk, err := isAccelConfigReady()
		if err != nil {
			isSupportDsa = false
			return false, err
		}
		isSupportDsa = deviceOk && systemCallOk
	}
	return isSupportDsa, nil
}

func GetDsaStatePath() string {
	return system.Conf.SysRootDir + statePath
}

func GetMigrateConfigPath() string {
	return sysKernelMmDir + migratePath
}

func GetDsaBatchMigrateEnabledPath() string {
	return sysKernelMmDir + migratePath + batchMigrateFile
}

func GetDsaDmaMigrateEnablePath() string {
	return sysKernelMmDir + migratePath + dmaMigrateFile
}

func GetDsaDmaMigrateModePath() string {
	return sysKernelMmDir + migratePath + dmaMigrateModeFile
}

func checkDsaState() error {
	// scan sysfs tree
	configDsaLock.Lock()
	defer configDsaLock.Unlock()
	statePattern := system.Conf.SysRootDir + stateFile
	matches, err := filepath.Glob(statePattern)
	if err != nil {
		return err
	}

	var init bool
	var enabled bool
	for _, fpath := range matches {
		// Read queue state entry
		state, err := ioutil.ReadFile(fpath)
		if err != nil {
			return err
		}
		if strings.TrimSpace(string(state)) == "enabled" && (!init || enabled) {
			init = true
			enabled = true
			continue
		}
		if strings.TrimSpace(string(state)) != "enabled" && (!init || !enabled) {
			init = true
			enabled = false
			continue
		}
		return fmt.Errorf("dsa state is abnormal")
	}

	enabledDsa = enabled
	return nil
}

func ManageDsa(enable bool) error {
	checkErr := checkDsaState()
	if checkErr != nil {
		klog.Warningf("check dsa state err %s", checkErr.Error())
	}
	if enable && (checkErr != nil || !enabledDsa) {
		var errB bytes.Buffer
		// dsa.conf for BM machines, dsa_vm.conf for VM machines
		command := exec.Command("bash", filepath.Join(binPath, enableScript))
		command.Stderr = &errB
		out, err := command.Output()
		if err != nil {
			return fmt.Errorf("failed to exec enableSript: %v, outStr: %s", err, string(out))
		}
	}
	if !enable && (checkErr != nil || enabledDsa) {
		var errB bytes.Buffer
		command := exec.Command("bash", filepath.Join(binPath, disableScript))
		command.Stderr = &errB
		out, err := command.Output()
		if err != nil {
			return fmt.Errorf("failed to exec disableScript: %v, outStr: %s", err, string(out))
		}
	}
	return nil
}

func MergeDefaultDsaAccelerateConfig(old *ackapis.DsaAccelerateStrategy) (*ackapis.DsaAccelerateStrategy, error) {
	def := util.DefaultDsaAccelerateStrategy()
	if old == nil {
		return def, nil
	}
	merged, err := util.MergeCfg(def, old)
	if err != nil {
		return nil, err
	}
	new, ok := merged.(*ackapis.DsaAccelerateStrategy)
	if !ok {
		return nil, fmt.Errorf("no default dsa accelerate error")
	}
	return new, nil
}
