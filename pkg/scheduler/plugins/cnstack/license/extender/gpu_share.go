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

package extender

import (
	"crypto/tls"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/spf13/pflag"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/klog/v2"
	"k8s.io/kubernetes/pkg/api/v1/resource"
	"k8s.io/kubernetes/pkg/scheduler/framework"
)

var cnstackHttpLicenseAddr = "https://hub-ingress.kube-public.svc/license-server"
var gpushareResourceName = []string{"aliyun.com/gpu-mem"}
var httpClient *http.Client

type CNStackHttpResponse struct {
	Status     string                 `json:"status,omitempty"`
	ExpireTime string                 `json:"expireTime,omitempty"`
	Info       map[string]interface{} `json:"info,omitempty"`
	Message    string                 `json:"message,omitempty"`
}

const (
	CNStackAuthorizeTypeTrial   = "OnTrial"
	CNStackAuthorizeTypeValid   = "Valid"
	CNStackAuthorizeTypeInvalid = "Invalid"

	CNStackGpuShareKey = "gpuShareCard"
)

func init() {
	tr := (http.DefaultTransport.(*http.Transport)).Clone()
	tr.TLSClientConfig = &tls.Config{
		InsecureSkipVerify: true,
	}
	httpClient = &http.Client{
		Transport: tr,
		Timeout:   30 * time.Second,
	}

	pflag.StringVar(&cnstackHttpLicenseAddr, "cnstack-http-license-server-addr", cnstackHttpLicenseAddr, "aecp http license server addr")
}

func GPUShareLicenseCheckFunc(handle framework.Handle) bool {
	klog.V(6).Infof("get cnstack http license.")
	resp, err := httpClient.Get(cnstackHttpLicenseAddr + "/license-info?name=cnstack&namespace=acs-system")
	if err != nil {
		klog.V(6).Infof("cnstack http license api error: %v", err)
		return false
	}
	defer resp.Body.Close()
	bytes, err := io.ReadAll(resp.Body)
	if err != nil {
		klog.Errorf("read response body error:%v", err)
		return false
	}
	klog.V(6).Infof("cnstack http license response: %v", string(bytes))
	if resp.StatusCode != http.StatusOK {
		klog.Errorf("read response code: %v, body error:%v", resp.StatusCode, string(bytes))
		return false
	}

	return isLicenseValid(bytes)
}

func isLicenseValid(bytes []byte) bool {
	cr := &CNStackHttpResponse{}
	err := json.Unmarshal(bytes, cr)
	if err != nil {
		klog.Errorf("json unmarshal str:%v, error:%v", string(bytes), err)
		return false
	}
	switch cr.Status {
	case CNStackAuthorizeTypeValid:
		if len(cr.Info) < 1 {
			return false
		}

		c := getGPUShareCount(cr.Info)
		if c <= 0 {
			return false
		}

		return true
	case CNStackAuthorizeTypeTrial:
		return true
	}

	return false
}

func getGPUShareCount(info map[string]interface{}) int64 {
	obj, ok := info[CNStackGpuShareKey]
	if !ok {
		return 0
	}
	switch v := obj.(type) {
	case string:
		c, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return 0
		}
		return c
	}
	return 0
}

func GPUShareResponsibleForPodFunc(handle framework.Handle, pod *corev1.Pod) bool {
	for _, v := range gpushareResourceName {
		if resource.GetResourceRequest(pod, corev1.ResourceName(v)) > 0 {
			return true
		}
	}
	return false
}
