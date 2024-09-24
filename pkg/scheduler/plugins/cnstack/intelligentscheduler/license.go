package intelligentscheduler

import (
	"sync"
	"time"
)

const (
	// License默认过期间隔，默认5分钟后过期
	LicenseDefaultExpirationDuration = 5 * time.Minute
	// License默认刷新间隔，默认每隔3分钟刷新一次
	LicenseDefaultRefreshDuration = 3 * time.Minute
)

type License struct {
	mutex          *sync.RWMutex
	IsLegality     bool
	LastUpdateTime time.Time
}

// InitLicense 初始化License对象
func InitLicense() *License {
	return &License{
		mutex:          new(sync.RWMutex),
		IsLegality:     getLicenseLegalityFromCNStack(),
		LastUpdateTime: time.Now(),
	}
}

// RefreshLicense 刷新license
func (l *License) RefreshLicense() {
	for {
		l.mutex.Lock()
		l.IsLegality = getLicenseLegalityFromCNStack()
		l.LastUpdateTime = time.Now()
		l.mutex.Unlock()
		time.Sleep(LicenseDefaultRefreshDuration)
	}
}

// CheckLicenseLegality 校验License
func (l *License) CheckLicenseLegality() bool {
	l.mutex.RLock()
	defer l.mutex.RUnlock()
	currentTime := time.Now()
	timeDifference := currentTime.Sub(l.LastUpdateTime)
	return l.IsLegality && timeDifference < LicenseDefaultExpirationDuration
}

func getLicenseLegalityFromCNStack() bool {
	//TODO 调用底座接口, 通过判断当前PGI的数量来校验License的合法性
	return true
}
