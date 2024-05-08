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

package logicalresourcenode

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	apiext "github.com/koordinator-sh/koordinator/apis/extension"
	apiextunified "github.com/koordinator-sh/koordinator/apis/extension/unified"
	schedulingv1alpha1 "github.com/koordinator-sh/koordinator/apis/scheduling/v1alpha1"
	"github.com/koordinator-sh/koordinator/pkg/util/sloconfig"
)

const (
	configmapName = "lrn-config"
)

var (
	defaultSyncNodeLabelKeys = []string{
		apiext.LabelGPUModel,
		apiextunified.LabelGPUModelSeries,
		apiextunified.LabelNodeASWID,
		apiextunified.LabelNodePointOfDelivery,
	}

	defaultSyncNodeConditionTypes = []string{
		string(corev1.NodeReady),
	}

	defaultSkipSyncReservationLabelKeys = []string{
		schedulingv1alpha1.LabelNodeNameOfLogicalResourceNode,
		schedulingv1alpha1.LabelLogicalResourceNodeReservationGeneration,
		schedulingv1alpha1.LabelLogicalResourceNodeInitializing,
	}

	defaultSyncReservationAnnotationKeys = []string{
		apiext.AnnotationDeviceAllocateHint,
		apiext.AnnotationDeviceJointAllocate,
		schedulingv1alpha1.AnnotationVPCQoSThreshold,
	}

	defaultReservationTerminationGracePeriodSeconds = 10
)

type Config struct {
	Common CommonConfig
}

type CommonConfig struct {
	// Node label keys that should be synced to LRN.
	SyncNodeLabelKeys []string `json:"syncNodeLabelKeys"`
	// Node condition types that should be synced to LRN status.
	SyncNodeConditionTypes []string `json:"syncNodeConditionTypes"`

	// Specific LRN labels that should not sync to Reservation.
	SkipSyncReservationLabelKeys []string `json:"skipSyncReservationLabelKeys"`
	// LRN annotations that should sync to Reservation.
	SyncReservationAnnotationKeys []string `json:"syncReservationAnnotationKeys"`

	// The termination grace period that will wait after Reservation being terminating.
	ReservationTerminationGracePeriodSeconds int `json:"reservationTerminationGracePeriodSeconds"`

	// Whether to enable ENI QoS Group for LRN.
	EnableQoSGroup bool `json:"enableQoSGroup"`

	// LegacyMode is deprecated which hooks pods by existing LRNs.
	LegacyMode bool `json:"legacyMode,omitempty"`
}

var (
	// Cache config for a while, to avoid frequently get and unmarshal
	innerConfigLock   sync.Mutex
	innerConfig       *Config
	innerConfigLastTS time.Time
	enableCache       = true
)

func GetConfig(c client.Reader) (*Config, error) {
	innerConfigLock.Lock()
	defer innerConfigLock.Unlock()

	if enableCache && innerConfig != nil && time.Since(innerConfigLastTS) < time.Second*3 {
		return innerConfig, nil
	}

	newCfg := &Config{}
	cm := corev1.ConfigMap{}
	if err := c.Get(context.TODO(), types.NamespacedName{Namespace: sloconfig.ConfigNameSpace, Name: configmapName}, &cm); err != nil {
		if !errors.IsNotFound(err) {
			return nil, fmt.Errorf("failed to get lrn configmap: %v", err)
		}
	} else {
		if err = parseConfigmap(newCfg, cm.Data); err != nil {
			return nil, fmt.Errorf("failed to parse lrn configmap: %v", err)
		}
	}

	newCfg.SetDefaults()
	innerConfig = newCfg
	innerConfigLastTS = time.Now()

	return newCfg, nil
}

func parseConfigmap(cfg *Config, data map[string]string) error {
	if val, exists := data["common"]; exists {
		if err := json.Unmarshal([]byte(val), &cfg.Common); err != nil {
			return fmt.Errorf("failed to unmarshal common in lrn configmap: %v", err)
		}
	}

	return nil
}

func (c *Config) SetDefaults() *Config {
	c.Common.SyncNodeLabelKeys = append(c.Common.SyncNodeLabelKeys, defaultSyncNodeLabelKeys...)
	c.Common.SyncNodeConditionTypes = append(c.Common.SyncNodeConditionTypes, defaultSyncNodeConditionTypes...)
	c.Common.SkipSyncReservationLabelKeys = append(c.Common.SkipSyncReservationLabelKeys, defaultSkipSyncReservationLabelKeys...)
	c.Common.SyncReservationAnnotationKeys = append(c.Common.SyncReservationAnnotationKeys, defaultSyncReservationAnnotationKeys...)

	if c.Common.ReservationTerminationGracePeriodSeconds <= 0 {
		c.Common.ReservationTerminationGracePeriodSeconds = defaultReservationTerminationGracePeriodSeconds
	}

	return c
}
