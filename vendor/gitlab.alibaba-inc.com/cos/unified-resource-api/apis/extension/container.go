package extension

import v1 "k8s.io/api/core/v1"

func IsGpuContainer(c *v1.Container) bool {
	res := c.Resources.Requests
	for _, resName := range []v1.ResourceName{GPUResourceNvidia, GPUResourceAlibaba, GPUResourceAliyun,
		DeprecatedGpuCountName, DeprecatedGpuMemName, DeprecatedGpuMilliCoreName, GPUResourceMem, GPUResourceMemRatio,
		GPUResourceCore, GPUResourceEncode, GPUResourceDecode, FuxiGpu} {
		if val, ok := res[resName]; ok && (val.Value() > 0) {
			return true
		}
	}
	return false
}

// a container is gpu-share if the gpu resources it requests is not in full cards.
func IsGpuShareContainer(c *v1.Container) bool {
	res := c.Resources.Requests

	// if a container requests gpu mem in bytes, it is gpu-share.
	for _, resName := range []v1.ResourceName{GPUResourceMem, DeprecatedGpuMemName} {
		if val, ok := res[resName]; ok && (val.Value() > 0) {
			return true
		}
	}

	// if a container requests other gpu subresources, they must in full 100s and of the same value;
	// otherwise, it is gpu-share.
	var subResVal int64
	for _, resName := range []v1.ResourceName{GPUResourceAlibaba, FuxiGpu, GPUResourceMemRatio, GPUResourceCore, GPUResourceEncode, GPUResourceDecode} {
		if value, ok := res[resName]; ok && (value.Value() > 0) {
			if value.Value()%100 != 0 {
				return true
			}
			if subResVal == 0 {
				subResVal = value.Value()
			} else {
				if subResVal != value.Value() {
					return true
				}
			}
		}
	}

	return false
}
