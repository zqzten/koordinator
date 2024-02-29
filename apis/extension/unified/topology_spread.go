package unified

import (
	"encoding/json"

	"github.com/koordinator-sh/koordinator/apis/extension"
)

const (
	AnnotationMatchLabelKeysInPodTopologySpread = extension.SchedulingDomainPrefix + "/match-label-keys-in-topology-spread"
)

func GetMatchLabelKeysInPodTopologySpread(annotations map[string]string) ([]string, error) {
	if rawMatchLabelKeys, ok := annotations[AnnotationMatchLabelKeysInPodTopologySpread]; ok {
		var matchLabelKeys []string
		if err := json.Unmarshal([]byte(rawMatchLabelKeys), &matchLabelKeys); err != nil {
			return nil, err
		}
		return matchLabelKeys, nil
	}
	return nil, nil
}
