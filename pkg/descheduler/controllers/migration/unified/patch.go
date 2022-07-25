package unified

import (
	"encoding/json"
	"reflect"
	"strings"
	"unsafe"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	nullHolder     = "nullHolder"
	nullHolderText = "\"nullHolder\""
)

func DumpJSON(v interface{}) string {
	str, err := json.Marshal(v)
	if err != nil {
		return err.Error()
	}
	return ToString(str)
}

// ToString convert slice to string without mem copy.
func ToString(b []byte) (s string) {
	pbytes := (*reflect.SliceHeader)(unsafe.Pointer(&b))
	pstring := (*reflect.StringHeader)(unsafe.Pointer(&s))
	pstring.Data = pbytes.Data
	pstring.Len = pbytes.Len
	return
}

type CommonPatch struct {
	PatchType types.PatchType        `json:"patchType"`
	PatchData map[string]interface{} `json:"data"`
}

func NewStrategicPatch() *CommonPatch {
	return &CommonPatch{PatchType: types.StrategicMergePatchType, PatchData: make(map[string]interface{})}
}

// Type implements Patch.
func (s *CommonPatch) Type() types.PatchType {
	return s.PatchType
}

// Data implements Patch.
func (s *CommonPatch) Data(obj client.Object) ([]byte, error) {
	return []byte(s.String()), nil
}

func (s *CommonPatch) String() string {
	jsonStr := DumpJSON(s.PatchData)
	return strings.Replace(jsonStr, nullHolderText, "null", -1)
}

func (s *CommonPatch) AddFinalizer(item string) *CommonPatch {
	switch s.PatchType {
	case types.StrategicMergePatchType:
		if _, ok := s.PatchData["metadata"]; !ok {
			s.PatchData["metadata"] = make(map[string]interface{})
		}
		metadata := s.PatchData["metadata"].(map[string]interface{})

		if oldList, ok := metadata["finalizers"]; !ok {
			metadata["finalizers"] = []string{item}
		} else {
			metadata["finalizers"] = append(oldList.([]string), item)
		}
	}
	return s
}

func (s *CommonPatch) RemoveFinalizer(item string) *CommonPatch {
	switch s.PatchType {
	case types.StrategicMergePatchType:
		if _, ok := s.PatchData["metadata"]; !ok {
			s.PatchData["metadata"] = make(map[string]interface{})
		}
		metadata := s.PatchData["metadata"].(map[string]interface{})

		if oldList, ok := metadata["$deleteFromPrimitiveList/finalizers"]; !ok {
			metadata["$deleteFromPrimitiveList/finalizers"] = []string{item}
		} else {
			metadata["$deleteFromPrimitiveList/finalizers"] = append(oldList.([]string), item)
		}
	}
	return s
}

func (s *CommonPatch) OverrideFinalizer(items []string) *CommonPatch {
	switch s.PatchType {
	case types.MergePatchType:
		if _, ok := s.PatchData["metadata"]; !ok {
			s.PatchData["metadata"] = make(map[string]interface{})
		}
		metadata := s.PatchData["metadata"].(map[string]interface{})

		metadata["finalizers"] = items
	}
	return s
}

func (s *CommonPatch) InsertLabel(key, value string) *CommonPatch {
	switch s.PatchType {
	case types.StrategicMergePatchType:
		if _, ok := s.PatchData["metadata"]; !ok {
			s.PatchData["metadata"] = make(map[string]interface{})
		}
		metadata := s.PatchData["metadata"].(map[string]interface{})

		if oldMap, ok := metadata["labels"]; !ok {
			metadata["labels"] = map[string]string{key: value}
		} else {
			oldMap.(map[string]string)[key] = value
		}
	}
	return s
}

func (s *CommonPatch) DeleteLabel(key string) *CommonPatch {
	switch s.PatchType {
	case types.StrategicMergePatchType:
		if _, ok := s.PatchData["metadata"]; !ok {
			s.PatchData["metadata"] = make(map[string]interface{})
		}
		metadata := s.PatchData["metadata"].(map[string]interface{})

		if oldMap, ok := metadata["labels"]; !ok {
			metadata["labels"] = map[string]string{key: nullHolder}
		} else {
			oldMap.(map[string]string)[key] = nullHolder
		}
	}
	return s
}

func (s *CommonPatch) InsertAnnotation(key, value string) *CommonPatch {
	switch s.PatchType {
	case types.StrategicMergePatchType:
		if _, ok := s.PatchData["metadata"]; !ok {
			s.PatchData["metadata"] = make(map[string]interface{})
		}
		metadata := s.PatchData["metadata"].(map[string]interface{})

		if oldMap, ok := metadata["annotations"]; !ok {
			metadata["annotations"] = map[string]string{key: value}
		} else {
			oldMap.(map[string]string)[key] = value
		}
	}
	return s
}

func (s *CommonPatch) DeleteAnnotation(key string) *CommonPatch {
	switch s.PatchType {
	case types.StrategicMergePatchType:
		if _, ok := s.PatchData["metadata"]; !ok {
			s.PatchData["metadata"] = make(map[string]interface{})
		}
		metadata := s.PatchData["metadata"].(map[string]interface{})

		if oldMap, ok := metadata["annotations"]; !ok {
			metadata["annotations"] = map[string]string{key: nullHolder}
		} else {
			oldMap.(map[string]string)[key] = nullHolder
		}
	}
	return s
}

func (s *CommonPatch) UpdatePodCondition(condition v1.PodCondition) *CommonPatch {
	switch s.PatchType {
	case types.StrategicMergePatchType:
		if _, ok := s.PatchData["status"]; !ok {
			s.PatchData["status"] = make(map[string]interface{})
		}
		status := s.PatchData["status"].(map[string]interface{})

		if oldList, ok := status["conditions"]; !ok {
			status["conditions"] = []v1.PodCondition{condition}
		} else {
			status["conditions"] = append(oldList.([]v1.PodCondition), condition)
		}
	}
	return s
}

func (s *CommonPatch) UpdatePodContainer(container v1.Container) *CommonPatch {
	switch s.PatchType {
	case types.StrategicMergePatchType:
		if _, ok := s.PatchData["spec"]; !ok {
			s.PatchData["spec"] = make(map[string]interface{})
		}
		spec := s.PatchData["spec"].(map[string]interface{})
		if oldList, ok := spec["containers"]; !ok {
			spec["containers"] = []v1.Container{container}
		} else {
			spec["containers"] = append(oldList.([]v1.Container), container)
		}
	}
	return s
}
